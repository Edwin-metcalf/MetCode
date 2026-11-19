package ui

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Edwin-metcalf/MetCode/internal/agent"
	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/plan"
	"github.com/Edwin-metcalf/MetCode/internal/verify"
)

// captureStdout runs f and returns everything it printed.
// It swaps os.Stdout, so tests using it must not call t.Parallel().
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// newTestCLI builds a CLI whose Scanner reads scripted input instead of stdin.
func newTestCLI(host, input string) *CLI {
	return &CLI{
		Scanner:      bufio.NewScanner(strings.NewReader(input)),
		Agent:        &agent.Agent{Host: host, Model: "old-model"},
		SystemPrompt: ollama.Message{Role: "system", Content: "sys"},
	}
}

func TestHandleCommand_UnknownCommandPrintsHint(t *testing.T) {
	c := newTestCLI("", "")
	history := []ollama.Message{c.SystemPrompt}

	out := captureStdout(t, func() { c.handleCommand("/nope", &history) })

	if !strings.Contains(out, "/help") {
		t.Errorf("expected a hint pointing at /help, got %q", out)
	}
}

func TestHandleCommand_VerifySetShowOff(t *testing.T) {
	c := newTestCLI("", "")
	history := []ollama.Message{c.SystemPrompt}

	out := captureStdout(t, func() {
		c.handleCommand("/verify go test ./...", &history)
		c.handleCommand("/verify", &history)
	})

	if !strings.Contains(out, "verification set to: go test ./...") {
		t.Errorf("set output = %q", out)
	}
	if !strings.Contains(out, "verification is set to: go test ./...") {
		t.Errorf("show output = %q", out)
	}
	if c.Agent.Verify == nil {
		t.Error("verify hook should be set")
	}
	if want := []string{"go", "test", "./..."}; !slices.Equal(c.VerifyCommand, want) {
		t.Errorf("VerifyCommand = %q, want %q", c.VerifyCommand, want)
	}

	out = captureStdout(t, func() { c.handleCommand("/verify off", &history) })
	if !strings.Contains(out, "verification disabled") {
		t.Errorf("off output = %q", out)
	}
	if c.Agent.Verify != nil || c.VerifyCommand != nil {
		t.Error("verification should be disabled")
	}
}

func TestPrintVerify(t *testing.T) {
	cases := []struct {
		name  string
		res   verify.Result
		final bool
		want  string
	}{
		{
			name: "pass",
			res:  verify.Result{Command: []string{"go", "test"}, Passed: true},
			want: "[verify] go test -> pass",
		},
		{
			name: "fail retrying",
			res:  verify.Result{Command: []string{"go", "test"}, Output: "FAIL: broken"},
			want: "[verify] go test -> fail, retrying",
		},
		{
			name:  "fail final",
			res:   verify.Result{Command: []string{"go", "test"}, Output: "FAIL: broken"},
			final: true,
			want:  "[verify] go test -> fail, retry limit reached",
		},
		{
			name: "start error",
			res:  verify.Result{Command: []string{"go", "test"}, Err: errors.New("boom")},
			want: "[verify] go test -> error",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := captureStdout(t, func() { printVerify(c.res, c.final) })
			if !strings.Contains(out, c.want) {
				t.Errorf("output = %q, want it to contain %q", out, c.want)
			}
			if c.res.Output != "" && !strings.Contains(out, c.res.Output) {
				t.Errorf("output = %q, want test output %q", out, c.res.Output)
			}
		})
	}
}

func TestHandleCommand_ClearResetsHistoryAndPlan(t *testing.T) {
	c := newTestCLI("", "")
	c.Agent.Plan = []plan.Item{{Id: 1, Description: "step", Status: plan.StatusPending}}
	history := []ollama.Message{
		c.SystemPrompt,
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}

	c.handleCommand("/clear", &history)

	if len(history) != 1 || history[0].Role != "system" {
		t.Errorf("history should be just the system prompt, got %+v", history)
	}
	if len(c.Agent.Plan) != 0 {
		t.Errorf("plan should be cleared, got %d items: %v", len(c.Agent.Plan), c.Agent.Plan[0].Description)
	}
}

func TestHandleCommand_ChangeModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"models":[{"name":"llama3.1"},{"name":"qwen2.5-coder"}]}`)
	}))
	defer srv.Close()

	cases := []struct {
		name  string
		input string // what the user types at the model prompt
		want  string
	}{
		{"pick by number", "2\n", "qwen2.5-coder"},
		{"pick by name", "llama3.1\n", "llama3.1"},
		{"number out of range keeps current", "99\n", "old-model"},
		{"garbage keeps current", "abc\n", "old-model"},
		{"no input keeps current", "", "old-model"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestCLI(srv.URL, tc.input)
			history := []ollama.Message{c.SystemPrompt}

			captureStdout(t, func() { c.handleCommand("/change-model", &history) })

			if c.Agent.Model != tc.want {
				t.Errorf("model = %q, want %q", c.Agent.Model, tc.want)
			}
		})
	}
}
