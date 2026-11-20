package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/verify"
)

func editReply(path, content string) ollama.ChatResponse {
	return textReply(fmt.Sprintf(`{"name":"edit_file","arguments":{"path":%q,"content":%q}}`, path, content))
}

type verifyScript struct {
	results []verify.Result
	calls   int
	roots   []string
}

func (s *verifyScript) run(root string) verify.Result {
	i := min(s.calls, len(s.results)-1)
	s.calls++
	s.roots = append(s.roots, root)
	return s.results[i]
}

func passResult() verify.Result {
	return verify.Result{Command: []string{"go", "test", "./..."}, Passed: true}
}

func failResult(output string) verify.Result {
	return verify.Result{Command: []string{"go", "test", "./..."}, Output: output}
}

func countVerifyFeedback(history []ollama.Message) int {
	n := 0
	for _, m := range history {
		if m.Role == "system" && strings.HasPrefix(m.Content, "Verification failed") {
			n++
		}
	}
	return n
}

func TestTurn_VerifiesAfterEdit(t *testing.T) {
	fakeApproval(t, true)
	fixed := "package fixture\n\nfunc Add(a, b int) int { return a + b }\n"

	host := fakeOllama(t, editReply("add.go", fixed), textReply("done"))
	root := t.TempDir()
	script := &verifyScript{results: []verify.Result{passResult()}}
	reports := 0
	a := &Agent{
		Model: "test", Host: host, Root: root,
		Verify: script.run, VerifyReport: func(verify.Result, bool) { reports++ },
	}
	var stats TurnStats
	a.Stats = func(s TurnStats) { stats = s }
	var history []ollama.Message

	reply, err := a.Turn(&history, "fix add.go")
	if err != nil {
		t.Fatal(err)
	}

	if stats.Calls != 2 || stats.Rounds != 1 || stats.VerifyAttempts != 1 || !stats.VerifyPassed {
		t.Errorf("Calls=%d Rounds=%d VerifyAttempts=%d VerifyPassed=%v", stats.Calls, stats.Rounds, stats.VerifyAttempts, stats.VerifyPassed)
	}
	if script.calls != 1 || reports != 1 {
		t.Errorf("verifier calls=%d reports=%d, want 1 each", script.calls, reports)
	}
	if countVerifyFeedback(history) != 0 {
		t.Error("a passing verification must not add feedback to history")
	}
	if reply != "done" {
		t.Errorf("reply = %q, want done", reply)
	}
	body, err := os.ReadFile(filepath.Join(root, "add.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != fixed {
		t.Errorf("file = %q, want %q", body, fixed)
	}
}

func TestTurn_VerifyFailureLetsModelFix(t *testing.T) {
	fakeApproval(t, true)
	broken := "package fixture\n\nfunc Add(a, b int) int { return a - b }\n"
	fixed := "package fixture\n\nfunc Add(a, b int) int { return a + b }\n"

	host := fakeOllama(t,
		editReply("add.go", broken),
		textReply("done"),
		editReply("add.go", fixed),
		textReply("done"),
	)
	root := t.TempDir()
	verifierCalls := 0
	a := &Agent{Model: "test", Host: host, Root: root}
	a.Verify = func(root string) verify.Result {
		verifierCalls++
		body, err := os.ReadFile(filepath.Join(root, "add.go"))
		if err != nil {
			return failResult(err.Error())
		}
		if strings.Contains(string(body), "return a + b") {
			return passResult()
		}
		return failResult("FAIL: want 5, got 1")
	}
	var stats TurnStats
	a.Stats = func(s TurnStats) { stats = s }
	var history []ollama.Message

	if _, err := a.Turn(&history, "fix add.go"); err != nil {
		t.Fatal(err)
	}

	if stats.Calls != 4 || stats.Rounds != 2 || stats.VerifyAttempts != 2 || !stats.VerifyPassed {
		t.Errorf("Calls=%d Rounds=%d VerifyAttempts=%d VerifyPassed=%v", stats.Calls, stats.Rounds, stats.VerifyAttempts, stats.VerifyPassed)
	}
	if verifierCalls != 2 {
		t.Errorf("verifier calls = %d, want 2", verifierCalls)
	}
	if got := countVerifyFeedback(history); got != 1 {
		t.Errorf("verify feedback = %d, want 1", got)
	}
}

func TestTurn_VerifyFailureStopsWithoutNewEdit(t *testing.T) {
	fakeApproval(t, true)
	host := fakeOllama(t,
		editReply("add.go", "package fixture\nfunc Add(a, b int) int { return a - b }\n"),
		textReply("done"),
		textReply("still done"),
	)
	script := &verifyScript{results: []verify.Result{failResult("FAIL")}}
	a := &Agent{Model: "test", Host: host, Root: t.TempDir(), Verify: script.run}
	var stats TurnStats
	a.Stats = func(s TurnStats) { stats = s }
	var history []ollama.Message

	if _, err := a.Turn(&history, "fix add.go"); err != nil {
		t.Fatal(err)
	}

	if stats.Calls != 3 || stats.VerifyAttempts != 1 || stats.VerifyPassed {
		t.Errorf("Calls=%d VerifyAttempts=%d VerifyPassed=%v", stats.Calls, stats.VerifyAttempts, stats.VerifyPassed)
	}
	if got := countVerifyFeedback(history); got != 1 {
		t.Errorf("verify feedback = %d, want 1", got)
	}
}

func TestTurn_VerifyCappedAtTwoAttempts(t *testing.T) {
	fakeApproval(t, true)
	host := fakeOllama(t,
		editReply("add.go", "package fixture\nfunc Add(a, b int) int { return a - b }\n"),
		textReply("done"),
		editReply("add.go", "package fixture\nfunc Add(a, b int) int { return a + b }\n"),
		textReply("done"),
	)
	script := &verifyScript{results: []verify.Result{failResult("FAIL")}}
	a := &Agent{Model: "test", Host: host, Root: t.TempDir(), Verify: script.run}
	var stats TurnStats
	a.Stats = func(s TurnStats) { stats = s }
	var history []ollama.Message

	if _, err := a.Turn(&history, "fix add.go"); err != nil {
		t.Fatal(err)
	}

	if stats.Calls != 4 || stats.Rounds != 2 || stats.VerifyAttempts != 2 || stats.VerifyPassed {
		t.Errorf("Calls=%d Rounds=%d VerifyAttempts=%d VerifyPassed=%v", stats.Calls, stats.Rounds, stats.VerifyAttempts, stats.VerifyPassed)
	}
	if got := countVerifyFeedback(history); got != 2 {
		t.Errorf("verify feedback = %d, want 2", got)
	}
}

func TestTurn_VerifyStartErrorStops(t *testing.T) {
	fakeApproval(t, true)
	host := fakeOllama(t,
		editReply("add.go", "package fixture\nfunc Add(a, b int) int { return a + b }\n"),
		textReply("done"),
		textReply("should not be called"),
	)
	script := &verifyScript{results: []verify.Result{{Command: []string{"go", "test"}, Err: errors.New("boom")}}}
	a := &Agent{Model: "test", Host: host, Root: t.TempDir(), Verify: script.run}
	var stats TurnStats
	a.Stats = func(s TurnStats) { stats = s }
	var history []ollama.Message

	if _, err := a.Turn(&history, "fix add.go"); err != nil {
		t.Fatal(err)
	}

	if stats.Calls != 2 || stats.VerifyAttempts != 1 || stats.VerifyPassed {
		t.Errorf("Calls=%d VerifyAttempts=%d VerifyPassed=%v", stats.Calls, stats.VerifyAttempts, stats.VerifyPassed)
	}
	if got := countMessages(history, "Verification could not run"); got != 1 {
		t.Errorf("verification error messages = %d, want 1", got)
	}
}

func TestTurn_NoEditSkipsVerify(t *testing.T) {
	script := &verifyScript{results: []verify.Result{passResult()}}
	host := fakeOllama(t, textReply("just chatting"))
	a := &Agent{Model: "test", Host: host, Root: t.TempDir(), Verify: script.run}
	var stats TurnStats
	a.Stats = func(s TurnStats) { stats = s }
	var history []ollama.Message

	if _, err := a.Turn(&history, "hello"); err != nil {
		t.Fatal(err)
	}

	if script.calls != 0 || stats.VerifyAttempts != 0 {
		t.Errorf("verifier calls=%d VerifyAttempts=%d, want 0", script.calls, stats.VerifyAttempts)
	}
}

func TestTurn_CanceledEditSkipsVerify(t *testing.T) {
	fakeApproval(t, false)
	script := &verifyScript{results: []verify.Result{passResult()}}
	host := fakeOllama(t,
		editReply("add.go", "package fixture\nfunc Add(a, b int) int { return a + b }\n"),
		textReply("done"),
	)
	root := t.TempDir()
	a := &Agent{Model: "test", Host: host, Root: root, Verify: script.run, ExpectEdit: true}
	var stats TurnStats
	a.Stats = func(s TurnStats) { stats = s }
	var history []ollama.Message

	if _, err := a.Turn(&history, "fix add.go"); err != nil {
		t.Fatal(err)
	}

	if script.calls != 0 || stats.VerifyAttempts != 0 || stats.Nudges != 0 {
		t.Errorf("verifier calls=%d VerifyAttempts=%d Nudges=%d, want 0", script.calls, stats.VerifyAttempts, stats.Nudges)
	}
	if _, err := os.Stat(filepath.Join(root, "add.go")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("canceled edit created the file: %v", err)
	}
}

func TestVerificationFailureMessageClipsOutput(t *testing.T) {
	msg := verificationFailureMessage(failResult(strings.Repeat("z", 9000)))

	// a named command reads as "run this" to llama, so the model never sees it
	if strings.Contains(msg, "Command:") || strings.Contains(msg, "go test") {
		t.Errorf("message names the command: %q", msg[:200])
	}
	if !strings.Contains(msg, "edit_file") {
		t.Errorf("message does not point at edit_file: %q", msg[:200])
	}
	if got := strings.Count(msg, "z"); got != maxVerifyFeedback {
		t.Errorf("output runes = %d, want %d", got, maxVerifyFeedback)
	}
}

func countMessages(history []ollama.Message, prefix string) int {
	n := 0
	for _, m := range history {
		if m.Role == "system" && strings.HasPrefix(m.Content, prefix) {
			n++
		}
	}
	return n
}
