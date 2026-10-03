package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
