package bench

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadTask writes a one-task tree (task.json plus the given fixture files)
// and returns the loaded task.
func loadTask(t *testing.T, name, taskJSON string, files map[string]string) Task {
	t.Helper()
	root := t.TempDir()
	all := map[string]string{name + "/task.json": taskJSON}
	for rel, content := range files {
		all[name+"/files/"+rel] = content
	}
	writeFiles(t, root, all)
	tasks, err := LoadTasks(root)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("LoadTasks: %v (%d tasks)", err, len(tasks))
	}
	return tasks[0]
}

const addTaskJSON = `{"prompt": "Fix Add.", "test_cmd": ["go", "test", "./..."], "timeout_sec": 120}`

func workspaceCount() int {
	m, _ := filepath.Glob(filepath.Join(os.TempDir(), "agent-workspace-*"))
	return len(m)
}

// a prompt that names a file the fixture does not ship can never be satisfied,
// so it must be caught before a matrix is run.
func TestCheckPromptFiles(t *testing.T) {
	fixture := map[string]string{
		"go.mod": addGoMod, "add.go": addBuggy, "add_test.go": addTest,
	}
	cases := []struct {
		name    string
		prompt  string
		wantErr string // "" means the prompt is sound
	}{
		{"files that exist", "Fix add.go so add_test.go passes.", ""},
		{"missing source file", "Fix the bug in change-constant.go.", "change-constant.go"},
		{"missing test file", "Make missing_test.go pass.", "missing_test.go"},
		{"no file names", "Fix the Add function.", ""},
		{"nested path uses the base name", "Edit pkg/add.go.", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"prompt": %q, "test_cmd": ["go", "test", "./..."]}`, tc.prompt)
			task := loadTask(t, "case", body, fixture)

			err := checkPromptFiles(task)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error mentioning %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error should name %q, got %q", tc.wantErr, err)
			}
			if !strings.Contains(err.Error(), task.Name) {
				t.Errorf("error should name the task %q, got %q", task.Name, err)
			}
		})
	}
}

// ValidateTask has to surface the prompt check, not just the helper.
func TestValidateTask_PromptMentionsMissingFile(t *testing.T) {
	task := loadTask(t, "bad-prompt",
		`{"prompt": "Fix nothing.go.", "test_cmd": ["go", "test", "./..."]}`,
		map[string]string{"go.mod": addGoMod, "add.go": addBuggy, "add_test.go": addTest})

	err := ValidateTask(task)
	if err == nil {
		t.Fatal("a prompt naming a missing file must be rejected")
	}
	if !strings.Contains(err.Error(), "nothing.go") {
		t.Errorf("error should name the missing file, got %q", err)
	}
}

func TestValidateTask_FailingFixtureIsSound(t *testing.T) {
	task := loadTask(t, "add-bug", addTaskJSON, map[string]string{
		"go.mod": addGoMod, "add.go": addBuggy, "add_test.go": addTest,
	})

	if err := ValidateTask(task); err != nil {
		t.Errorf("a fixture that fails its tests is sound, got error: %v", err)
	}
}

func TestValidateTask_AlreadyPassingFixtureIsAnError(t *testing.T) {
	task := loadTask(t, "add-ok", addTaskJSON, map[string]string{
		"go.mod": addGoMod, "add.go": addFixed, "add_test.go": addTest,
	})

	err := ValidateTask(task)
	if err == nil {
		t.Fatal("a fixture that already passes must be rejected")
	}
	if !strings.Contains(err.Error(), "add-ok") {
		t.Errorf("error should name the task, got %q", err)
	}
}

func TestValidateTask_NoToolsTaskIsSkipped(t *testing.T) {
	// no files/ folder at all: validate must not even try to build a workspace
	task := Task{Name: "greeting", Dir: t.TempDir(), Prompt: "hi", NoTools: true}

	if err := ValidateTask(task); err != nil {
		t.Errorf("a no_tools task has nothing to validate, got: %v", err)
	}
}

func TestValidateTask_DoesNotLeakWorkspaces(t *testing.T) {
	task := loadTask(t, "add-bug", addTaskJSON, map[string]string{
		"go.mod": addGoMod, "add.go": addBuggy, "add_test.go": addTest,
	})

	before := workspaceCount()
	if err := ValidateTask(task); err != nil {
		t.Fatal(err)
	}
	if after := workspaceCount(); after != before {
		t.Errorf("ValidateTask left %d temp workspace(s) behind", after-before)
	}
}

func TestValidateAll_ReturnsOnlyTheProblems(t *testing.T) {
	good := loadTask(t, "good", addTaskJSON, map[string]string{
		"go.mod": addGoMod, "add.go": addBuggy, "add_test.go": addTest,
	})
	bad := loadTask(t, "bad", addTaskJSON, map[string]string{
		"go.mod": addGoMod, "add.go": addFixed, "add_test.go": addTest,
	})
	skipped := Task{Name: "greeting", Dir: t.TempDir(), Prompt: "hi", NoTools: true}

	problems := ValidateAll([]Task{good, bad, skipped})

	if len(problems) != 1 {
		t.Fatalf("want exactly 1 problem (the already-passing fixture), got %d: %v", len(problems), problems)
	}
	if !strings.Contains(problems[0].Error(), "bad") {
		t.Errorf("the problem should name task %q, got %q", "bad", problems[0])
	}
}
