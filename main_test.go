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

	result := readDirectoryHelper(call)

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
	result := readDirectoryHelper(call)
	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %q", result.Content)
	}
}
