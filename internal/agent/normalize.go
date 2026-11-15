package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/tools"
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

// toolAliases maps names models invent to the canonical tools.All name
var toolAliases = map[string]string{
	"create_file": "edit_file",
	"update_file": "edit_file",
	"write_file":  "edit_file",
}

// resolveToolName follows an alias and reports whether the result is a real tool.
func resolveToolName(name string) (string, bool) {
	if canon, ok := toolAliases[name]; ok {
		name = canon
	}
	for _, t := range tools.All {
		if t.Function.Name == name {
			return name, true
		}
	}

	return name, false
}

// canonicalizeNativeCalls rewrites alias names on a real tool_calls response.
// normalizeMessage only recovers calls written as plain text, so without this a
// server-native write_file would reach Handle as an unknown tool. Unknown names
// are left alone and still fail through the default case.
func canonicalizeNativeCalls(m ollama.Message) ollama.Message {
	for i := range m.ToolCalls {
		if canon, ok := resolveToolName(m.ToolCalls[i].Function.Name); ok {
			m.ToolCalls[i].Function.Name = canon
		}
	}
	return m
}

func (a *Agent) normalizeMessage(m ollama.Message) ollama.Message {
	//right now only four cases but might have more possibly expand to more helpers in that case
	//future case if the user is actually asking for a json that hs {"name": "example"} it will try to tool call it
	//new hase ```json just saw

	// Real tool_calls are canonicalized by canonicalizeNativeCalls in the agent
	// loop before this runs. This function only recovers calls the model wrote
	// as plain text.
	if len(m.ToolCalls) > 0 {
		return m
	}

	content := strings.TrimSpace(m.Content)

	// A <tool_call> wrapper is not a JSON object, so it is unwrapped by tag.
	// stripCodeFence is only kept for this branch: the tag may sit inside a
	// ```json block.
	stripped := stripCodeFence(content)
	if strings.HasPrefix(stripped, "<tool_call>") {
		noTagString := strings.TrimPrefix(stripped, "<tool_call>")
		noTagString = strings.TrimSuffix(noTagString, "</tool_call>")
		noTagString = strings.TrimSpace(noTagString)

		noPrefixMessage := ollama.Message{Role: m.Role, Content: noTagString, ToolCalls: m.ToolCalls}
		return a.normalizeMessage(noPrefixMessage)
	}

	rawData, ok := findCall(content)
	if !ok {
		// only complain about a message that tried to be a call, so prose and
		// plain json replies stay quiet
		if !a.Quiet && strings.HasPrefix(content, "{") && strings.Contains(content, `"name"`) {
			fmt.Println("error unmarshaling tool call: no tool call found in the message")
		}
		return m
	}

	name, args := rawData.nameAndArgs()
	name, _ = resolveToolName(name)

	return ollama.Message{Role: m.Role, Content: "", ToolCalls: []ollama.ToolCall{{Function: ollama.CalledFunction{Name: name, Arguments: args}}}}

}

// findCall returns the first tool call the model wrote as plain text. It tries
// every '{' in the message, not just the first one, because models paste the
// finished code before the call: in the withNudge study 10 of qwen2.5-coder's
// 21 zero-edit runs had a complete edit_file call behind a ```go block, and
// "the first brace" landed on a Go function body instead. Same reason
// "the first fence" was wrong — see stripCodeFence.
//
// A known tool name wins over an earlier unknown one, so a map literal such as
// map[string]string{"name": "x"} in pasted code cannot shadow the real call.
func findCall(s string) (rawCall, bool) {
	var fallback rawCall
	haveFallback := false

	for pos := 0; ; {
		i := strings.Index(s[pos:], "{")
		if i < 0 {
			return fallback, haveFallback
		}
		i += pos
		pos = i + 1

		// Decode reads one JSON value and leaves the rest of the reader alone.
		// That is the difference from json.Unmarshal, which failed with
		// "Extra data" as soon as a second call followed the first. Go code
		// fails on its first token, so the per-brace retry is cheap.
		var raw rawCall
		if err := json.NewDecoder(strings.NewReader(s[i:])).Decode(&raw); err != nil {
			continue
		}
		name, args := raw.nameAndArgs()
		if name == "" {
			continue
		}
		if _, known := resolveToolName(name); known {
			return raw, true
		}
		// an unknown name with arguments is still a call: Turn routes it to
		// Handle so the model gets "unknown tool" back and can correct itself.
		// one without arguments is more likely a plain json reply than a call
		if args != nil && !haveFallback {
			fallback, haveFallback = raw, true
		}
	}
}

// nameAndArgs reads the two shapes a text call comes in: {name, arguments} and
// {function: {name, arguments}}. "parameters" is accepted for both, because
// llama3.1 reaches for the schema's word instead of the call's word.
func (r rawCall) nameAndArgs() (string, map[string]any) {
	if r.Function != nil {
		if r.Function.Arguments != nil {
			return r.Function.Name, r.Function.Arguments
		}
		return r.Function.Name, r.Function.Parameters
	}
	if r.Arguments != nil {
		return r.Name, r.Arguments
	}
	return r.Name, r.Parameters
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
