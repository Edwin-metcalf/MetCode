package agent

import (
	"reflect"
	"testing"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
)

func TestNormalizeMessage(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantCall bool
		wantName string
		wantArgs map[string]any
	}{
		{
			// the shape qwen2.5-coder emits: flat, with "arguments"
			name:     "flat arguments",
			content:  `{"name":"edit_file","arguments":{"path":"a.go","old_text":"","new_text":"x"}}`,
			wantCall: true,
			wantName: "edit_file",
			wantArgs: map[string]any{"path": "a.go", "old_text": "", "new_text": "x"},
		},
		{
			// the shape llama3.1 emits: flat, but the model reached for the
			// schema's word "parameters" instead of the call's word "arguments"
			name:     "flat parameters",
			content:  `{"name":"edit_file","parameters":{"path":"a.go"}}`,
			wantCall: true,
			wantName: "edit_file",
			wantArgs: map[string]any{"path": "a.go"},
		},
		{
			// the shape Ollama's own wire format uses: nested under "function"
			name:     "nested function arguments",
			content:  `{"type":"function","function":{"name":"edit_file","arguments":{"path":"a.go"}}}`,
			wantCall: true,
			wantName: "edit_file",
			wantArgs: map[string]any{"path": "a.go"},
		},
		{
			// nested AND the wrong key, to exercise the Function.Parameters path
			name:     "nested function parameters",
			content:  `{"function":{"name":"edit_file","parameters":{"path":"a.go"}}}`,
			wantCall: true,
			wantName: "edit_file",
			wantArgs: map[string]any{"path": "a.go"},
		},
		{
			// the template's <tool_call> wrapper around flat arguments
			name: "tool_call tags",
			content: `<tool_call>
{"name":"edit_file","arguments":{"path":"a.go"}}
</tool_call>`,
			wantCall: true,
			wantName: "edit_file",
			wantArgs: map[string]any{"path": "a.go"},
		},
		{
			// a no-argument tool: arguments is an empty object, still a call
			name:     "empty arguments object",
			content:  `{"name":"list_directory","arguments":{}}`,
			wantCall: true,
			wantName: "list_directory",
			wantArgs: map[string]any{},
		},
		{
			// qwen2.5-coder wraps the call in a json-tagged markdown fence
			name:     "fenced with json tag",
			content:  "```json\n{\"name\": \"run_command\", \"arguments\": {\"command\": \"cat\", \"arguments\": [\"job.go\"]}}\n```",
			wantCall: true,
			wantName: "run_command",
			wantArgs: map[string]any{"command": "cat", "arguments": []any{"job.go"}},
		},
		{
			// a bare fence with no language tag
			name:     "fenced bare",
			content:  "```\n{\"name\":\"edit_file\",\"arguments\":{\"path\":\"a.go\"}}\n```",
			wantCall: true,
			wantName: "edit_file",
			wantArgs: map[string]any{"path": "a.go"},
		},
		{
			// a json5 tag is still a fence
			name:     "fenced with json5 tag",
			content:  "```json5\n{\"name\":\"edit_file\",\"arguments\":{\"path\":\"a.go\"}}\n```",
			wantCall: true,
			wantName: "edit_file",
			wantArgs: map[string]any{"path": "a.go"},
		},
		{
			// the fence wrapped around the whole tool_call block
			name:     "fenced around tool_call tags",
			content:  "```json\n<tool_call>\n{\"name\":\"edit_file\",\"arguments\":{\"path\":\"a.go\"}}\n</tool_call>\n```",
			wantCall: true,
			wantName: "edit_file",
			wantArgs: map[string]any{"path": "a.go"},
		},
		{
			// the fence inside the tool_call tags
			name:     "fenced inside tool_call tags",
			content:  "<tool_call>\n```json\n{\"name\":\"edit_file\",\"arguments\":{\"path\":\"a.go\"}}\n```\n</tool_call>",
			wantCall: true,
			wantName: "edit_file",
			wantArgs: map[string]any{"path": "a.go"},
		},
		{
			// a fenced block that is valid JSON but not a tool call stays put
			name:     "fenced json that is not a call",
			content:  "```json\n{\"foo\": 1}\n```",
			wantCall: false,
		},
		{
			// prose with a fenced code sample must pass through untouched
			name:     "prose with fenced code is not a call",
			content:  "Here is the file:\n```go\nfunc main() {}\n```",
			wantCall: false,
		},
		{
			// plain prose must pass through untouched
			name:     "prose is not a call",
			content:  "hello there, how can I help?",
			wantCall: false,
		},
		{
			// valid JSON, but carries no tool call
			name:     "json without a name is not a call",
			content:  `{"foo":1}`,
			wantCall: false,
		},
		{
			name:     "empty content is not a call",
			content:  "",
			wantCall: false,
		},
		{
			// looks like an attempt but is not parseable: leave it alone
			name:     "malformed json is not a call",
			content:  `{"name": "edit_file", "arguments": }`,
			wantCall: false,
		},

		{
			// add-bug rows 1-4: a sentence of reasoning, then the call
			name:     "prose then a call",
			content:  "I'll start by listing the files.\n\n{\"name\":\"list_directory\",\"arguments\":{}}",
			wantCall: true,
			wantName: "list_directory",
			wantArgs: map[string]any{},
		},
		{
			// add-bug-descriptive rows 5, 6, 9: two calls back to back.
			// only the first is returned, because firstCallOnly keeps one call
			// per message anyway; the second must not make decoding fail
			name:     "two calls back to back",
			content:  "{\"name\":\"list_directory\",\"arguments\":{}}\n\n{\"name\":\"read_file\",\"arguments\":{\"path\":\"add.go\"}}",
			wantCall: true,
			wantName: "list_directory",
			wantArgs: map[string]any{},
		},
		{
			// write_file was removed from the schema; models still emit it, so
			// it resolves to edit_file with its arguments intact
			name:     "aliased write_file",
			content:  `{"name":"write_file","arguments":{"path":"a.go","old_text":"","new_text":"x"}}`,
			wantCall: true,
			wantName: "edit_file",
			wantArgs: map[string]any{"path": "a.go", "old_text": "", "new_text": "x"},
		},
		{
			name:     "aliased update_file",
			content:  `{"name":"update_file","arguments":{"path":"a.go","old_text":"a","new_text":"b"}}`,
			wantCall: true,
			wantName: "edit_file",
			wantArgs: map[string]any{"path": "a.go", "old_text": "a", "new_text": "b"},
		},
		{
			// an unknown name with arguments is still a call: Turn routes it to
			// Handle so the model gets "unknown tool" back and can correct itself
			name:     "unknown name with arguments stays a call",
			content:  `{"name":"frobnicate","arguments":{"path":"a.go"}}`,
			wantCall: true,
			wantName: "frobnicate",
			wantArgs: map[string]any{"path": "a.go"},
		},
		{
			// an unknown name with no arguments is more likely a JSON reply than
			// a call, so it is left alone
			name:     "unknown name without arguments is not a call",
			content:  `{"name":"frobnicate"}`,
			wantCall: false,
		},
	}

	a := &Agent{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := a.normalizeMessage(ollama.Message{Role: "assistant", Content: tt.content})

			if !tt.wantCall {
				if len(got.ToolCalls) != 0 {
					t.Fatalf("expected no tool call, got %+v", got.ToolCalls)
				}
				if got.Content != tt.content {
					t.Errorf("content = %q, want it left untouched (%q)", got.Content, tt.content)
				}
				return
			}

			if len(got.ToolCalls) != 1 {
				t.Fatalf("got %d tool calls, want 1", len(got.ToolCalls))
			}
			if got.ToolCalls[0].Function.Name != tt.wantName {
				t.Errorf("name = %q, want %q", got.ToolCalls[0].Function.Name, tt.wantName)
			}
			if !reflect.DeepEqual(got.ToolCalls[0].Function.Arguments, tt.wantArgs) {
				t.Errorf("arguments = %#v, want %#v", got.ToolCalls[0].Function.Arguments, tt.wantArgs)
			}
			// cleared so the chat template renders the tool call instead of text
			if got.Content != "" {
				t.Errorf("content = %q, want empty", got.Content)
			}
		})
	}
}

