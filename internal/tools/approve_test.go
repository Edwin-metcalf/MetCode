package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
)

// recordApprove swaps the package-level Approve hook for a recorder that answers
// every prompt with answer, and restores the old hook when the test ends. The
// kind it is asked with is a contract bench relies on (bench/result.go declines
// kind "command" and allows everything else), so the tests below assert it.
func recordApprove(t *testing.T, answer bool) *[]string {
	t.Helper()
	old := Approve
	kinds := []string{}
	Approve = func(kind string) bool {
		kinds = append(kinds, kind)
		return answer
	}
	t.Cleanup(func() { Approve = old })
	return &kinds
}

func TestApprove_RunCommandAsksWithCommandKind(t *testing.T) {
	kinds := recordApprove(t, false)

	call := &ollama.ToolCall{Function: ollama.CalledFunction{
		Name:      "run_command",
		Arguments: map[string]any{"command": "go", "arguments": []any{"version"}},
	}}

	result := runCommandHelper(call, t.TempDir())

	if len(*kinds) != 1 || (*kinds)[0] != "command" {
		t.Errorf("run_command asked Approve with %v, want [command]", *kinds)
	}
	if result.Content != "command canceled by user" {
		t.Errorf("a declined command should say so, got %q", result.Content)
	}
}

func TestApprove_EditFileAsksWithEditKind(t *testing.T) {
	kinds := recordApprove(t, true)

	root := t.TempDir()
	path := filepath.Join(root, "code.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc old() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	call := &ollama.ToolCall{Function: ollama.CalledFunction{
		Name:      "edit_file",
		Arguments: map[string]any{"path": "code.go", "old_text": "func old() {}", "new_text": "func fresh() {}"},
	}}

	result := editFileHelper(call, root)

	if len(*kinds) != 1 || (*kinds)[0] != "edit" {
		t.Errorf("edit_file asked Approve with %v, want [edit]", *kinds)
	}
	if strings.HasPrefix(result.Content, "error") {
		t.Fatalf("an approved edit should succeed, got %q", result.Content)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "func fresh() {}") {
		t.Errorf("edit not applied, file = %q", got)
	}
}

func TestApprove_CreateFileAsksWithEditKind(t *testing.T) {
	kinds := recordApprove(t, true)

	root := t.TempDir()
	call := &ollama.ToolCall{Function: ollama.CalledFunction{
		Name:      "edit_file",
		Arguments: map[string]any{"path": "new.txt", "old_text": "", "new_text": "hello\n"},
	}}

	result := editFileHelper(call, root)

	if len(*kinds) != 1 || (*kinds)[0] != "edit" {
		t.Errorf("creating a file asked Approve with %v, want [edit]", *kinds)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); err != nil {
		t.Errorf("approved create should write the file, stat err = %v (result %q)", err, result.Content)
	}
}

// an edit that cannot apply is refused before the confirm, so the user is never
// asked to approve a change that will not happen.
func TestApprove_NotAskedWhenEditCannotApply(t *testing.T) {
	kinds := recordApprove(t, true)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	call := &ollama.ToolCall{Function: ollama.CalledFunction{
		Name:      "edit_file",
		Arguments: map[string]any{"path": "notes.txt", "old_text": "not present", "new_text": "x"},
	}}

	result := editFileHelper(call, root)

	if len(*kinds) != 0 {
		t.Errorf("a doomed edit must not prompt, asked Approve with %v", *kinds)
	}
	if !strings.HasPrefix(result.Content, "error") {
		t.Errorf("expected an error, got %q", result.Content)
	}
}
