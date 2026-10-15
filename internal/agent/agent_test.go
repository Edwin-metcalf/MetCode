package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
)

// fakeOllama serves the replies in order, repeating the last one forever.
func fakeOllama(t *testing.T, replies ...ollama.ChatResponse) string {
	t.Helper()
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := min(n, len(replies)-1)
		n++
		json.NewEncoder(w).Encode(replies[i])
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func toolReply(name string) ollama.ChatResponse {
	return ollama.ChatResponse{Message: ollama.Message{
		Role:      "tool",
		ToolCalls: []ollama.ToolCall{{Function: ollama.CalledFunction{Name: name}}},
	}}
}

func TestTurn_ToolCallThenAnswer(t *testing.T) {
	host := fakeOllama(t,
		toolReply("list_directory"),
		ollama.ChatResponse{Message: ollama.Message{Role: "tool", Content: "all done"}},
	)
	a := &Agent{Model: "test", Host: host, Root: t.TempDir()}
	var history []ollama.Message

	reply, err := a.Turn(&history, "what files are here?")
	if err != nil {
		t.Fatalf("Turn returned error: %v", err)
	}
	if reply != "all done" {
		t.Errorf("reply = %q, want %q", reply, "all done")
	}
	// user, assistant (tool call), tool result, assistant (answer)
	if len(history) != 4 {
		t.Fatalf("history has %d messages, want 4", len(history))
	}
	if history[2].Role != "tool" {
		t.Errorf("history[2].Role = %q, want tool", history[2].Role)
	}
}
func TestTurn_StatsSumAcrossRounds(t *testing.T) {
	host := fakeOllama(t,
		ollama.ChatResponse{
			Message: ollama.Message{Role: "assistant", ToolCalls: []ollama.ToolCall{
				{Function: ollama.CalledFunction{Name: "list_directory"}},
			}},
			PromptEvalCount:    2000,
			EvalCount:          10,
			PromptEvalDuration: int64(time.Second),
		},
		ollama.ChatResponse{
			Message:         ollama.Message{Role: "assistant", Content: "all done"},
			PromptEvalCount: 300,
			EvalCount:       5,
			EvalDuration:    int64(500 * time.Millisecond),
		},
	)
	a := &Agent{Model: "test", Host: host, Root: t.TempDir()}

	var got TurnStats
	a.Stats = func(s TurnStats) { got = s }

	var history []ollama.Message
	if _, err := a.Turn(&history, "what files are here?"); err != nil {
		t.Fatalf("Turn returned error: %v", err)
	}

	if got.Calls != 2 {
		t.Errorf("Calls = %d, want 2 (one tool round plus the final answer)", got.Calls)
	}
	if got.Rounds != 1 {
		t.Errorf("Rounds = %d, want 1", got.Rounds)
	}
	if got.PromptTokens != 2300 {
		t.Errorf("PromptTokens = %d, want 2300 (2000 + 300)", got.PromptTokens)
	}
	if got.OutputTokens != 15 {
		t.Errorf("OutputTokens = %d, want 15 (10 + 5)", got.OutputTokens)
	}
	if got.PrefillTime != time.Second {
		t.Errorf("PrefillTime = %v, want 1s", got.PrefillTime)
	}
	if got.GenTime != 500*time.Millisecond {
		t.Errorf("GenTime = %v, want 500ms", got.GenTime)
	}
	if got.WallTime <= 0 {
		t.Errorf("WallTime = %v, want > 0", got.WallTime)
	}
}

// the defer means a turn that dies still reports what it measured, exactly once
func TestTurn_StatsCalledOnceOnError(t *testing.T) {
	host := fakeOllama(t, toolReply("list_directory")) // never stops asking for a tool

	a := &Agent{Model: "test", Host: host, Root: t.TempDir()}
	calls := 0
	a.Stats = func(TurnStats) { calls++ }

	var history []ollama.Message
	if _, err := a.Turn(&history, "keep going"); err == nil {
		t.Fatal("want an error once the tool-round cap is hit, got nil")
	}
	if calls != 1 {
		t.Errorf("Stats called %d times, want exactly 1", calls)
	}
}

// normalizeMessage is the unit under test: it turns a tool call the model
// wrote as plain text into a real ToolCall on the message.
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
func TestTurn_SalvagesToolCallFromContent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hi.txt"), []byte("hello file"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		content string
	}{
		{"flat arguments", `{"name":"read_file","arguments":{"path":"hi.txt"}}`},
		{"flat parameters", `{"name":"read_file","parameters":{"path":"hi.txt"}}`},
		{"nested function", `{"function":{"name":"read_file","arguments":{"path":"hi.txt"}}}`},
		{"tool_call tags", `<tool_call>
{"name":"read_file","arguments":{"path":"hi.txt"}}
</tool_call>`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := fakeOllama(t,
				ollama.ChatResponse{Message: ollama.Message{Role: "assistant", Content: tt.content}},
				ollama.ChatResponse{Message: ollama.Message{Role: "assistant", Content: "done"}},
			)
			a := &Agent{Model: "test", Host: host, Root: root}
			var stats TurnStats
			a.Stats = func(s TurnStats) { stats = s }

			var history []ollama.Message
			reply, err := a.Turn(&history, "read hi.txt")
			if err != nil {
				t.Fatalf("Turn returned error: %v", err)
			}

			if reply != "done" {
				t.Errorf("reply = %q, want %q", reply, "done")
			}
			if stats.Rounds != 1 {
				t.Errorf("Rounds = %d, want 1 (the salvaged call should drive one tool round)", stats.Rounds)
			}
			// user, assistant (salvaged tool call), tool result, assistant (answer)
			if len(history) != 4 {
				t.Fatalf("history has %d messages, want 4", len(history))
			}
			if len(history[1].ToolCalls) != 1 {
				t.Fatalf("assistant message has %d tool calls, want 1", len(history[1].ToolCalls))
			}
			if got := history[1].ToolCalls[0].Function.Name; got != "read_file" {
				t.Errorf("salvaged tool name = %q, want read_file", got)
			}
			if history[1].Content != "" {
				t.Errorf("assistant content = %q, want empty", history[1].Content)
			}
			if history[2].Role != "tool" {
				t.Errorf("history[2].Role = %q, want tool", history[2].Role)
			}
			// the real proof the arguments survived: the tool read the file
			if history[2].Content != "hello file" {
				t.Errorf("tool result = %q, want %q", history[2].Content, "hello file")
			}
		})
	}
}

