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

	// find the first '{' and decode from there instead of requiring the message
	// to start with one. Two shapes from baseline-2 needed this:
	//   - a sentence of reasoning, then the call (add-bug rows 1-4)
	//   - two calls written back to back (add-bug-descriptive rows 5, 6, 9)
	// Index returns -1 when there is no brace at all, which is the "nothing to
	// parse" case — note that is the opposite of what `curlyIdx == 0` tested:
	// 0 means the message starts with '{', i.e. the ordinary case.
	start := strings.Index(stripped, "{")
	if start < 0 {
		return m
	}

	// Decode reads exactly one JSON value and leaves the rest of the reader
	// alone. That is the whole difference from json.Unmarshal, which decodes the
	// entire string and failed with "Extra data: line 3 column 1" as soon as a
	// second call followed the first. We keep only the first call, which matches
	// firstCallOnly's one-call-per-message rule.
	var rawData rawCall
	if err := json.NewDecoder(strings.NewReader(stripped[start:])).Decode(&rawData); err != nil {
		if !a.Quiet {
			fmt.Printf("error unmarshaling tool call: %v\n", err)
		}
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
	lines := strings.Split(s, "\n")

	open := -1
	marker := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "```") && !strings.HasPrefix(trimmed, "~~~") {
			continue
		}
		open = i
		marker = trimmed[:3]
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, marker))
		if rest != "" && !strings.HasPrefix(rest, "{") && !strings.HasPrefix(rest, "<") {
			// Drop the optional language tag on the opening line.
			if _, after, found := strings.Cut(rest, " "); found {
				rest = strings.TrimSpace(after)
			} else {
				rest = ""
			}
		}
		if strings.HasPrefix(rest, "{") || strings.HasPrefix(rest, "<") {
			// The fence and its content share one line.
			if close := strings.Index(rest, marker); close >= 0 {
				rest = strings.TrimSpace(rest[:close])
			}
			return strings.TrimSpace(rest)
		}
		break
	}
	if open < 0 {
		return s
	}

	for i := open + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), marker) {
			return strings.TrimSpace(strings.Join(lines[open+1:i], "\n"))
		}
	}
	// An unclosed fence still carries the call; take the rest of the message.
	return strings.TrimSpace(strings.Join(lines[open+1:], "\n"))
}
