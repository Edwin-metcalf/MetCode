package bench

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Edwin-metcalf/MetCode/internal/agent"
	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/prompt"
	"github.com/Edwin-metcalf/MetCode/internal/tools"
)

// ---------- helpers ----------

// build turns "kind:text" steps into a history. kinds: user, asst, call, res.
// "call:edit_file" is an assistant message calling edit_file; "res:..." is a tool result.
func build(t *testing.T, steps ...string) []ollama.Message {
	t.Helper()
	var h []ollama.Message
	for _, s := range steps {
		kind, val, _ := strings.Cut(s, ":")
		switch kind {
		case "user":
			h = append(h, ollama.Message{Role: "user", Content: val})
		case "asst":
			h = append(h, ollama.Message{Role: "assistant", Content: val})
		case "res":
			h = append(h, ollama.Message{Role: "tool", Content: val})
		case "call":
			// built through JSON so the test doesn't need ollama's inner type names
			b, _ := json.Marshal(map[string]any{
				"role": "assistant",
				"tool_calls": []any{map[string]any{
					"function": map[string]any{"name": val, "arguments": map[string]any{}},
				}},
			})
			var m ollama.Message
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			if len(m.ToolCalls) != 1 || m.ToolCalls[0].Function.Name != val {
				t.Fatalf("ollama.Message did not decode a tool call from %s", b)
			}
			h = append(h, m)
		default:
			t.Fatalf("unknown step kind %q", kind)
		}
	}
	return h
}

func reply(msg map[string]any) string {
	b, _ := json.Marshal(map[string]any{
		"message": msg, "done": true, "prompt_eval_count": 10, "eval_count": 5,
	})
	return string(b)
}

func textReply(s string) string {
	return reply(map[string]any{"role": "assistant", "content": s})
}

func toolReply(name string, args map[string]any) string {
	return reply(map[string]any{
		"role": "assistant", "content": "",
		"tool_calls": []any{map[string]any{
			"function": map[string]any{"name": name, "arguments": args},
		}},
	})
}

// fakeOllama serves scripted replies in order, repeating the last one forever,
// and records every request body.
type fakeOllama struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
}

func newFakeOllama(t *testing.T, replies ...string) *fakeOllama {
	t.Helper()
	f := &fakeOllama{}
	n := 0
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.bodies = append(f.bodies, string(body))
		i := n
		if n < len(replies)-1 {
			n++
		}
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, replies[i])
	}))
	t.Cleanup(f.Close)
	return f
}

func runTask(t *testing.T, task Task, srv *fakeOllama) Result {
	t.Helper()
	res, err := Run(task, Config{Model: "fake-model", Host: srv.URL, RunIndex: 3, GitCommit: "abc123"})
	if err != nil {
		t.Fatalf("Run returned a harness error: %v", err)
	}
	return res
}

