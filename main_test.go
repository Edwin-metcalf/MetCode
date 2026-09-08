package main

import (
	"fmt"
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
		fmt.Printf("Role is not tool it is &v", result.Role)
	}
	if result.Content == "" {
		fmt.Println("Content is empty ")
	}
}
