package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/tools"
)

// textReply is the shape a model uses when it answers in prose instead of
// calling a tool: role assistant, no tool_calls.
func textReply(content string) ollama.ChatResponse {
	return ollama.ChatResponse{Message: ollama.Message{Role: "assistant", Content: content}}
}

// fakeApproval keeps edit_file from reading os.Stdin and silences the diff.
func fakeApproval(t *testing.T, approve bool) {
	t.Helper()
	oldApprove, oldQuiet := tools.Approve, tools.Quiet
	tools.Approve = func(string) bool { return approve }
	tools.Quiet = true
	t.Cleanup(func() { tools.Approve, tools.Quiet = oldApprove, oldQuiet })
}

func countNudges(history []ollama.Message) int {
	n := 0
	for _, m := range history {
		if m.Role == "system" && strings.HasPrefix(m.Content, editNudge) {
			n++
		}
	}
	return n
}

// the nudge is a second copy of the schema's payload keys, so it has to be
// checked against tools.All or it drifts the same way the system prompt did.
func TestEditNudgeMatchesSchema(t *testing.T) {
	var props map[string]any
	for _, tool := range tools.All {
		if tool.Function.Name == "edit_file" {
			props = tool.Function.Parameters.Properties
			break
		}
	}
	if props == nil {
		t.Fatal("no edit_file tool in tools.All")
	}

	for _, key := range []string{"path", "old_text", "new_text", "content"} {
		if !strings.Contains(editNudge, key) {
			t.Errorf("nudge does not name %q, so the model is not told about it", key)
		}
		if _, ok := props[key]; !ok {
			t.Errorf("nudge names %q, which edit_file's schema does not declare", key)
		}
	}

	for _, alias := range []string{"write_file", "create_file", "update_file"} {
		if strings.Contains(editNudge, alias) {
			t.Errorf("nudge names %q, which is an alias, not a schema tool", alias)
		}
	}

	// the read escape hatch cost 14 of 19 nudges on llama3.1: it re-read the
	// file it had just read instead of sending a payload.
	if strings.Contains(editNudge, "call read_file first") {
		t.Error("nudge points at read_file again; it must send the payload instead")
	}
}

func TestTurn_NudgesOnceWhenNoEdit(t *testing.T) {
	host := fakeOllama(t, textReply("Change a - b to a + b."))
	a := &Agent{Model: "test", Host: host, Root: t.TempDir(), ExpectEdit: true}
	var stats TurnStats
	a.Stats = func(s TurnStats) { stats = s }
	var history []ollama.Message

	reply, err := a.Turn(&history, "fix add.go")
	if err != nil {
		t.Fatalf("Turn returned error: %v", err)
	}

	if stats.Calls != 2 || stats.Nudges != 1 || stats.Rounds != 0 {
		t.Errorf("Calls=%d Nudges=%d Rounds=%d, want 2, 1, 0", stats.Calls, stats.Nudges, stats.Rounds)
	}
	if got := countNudges(history); got != 1 {
		t.Errorf("nudge messages = %d, want exactly 1", got)
	}
	// user, assistant prose, system nudge, assistant prose
	if len(history) != 4 || history[2].Role != "system" {
		t.Fatalf("history = %#v, want the nudge between the two answers", history)
	}
	if reply != "Change a - b to a + b." {
		t.Errorf("reply = %q, want the last answer", reply)
	}
}

