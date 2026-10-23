// agent runs the loop between the model and the tools
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/plan"
	"github.com/Edwin-metcalf/MetCode/internal/tools"
)

const (
	maxToolRounds = 12
	maxFailRounds = 3
)

var (
	ErrMaxRounds       = errors.New("too many tool-call rounds in one turn")
	ErrTooManyFailures = errors.New("tool calls kept failing")
)

type Agent struct {
	Model string
	Host  string
	Root  string
	Plan  []plan.Item
	// returns runs when the request finishes (the UI uses it for the spinner).
	Wait  func() func()
	Stats func(TurnStats)
}

type TurnStats struct {
	Calls        int
	Rounds       int
	PromptTokens int
	OutputTokens int
	PrefillTime  time.Duration
	GenTime      time.Duration
	LoadTime     time.Duration
	WallTime     time.Duration
}

// constants for communication with the model
const temperature = 0.2
const numCTX = 8192

func addStats(s *TurnStats, r ollama.ChatResponse) {
	s.PromptTokens += r.PromptEvalCount
	s.OutputTokens += r.EvalCount
	s.PrefillTime += time.Duration(r.PromptEvalDuration)
	s.GenTime += time.Duration(r.EvalDuration)
	s.LoadTime += time.Duration(r.LoadDuration)
}

// logCall folds one model call into the turn totals and prints a line for it.
// printing lives here rather than in addStats so the accumulator stays pure
func logCall(s *TurnStats, r ollama.ChatResponse) {
	addStats(s, r)
	s.Calls++

	perSec := 0.0
	if r.EvalDuration > 0 {
		perSec = float64(r.EvalCount) / time.Duration(r.EvalDuration).Seconds()
	}
	fmt.Printf("[call] %d prompt_tok=%d eval_tok=%d prefill=%s gen=%s (%.1f tok/s) load=%s\n",
		s.Calls, r.PromptEvalCount, r.EvalCount,
		time.Duration(r.PromptEvalDuration).Round(time.Millisecond),
		time.Duration(r.EvalDuration).Round(time.Millisecond), perSec,
		time.Duration(r.LoadDuration).Round(time.Millisecond))
}

func BuildChatRequest(history []ollama.Message, model string) ollama.ChatRequest {
	var outgoing ollama.ChatRequest

	outgoing.Messages = history
	outgoing.Model = model
	outgoing.Stream = false
	outgoing.Options.Temperature = temperature
	outgoing.Options.NumCtx = numCTX
	outgoing.Tools = tools.All

	return outgoing
}

// used for compact right now but could be for others, by not including any tool definitions we lower our context by like 600
func BuildChatRequestNoTools(history []ollama.Message, model string) ollama.ChatRequest {
	var outgoing ollama.ChatRequest

	outgoing.Messages = history
	outgoing.Model = model
	outgoing.Stream = false
	outgoing.Options.Temperature = temperature
	outgoing.Options.NumCtx = numCTX
	outgoing.Tools = []ollama.Tool{}

	return outgoing
}

func firstCallOnly(m ollama.Message) ollama.Message {
	if len(m.ToolCalls) > 1 {
		m.ToolCalls = m.ToolCalls[:1]
	}
	return m
}

// noToolsFlag should be true if you do not want to include tools on the call
func (a *Agent) chat(history []ollama.Message, noToolsFlag bool) (ollama.ChatResponse, error) {
	if a.Wait != nil {
		stop := a.Wait()
		defer stop()
	}
	if noToolsFlag {
		return ollama.Chat(BuildChatRequestNoTools(history, a.Model), a.Host)
	}
	return ollama.Chat(BuildChatRequest(history, a.Model), a.Host)
}

func (a *Agent) Turn(history *[]ollama.Message, userInput string) (string, error) {
	start := time.Now()
	var stats TurnStats

	defer func() {
		stats.WallTime = time.Since(start)
		if a.Stats != nil {
			a.Stats(stats)
		}
	}()
	return a.runTurn(history, userInput, &stats)

}

type rawCall struct {
	Name       string         `json:"name"`
	Arguments  map[string]any `json:"arguments"`
	Parameters map[string]any `json:"parameters"`
	Function   *struct {
		Name       string         `json:"name"`
		Arguments  map[string]any `json:"arguments"`
		Parameters map[string]any `json:"parameters"`
	} `json:"function"`
}

// function desgined to take a ollama.message slice and return a string of its conent summarized
func (a *Agent) CompactHistory(history *[]ollama.Message) (string, error) {
	summarizePrompt := `Summarize the conversation above so it can replace the full history. 
		Keep: file paths, function/class names, decisions made, current task state, errors encountered, what was accomplished.
		Drop: greetings and conversational filler. 
		Respond with the summary only, as plain text. Do not call tools.

	`
	//summarizeMessage := ollama.Message{Role: "user", Content: summarizePrompt, ToolCalls: []ollama.ToolCall{}}
	//historyPlus := append(slices.Clone(*history), summarizeMessage)
	historyPlus := []ollama.Message{{
		Role: "user",
		Content: "Below is a conversation transcript. You will be asked to summarize it. \n" +
			"<conversation>\n" + transcript(*history) + "</conversation>\n" +
			summarizePrompt,
		ToolCalls: []ollama.ToolCall{},
	}}

	//summarize everything but system prompt dont need that
	chatResponse, err := a.chat(historyPlus, true)
	if err != nil {
		return "", err
	}

	summarizedMessage := strings.TrimSpace(chatResponse.Message.Content)
	if summarizedMessage == "" {
		fmt.Println("summary was empty")
		return "", fmt.Errorf("error: summary is empty")
	}

	return summarizedMessage, nil
}

