package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