// the whole point: a turn that would have ended in prose gets one more chance
// and this time the model calls the tool.
func TestTurn_NudgeConvertsProseToEdit(t *testing.T) {
	fakeApproval(t, true)
	fixed := "package fixture\n\nfunc Add(a, b int) int { return a + b }\n"

	host := fakeOllama(t,
		textReply("I would change Add to return a + b."),
		textReply(`{"name":"edit_file","arguments":{"path":"add.go","content":"package fixture\n\nfunc Add(a, b int) int { return a + b }\n"}}`),
		textReply("done"),
	)
	root := t.TempDir()
	a := &Agent{Model: "test", Host: host, Root: root, ExpectEdit: true}
	var stats TurnStats
	a.Stats = func(s TurnStats) { stats = s }
	var history []ollama.Message

	reply, err := a.Turn(&history, "fix add.go")
	if err != nil {
		t.Fatalf("Turn returned error: %v", err)
	}

	if stats.Calls != 3 || stats.Nudges != 1 || stats.Rounds != 1 {
		t.Errorf("Calls=%d Nudges=%d Rounds=%d, want 3, 1, 1", stats.Calls, stats.Nudges, stats.Rounds)
	}
	if got := countNudges(history); got != 1 {
		t.Errorf("nudge messages = %d, want exactly 1", got)
	}
	body, err := os.ReadFile(filepath.Join(root, "add.go"))
	if err != nil {
		t.Fatalf("reading the edited file: %v", err)
	}
	if string(body) != fixed {
		t.Errorf("add.go = %q, want %q", body, fixed)
	}
	if reply != "done" {
		t.Errorf("reply = %q, want %q", reply, "done")
	}
}

// the common failure shape: an edit_file call errors, the model answers in
// prose, and the nudge gives it a chance to send a usable payload.
func TestTurn_NudgeAfterFailedEdit(t *testing.T) {
	fakeApproval(t, true)

	host := fakeOllama(t,
		textReply(`{"name":"edit_file","arguments":{"path":"x.txt"}}`),
		textReply("I could not apply that."),
		textReply(`{"name":"edit_file","arguments":{"path":"x.txt","content":"hi"}}`),
		textReply("done"),
	)
	root := t.TempDir()
	a := &Agent{Model: "test", Host: host, Root: root, ExpectEdit: true}
	var stats TurnStats
	a.Stats = func(s TurnStats) { stats = s }
	var history []ollama.Message

	if _, err := a.Turn(&history, "make x.txt"); err != nil {
		t.Fatalf("Turn returned error: %v", err)
	}

	if stats.Calls != 4 || stats.Nudges != 1 || stats.Rounds != 2 {
		t.Errorf("Calls=%d Nudges=%d Rounds=%d, want 4, 1, 2", stats.Calls, stats.Nudges, stats.Rounds)
	}

	nudgeIdx, errorIdx := -1, -1
	for i, m := range history {
		if m.Role == "system" && strings.HasPrefix(m.Content, editNudge) {
			nudgeIdx = i
		}
		if m.Role == "tool" && strings.HasPrefix(m.Content, "error") && errorIdx == -1 {
			errorIdx = i
		}
	}
	if nudgeIdx == -1 || errorIdx == -1 || errorIdx > nudgeIdx {
		t.Errorf("nudge at %d, first tool error at %d; the error must come first", nudgeIdx, errorIdx)
	}
	if body, err := os.ReadFile(filepath.Join(root, "x.txt")); err != nil || string(body) != "hi" {
		t.Errorf("x.txt = %q (err %v), want %q", body, err, "hi")
	}
}

func TestTurn_NoNudgeAfterSuccessfulEdit(t *testing.T) {
	fakeApproval(t, true)

	host := fakeOllama(t,
		textReply(`{"name":"edit_file","arguments":{"path":"x.txt","content":"hi"}}`),
		textReply("done"),
	)
	a := &Agent{Model: "test", Host: host, Root: t.TempDir(), ExpectEdit: true}
	var stats TurnStats
	a.Stats = func(s TurnStats) { stats = s }
	var history []ollama.Message

	if _, err := a.Turn(&history, "make x.txt"); err != nil {
		t.Fatalf("Turn returned error: %v", err)
	}

	if stats.Calls != 2 || stats.Nudges != 0 {
		t.Errorf("Calls=%d Nudges=%d, want 2, 0", stats.Calls, stats.Nudges)
	}
	if got := countNudges(history); got != 0 {
		t.Errorf("nudge messages = %d, want none after a successful edit", got)
	}
}

