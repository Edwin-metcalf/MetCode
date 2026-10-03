package tools

import (
	"fmt"
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

	oldText, ok := call.Function.Arguments["old_text"].(string)
	if !ok {
		return ollama.Message{Role: "tool", Content: "error no old text found"}
	}

	newText, ok := call.Function.Arguments["new_text"].(string)
	if !ok {
		return ollama.Message{Role: "tool", Content: "error no new text found"}
	}

	fileContentBytes, err := os.ReadFile(path)
	if err != nil {
		return ollama.Message{Role: "tool", Content: fmt.Sprintf("error: finding file: %v", err)}
	}
	fileContent := string(fileContentBytes)
	count := strings.Count(fileContent, oldText)

	if count == 0 {
		return ollama.Message{Role: "tool", Content: "error old text not found in file"}
	}
	if count > 1 {
		return ollama.Message{Role: "tool", Content: "error: old text matches multiple locations be specific"}
	}

	newContent := strings.Replace(fileContent, oldText, newText, 1)

	fmt.Println("----- proposed change to ", path, "------")
	fmt.Println(prefixLines(oldText, "- "))
	fmt.Println(prefixLines(newText, "+ "))
	fmt.Println("------------------------------")
	fmt.Println("apply this edit? (y/n)")

	var response string
	fmt.Scanln(&response)

	if strings.ToLower(strings.TrimSpace(response)) != "y" {
		return ollama.Message{Role: "tool", Content: "Edit canceled by user"}
	}

	err = os.WriteFile(path, []byte(newContent), 0o644)
	if err != nil {
		return ollama.Message{Role: "tool", Content: fmt.Sprintf("error writing file: %v", err)}
	}
	return ollama.Message{Role: "tool", Content: fmt.Sprintf("%v file editted succesfully", path)}
}

func createFileHelper(call *ollama.ToolCall, root string) ollama.Message {
	fileName, ok := call.Function.Arguments["path"].(string)
	if !ok {
		return ollama.Message{
			Role:    "tool",
			Content: "error: invalid file name: ",
		}
	}

	fileName, err := resolveSafePath(root, fileName)
	if err != nil {
		return ollama.Message{Role: "tool", Content: "error: path is possibly dangerous"}
	}

	// deal with creating the directories if the path is not just example.go but place/example.go
	dir := filepath.Dir(fileName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ollama.Message{Role: "tool", Content: fmt.Sprintf("error creating directory %v", err)}
	}

	newFile, err := os.Create(fileName)
	if err != nil {
		content := fmt.Sprintf("error creating file: %v", err)
		return ollama.Message{
			Role:    "tool",
			Content: content,
		}
	}
	defer newFile.Close()

	outGoingContent := fmt.Sprintf("%v file created", newFile.Name())
	outGoingMessage := ollama.Message{
		Role:    "tool",
		Content: outGoingContent,
	}
	return outGoingMessage
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
