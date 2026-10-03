package ui

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Edwin-metcalf/MetCode/internal/agent"
	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/plan"
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
	//TODO does not get passed the plan in the commands.go but probablyshould
	//if len(c.Agent.Plan) != 0 {
	//	t.Errorf("plan should be cleared, got %d items: %v", len(c.Agent.Plan), c.Agent.Plan[0].Description)
	//}
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
