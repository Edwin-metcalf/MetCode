package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const banner = `
 __  __      _   ____          _
|  \/  | ___| |_/ ___|___   __| | ___
| |\/| |/ _ \ __| |   / _ \ / _\ |/ _ \
| |  | |  __/ |_| |__| (_) | (_| |  __/
|_|  |_|\___|\__\____\___/ \__,_|\___|
	`

type App struct {
	CurrentModel string
	Scanner      *bufio.Scanner
}
type message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []toolCall `json:"tool_calls"`
}
type toolCall struct {
	Function calledFunction `json:"function"`
}
type chatRequest struct {
	Messages []message `json:"messages"`
	Model    string    `json:"model"`
	Stream   bool      `json:"stream"`
	Tools    []tool    `json:"tools"`
	Options  options   `json:"options"`
}
type chatResponse struct {
	Message message `json:"message"`
}
type models struct {
	Name string `json:"name"`
}
type modelWrapper struct {
	Models []models `json:"models"`
}
type parameters struct {
	Type       string         `json:"type"`
	Required   []string       `json:"required"`
	Properties map[string]any `json:"properties"`
}
type toolFunction struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  parameters `json:"parameters"`
}
type calledFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type tool struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}
type options struct {
	Temperature float64 `json:"temperature"`
}

var allTools = []tool{
	// this is global(package level) dont edit could be risky just copy if needed
	{
		Type: "function",
		Function: toolFunction{
			Name:        "list_directory",
			Description: "list all files in the current directory. Use when asked about directory content or when you need the name of files to then read them or edit them.",
			Parameters: parameters{
				Type:       "object",
				Required:   []string{},
				Properties: map[string]any{},
			},
		},
	},
	{
		Type: "function",
		Function: toolFunction{
			Name:        "read_file",
			Description: "reads the context of a file and returns it as plain text. Use when the user asks about context of a file or wants you to refer to code/text from a file that is not in the conversation.",
			Parameters: parameters{
				Type:     "object",
				Required: []string{"path"},
				Properties: map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "relative path to the file, relative to the current working directory. Example 'main.go' or 'internal/utils/helper.go'. Use list_directory first to get exact file names",
					},
				},
			},
		},
	},
	{
		Type: "function",
		Function: toolFunction{
			Name:        "create_file",
			Description: "creates a new empty file. Use edit_file after to add content",
			Parameters: parameters{
				Type:     "object",
				Required: []string{"path"},
				Properties: map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "relative path and name of a file. Relative to the current working directory. Examples: 'example.go' or 'stuff/example.go'",
					},
				},
			},
		},
	},
	{
		Type: "function",
		Function: toolFunction{
			Name:        "edit_file",
			Description: "edit a file. If the file is empty (e.g. just created with create_file), use an empty string for old_text. If the file is not empty old_text will have its contents read with read_file and you are editing those contents",
			Parameters: parameters{
				Type:     "object",
				Required: []string{"path", "old_text", "new_text"},
				Properties: map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "relative path to the file you want to edit. Relative to the current working directory. Example: 'example.go' or 'stuff/things/example.go'",
					},
					"old_text": map[string]any{
						"type":        "string",
						"description": "the text that was in the file. old_text must match the file's content exactly, including whitespace and indentation or the edit will fail.",
					},
					"new_text": map[string]any{
						"type":        "string",
						"description": "the new text that is going into the file after the edit",
					},
				},
			},
		},
	},
}

var protectedFiles = map[string]bool{
	// add files you want to make sure you protect even if the ai wants to edit them
	"main.go":          true,
	"main_test.go":     true,
	"go.mod":           true,
	"system_prompt.md": true,
}

func isProtected(path string) bool {
	return protectedFiles[filepath.Base(path)]
}

func buildChatRequest(history []message, model string) chatRequest {
	var outgoing chatRequest

	outgoing.Messages = history
	outgoing.Model = model
	outgoing.Stream = false
	outgoing.Options.Temperature = 0.1
	outgoing.Tools = allTools

	return outgoing
}

