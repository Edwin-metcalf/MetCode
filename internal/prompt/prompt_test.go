package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreate_FirstRunCreatesFile(t *testing.T) {
	home := t.TempDir()

	got, err := LoadOrCreateSystemPrompt(home)
	if err != nil {
		t.Fatalf("LoadOrCreate returned error: %v", err)
	}
	if got != defaultSystemPrompt {
		t.Error("first run should return the embedded default prompt")
	}

	path := filepath.Join(home, ".metcode", "system_prompt.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected %s to be created: %v", path, err)
	}
	if string(data) != defaultSystemPrompt {
		t.Error("created file should contain the default prompt")
	}
}
