package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
)

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