func postRequest(outGoingMessage chatRequest) chatResponse {
	// this is probably not needed but whatever
	b, err := json.Marshal(outGoingMessage)
	if err != nil {
		log.Fatalf("failed to serialize to JSON")
	}

	body := bytes.NewBuffer(b)
	url := "http://localhost:11434/api/chat"

	resp, err := http.Post(url, "application/json", body)
	if err != nil {
		log.Fatalf("failed to create resource")
	}
	defer resp.Body.Close()
	var output chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
		log.Fatalf("failed to read response body %v", err)
	}
	return output
}

func startSpinner(done chan bool) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0
	for {
		select {
		case <-done:
			fmt.Print("\r")
			return
		default:
			fmt.Printf("\r%s thinking...", frames[i%len(frames)])
			time.Sleep(200 * time.Millisecond)
			i++
		}
	}
}

func getModels() []string {
	resp, err := http.Get("http://localhost:11434/api/tags")
	if err != nil {
		log.Fatalf("call to get ollama ls failed: %v", err)
	}
	defer resp.Body.Close()
	var modelOutput modelWrapper
	if err := json.NewDecoder(resp.Body).Decode(&modelOutput); err != nil {
		log.Fatalf("failed to get model output %v:", err)
	}

	stringOutput := make([]string, len(modelOutput.Models))
	for i := 0; i < len(modelOutput.Models); i++ {
		stringOutput[i] = modelOutput.Models[i].Name
	}
	return stringOutput
}

func handleToolCall(call toolCall) message {
	toolName := call.Function.Name
	switch toolName {
	case "list_directory":
		dirSlice, err := os.ReadDir(".")
		var dirString string

		if err != nil {
			dirString = "error reading directory: " + err.Error()
		}

		for _, val := range dirSlice {
			dirString = dirString + " " + val.Name()
		}

		ldMessage := message{
			Role:    "tool",
			Content: dirString,
			// what do do with the tool calls?,
		}
		return ldMessage
	case "read_file":
		return readFileHelper(&call)
	case "create_file":
		return createFileHelper(&call)
	case "edit_file":
		return editFileHelper(&call)
	default:
		return message{}
	}
}

func createFileHelper(call *toolCall) message {
	fileName, ok := call.Function.Arguments["path"].(string)
	if !ok {
		return message{
			Role:    "tool",
			Content: "error: invalid file name: ",
		}
	}

	// deal with creating the directories if the path is not just example.go but place/example.go
	dir := filepath.Dir(fileName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return message{Role: "tool", Content: fmt.Sprintf("error creating directory %v", err)}
	}

	newFile, err := os.Create(fileName)
	if err != nil {
		content := fmt.Sprintf("error creating file: %v", err)
		return message{
			Role:    "tool",
			Content: content,
		}
	}
	defer newFile.Close()

	outGoingContent := fmt.Sprintf("%v file created", newFile.Name())
	outGoingMessage := message{
		Role:    "tool",
		Content: outGoingContent,
	}
	return outGoingMessage
}

func editFileHelper(call *toolCall) message {
	path, ok := call.Function.Arguments["path"].(string)
	if !ok {
		return message{Role: "tool", Content: "error: no path provided"}
	}
	if isProtected(path) {
		return message{Role: "tool", Content: "error: file is protected"}
	}

	oldText, ok := call.Function.Arguments["old_text"].(string)
	if !ok {
		return message{Role: "tool", Content: "error no old text found"}
	}

	newText, ok := call.Function.Arguments["new_text"].(string)
	if !ok {
		return message{Role: "tool", Content: "error no new text found"}
	}

	fileContentBytes, err := os.ReadFile(path)
	if err != nil {
		return message{Role: "tool", Content: fmt.Sprintf("error: finding file: %v", err)}
	}
	fileContent := string(fileContentBytes)
	count := strings.Count(fileContent, oldText)

	if count == 0 {
		return message{Role: "tool", Content: "error old text not found in file"}
	}
	if count > 1 {
		return message{Role: "tool", Content: "error: old text matches multiple locations be specific"}
	}

	newContent := strings.Replace(fileContent, oldText, newText, 1)
	err = os.WriteFile(path, []byte(newContent), 0o644)
	if err != nil {
		return message{Role: "tool", Content: fmt.Sprintf("error writing file: %v", err)}
	}
	return message{Role: "tool", Content: fmt.Sprintf("%v file editted succesfully", path)}
}