// a hallucinated tool name is still routed to Handle, so the model gets a
// readable error back and can correct itself (rather than us printing raw JSON)
func TestTurn_HallucinatedToolNameIsRoutedToHandle(t *testing.T) {
	host := fakeOllama(t,
		ollama.ChatResponse{Message: ollama.Message{
			Role:    "assistant",
			Content: `{"name":"create_file","arguments":{"path":"x.txt","new_text":"hi"}}`,
		}},
		ollama.ChatResponse{Message: ollama.Message{Role: "assistant", Content: "I will use edit_file instead"}},
	)
	a := &Agent{Model: "test", Host: host, Root: t.TempDir()}
	var history []ollama.Message

	reply, err := a.Turn(&history, "make a file")
	if err != nil {
		t.Fatalf("Turn returned error: %v", err)
	}
	if len(history) != 4 {
		t.Fatalf("history has %d messages, want 4", len(history))
	}
	if got := history[1].ToolCalls[0].Function.Name; got != "create_file" {
		t.Errorf("salvaged tool name = %q, want create_file", got)
	}
	if !strings.HasPrefix(history[2].Content, "error") {
		t.Errorf("tool result = %q, want an error the model can react to", history[2].Content)
	}
	if reply != "I will use edit_file instead" {
		t.Errorf("reply = %q, want the model's follow-up", reply)
	}
}

// the guard that keeps this feature from turning every reply into a tool call
func TestTurn_PlainProseIsNotSalvaged(t *testing.T) {
	host := fakeOllama(t, ollama.ChatResponse{Message: ollama.Message{
		Role:    "assistant",
		Content: "hello there",
	}})
	a := &Agent{Model: "test", Host: host, Root: t.TempDir()}
	var stats TurnStats
	a.Stats = func(s TurnStats) { stats = s }

	var history []ollama.Message
	reply, err := a.Turn(&history, "hi")
	if err != nil {
		t.Fatalf("Turn returned error: %v", err)
	}

	if reply != "hello there" {
		t.Errorf("reply = %q, want %q", reply, "hello there")
	}
	if stats.Rounds != 0 {
		t.Errorf("Rounds = %d, want 0 (no tool should have run)", stats.Rounds)
	}
	// user, assistant only
	if len(history) != 2 {
		t.Fatalf("history has %d messages, want 2", len(history))
	}
	if len(history[1].ToolCalls) != 0 {
		t.Errorf("assistant message has %d tool calls, want 0", len(history[1].ToolCalls))
	}
}
