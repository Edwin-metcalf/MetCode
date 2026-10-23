// agent runs the loop between the model and the tools helper files like normalize and compact also in the package
// normalize helps normalize reponses and clean up wierd model quircks
// compact uses a transcript and a fixed prompt to shrink the conversation
package agent

import (
	"errors"
	"fmt"
	"strings"
	"time"

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