// helper const for a trimMiddle to keep the important parts of long text for our sumarize
// cut points are on boundaries so nothing is split
const (
	maxToolText = 900
	trimKeep    = 400
)

func trimMiddle(s string) string {
	if len(s) <= maxToolText {
		return s
	}

	head, tail := trimKeep, len(s)-trimKeep

	for head > 0 && !utf8.RuneStart(s[head]) {
		head--
	}
	for tail < len(s) && !utf8.RuneStart(s[tail]) {
		tail++
	}
	return s[:head] + "\n...[truncated]...\n" + s[tail:]
}

// dont need to see all the conversation, lets build a custom transcript with the more important information
func transcript(history []ollama.Message) string {
	var b strings.Builder
	for _, m := range history {
		if m.Role == "system" {
			continue
		}

		if m.Role == "tool" {
			fmt.Fprintf(&b, "TOOL RESULT: %s\n", trimMiddle(m.Content))
			continue
		}
		if m.Content != "" {
			fmt.Fprintf(&b, "%s: %s\n", strings.ToUpper(m.Role), m.Content)
		}

		for _, toolCall := range m.ToolCalls {
			args, _ := json.Marshal(toolCall.Function.Arguments)
			fmt.Fprintf(&b, "TOOL CALL: %s %s\n", toolCall.Function.Name, trimMiddle(string(args)))
		}
	}
	return b.String()
}

func (a *Agent) normalizeMessage(m ollama.Message) ollama.Message {
	//right now only four cases but might have more possibly expand to more helpers in that case
	//future case if the user is actually asking for a json that hs {"name": "example"} it will try to tool call it
	//new hase ```json just saw

	if len(m.ToolCalls) > 0 {
		return m
	}

	stripped := stripCodeFence(strings.TrimSpace(m.Content))

	if strings.HasPrefix(stripped, "<tool_call>") {
		noTagString := strings.TrimPrefix(stripped, "<tool_call>")
		noTagString = strings.TrimSuffix(noTagString, "</tool_call>")
		noTagString = strings.TrimSpace(noTagString)

		noPrefixMessage := ollama.Message{Role: m.Role, Content: noTagString, ToolCalls: m.ToolCalls}
		return a.normalizeMessage(noPrefixMessage)
	}

	if !strings.HasPrefix(stripped, "{") {
		return m
	}

	var rawData rawCall

	err := json.Unmarshal([]byte(stripped), &rawData)

	if err != nil {
		fmt.Printf("error unmarshaling tool call: %v\n", err)
		return m
	}
	name, args := "", map[string]any(nil)

	if rawData.Function != nil {
		name = rawData.Function.Name
		args = rawData.Function.Arguments
		if args == nil {
			args = rawData.Function.Parameters
		}
	} else {
		name = rawData.Name
		args = rawData.Arguments

		if args == nil {
			args = rawData.Parameters
		}
	}

	if name == "" {
		return m
	}

	return ollama.Message{Role: m.Role, Content: "", ToolCalls: []ollama.ToolCall{{Function: ollama.CalledFunction{Name: name, Arguments: args}}}}

}

// stripCodeFence removes a markdown code fence wrapped around content: the
// opening line (``` plus an optional language tag such as json) and a closing
// ``` line. Content that does not start with a fence is returned unchanged.
func stripCodeFence(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)

	if !strings.HasPrefix(s, "```") && !strings.HasPrefix(s, "~~~") {
		return s
	}

	lines := strings.Split(s, "\n")

	if len(lines) <= 1 {
		return s
	}
	//drop the opening fence and final fence
	lines = lines[1:]

	if len(lines) > 0 {
		last := lines[len(lines)-1]
		if last == "```" || last == "~~~" || strings.HasPrefix(last, "```") || strings.HasPrefix(last, "~~~") {
			lines = lines[:len(lines)-1]
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))

}

func (a *Agent) runTurn(history *[]ollama.Message, userInput string, stats *TurnStats) (string, error) {
	//turns user input to history and runs the model tool loop until we have an anser without a tool call

	*history = append(*history, ollama.Message{Role: "user", Content: userInput})

	resp, err := a.chat(*history, false)
	if err != nil {
		//dont add empty
		*history = (*history)[:len(*history)-1]
		return "", err
	}
	logCall(stats, resp)

	resp.Message = firstCallOnly(resp.Message)
	resp.Message = a.normalizeMessage(resp.Message)
	*history = append(*history, resp.Message)

	failRounds := 0

	for round := 0; len(resp.Message.ToolCalls) > 0; round++ {
		stats.Rounds += 1
		if round >= maxToolRounds {
			return "", ErrMaxRounds
		}

		allFailed := true

		for _, call := range resp.Message.ToolCalls {
			result := tools.Handle(call, a.Root, &a.Plan)

			if !strings.HasPrefix(result.Content, "error") {
				allFailed = false
			}
			*history = append(*history, result)
		}

		if allFailed {
			failRounds++
		} else {
			failRounds = 0
		}

		if failRounds > maxFailRounds {
			return "", ErrTooManyFailures
		}

		resp, err = a.chat(*history, false)
		if err != nil {
			return "", err
		}

		logCall(stats, resp)
		resp.Message = firstCallOnly(resp.Message)
		resp.Message = a.normalizeMessage(resp.Message)
		*history = append(*history, resp.Message)
	}
	return resp.Message.Content, nil

}
