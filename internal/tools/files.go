package tools

import (
	"errors"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
)

var protectedFiles = map[string]bool{
	// add files you want to make sure you protect even if the ai wants to edit them
	//this is kinda ahh what if these files are in the directory that wants to be edited how to not edit the original
	//"main.go":          true,
	"main_test.go":     true,
	"go.mod":           true,
	"go.sum":           true,
	"system_prompt.md": true,
}

func isProtected(path string) bool {
	return protectedFiles[filepath.Base(path)]
}

// arg state tells what happened when looking up a tool argument. Missing and wrong type
// are seperate erros. So this helps tell the model a better return message.
type argState int

const (
	argFound argState = iota
	argMissing
	argWrongType
)

func stringArg(args map[string]any, key string) (string, argState) {
	raw, present := args[key]
	if !present {
		return "", argMissing
	}
	value, ok := raw.(string)
	if !ok {
		return "", argWrongType
	}
	return value, argFound
}

func readFileHelper(call *ollama.ToolCall, root string) ollama.Message {
	path, ok := call.Function.Arguments["path"].(string)
	if !ok {
		return ollama.Message{
			Role:    "tool",
			Content: "error: no path provided",
		}
	}
	path, err := resolveSafePath(root, path)
	if err != nil {
		return ollama.Message{
			Role:    "tool",
			Content: "error: path is possibly dangerous",
		}
	}
	//what to do if null or not the right path need to add error handling
	//
	fileContentBytes, err := os.ReadFile(path)
	if err != nil {
		// dont need to log fatal but need to handle if we cant read from that path
		//
		content := fmt.Sprintf("error reading file in path: %v", err)
		return ollama.Message{
			Role:    "tool",
			Content: content,
		}
	}

	fileContents := string(fileContentBytes)

	if fileContents == "" {
		fileContents = "(file is empty if you to call edit file set old_text to empty string)"
	}

	outGoingMessage := ollama.Message{
		Role:    "tool",
		Content: fileContents,
	}
	return outGoingMessage
}

