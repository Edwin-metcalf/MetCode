package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/tools"
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
		{"fenced json", "```json\n{\"name\":\"read_file\",\"arguments\":{\"path\":\"hi.txt\"}}\n```"},
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
// readable error back and can correct itself (rather than us printing raw JSON).
// "frobnicate" is used on purpose: create_file/write_file/update_file are aliased
// to edit_file now, so they would succeed instead of reaching the unknown-tool
// path. Aliases are covered by TestTurn_AliasIsResolvedToEditFile.
func TestTurn_HallucinatedToolNameIsRoutedToHandle(t *testing.T) {
	host := fakeOllama(t,
		ollama.ChatResponse{Message: ollama.Message{
			Role:    "assistant",
			Content: `{"name":"frobnicate","arguments":{"path":"x.txt"}}`,
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
	if got := history[1].ToolCalls[0].Function.Name; got != "frobnicate" {
		t.Errorf("salvaged tool name = %q, want frobnicate", got)
	}
	if !strings.HasPrefix(history[2].Content, "error") {
		t.Errorf("tool result = %q, want an error the model can react to", history[2].Content)
	}
	if reply != "I will use edit_file instead" {
		t.Errorf("reply = %q, want the model's follow-up", reply)
	}
}

// a text call that names a tool the schema no longer has is resolved to the
// canonical tool instead of failing as unknown. old_text must be present (even
// empty) or edit_file rejects the call before it can create the file.
func TestTurn_AliasIsResolvedToEditFile(t *testing.T) {
	host := fakeOllama(t,
		ollama.ChatResponse{Message: ollama.Message{
			Role:    "assistant",
			Content: `{"name":"create_file","arguments":{"path":"x.txt","old_text":"","new_text":"hi"}}`,
		}},
		ollama.ChatResponse{Message: ollama.Message{Role: "assistant", Content: "done"}},
	)
	root := t.TempDir()
	a := &Agent{Model: "test", Host: host, Root: root}
	var history []ollama.Message

	// edit_file asks for a y/n confirmation; answer it without touching os.Stdin
	oldApprove, oldQuiet := tools.Approve, tools.Quiet
	tools.Approve = func(string) bool { return true }
	tools.Quiet = true
	t.Cleanup(func() { tools.Approve, tools.Quiet = oldApprove, oldQuiet })

	reply, err := a.Turn(&history, "make a file")
	if err != nil {
		t.Fatalf("Turn returned error: %v", err)
	}
	if reply != "done" {
		t.Errorf("reply = %q, want %q", reply, "done")
	}
	if len(history) != 4 {
		t.Fatalf("history has %d messages, want 4", len(history))
	}
	if got := history[1].ToolCalls[0].Function.Name; got != "edit_file" {
		t.Errorf("aliased tool name = %q, want edit_file", got)
	}
	if want := "x.txt file created successfully"; !strings.HasSuffix(history[2].Content, want) {
		t.Errorf("tool result = %q, want it to end with %q", history[2].Content, want)
	}
	// the real proof the call reached the file handler, not just the switch
	body, err := os.ReadFile(filepath.Join(root, "x.txt"))
	if err != nil {
		t.Fatalf("reading the created file: %v", err)
	}
	if string(body) != "hi" {
		t.Errorf("x.txt = %q, want %q", body, "hi")
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

// ---- compaction ----

// captureOllama replies with the given summary and records the request it
// received, so tests can inspect what CompactHistory actually sent.
func captureOllama(t *testing.T, summary string, got *ollama.ChatRequest) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(got); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		json.NewEncoder(w).Encode(ollama.ChatResponse{
			Message: ollama.Message{Role: "assistant", Content: summary},
		})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestTrimMiddle_ShortUnchanged(t *testing.T) {
	s := strings.Repeat("a", maxToolText)
	if got := trimMiddle(s); got != s {
		t.Fatalf("text at the threshold should pass through unchanged")
	}
}

func TestTrimMiddle_LongKeepsEnds(t *testing.T) {
	s := "START" + strings.Repeat("x", 3000) + "END"
	got := trimMiddle(s)

	if len(got) >= len(s) {
		t.Fatalf("output (%d) should be shorter than input (%d)", len(got), len(s))
	}
	if !strings.HasPrefix(got, "START") || !strings.HasSuffix(got, "END") {
		t.Fatalf("head and tail should survive, got %q...%q", got[:10], got[len(got)-10:])
	}
	if !strings.Contains(got, "[truncated]") {
		t.Fatalf("missing truncation marker")
	}
}

func TestTrimMiddle_NeverGrowsJustOverThreshold(t *testing.T) {
	s := strings.Repeat("a", maxToolText+1)
	if got := trimMiddle(s); len(got) > len(s) {
		t.Fatalf("output (%d) grew past input (%d)", len(got), len(s))
	}
}

func TestTrimMiddle_DoesNotSplitRunes(t *testing.T) {
	// the leading "a" shifts every "é" onto odd byte offsets, so the raw
	// cut at byte 400 lands in the middle of a character
	s := "a" + strings.Repeat("é", 600)
	got := trimMiddle(s)
	if !utf8.ValidString(got) {
		t.Fatalf("result is not valid UTF-8: a character was split")
	}
}

func TestTranscript(t *testing.T) {
	history := []ollama.Message{
		{Role: "system", Content: "SECRET SYSTEM PROMPT"},
		{Role: "user", Content: "what does agent.go do?"},
		{Role: "assistant", ToolCalls: []ollama.ToolCall{{
			Function: ollama.CalledFunction{
				Name:      "read_file",
				Arguments: map[string]any{"path": "internal/agent/agent.go"},
			},
		}}},
		{Role: "tool", Content: "package agent"},
		{Role: "assistant", Content: "It runs the tool loop."},
	}
	got := transcript(history)

	if strings.Contains(got, "SECRET SYSTEM PROMPT") {
		t.Errorf("system prompt should be skipped")
	}
	for _, want := range []string{
		"USER: what does agent.go do?",
		"TOOL CALL: read_file",
		"internal/agent/agent.go", // the path must survive
		"TOOL RESULT: package agent",
		"ASSISTANT: It runs the tool loop.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("transcript missing %q\n--- got ---\n%s", want, got)
		}
	}
	// the tool-call-only assistant message has no text, so no blank label
	if strings.Contains(got, "ASSISTANT: \n") {
		t.Errorf("empty assistant content should not produce a blank line")
	}
}

func TestTranscript_TrimsLongToolResult(t *testing.T) {
	big := "HEAD" + strings.Repeat("x", 5000) + "TAIL"
	got := transcript([]ollama.Message{{Role: "tool", Content: big}})

	if len(got) > 1000 {
		t.Errorf("tool result not trimmed, transcript is %d bytes", len(got))
	}
	if !strings.Contains(got, "HEAD") || !strings.Contains(got, "TAIL") {
		t.Errorf("head and tail of the tool result should be kept")
	}
}

func TestCompactHistory_RequestShape(t *testing.T) {
	var req ollama.ChatRequest
	host := captureOllama(t, "the summary", &req)
	a := &Agent{Model: "test", Host: host}

	history := []ollama.Message{
		{Role: "system", Content: "SECRET SYSTEM PROMPT"},
		{Role: "user", Content: "read my file"},
		{Role: "assistant", ToolCalls: []ollama.ToolCall{{
			Function: ollama.CalledFunction{
				Name:      "read_file",
				Arguments: map[string]any{"path": "main.go"},
			},
		}}},
		{Role: "tool", Content: "package main"},
	}

	summary, err := a.CompactHistory(&history)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary != "the summary" {
		t.Errorf("got summary %q", summary)
	}

	// one flat user message, never an assistant message (the original bug)
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
		t.Fatalf("want exactly one user message, got %+v", req.Messages)
	}
	// no tool definitions on the compaction call
	if len(req.Tools) != 0 {
		t.Errorf("compaction call should send no tools, got %d", len(req.Tools))
	}

	content := req.Messages[0].Content
	if strings.Contains(content, "SECRET SYSTEM PROMPT") {
		t.Errorf("system prompt should not be summarized")
	}
	if !strings.Contains(content, "main.go") {
		t.Errorf("file path from the tool call should reach the model")
	}

	// the instruction must come after the transcript
	end := strings.Index(content, "</conversation>")
	if end < 0 {
		t.Fatalf("transcript should be wrapped in <conversation> tags")
	}
	if !strings.Contains(strings.ToLower(content[end:]), "summar") {
		t.Errorf("instruction should come after the transcript, got tail %q", content[end:])
	}
}

func TestCompactHistory_DoesNotMutateHistory(t *testing.T) {
	var req ollama.ChatRequest
	host := captureOllama(t, "the summary", &req)
	a := &Agent{Model: "test", Host: host}

	history := make([]ollama.Message, 2, 10) // spare capacity on purpose
	history[0] = ollama.Message{Role: "system", Content: "sys"}
	history[1] = ollama.Message{Role: "user", Content: "hi"}

	if _, err := a.CompactHistory(&history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Errorf("history length changed to %d", len(history))
	}
	if extra := history[:3][2]; extra.Content != "" || extra.Role != "" {
		t.Errorf("compaction wrote into the history's backing array: %+v", extra)
	}
}

func TestCompactHistory_EmptySummaryIsError(t *testing.T) {
	host := fakeOllama(t, ollama.ChatResponse{
		Message: ollama.Message{Role: "assistant", Content: ""},
	})
	a := &Agent{Model: "test", Host: host}

	history := []ollama.Message{{Role: "user", Content: "hi"}}
	if _, err := a.CompactHistory(&history); err == nil {
		t.Fatalf("an empty summary should return an error")
	}
}
