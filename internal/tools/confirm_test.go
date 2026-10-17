package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
)

// confirmAnswerCases covers the y/n confirm rule: after trimming surrounding
// whitespace and ignoring case, y, yes and 1 accept; n, no and 0 decline;
// anything else is rejected.
var confirmAcceptedAnswers = []string{"y", "Y", "yes", "YES", "Yes", "1", "  y  ", "\tyes\t", " 1 "}
var confirmRejectedAnswers = []string{"n", "no", "NO", "0", "", "maybe", "yy", "ye", "2", "y n", "yes please"}

func TestConfirm_AcceptsLineAnswers(t *testing.T) {
	for _, answer := range confirmAcceptedAnswers {
		t.Run(answer, func(t *testing.T) {
			feedStdin(t, answer+"\n")
			root := t.TempDir()
			path := filepath.Join(root, "accepted.txt")

			result := createFileHelper(path, "content")

			if result.Content == "Edit canceled by user" {
				t.Fatalf("answer %q should accept, got %q", answer, result.Content)
			}
			if _, err := os.Stat(path); err != nil {
				t.Errorf("answer %q should create the file, stat err = %v", answer, err)
			}
		})
	}
}

func TestConfirm_RejectsOtherAnswers(t *testing.T) {
	for _, answer := range confirmRejectedAnswers {
		t.Run(answer, func(t *testing.T) {
			feedStdin(t, answer+"\n")
			root := t.TempDir()
			path := filepath.Join(root, "rejected.txt")

			result := createFileHelper(path, "content")

			if result.Content != "Edit canceled by user" {
				t.Errorf("answer %q should be rejected, got %q", answer, result.Content)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("answer %q must not create the file, stat err = %v", answer, err)
			}
		})
	}
}

// A rejected line is consumed whole: its leftover text must not be read as the
// answer to the next prompt.
func TestConfirm_DiscardsRestOfRejectedLine(t *testing.T) {
	feedStdin(t, "y z\nyes\n")
	root := t.TempDir()

	first := createFileHelper(filepath.Join(root, "first.txt"), "a")
	if first.Content != "Edit canceled by user" {
		t.Fatalf("\"y z\" should be rejected, got %q", first.Content)
	}
	second := createFileHelper(filepath.Join(root, "second.txt"), "b")
	if strings.Contains(second.Content, "canceled") {
		t.Errorf("the next prompt should read \"yes\" from its own line, got %q", second.Content)
	}
}

func TestConfirm_CommandLineAnswers(t *testing.T) {
	call := func() *ollama.ToolCall {
		return &ollama.ToolCall{Function: ollama.CalledFunction{
			Name:      "run_command",
			Arguments: map[string]any{"command": "go", "arguments": []any{"version"}},
		}}
	}

	feedStdin(t, " YES \n")
	if result := runCommandHelper(call(), t.TempDir()); result.Content == "command canceled by user" {
		t.Errorf("\" YES \" should accept, got %q", result.Content)
	}

	feedStdin(t, "no\n")
	if result := runCommandHelper(call(), t.TempDir()); result.Content != "command canceled by user" {
		t.Errorf("\"no\" should decline, got %q", result.Content)
	}
}

func TestConfirm_EditExistingFileAcceptsYes(t *testing.T) {
	feedStdin(t, "yes\n")
	root := t.TempDir()
	path := filepath.Join(root, "edit.txt")
	if err := os.WriteFile(path, []byte("old line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	call := &ollama.ToolCall{Function: ollama.CalledFunction{
		Name:      "edit_file",
		Arguments: map[string]any{"path": "edit.txt", "old_text": "old line", "new_text": "new line"},
	}}

	result := editFileHelper(call, root)

	if result.Content == "Edit canceled by user" {
		t.Fatalf("\"yes\" should accept the edit, got %q", result.Content)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "new line") {
		t.Errorf("edit not applied, file = %q", got)
	}
}