func total(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

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
	defer func() { os.Stdout = old }()

	f()

	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// UNVERIFIED: these tool names and argument shapes are guesses.
// Check them against the schemas in tools.All and adjust. Only the tests
// marked "needs tool schema" depend on them.
const (
	toolRead = "read_file"
	toolEdit = "edit_file"
	toolRun  = "run_command"
)

var (
	readAddArgs    = map[string]any{"path": "add.go"}
	readMissedArgs = map[string]any{"path": "does-not-exist.go"}
	fixAddArgs     = map[string]any{"path": "add.go", "old_text": "a - b", "new_text": "a + b"}
	runArgs        = map[string]any{"command": "go", "arguments": []any{"test", "./..."}}
)

// ---------- summarize ----------

func TestSummarize(t *testing.T) {
	cases := []struct {
		name        string
		steps       []string
		calls, errs map[string]int
	}{
		{"empty history", nil, map[string]int{}, map[string]int{}},
		{
			"no tools used",
			[]string{"user:hi", "asst:hello"},
			map[string]int{}, map[string]int{},
		},
		{
			"same tool twice, one fails, another tool between",
			[]string{
				"user:fix it",
				"call:edit_file", "res:ok",
				"call:read_file", "res:package fixture",
				"call:edit_file", "res:error: no match",
				"asst:done",
			},
			map[string]int{"edit_file": 2, "read_file": 1},
			map[string]int{"edit_file": 1},
		},
		{
			"errors go to the tool that failed, not the previous one",
			[]string{
				"call:read_file", "res:error: not found",
				"call:edit_file", "res:error: bad edit",
			},
			map[string]int{"read_file": 1, "edit_file": 1},
			map[string]int{"read_file": 1, "edit_file": 1},
		},
		{
			// Turn treats this as progress, so summarize must not call it an error
			"declined command is not a tool error",
			[]string{"call:run_command", "res:command canceled by user"},
			map[string]int{"run_command": 1},
			map[string]int{},
		},
		{
			"only tool results can be errors",
			[]string{"user:error: my build crashed", "asst:error handling is hard"},
			map[string]int{}, map[string]int{},
		},
		{
			"many calls to one tool",
			[]string{
				"call:read_file", "res:a", "call:read_file", "res:b",
				"call:read_file", "res:c", "call:read_file", "res:d",
				"call:read_file", "res:e",
			},
			map[string]int{"read_file": 5}, map[string]int{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls, errs := summarize(build(t, tc.steps...))
			if !reflect.DeepEqual(calls, tc.calls) {
				t.Errorf("calls = %v, want %v", calls, tc.calls)
			}
			if !reflect.DeepEqual(errs, tc.errs) {
				t.Errorf("errs = %v, want %v", errs, tc.errs)
			}
		})
	}
}

// ---------- Result as data ----------

func TestResultJSONKeys(t *testing.T) {
	b, err := json.Marshal(Result{ToolCalls: map[string]int{}, ToolErrors: map[string]int{}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"task", "model", "run_index", "num_ctx", "temperature", "prompt_hash", "git_commit",
		"passed", "tampered", "ended", "edit_applied", "final_reply", "transcript",
		"tool_calls", "tool_errors", "unnecessary_calls", "declined_commands",
		"rounds", "prompt_tokens", "output_tokens", "wall_ms",
	}
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("missing JSON key %q", k)
		}
	}
	if len(got) != len(want) {
		t.Errorf("Result has %d JSON keys, want %d: %s", len(got), len(want), b)
	}
}

func TestResultJSONRoundTrip(t *testing.T) {
	in := Result{
		Task: "t", Model: "m", RunIndex: 2, NumCtx: 8192, Temperature: 0.2,
		PromptHash: "h", GitCommit: "g",
		Passed: true, Tampered: true, Ended: "max_rounds", EditApplied: true,
		ToolCalls: map[string]int{"read_file": 3}, ToolErrors: map[string]int{"edit_file": 1},
		UnnecessaryCalls: 4, DeclinedCommands: 5,
		Rounds: 6, PromptTokens: 7, OutputTokens: 8, WallMS: 9,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Result
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip changed the result:\n in  %+v\n out %+v", in, out)
	}
}

func TestSummarizeMapsMarshalAsObjectsNotNull(t *testing.T) {
	// report reads these lines back, so {} and null must not be confused
	calls, errs := summarize(nil)
	b, _ := json.Marshal(Result{ToolCalls: calls, ToolErrors: errs})
	for _, want := range []string{`"tool_calls":{}`, `"tool_errors":{}`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("want %s in %s", want, b)
		}
	}
}

// ---------- Run, against a fake Ollama ----------
// A no-op model must fail and a scripted correct edit must pass: that is
// Milestone D, the self-test that lets you trust the numbers.

func TestRun_NoOpModelFails(t *testing.T) {
	task, _ := newAddWorkspace(t)
	srv := newFakeOllama(t, textReply("I looked and did nothing."))

	res := runTask(t, task, srv)

	if res.Ended != "ok" {
		t.Errorf("Ended = %q, want ok", res.Ended)
	}
	if res.Passed || res.Tampered || res.EditApplied {
		t.Errorf("a no-op model must not pass, tamper or edit: %+v", res)
	}
	if total(res.ToolCalls) != 0 || res.Rounds != 0 {
		t.Errorf("no tools were called, got calls %v rounds %d", res.ToolCalls, res.Rounds)
	}
	if res.PromptTokens != 10 || res.OutputTokens != 5 {
		t.Errorf("tokens = %d/%d, want 10/5 from the scripted reply", res.PromptTokens, res.OutputTokens)
	}
}