// a declined edit is a decision, not a failure, so the model is not nagged.
func TestTurn_DeclinedEditIsNotNudged(t *testing.T) {
	fakeApproval(t, false)

	host := fakeOllama(t,
		textReply(`{"name":"edit_file","arguments":{"path":"x.txt","content":"hi"}}`),
		textReply("OK, I will leave the file alone."),
	)
	a := &Agent{Model: "test", Host: host, Root: t.TempDir(), ExpectEdit: true}
	var stats TurnStats
	a.Stats = func(s TurnStats) { stats = s }
	var history []ollama.Message

	if _, err := a.Turn(&history, "make x.txt"); err != nil {
		t.Fatalf("Turn returned error: %v", err)
	}

	if stats.Nudges != 0 || countNudges(history) != 0 {
		t.Errorf("a declined edit must not be nudged: Nudges=%d", stats.Nudges)
	}
}

// the chat client leaves ExpectEdit off, so a plain answer stays a plain answer.
func TestTurn_NoNudgeWhenNotExpected(t *testing.T) {
	host := fakeOllama(t, textReply("hello"))
	a := &Agent{Model: "test", Host: host, Root: t.TempDir()}
	var stats TurnStats
	a.Stats = func(s TurnStats) { stats = s }
	var history []ollama.Message

	reply, err := a.Turn(&history, "hello")
	if err != nil {
		t.Fatalf("Turn returned error: %v", err)
	}

	if stats.Calls != 1 || stats.Nudges != 0 || stats.Rounds != 0 {
		t.Errorf("Calls=%d Nudges=%d Rounds=%d, want 1, 0, 0", stats.Calls, stats.Nudges, stats.Rounds)
	}
	if len(history) != 2 {
		t.Errorf("history has %d messages, want 2 (user, answer)", len(history))
	}
	if reply != "hello" {
		t.Errorf("reply = %q, want %q", reply, "hello")
	}
}

// llama3.1 answered "error: missing required parameter old_text" with "the bug
// has been fixed" 25 times out of 25. The nudge repeats the failure back so the
// model cannot mistake its own prose for a result.
func TestTurn_NudgeQuotesEditError(t *testing.T) {
	fakeApproval(t, true)

	host := fakeOllama(t,
		textReply(`{"name":"edit_file","arguments":{"path":"add.go"}}`),
		textReply("The bug in add.go has been fixed."),
		textReply("nothing more"),
	)
	a := &Agent{Model: "test", Host: host, Root: t.TempDir(), ExpectEdit: true}
	var history []ollama.Message

	if _, err := a.Turn(&history, "fix add.go"); err != nil {
		t.Fatalf("Turn returned error: %v", err)
	}

	var nudge string
	for _, m := range history {
		if m.Role == "system" && strings.HasPrefix(m.Content, editNudge) {
			nudge = m.Content
		}
	}
	if nudge == "" {
		t.Fatal("no nudge message in history")
	}
	if !strings.Contains(nudge, "Your last edit_file did not apply: ") {
		t.Errorf("nudge does not announce the failure: %q", nudge)
	}
	// the quoted text has to come from the handler, not a paraphrase, or the
	// model is told a different problem than the one it made.
	if !strings.Contains(nudge, "edit_file with only path") {
		t.Errorf("nudge does not quote the bare-path error: %q", nudge)
	}
}

// with no edit_file error there is nothing to quote, so the nudge stays the
// plain constant. This keeps the two variants distinguishable in the bench.
func TestTurn_NudgeWithoutErrorIsShort(t *testing.T) {
	host := fakeOllama(t, textReply("The bug is in add.go."))
	a := &Agent{Model: "test", Host: host, Root: t.TempDir(), ExpectEdit: true}
	var history []ollama.Message

	if _, err := a.Turn(&history, "fix add.go"); err != nil {
		t.Fatalf("Turn returned error: %v", err)
	}

	var nudge string
	for _, m := range history {
		if m.Role == "system" {
			nudge = m.Content
		}
	}
	if nudge != editNudge {
		t.Errorf("nudge = %q, want the plain constant %q", nudge, editNudge)
	}
}

func TestClip(t *testing.T) {
	if got := clip("short", 10); got != "short" {
		t.Errorf("clip(%q, 10) = %q, want unchanged", "short", got)
	}
	// runes, not bytes: a quoted error with a non-ASCII path must not be cut
	// mid-character.
	got := clip(strings.Repeat("é", 5), 3)
	if got != "ééé..." {
		t.Errorf("clip of 5 multi-byte runes to 3 = %q, want 3 runes + ellipsis", got)
	}
}
