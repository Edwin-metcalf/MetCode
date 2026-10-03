package conversation

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
)

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir() // auto-deleted when the test ends, so your real ~/.metcode is never touched
	history := []ollama.Message{
		{Role: "system", Content: "you are metcode"},
		{Role: "user", Content: "hi"},
	}

	name, err := Save(dir, "chat", history)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if name != "chat.txt" {
		t.Errorf("saved name = %q, want %q", name, "chat.txt")
	}

	got, err := Load(dir, name)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !reflect.DeepEqual(got, history) {
		t.Errorf("loaded history differs:\n got: %+v\nwant: %+v", got, history)
	}
}

func TestSave_RejectsBadNames(t *testing.T) {
	bad := []string{"", "   ", ".", "..", "../escape", "sub/chat", "/abs/path"}

	for _, name := range bad {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "convos")

			_, err := Save(dir, name, nil)
			if !errors.Is(err, ErrInvalidName) {
				t.Errorf("Save(%q) error = %v, want ErrInvalidName", name, err)
			}

			// nothing should have been written, including outside dir
			entries, _ := os.ReadDir(parent)
			if len(entries) != 0 {
				t.Errorf("Save(%q) wrote something into the parent dir: %v", name, entries)
			}
		})
	}
}

func TestSave_AddsTxtSuffixOnce(t *testing.T) {
	cases := map[string]string{
		"chat":     "chat.txt",
		"chat.txt": "chat.txt",
		"  chat  ": "chat.txt",
	}

	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			dir := t.TempDir()

			got, err := Save(dir, input, nil)
			if err != nil {
				t.Fatalf("Save(%q) returned error: %v", input, err)
			}
			if got != want {
				t.Errorf("Save(%q) = %q, want %q", input, got, want)
			}
			if _, err := os.Stat(filepath.Join(dir, got)); err != nil {
				t.Errorf("expected file %q to exist: %v", got, err)
			}
		})
	}
}
