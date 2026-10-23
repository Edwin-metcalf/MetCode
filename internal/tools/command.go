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
		// reject absolute and rooted paths rather then silently resolcing them
		if isRootedPath(rawPath) {
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

	if !readConfirmAnswer() {
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

// isRootedPath is a helper that reorts wether path is absolute or rooted on any platform
// this should fix windows specific issue whwere filepath.IsAbs("/etc") is false
// so we check for leading slash or backslash and any name (C: , \\server)
func isRootedPath(p string) bool {
	return filepath.IsAbs(p) ||
		filepath.VolumeName(p) != "" ||
		strings.HasPrefix(p, "/") ||
		strings.HasPrefix(p, `\`)
}
