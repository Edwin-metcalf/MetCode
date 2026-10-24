package bench

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeFiles creates every file in files (paths are slash-separated and
// relative to root), making parent directories as needed.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadTasks(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"greeting/task.json":   `{"prompt": "hello", "no_tools": true}`,
		"add-bug/task.json":    `{"prompt": "Make the tests pass.", "test_cmd": ["go", "test", "./..."], "timeout_sec": 60}`,
		"add-bug/files/go.mod": "module fixture\n\ngo 1.21\n",
		// a folder with no task.json is not a task, so it is skipped
		"notes/ideas.txt": "scratch",
	})

	tasks, err := LoadTasks(root)
	if err != nil {
		t.Fatalf("LoadTasks returned error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2 (notes/ has no task.json)", len(tasks))
	}

	// sorted by name, so every run visits the tasks in the same order
	if tasks[0].Name != "add-bug" || tasks[1].Name != "greeting" {
		t.Fatalf("tasks out of order: %q, %q", tasks[0].Name, tasks[1].Name)
	}

	add := tasks[0]
	if add.Prompt != "Make the tests pass." {
		t.Errorf("Prompt = %q", add.Prompt)
	}
	if !slices.Equal(add.TestCmd, []string{"go", "test", "./..."}) {
		t.Errorf("TestCmd = %v", add.TestCmd)
	}
	if add.TimeoutSec != 60 {
		t.Errorf("TimeoutSec = %d, want 60", add.TimeoutSec)
	}
	if add.NoTools {
		t.Errorf("add-bug should not be a no_tools task")
	}
	if add.Dir != filepath.Join(root, "add-bug") {
		t.Errorf("Dir = %q, want %q", add.Dir, filepath.Join(root, "add-bug"))
	}

	// a no_tools task has nothing to grade, so it needs no test_cmd
	if !tasks[1].NoTools || len(tasks[1].TestCmd) != 0 {
		t.Errorf("greeting should be no_tools with no test_cmd, got %+v", tasks[1])
	}
}

func TestLoadTasks_MalformedJSONNamesTheTask(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"broken/task.json": `{"prompt": `,
	})

	_, err := LoadTasks(root)
	if err == nil {
		t.Fatal("malformed task.json should be an error, not a silently skipped task")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error should say which task is broken, got %q", err)
	}
}

func TestNewWorkspace_CopiesFixtureAndIsolatesIt(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"demo/task.json":         `{"prompt": "x"}`,
		"demo/files/go.mod":      "module fixture\n\ngo 1.21\n",
		"demo/files/pkg/util.go": "package pkg\n",
	})
	tasks, err := LoadTasks(root)
	if err != nil {
		t.Fatal(err)
	}

	ws, cleanup, err := NewWorkspace(tasks[0])
	if err != nil {
		t.Fatalf("NewWorkspace returned error: %v", err)
	}

	// nested files are copied
	got, err := os.ReadFile(filepath.Join(ws, "pkg", "util.go"))
	if err != nil || string(got) != "package pkg\n" {
		t.Errorf("nested file not copied: content %q, err %v", got, err)
	}

	// the workspace is the project the model sees; the grader's config is not part of it
	if _, err := os.Stat(filepath.Join(ws, "task.json")); !os.IsNotExist(err) {
		t.Errorf("task.json must not be copied into the workspace, stat err = %v", err)
	}

	// editing the workspace leaves the fixture alone
	if err := os.WriteFile(filepath.Join(ws, "pkg", "util.go"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig, err := os.ReadFile(filepath.Join(root, "demo", "files", "pkg", "util.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(orig) != "package pkg\n" {
		t.Errorf("fixture was modified through the workspace: %q", orig)
	}

	// two workspaces for the same task never share a directory
	ws2, cleanup2, err := NewWorkspace(tasks[0])
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup2()
	if ws2 == ws {
		t.Errorf("two workspaces share the directory %q", ws)
	}

	cleanup()
	if _, err := os.Stat(ws); !os.IsNotExist(err) {
		t.Errorf("cleanup should remove the workspace, stat err = %v", err)
	}
}