func readFileHelper(call *toolCall) message {
	path, ok := call.Function.Arguments["path"].(string)
	if !ok {
		return message{
			Role:    "tool",
			Content: "error: no path provided",
		}
	}
	//what to do if null or not the right path need to add error handling
	//
	fileContentBytes, err := os.ReadFile(path)
	if err != nil {
		// dont need to log fatal but need to handle if we cant read from that path
		//
		content := fmt.Sprintf("error reading file in path: %v", err)
		return message{
			Role:    "tool",
			Content: content,
		}
	}

	fileContents := string(fileContentBytes)

	outGoingMessage := message{
		Role:    "tool",
		Content: fileContents,
	}
	return outGoingMessage
}

func toolCallHelper(toolCalls []toolCall, history *[]message, model string, depth int) {
	if depth > 8 {
		fmt.Println("depth of tool call hit 5 breaking out")
		return
	}
	fmt.Printf("depth: %v\n", depth)
	for _, call := range toolCalls {
		toolMessage := handleToolCall(call)
		*history = append(*history, toolMessage)
	}
	postToolChatRequest := buildChatRequest(*history, model)
	done := make(chan bool)
	go startSpinner(done)
	toolResponse := postRequest(postToolChatRequest)
	done <- true

	*history = append(*history, toolResponse.Message)

	if len(toolResponse.Message.ToolCalls) > 0 {
		toolCallHelper(toolResponse.Message.ToolCalls, history, model, depth+1)
	} else {
		fmt.Printf("DEBUG: %+v", toolResponse.Message)
		fmt.Println(toolResponse.Message.Content)

	}
}

func (a *App) chooseModels() string {
	models := getModels()
	fmt.Println("Available models to choose from")

	for idx, val := range models {
		modelNum := strconv.Itoa(idx + 1)
		fmt.Println(modelNum + ": " + val)
	}
	if !a.Scanner.Scan() {
		return a.CurrentModel
	}
	modelChosen := strings.TrimSpace(a.Scanner.Text())

	if !slices.Contains(models, modelChosen) {
		modelNum, err := strconv.Atoi(modelChosen)
		if err != nil {
			// what error should I throw here?
			fmt.Printf("Invalif input '%s' keeping current model", modelChosen)
			return a.CurrentModel
		}

		if 1 <= modelNum && modelNum <= len(models) {
			modelChosen = models[modelNum-1]
		} else {
			fmt.Println("No model associated with that number keeping old")
			return a.CurrentModel
		}
	}
	return modelChosen
}

func (a *App) handleCLICommand(command string) {
	switch command {
	case "/change-model":
		a.CurrentModel = a.chooseModels()
	}
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)

	metCodeApp := &App{
		CurrentModel: "",
		Scanner:      scanner,
	}
	// get a scanner that runs on the while loop which should give us a running way to engage with the models

	fmt.Println(banner)
	fmt.Println("Welcome to MetCode")
	metCodeApp.CurrentModel = metCodeApp.chooseModels()

	var history []message
	var sysContent string
	sysContentRaw, err := os.ReadFile("system_prompt.md")
	if err != nil {
		sysContent = "act normally use tools only if applicable and be helpful become an expert in whatever field is asked and make sure your work is accurate"
	} else {
		sysContent = string(sysContentRaw)
	}

	systemMessage := message{
		Role:    "system",
		Content: sysContent,
	}
	history = append(history, systemMessage)
	for {
		fmt.Println("enter your prompt: ")
		if !metCodeApp.Scanner.Scan() {
			break
		}
		input := strings.TrimSpace(metCodeApp.Scanner.Text())

		if input == "exit" || input == "quit" {
			fmt.Println("shutting down")
			break
		}
		if input[0] == '\\' {
			metCodeApp.handleCLICommand(input)
			continue
		}

		if input == "" {
			continue
		}

		var newMessage message
		newMessage.Content = input
		newMessage.Role = "user"
		history = append(history, newMessage)

		outgoingChatRequest := buildChatRequest(history, metCodeApp.CurrentModel)

		done := make(chan bool)
		go startSpinner(done)
		response := postRequest(outgoingChatRequest)
		done <- true
		history = append(history, response.Message)

		if len(response.Message.ToolCalls) > 0 {
			// call a tool call which then will re prompt the AI
			toolCallHelper(response.Message.ToolCalls, &history, metCodeApp.CurrentModel, 0)
		} else {
			fmt.Println(response.Message.Content)
		}

	}
}
