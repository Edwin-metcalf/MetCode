package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
)

// helper type to normalize wierd tool calls
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
