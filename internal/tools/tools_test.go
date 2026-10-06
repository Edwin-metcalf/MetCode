package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/plan"
)

func TestReadFileHelper_ValidFile(t *testing.T) {
	root := t.TempDir()
	want := "hello metcode"
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "read_file",
			Arguments: map[string]any{"path": "hello.txt"},
		},
	}

	result := readFileHelper(call, root)

	if result.Role != "tool" {
		t.Errorf("Role is not tool it is %v", result.Role)
	}
	if result.Content == "" {
		t.Errorf("expected %q, got %q", want, result.Content)
	}
}

func TestReadDirectoryHelper_NoPath(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "read_file",
			Arguments: map[string]any{},
		},
	}
	result := readFileHelper(call, ".")
	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %q", result.Content)
	}
}

func TestProtectedFiles(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "main.go", "old_text": "package main\ngood code:)", "new_text": "bad code:("},
		},
	}
	result := editFileHelper(call, ".")

	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %v", result.Content)
	}
}

func TestEditFileValid(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "example.go", "old_text": "bad stuff :(", "new_text": "good stuff :)"},
		},
	}
	result := editFileHelper(call, ".")

	if result.Role != "tool" {
		t.Errorf("error: Role is not a tool it is: %v", result.Role)
	}

	if result.Content == "" {
		t.Error("content is empty")
	}
}

func TestReadFileHelper_PathEscapesRoot(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "read_file",
			Arguments: map[string]any{"path": "../../../passwords"},
		},
	}

	result := readFileHelper(call, ".")
	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %q", result.Content)
	}
}

func TestEditFileHelper_PathEscapesRoot(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "../outside.go", "old_text": "foo", "new_text": "bar"},
		},
	}
	result := editFileHelper(call, ".")
	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %q", result.Content)
	}
}

func TestUpdatePlanItemHelper_ValidUpdate(t *testing.T) {
	items := []plan.Item{
		{Id: 1.0, Description: "read main.go", Status: plan.StatusPending},
		{Id: 2.0, Description: "write tests", Status: plan.StatusPending},
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name: "update_plan",
			Arguments: map[string]any{
				"id":     1.0,
				"status": "in_progress",
			},
		},
	}

	result := updatePlanItemHelper(call, &items)

	if strings.Contains(result.Content, "error") {
		t.Errorf("expected success but got %q", result.Content)
	}

	if items[0].Status != plan.StatusInProgress {
		t.Errorf("expected item 0 to be in_progress, got %v", items[0].Status)
	}

	if items[1].Status != plan.StatusPending {
		t.Errorf("item 1 should be untouched, got %v", items[1].Status)
	}
}

func TestUpdatePlanItemHelper_InvalidStatus(t *testing.T) {
	items := []plan.Item{
		{Id: 1.0, Description: "read main.go", Status: plan.StatusPending},
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name: "update_plan",
			Arguments: map[string]any{
				"id":     1.0,
				"status": "completed", // not one of the three valid values
			},
		},
	}

	result := updatePlanItemHelper(call, &items)

	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected rejection of invalid status, got %q", result.Content)
	}
	if items[0].Status != plan.StatusPending {
		t.Errorf("item should be untouched after rejected update, got %v", items[0].Status)
	}
}

func TestCreatePlanHelper_MissingDescriptions(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "create_plan",
			Arguments: map[string]any{},
		},
	}
	var items []plan.Item
	result := createPlanHelper(call, &items)

	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %q", result.Content)
	}
	if len(items) != 0 {
		t.Errorf("plan should remain empty, got %d items", len(items))
	}
}

func TestCreatePlanHelper_DoesNotClobberExistingPlan(t *testing.T) {
	items := []plan.Item{{Id: 1.0, Description: "existing step", Status: plan.StatusInProgress}}
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "create_plan",
			Arguments: map[string]any{},
		},
	}
	_ = createPlanHelper(call, &items)

	if len(items) != 1 {
		t.Errorf("expected existing plan to survive a rejected call, got %d items", len(items))
	}
	if items[0].Status != plan.StatusInProgress {
		t.Errorf("existing item should be untouched")
	}
}