func editFileHelper(call *ollama.ToolCall, root string) ollama.Message {
	path, ok := call.Function.Arguments["path"].(string)
	if !ok {
		return ollama.Message{Role: "tool", Content: "error: no path provided"}
	}
	if isProtected(path) {
		return ollama.Message{Role: "tool", Content: "error: file is protected"}
	}
	path, err := resolveSafePath(root, path)
	if err != nil {
		return ollama.Message{Role: "tool", Content: "error: path is possibly dangerous"}
	}

	// Two payload shapes share this handler. old_text + new_text replaces a
	// snippet; content (or text) replaces the whole file. Small models send the
	// whole file far more often than an exact anchor, so content is handled here
	// instead of being rejected on every call.
	oldText, oldState := stringArg(call.Function.Arguments, "old_text")
	wholeText, wholeState, wholeKey := wholeFileArg(call.Function.Arguments)

	if oldState == argMissing {
		switch wholeState {
		case argFound:
			return writeWholeFileHelper(path, wholeText)
		case argWrongType:
			return ollama.Message{Role: "tool", Content: fmt.Sprintf("error: %s must be a string, not a different JSON type", wholeKey)}
		}
		return ollama.Message{Role: "tool", Content: "error: missing required parameter old_text: provide old_text + new_text to replace a snippet, or content to replace the whole file. To create a file, send the complete file as content."}
	}
	if oldState == argWrongType {
		return ollama.Message{Role: "tool", Content: "error: old_text must be a string, not a different JSON type"}
	}

	// new_text is only the text that replaces the old_text snippet. old_text
	// plus a whole-file payload is the dangerous shape: applying content at the
	// snippet site would splice the file into itself, so it stays an error that
	// names the right key.
	newText, newState := stringArg(call.Function.Arguments, "new_text")
	switch newState {
	case argWrongType:
		return ollama.Message{Role: "tool", Content: "error: new_text must be a string, not a different JSON type"}
	case argMissing:
		if wholeState != argMissing {
			return ollama.Message{Role: "tool", Content: fmt.Sprintf("error: edit_file takes new_text, not %s. new_text is only the text that replaces old_text, not the whole file. To replace the whole file, send content without old_text.", wholeKey)}
		}
		return ollama.Message{Role: "tool", Content: "error: missing required parameter new_text: the replacement text for old_text"}
	}

	if hasLiteralNewlines(newText) {
		return ollama.Message{Role: "tool", Content: "error: new_text contains the characters \\n instead of real line breaks. Resend using actual newlines"}
	}

	fileContentBytes, err := os.ReadFile(path)

	if errors.Is(err, os.ErrNotExist) {

		if oldText != "" {
			return ollama.Message{Role: "tool", Content: "error: file does not exist but old_text is not empty"}
		}

		return createFileHelper(path, newText)

	} else if err != nil {
		//other errs
		return ollama.Message{Role: "tool", Content: fmt.Sprintf("error: finding file: %v", err)}

	}

	//there is a file with content
	fileContent := string(fileContentBytes)
	count := strings.Count(fileContent, oldText)

	var newContent string
	switch {
	case count > 1:
		return ollama.Message{Role: "tool", Content: "error: old_text matches multiple locations (or is empty but the file is not). Include more surrounding lines so it matches exactly once."}
	case count == 1:
		newContent = strings.Replace(fileContent, oldText, newText, 1)
	default:
		// exact match failed, so try again ignoring indentation
		if looksLikeToolCall(oldText) {
			return ollama.Message{Role: "tool", Content: "error: old_text looks like a function call. old_text must be text copied from read_file output, not a tool call."}
		}
		start, end, err := findLooseMatch(fileContent, oldText)
		if err != nil {
			return ollama.Message{Role: "tool", Content: fmt.Sprintf("error: old_text not found in file (%v). Call read_file and copy old_text exactly from its output. Use a short unique snippet.", err)}
		}
		region := fileContent[start:end]
		newText = reindentLike(newText, region)
		if strings.HasSuffix(region, "\n") && newText != "" && !strings.HasSuffix(newText, "\n") {
			newText += "\n"
		}
		newContent = fileContent[:start] + newText + fileContent[end:]
	}

	// Reject an edit that breaks a Go file which parsed before the edit.
	// Files that were already broken are not blocked, so they can still be fixed.
	if strings.HasSuffix(path, ".go") {
		_, origErr := format.Source(fileContentBytes)
		_, newErr := format.Source([]byte(newContent))
		if origErr == nil && newErr != nil {
			return ollama.Message{Role: "tool", Content: fmt.Sprintf("error: this edit would make %s invalid Go (%v). The edit was not applied. Fix new_text and retry.", filepath.Base(path), newErr)}
		}
	}

	printLoud("----- proposed change to ", path, "------")
	printLoud(prefixLines(oldText, "- "))
	printLoud(prefixLines(newText, "+ "))
	printLoud("------------------------------")
	printLoud("apply this edit? (y/n)")

	if !Approve("edit") {
		return ollama.Message{Role: "tool", Content: "Edit canceled by user"}
	}

	err = os.WriteFile(path, []byte(newContent), 0o644)
	if err != nil {
		return ollama.Message{Role: "tool", Content: fmt.Sprintf("error writing file: %v", err)}
	}
	return ollama.Message{Role: "tool", Content: fmt.Sprintf("%v file editted succesfully", path)}
}

// createFileHelper creates a file through the same validated write path as a
// whole-file replacement, so approval and checks live in one place.
func createFileHelper(path string, newText string) ollama.Message {
	return writeWholeFileHelper(path, newText)
}

// wholeFileArg returns the payload a model sent to replace the whole file.
// content is the documented key; text is accepted because small models use it
// with the same meaning. The key is returned so an error can name what the
// model actually sent.
func wholeFileArg(args map[string]any) (value string, state argState, key string) {
	for _, k := range []string{"content", "text"} {
		if v, s := stringArg(args, k); s != argMissing {
			return v, s, k
		}
	}
	return "", argMissing, ""
}

