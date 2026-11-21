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

func TestListModels_FailurePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := ListModels(srv.URL)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestVersion_ReadsVersion(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		io.WriteString(w, `{"version":"0.5.1"}`)
	}))
	defer srv.Close()

	got, err := Version(srv.URL)
	if err != nil {
		t.Fatalf("Version returned error: %v", err)
	}
	if gotPath != "/api/version" {
		t.Errorf("request path = %q, want /api/version", gotPath)
	}
	if got != "0.5.1" {
		t.Errorf("version = %q, want 0.5.1", got)
	}
}

func TestVersion_FailurePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := Version(srv.URL)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

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

// The configured client is the one that runs, so a test or caller can bound
// ListModels and Version with its timeout and transport exactly like Chat.
func TestListModelsAndVersion_UseTheConfiguredClient(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/api/tags":
			io.WriteString(w, `{"models":[{"name":"llama3.1"}]}`)
		case "/api/version":
			io.WriteString(w, `{"version":"0.5.1"}`)
		}
	}))
	defer srv.Close()

	original := Client
	t.Cleanup(func() { Client = original })
	used := 0
	Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used++
		return http.DefaultTransport.RoundTrip(r)
	})}

	if _, err := ListModels(srv.URL); err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}
	if _, err := Version(srv.URL); err != nil {
		t.Fatalf("Version returned error: %v", err)
	}
	if used != 2 {
		t.Errorf("configured client handled %d requests, want 2", used)
	}
	if len(paths) != 2 || paths[0] != "/api/tags" || paths[1] != "/api/version" {
		t.Errorf("paths = %v, want [/api/tags /api/version]", paths)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