func TestCreateFileHelper_DoesNotTruncateExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	original := "important content"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "create_file",
			Arguments: map[string]any{"path": "notes.txt"},
		},
	}
	result := createFileHelper(call, root)

	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected an error for an existing file, got %q", result.Content)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("existing file was modified: now %q", got)
	}
}

func TestCreateFileHelper_RefusesProtectedFile(t *testing.T) {
	root := t.TempDir()
	original := "module example.com/m\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "create_file",
			Arguments: map[string]any{"path": "go.mod"},
		},
	}
	result := createFileHelper(call, root)

	if !strings.HasPrefix(result.Content, "error") {
		t.Errorf("create_file should refuse a protected file, got %q", result.Content)
	}
	got, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("protected file was modified: now %q", got)
	}
}

func TestCreateFileHelper_CreatesNewFileInSubdirectory(t *testing.T) {
	root := t.TempDir()

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "create_file",
			Arguments: map[string]any{"path": "sub/dir/example.go"},
		},
	}
	result := createFileHelper(call, root)

	if strings.HasPrefix(result.Content, "error") {
		t.Fatalf("expected success, got %q", result.Content)
	}
	if _, err := os.Stat(filepath.Join(root, "sub", "dir", "example.go")); err != nil {
		t.Errorf("expected nested file to exist: %v", err)
	}
}

func TestRunCommandHelper_RejectsPathEscapingRoot(t *testing.T) {
	cases := map[string]string{
		"parent escape": "../outside",
		"absolute path": "/etc",
		"interior dots": "a/../../etc",
		"nested escape": "sub/../../secrets",
	}

	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			call := &ollama.ToolCall{
				Function: ollama.CalledFunction{
					Name:      "run_command",
					Arguments: map[string]any{"command": "ls", "path": path},
				},
			}

			// returns before the y/n confirm, so this never touches stdin
			result := runCommandHelper(call, t.TempDir())

			if !strings.HasPrefix(result.Content, "error") {
				t.Errorf("path %q should be rejected, got %q", path, result.Content)
			}
		})
	}
}

// emptyStdin points os.Stdin at an empty reader so the y/n confirm in
// run_command returns immediately instead of blocking on the real terminal.
func emptyStdin(t *testing.T) {
	t.Helper()
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		r.Close()
	})
}

func TestRunCommandHelper_AcceptsPathInsideRoot(t *testing.T) {
	emptyStdin(t)

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "run_command",
			Arguments: map[string]any{"command": "ls", "path": "sub"},
		},
	}

	// a path inside root passes validation and reaches the confirm, which the
	// empty stdin declines
	result := runCommandHelper(call, root)

	if result.Content != "command canceled by user" {
		t.Errorf("expected the confirm to be reached, got %q", result.Content)
	}
}

func TestRunCommandHelper_DefaultsToRoot(t *testing.T) {
	root := t.TempDir()

	dir, err := resolveSafePath(root, "")
	if err != nil {
		t.Fatalf("empty path should resolve to root: %v", err)
	}
	if dir != root {
		t.Errorf("default dir = %q, want root %q", dir, root)
	}
}

func TestUpdatePlanItemHelper_UnknownIDIsAFailure(t *testing.T) {
	items := []plan.Item{
		{Id: 1.0, Description: "read main.go", Status: plan.StatusPending},
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name: "update_plan",
			Arguments: map[string]any{
				"id":     99.0,
				"status": "in_progress",
			},
		},
	}

	result := updatePlanItemHelper(call, &items)

	// the agent loop counts failures by this exact prefix (agent.go)
	if !strings.HasPrefix(result.Content, "error") {
		t.Errorf("unknown id must report as an error prefix, got %q", result.Content)
	}
	if len(items) != 1 || items[0].Status != plan.StatusPending {
		t.Errorf("plan should be untouched, got %+v", items)
	}
}