func TestRun_RecordsIdentity(t *testing.T) {
	task, _ := newAddWorkspace(t)
	srv := newFakeOllama(t, textReply("ok"))

	res := runTask(t, task, srv)

	sum := sha256.Sum256([]byte(prompt.Default()))
	if res.Task != task.Name || res.Model != "fake-model" || res.RunIndex != 3 || res.GitCommit != "abc123" {
		t.Errorf("identity fields wrong: %+v", res)
	}
	if res.NumCtx != agent.NumCTX || res.Temperature != agent.Temperature {
		t.Errorf("numctx/temperature = %d/%v, want %d/%v", res.NumCtx, res.Temperature, agent.NumCTX, agent.Temperature)
	}
	if res.PromptHash != hex.EncodeToString(sum[:]) {
		t.Errorf("PromptHash = %q", res.PromptHash)
	}
}

func TestRun_SendsModelSystemPromptAndTask(t *testing.T) {
	task, _ := newAddWorkspace(t)
	srv := newFakeOllama(t, textReply("ok"))
	runTask(t, task, srv)

	if len(srv.bodies) == 0 {
		t.Fatal("the fake server received no request")
	}
	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(srv.bodies[0]), &req); err != nil {
		t.Fatal(err)
	}
	if req.Model != "fake-model" {
		t.Errorf("request model = %q, want fake-model (is Agent.Model set?)", req.Model)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("want [system, user], got %d messages", len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != prompt.Default() {
		t.Errorf("first message should be the default system prompt, got role %q", req.Messages[0].Role)
	}
	if req.Messages[1].Role != "user" || req.Messages[1].Content != task.Prompt {
		t.Errorf("second message should be the task prompt, got %q", req.Messages[1].Content)
	}
}

func TestRun_RestoresApproveAndCleansWorkspace(t *testing.T) {
	task, _ := newAddWorkspace(t)
	srv := newFakeOllama(t, textReply("ok"))

	called := false
	old := tools.Approve
	tools.Approve = func(string) bool { called = true; return true }
	defer func() { tools.Approve = old }()

	before := workspaceCount()
	runTask(t, task, srv)
	if after := workspaceCount(); after != before {
		t.Errorf("Run left %d temp workspace(s) behind", after-before)
	}

	// functions can't be compared, so check it is our sentinel that is back
	tools.Approve("edit")
	if !called {
		t.Error("Run did not restore tools.Approve")
	}
}

func TestRun_ServerErrorIsModelError(t *testing.T) {
	task, _ := newAddWorkspace(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	res, err := Run(task, Config{Model: "fake-model", Host: srv.URL})
	if err != nil {
		t.Fatalf("a failing model is a data point, not a harness error: %v", err)
	}
	// a 500 is still a model failure for now; only transport failures are told
	// apart (see TestRun_UnreachableServerIsNotAModelError)
	if res.Ended != "model_error" {
		t.Errorf("Ended = %q, want model_error", res.Ended)
	}
	if res.Passed {
		t.Error("nothing was fixed, so it must not pass")
	}
}

func TestRun_UnreachableServerIsNotAModelError(t *testing.T) {
	task, _ := newAddWorkspace(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening on that address now

	res, err := Run(task, Config{Model: "fake-model", Host: url})
	if err != nil {
		t.Fatalf("an unreachable server is a data point, not a harness error: %v", err)
	}
	if res.Ended != "server_unreachable" {
		t.Errorf("Ended = %q, want server_unreachable", res.Ended)
	}
	if res.Passed {
		t.Error("nothing was fixed, so it must not pass")
	}
}

// stdout belongs to the JSON row, so Run must not print. The scripted edit is
// the noisiest path: [call], [tool], and the edit diff all used to print.
func TestRun_IsQuiet(t *testing.T) {
	task, _ := newAddWorkspace(t)
	srv := newFakeOllama(t, toolReply(toolEdit, fixAddArgs), textReply("Fixed Add."))

	out := captureStdout(t, func() { runTask(t, task, srv) })

	if out != "" {
		t.Errorf("Run printed to stdout, want nothing: %q", out)
	}
	if tools.Quiet {
		t.Error("Run did not restore tools.Quiet")
	}
}

func TestRun_NoToolsTaskIsNotGraded(t *testing.T) {
	task := loadTask(t, "greeting", `{"prompt": "Say hi.", "no_tools": true}`,
		map[string]string{"notes.txt": "scratch"})
	srv := newFakeOllama(t, textReply("hi!"))

	res := runTask(t, task, srv)

	if res.Ended != "ok" || res.Passed || res.UnnecessaryCalls != 0 || res.EditApplied {
		t.Errorf("unexpected result for a polite no_tools run: %+v", res)
	}
}

// ---------- Run: needs tool schema ----------

func TestRun_ScriptedCorrectEditPasses(t *testing.T) { // needs tool schema
	task, _ := newAddWorkspace(t)
	srv := newFakeOllama(t, toolReply(toolEdit, fixAddArgs), textReply("Fixed Add."))

	res := runTask(t, task, srv)

	if !res.Passed || res.Tampered {
		t.Errorf("a correct edit should pass: %+v", res)
	}
	if !res.EditApplied {
		t.Error("EditApplied should be true after a real edit")
	}
	if res.Ended != "ok" || res.Rounds != 1 {
		t.Errorf("Ended = %q Rounds = %d, want ok and 1", res.Ended, res.Rounds)
	}
	if res.ToolCalls[toolEdit] != 1 || total(res.ToolErrors) != 0 {
		t.Errorf("calls %v errors %v", res.ToolCalls, res.ToolErrors)
	}
}

func TestRun_CommandIsDeclinedAndCounted(t *testing.T) { // needs tool schema
	task, _ := newAddWorkspace(t)
	srv := newFakeOllama(t, toolReply(toolRun, runArgs), textReply("ok"))

	res := runTask(t, task, srv)

	if res.DeclinedCommands != 1 {
		t.Errorf("DeclinedCommands = %d, want 1", res.DeclinedCommands)
	}
	if res.ToolCalls[toolRun] != 1 {
		t.Errorf("ToolCalls = %v", res.ToolCalls)
	}
	if res.ToolErrors[toolRun] != 0 {
		t.Errorf("a declined command is not a tool error, got %v", res.ToolErrors)
	}
	if res.Passed || res.EditApplied {
		t.Errorf("nothing was edited: %+v", res)
	}
}

func TestRun_EndlessToolCallsHitMaxRounds(t *testing.T) { // needs tool schema
	task, _ := newAddWorkspace(t)
	srv := newFakeOllama(t, toolReply(toolRead, readAddArgs)) // repeats forever

	res := runTask(t, task, srv)

	if res.Ended != "max_rounds" {
		t.Errorf("Ended = %q, want max_rounds", res.Ended)
	}
	if res.ToolCalls[toolRead] < 12 || total(res.ToolErrors) != 0 {
		t.Errorf("calls %v errors %v", res.ToolCalls, res.ToolErrors)
	}
	if res.Passed || res.EditApplied {
		t.Errorf("reading files fixes nothing: %+v", res)
	}
}

func TestRun_FailingToolCallsHitTooManyFailures(t *testing.T) { // needs tool schema
	task, _ := newAddWorkspace(t)
	srv := newFakeOllama(t, toolReply(toolRead, readMissedArgs)) // repeats forever

	res := runTask(t, task, srv)

	if res.Ended != "too_many_failures" {
		t.Errorf("Ended = %q, want too_many_failures", res.Ended)
	}
	if res.ToolErrors[toolRead] == 0 || res.ToolErrors[toolRead] != res.ToolCalls[toolRead] {
		t.Errorf("every call should have failed: calls %v errors %v", res.ToolCalls, res.ToolErrors)
	}
}

func TestRun_ToolCallOnNoToolsTaskIsUnnecessary(t *testing.T) { // needs tool schema
	task := loadTask(t, "greeting", `{"prompt": "Say hi.", "no_tools": true}`,
		map[string]string{"notes.txt": "scratch"})
	srv := newFakeOllama(t,
		toolReply(toolRead, map[string]any{"path": "notes.txt"}), textReply("hi!"))

	res := runTask(t, task, srv)

	if res.UnnecessaryCalls != 1 {
		t.Errorf("UnnecessaryCalls = %d, want 1", res.UnnecessaryCalls)
	}
}
