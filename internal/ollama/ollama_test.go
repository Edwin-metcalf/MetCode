package ollama

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChat_SendsRequestAndDecodesToolCalls(t *testing.T) {
	var gotMethod, gotPath string
	var gotReq ChatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotReq)
		io.WriteString(w, `{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"read_file","arguments":{"path":"main.go"}}}]}}`)
	}))
	defer srv.Close()

	req := ChatRequest{
		Model:    "llama3.1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}
	resp, err := Chat(req, srv.URL)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	// what the server received
	if gotMethod != http.MethodPost || gotPath != "/api/chat" {
		t.Errorf("request was %s %s, want POST /api/chat", gotMethod, gotPath)
	}
	if gotReq.Model != "llama3.1" || len(gotReq.Messages) != 1 {
		t.Errorf("server received unexpected request: %+v", gotReq)
	}

	// what we decoded
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(resp.Message.ToolCalls))
	}
	call := resp.Message.ToolCalls[0]
	if call.Function.Name != "read_file" || call.Function.Arguments["path"] != "main.go" {
		t.Errorf("tool call decoded wrong: %+v", call)
	}
}
