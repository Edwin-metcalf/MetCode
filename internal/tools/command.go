package tools

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
)

func runCommandHelper(call *ollama.ToolCall, root string) ollama.Message {
	command, ok := call.Function.Arguments["command"].(string)
	if !ok || command == "" {
		return ollama.Message{
			Role:    "tool",
			Content: "error: invalid command",
		}
	}

	dir := root
	if rawPath, ok := call.Function.Arguments["path"].(string); ok && rawPath != "" {
		// filepath.Join would fold /etc into root/etc instead of escaping, so
		// reject absolute paths rather than silently resolving them
		if filepath.IsAbs(rawPath) {
			return ollama.Message{Role: "tool", Content: "error: path must be relative to the project root"}
		}
		resolved, err := resolveSafePath(root, rawPath)
		if err != nil {
			return ollama.Message{Role: "tool", Content: "error: path escapes project root"}
		}
		dir = resolved
	}

	var arguments []string
	if rawArgs, ok := call.Function.Arguments["arguments"].([]any); ok {
		for _, a := range rawArgs {
			if s, ok := a.(string); ok {
				arguments = append(arguments, s)
			}
		}
	}

	fmt.Println("----- MetCode wants to run a command ------")
	fmt.Println("directory: ", dir)
	fmt.Println("Command: ", command, strings.Join(arguments, " "))
	fmt.Println("------------------------------")
	fmt.Println("Run this command? (y/n)")

	var response string
	fmt.Scanln(&response)

	if strings.ToLower(strings.TrimSpace(response)) != "y" {
		return ollama.Message{Role: "tool", Content: "command canceled by user"}
	}
	cmd := exec.Command(command, arguments...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return ollama.Message{
			Role:    "tool",
			Content: fmt.Sprintf("error: Command Failed: %v \nOutput: %s", err, output),
		}
	}

	return ollama.Message{
		Role:    "tool",
		Content: fmt.Sprintf("Command ran succesfully, output: %v ", string(output)),
	}
}
