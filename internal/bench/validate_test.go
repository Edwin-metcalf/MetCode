package bench

import (
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
