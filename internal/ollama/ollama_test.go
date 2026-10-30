package ollama

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

// bench has to tell a dead server from a slow one, so Chat marks each case with
// a sentinel error instead of a plain message.
func TestChat_ClassifiesTransportFailures(t *testing.T) {
	t.Run("unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := srv.URL
		srv.Close() // nothing is listening on that address now

		_, err := Chat(ChatRequest{}, url)
		if !errors.Is(err, ErrUnreachable) {
			t.Errorf("err = %v, want ErrUnreachable", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			io.WriteString(w, `{"message":{"role":"assistant","content":"late"}}`)
		}))
		defer srv.Close()

		old := Client
		Client = &http.Client{Timeout: 20 * time.Millisecond}
		t.Cleanup(func() { Client = old })

		_, err := Chat(ChatRequest{}, srv.URL)
		if !errors.Is(err, ErrTimeout) {
			t.Errorf("err = %v, want ErrTimeout", err)
		}
	})
}
