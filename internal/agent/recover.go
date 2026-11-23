package agent

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
)

// recoverPastedFile turns a reply that pastes a whole Go file into an edit_file
// call. In the verify-addbug study 4 of qwen2.5-coder's 9 failures ended with a
// complete, correct add.go inside a ```go fence or a <tool_response> block that
// was never applied. Two others pasted the test file back, and on two-bugs qwen
// echoed the unchanged original, so every rule here is about refusing to guess:
//   - the message has no tool call (normalize found nothing)
//   - exactly one block is a full Go file that is not a test file
//   - exactly one non-test .go file was read since the last user message
//   - the block differs from the file on disk, so neither an echo of the
//     original nor an echo of an edit that already landed is written again
//
// The call it builds goes through edit_file like any other, so approval, the Go
// parse check, the read-only test rule and verify all still apply.
func recoverPastedFile(m ollama.Message, history []ollama.Message, root string) (ollama.Message, string, bool) {
	if len(m.ToolCalls) > 0 {
		return m, "", false
	}

	var files []string
	for _, block := range pastedBlocks(m.Content) {
		block = strings.TrimSpace(block)
		if !isWholeGoFile(block) {
			continue
		}
		if !slices.Contains(files, block) {
			files = append(files, block)
		}
	}
	if len(files) != 1 {
		return m, "", false
	}

	path, ok := onlyReadGoFile(history)
	if !ok {
		return m, "", false
	}
	// read_file already accepted this path, so it sits inside root
	current, err := os.ReadFile(filepath.Join(root, path))
	if err != nil || files[0] == strings.TrimSpace(string(current)) {
		return m, "", false
	}

	call := ollama.ToolCall{Function: ollama.CalledFunction{
		Name:      "edit_file",
		Arguments: map[string]any{"path": path, "content": files[0] + "\n"},
	}}
	return ollama.Message{Role: m.Role, Content: "", ToolCalls: []ollama.ToolCall{call}}, path, true
}

// pastedBlocks returns the bodies of ``` / ~~~ fences and <tool_response> tags.
// An unclosed block runs to the end of the message.
func pastedBlocks(s string) []string {
	var blocks []string

	rest := s
	for {
		open := strings.Index(rest, "<tool_response>")
		if open < 0 {
			break
		}
		rest = rest[open+len("<tool_response>"):]
		end := strings.Index(rest, "</tool_response>")
		if end < 0 {
			blocks = append(blocks, rest)
			break
		}
		blocks = append(blocks, rest[:end])
		rest = rest[end:]
	}

	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "```") && !strings.HasPrefix(trimmed, "~~~") {
			continue
		}
		marker := trimmed[:3]
		j := i + 1
		for j < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[j]), marker) {
			j++
		}
		blocks = append(blocks, strings.Join(lines[i+1:min(j, len(lines))], "\n"))
		i = j
	}
	return blocks
}

// isWholeGoFile reports whether src parses as a complete Go file that is not a
// test file. A fragment without a package clause fails the parse.
func isWholeGoFile(src string) bool {
	if !strings.HasPrefix(src, "package ") {
		return false
	}
	f, err := parser.ParseFile(token.NewFileSet(), "", src, parser.SkipObjectResolution)
	if err != nil {
		return false
	}
	for _, imp := range f.Imports {
		if imp.Path.Value == `"testing"` {
			return false
		}
	}
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && strings.HasPrefix(fn.Name.Name, "Test") {
			return false
		}
	}
	return true
}

// onlyReadGoFile returns the one non-test .go file read_file opened since the
// last user message. More than one, none, or a failed read means there is no
// safe target.
func onlyReadGoFile(history []ollama.Message) (string, bool) {
	start := 0
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			start = i
			break
		}
	}

	reads := map[string]bool{}
	for i := start; i < len(history); i++ {
		for _, call := range history[i].ToolCalls {
			if call.Function.Name != "read_file" {
				continue
			}
			p, _ := call.Function.Arguments["path"].(string)
			p = filepath.Clean(p)
			if filepath.Ext(p) != ".go" || strings.HasSuffix(p, "_test.go") {
				continue
			}
			ok := i+1 < len(history) && history[i+1].Role == "tool" &&
				!strings.HasPrefix(history[i+1].Content, "error")
			reads[p] = ok
		}
	}
	if len(reads) != 1 {
		return "", false
	}
	for p, ok := range reads {
		return p, ok
	}
	return "", false
}
