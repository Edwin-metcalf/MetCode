// Package prompt loads MetCode's system prompt, seeding ~/.metcode/system_prompt.md on first run.
package prompt

import (
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed system_prompt.md
var defaultSystemPrompt string

func LoadOrCreateSystemPrompt(home string) (string, error) {
	path := filepath.Join(home, ".metcode", "system_prompt.md")
	content, err := os.ReadFile(path)

	if err == nil {
		return string(content), nil
	}

	if !os.IsNotExist(err) {
		return "", err
	}

	dir := filepath.Dir(path)
	err = os.MkdirAll(dir, 0o755)
	if err != nil {
		return "", err
	}

	err = os.WriteFile(path, []byte(defaultSystemPrompt), 0o644)
	if err != nil {
		return "", err
	}

	return defaultSystemPrompt, nil
}