func hasLiteralNewlines(s string) bool {
	return !strings.Contains(s, "\n") && strings.Contains(s, `\n`)
}

// goFileError reports whether src is a complete Go source file. Unlike
// format.Source it rejects a bare declaration or statement list, so a fragment
// cannot pass as a whole file.
func goFileError(src []byte) error {
	_, err := parser.ParseFile(token.NewFileSet(), "", src, parser.SkipObjectResolution)
	return err
}

// writeWholeFileHelper is the only path that writes a whole file. It refuses
// the shapes that corrupt files: a fragment of a Go file, an empty body over an
// existing file, or literal \n, then shows a diff and asks for approval.
func writeWholeFileHelper(path string, content string) ollama.Message {
	if hasLiteralNewlines(content) {
		return ollama.Message{Role: "tool", Content: "error: content contains the characters \\n instead of real line breaks. Resend using actual newlines"}
	}

	oldBytes, readErr := os.ReadFile(path)
	exists := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return ollama.Message{Role: "tool", Content: fmt.Sprintf("error: finding file: %v", readErr)}
	}
	if exists && content == "" {
		return ollama.Message{Role: "tool", Content: "error: content is empty. Refusing to replace an existing file with nothing; send the complete new file contents."}
	}

	if strings.HasSuffix(path, ".go") {
		// format.Source alone is not enough here: it also accepts a bare
		// declaration or statement list, which is exactly the fragment shape
		// that would gut the file. Require a full file with a package clause.
		if newErr := goFileError([]byte(content)); newErr != nil {
			// Mirror the snippet path: never break a file that parses today, but
			// allow an already-broken file to be replaced.
			allow := false
			if exists {
				allow = goFileError(oldBytes) != nil
			}
			if !allow {
				return ollama.Message{Role: "tool", Content: fmt.Sprintf("error: content is not valid Go (%v). content must be the complete file including the package clause. To replace a snippet, use old_text + new_text. The edit was not applied.", newErr)}
			}
		}
	}

	printLoud("----- proposed change to ", path, "------")
	if exists {
		printLoud(prefixLines(string(oldBytes), "- "))
	}
	printLoud(prefixLines(content, "+ "))
	printLoud("------------------------------")
	printLoud("apply this edit? (y/n)")

	if !Approve("edit") {
		return ollama.Message{Role: "tool", Content: "Edit canceled by user"}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ollama.Message{Role: "tool", Content: fmt.Sprintf("error creating directory %v", err)}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return ollama.Message{Role: "tool", Content: fmt.Sprintf("error writing file: %v", err)}
	}
	if !exists {
		return ollama.Message{Role: "tool", Content: fmt.Sprintf("%v file created successfully", path)}
	}
	return ollama.Message{Role: "tool", Content: fmt.Sprintf("%v file editted succesfully", path)}
}

func prefixLines(text string, prefix string) string {
	if text == "" {
		return prefix
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func resolveSafePath(root string, requestedPath string) (string, error) {
	fullPath := filepath.Join(root, requestedPath)

	cleanedPath := filepath.Clean(fullPath)
	absPath, err := filepath.Abs(cleanedPath)
	if err != nil {
		return "", err
	}

	relativePath, err := filepath.Rel(root, absPath)
	// we want it to error then it means there is no relative
	if err != nil {
		return "", err
	}

	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root: %s", relativePath)
	}

	return absPath, nil
}

func listDirectoryHelper(root string) ollama.Message {
	dirSlice, err := os.ReadDir(root)
	var dirString string

	if err != nil {
		return ollama.Message{Role: "tool", Content: "error: reading directory"}
	}

	for _, val := range dirSlice {
		dirString = dirString + " " + val.Name()
	}

	ldMessage := ollama.Message{
		Role:    "tool",
		Content: dirString,
		// what do do with the tool calls?,
	}
	return ldMessage
}
