package tools

import (
	"errors"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
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
	requested := path
	path, err := resolveSafePath(root, path)
	if err != nil {
		return unsafePathError(root, requested, err)
	}
	//what to do if null or not the right path need to add error handling
	//
	if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
		return ollama.Message{
			Role:    "tool",
			Content: fmt.Sprintf("error: %s is a directory. Use list_directory to see what is in it.", requested),
		}
	}
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
	requested := path
	path, err := resolveSafePath(root, path)
	if err != nil {
		return unsafePathError(root, requested, err)
	}

	// Two payload shapes share this handler. old_text + new_text replaces a
	// snippet; content (or text) replaces the whole file. Small models send the
	// whole file far more often than an exact anchor, so content is handled here
	// instead of being rejected on every call.
	oldText, oldState := stringArg(call.Function.Arguments, "old_text")
	wholeText, wholeState, wholeKey := wholeFileArg(call.Function.Arguments)
	// A model that escaped its own newlines has to be decoded before anything
	// reads the text: old_text would never match the file, and new_text would be
	// rejected as literal \n. Both keys or neither, or the anchor stops lining up
	// with the replacement.
	oldText = repairEscapes(oldText)
	wholeText = repairEscapes(wholeText)

	if oldState == argMissing {
		switch wholeState {
		case argFound:
			return writeWholeFileHelper(path, wholeText)
		case argWrongType:
			return ollama.Message{Role: "tool", Content: fmt.Sprintf("error: %s must be a string, not a different JSON type", wholeKey)}
		}
		// new_text without old_text is a different mistake from a bare {path}
		// call: the model tried snippet mode and forgot the anchor. Keeping the
		// two messages apart is what lets bench/editfails.go classify them.
		if _, hasNewText := call.Function.Arguments["new_text"]; hasNewText {
			return ollama.Message{Role: "tool", Content: "error: missing required parameter old_text: new_text is the replacement text, so old_text must be sent with it. To replace the whole file, send content instead. Nothing was changed."}
		}
		return ollama.Message{Role: "tool", Content: "error: edit_file with only path changes nothing. Send the path with either content = the complete file, or old_text + new_text = replace a snippet. Nothing was changed."}
	}
	if oldState == argWrongType {
		return ollama.Message{Role: "tool", Content: "error: old_text must be a string, not a different JSON type"}
	}

	// new_text is only the text that replaces the old_text snippet. old_text
	// plus a whole-file payload is the dangerous shape: applying content at the
	// snippet site would splice the file into itself, so it stays an error that
	// names the right key.
	newText, newState := stringArg(call.Function.Arguments, "new_text")
	newText = repairEscapes(newText)
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

// repairEscapes fixes a payload a model double-encoded. Some models (llama3.1)
// escape their own newlines, so a real line break arrives as the two characters
// \ and n, and indentation as \ and t. Rejecting that is a dead end: llama
// resends the identical payload instead of re-encoding, and it did so until the
// run hit the failure budget.
//
// Only the unambiguous shape is repaired — no real newline anywhere and at least
// one \n escape — so genuine multi-line text and one-line snippets are never
// touched. Across every recorded study no payload had both real newlines and
// literal escapes, which is why the mixed shape is left alone.
func repairEscapes(s string) string {
	if !hasLiteralNewlines(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case 't':
				b.WriteByte('\t')
				i++
				continue
			case '"':
				b.WriteByte('"')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
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

	// replacing a file that was read before the approval prompt
	if exists {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return ollama.Message{Role: "tool", Content: fmt.Sprintf("error writing file: %v", err)}
		}
		return ollama.Message{Role: "tool", Content: fmt.Sprintf("%v file editted succesfully", path)}
	}

	// O_excl open fails if the user manual creates while confirming is waiting
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)

	if errors.Is(err, fs.ErrExist) {
		return ollama.Message{Role: "tool", Content: "error: file was created after the edit was proposed; nothing was written. Call read_file and make an edit against its current contents"}
	}

	if err != nil {
		return ollama.Message{Role: "tool", Content: fmt.Sprintf("error writing file: %v", err)}
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return ollama.Message{Role: "tool", Content: fmt.Sprintf("error writing file: %v", err)}
	}

	if err := f.Close(); err != nil {
		return ollama.Message{Role: "tool", Content: fmt.Sprintf("error writing file: %v", err)}
	}

	return ollama.Message{Role: "tool", Content: fmt.Sprintf("%v file created successfully", path)}
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

// errOutsideRoot marks a path that resolves outside the launch directory, whether
// through ".." or a symlink. unsafePathError turns it into a message the model can act on.
var errOutsideRoot = errors.New("path is outside the workspace root")

// unsafePathError is the error reply for a path resolveSafePath refused. It names
// the reason and the root so the model knows how to correct the path.
func unsafePathError(root string, requestedPath string, err error) ollama.Message {
	if errors.Is(err, errOutsideRoot) {
		absRoot, absErr := filepath.Abs(root)
		if absErr != nil {
			absRoot = root
		}
		return ollama.Message{Role: "tool", Content: fmt.Sprintf("error: %s is outside the workspace root %s. Use a path relative to it.", requestedPath, absRoot)}
	}
	return ollama.Message{Role: "tool", Content: fmt.Sprintf("error: %s cannot be used inside the workspace root: %v", requestedPath, err)}
}

func resolveSafePath(root string, requestedPath string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}

	// root itself may contain symlinks (on macos /var is a link to /private/var),
	// so resolve it once and compare against that.
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", err
	}

	fullPath := filepath.Join(absRoot, requestedPath)

	// find the deepest existing ancestor without following symlinks, so a broken
	// symlink is still visible instead of looking like a missing file.
	current := fullPath
	var missing []string
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing ancestor for %s", fullPath)
		}
		missing = append([]string{filepath.Base(current)}, missing...)
		current = parent
	}

	// A broken symlink fails here with fs.ErrNotExist. Reject it rather than
	// falling back to root, or a later write would follow it out of the jail.
	resolvedAncestor, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}

	resolvedPath := filepath.Join(append([]string{resolvedAncestor}, missing...)...)

	// Check containment against the resolved root.
	relativePath, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil {
		return "", err
	}

	if relativePath == ".." ||
		strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %s", errOutsideRoot, relativePath)
	}

	// keep the caller's spelling of root, so callers that pass a symlinked root
	// get a path back that matches what they gave us.
	return filepath.Join(absRoot, relativePath), nil
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