// a real tool call from Ollama must never be replaced by a text one
func TestNormalizeMessage_KeepsExistingToolCalls(t *testing.T) {
	a := &Agent{}
	real := ollama.ToolCall{Function: ollama.CalledFunction{
		Name:      "read_file",
		Arguments: map[string]any{"path": "x"},
	}}
	m := ollama.Message{
		Role:      "assistant",
		Content:   `{"name":"edit_file","arguments":{"path":"a.go"}}`,
		ToolCalls: []ollama.ToolCall{real},
	}

	got := a.normalizeMessage(m)

	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("existing tool call was overwritten: %+v", got.ToolCalls)
	}
}

// end to end: a content-only call must actually run through Turn, with its
// arguments reaching the tool.

func TestStripCodeFence(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "prose before the fence",
			in:   "Sure, I'll create that file:\n```json\n{\"name\":\"edit_file\"}\n```",
			want: "{\"name\":\"edit_file\"}",
		},
		{
			name: "indented closing fence",
			in:   "```json\n{\"name\":\"edit_file\"}\n   ```",
			want: "{\"name\":\"edit_file\"}",
		},
		{
			name: "trailing prose after the fence",
			in:   "```json\n{\"name\":\"list_directory\",\"arguments\":{}}\n```\nLet me know if you need anything else!",
			want: "{\"name\":\"list_directory\",\"arguments\":{}}",
		},
		{
			name: "single-line fence",
			in:   "```json {\"name\":\"list_directory\",\"arguments\":{}} ```",
			want: "{\"name\":\"list_directory\",\"arguments\":{}}",
		},
		{
			name: "unclosed fence",
			in:   "```json\n{\"name\":\"edit_file\"}",
			want: "{\"name\":\"edit_file\"}",
		},
		{
			name: "tilde fence",
			in:   "~~~json\n{\"a\":1}\n~~~",
			want: "{\"a\":1}",
		},
		{
			name: "four backticks",
			in:   "````json\n{\"a\":1}\n````",
			want: "{\"a\":1}",
		},
		{
			name: "CRLF input",
			in:   "```json\r\n{\"a\":1}\r\n```",
			want: "{\"a\":1}",
		},
		{
			name: "no fence passthrough",
			in:   "plain text",
			want: "plain text",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripCodeFence(tt.in); got != tt.want {
				t.Errorf("stripCodeFence() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeMessage_RecoversFencedCalls(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		args    map[string]any
	}{
		{
			name:    "prose before the fence",
			content: "Sure, I'll create that file:\n```json\n{\"name\":\"edit_file\",\"arguments\":{\"path\":\"a.go\"}}\n```",
			want:    "edit_file",
			args:    map[string]any{"path": "a.go"},
		},
		{
			name:    "indented closing fence",
			content: "```json\n{\"name\":\"edit_file\",\"arguments\":{\"path\":\"a.go\"}}\n   ```",
			want:    "edit_file",
			args:    map[string]any{"path": "a.go"},
		},
		{
			name:    "trailing prose after the fence",
			content: "```json\n{\"name\":\"list_directory\",\"arguments\":{}}\n```\nLet me know if you need anything else!",
			want:    "list_directory",
			args:    map[string]any{},
		},
		{
			name:    "single-line fence",
			content: "```json {\"name\":\"list_directory\",\"arguments\":{}} ```",
			want:    "list_directory",
			args:    map[string]any{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Agent{}
			got := a.normalizeMessage(ollama.Message{Role: "assistant", Content: tt.content})
			if len(got.ToolCalls) != 1 {
				t.Fatalf("got %d tool calls, want 1", len(got.ToolCalls))
			}
			if got.ToolCalls[0].Function.Name != tt.want {
				t.Errorf("name = %q, want %q", got.ToolCalls[0].Function.Name, tt.want)
			}
			if !reflect.DeepEqual(got.ToolCalls[0].Function.Arguments, tt.args) {
				t.Errorf("arguments = %#v, want %#v", got.ToolCalls[0].Function.Arguments, tt.args)
			}
		})
	}
}
