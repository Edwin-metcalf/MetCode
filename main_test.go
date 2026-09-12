package main

import (
	"strings"
	"testing"
)

func TestReadDirectoryHelper_ValidFile(t *testing.T) {
	call := &toolCall{
		Function: calledFunction{
			Name:      "read_file",
			Arguments: map[string]any{"path": "main.go"},
		},
	}

	result := readFileHelper(call)

	if result.Role != "tool" {
		t.Errorf("Role is not tool it is %v", result.Role)
	}
	if result.Content == "" {
		t.Error("Content is empty ")
	}
}

func TestReadDirectoryHelper_NoPath(t *testing.T) {
	call := &toolCall{
		Function: calledFunction{
			Name:      "read_file",
			Arguments: map[string]any{},
		},
	}
	result := readFileHelper(call)
	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %q", result.Content)
	}
}

func TestProtectedFiles(t *testing.T) {
	call := &toolCall{
		Function: calledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "main.go", "old_text": "package main\ngood code:)", "new_text": "bad code:("},
		},
	}
	result := editFileHelper(call)

	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %v", result.Content)
	}
}

func TestEditFileValid(t *testing.T) {
	call := &toolCall{
		Function: calledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "example.go", "old_text": "bad stuff :(", "new_text": "good stuff :)"},
		},
	}
	result := editFileHelper(call)

	if result.Role != "tool" {
		t.Errorf("error: Role is not a tool it is: %v", result.Role)
	}

	if result.Content == "" {
		t.Error("content is empty")
	}
}
