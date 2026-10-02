// conversation saves and loads chat history as JSON files
package conversation

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
)

var ErrInvalidName = errors.New("invalid conversation name")

func DefaultDir() (string, error) {
	//essentially just return '~/.metcode/conversations
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".metcode", "conversations"), nil
}

func List(dir string) ([]string, error) {
	dirSlice, err := os.ReadDir(dir)

	if errors.Is(err, os.ErrNotExist) {
		return []string{}, err

	} else if err != nil {
		return []string{}, err
	}

	if len(dirSlice) == 0 {
		return []string{}, nil
	}

	var stringSlice []string

	for _, val := range dirSlice {
		if val.IsDir() {
			continue
		}
		stringSlice = append(stringSlice, val.Name())
	}

	return stringSlice, nil
}

func Load(dir, name string) ([]ollama.Message, error) {
	if err := nameValidation(name); err != nil {
		return nil, err
	}

	if !strings.HasSuffix(name, ".txt") {
		name += ".txt"
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, err
	}
	var loaded []ollama.Message
	if err := json.Unmarshal(data, &loaded); err != nil {
		return nil, err
	}
	return loaded, nil
}

func Save(dir, name string, history []ollama.Message) (string, error) {
	if err := nameValidation(name); err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	conversationName := strings.TrimSpace(name)

	if !strings.HasSuffix(conversationName, ".txt") {
		conversationName += ".txt"
	}

	data, err := json.MarshalIndent(history, "", "	")
	if err != nil {
		return "", err
	}

	err = os.WriteFile(filepath.Join(dir, conversationName), data, 0644)
	if err != nil {
		return "", err
	}
	return conversationName, nil
}

func nameValidation(name string) error {
	name = strings.TrimSpace(name)
	if name != filepath.Base(name) {
		return ErrInvalidName
	}
	if name == "" || name == "." || name == ".." {
		return ErrInvalidName
	}

	return nil
}
