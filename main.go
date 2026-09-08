package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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

func buildChatRequest(history []message, model string) chatRequest {
	var outgoing chatRequest

	outgoing.Messages = history
	outgoing.Model = model
	outgoing.Stream = false
	outgoing.Options.Temperature = 0.1
	// empty list directory function

	listDirectoryFunction := toolFunction{
		Name:        "list_directory",
		Description: "this function list all things in the current directory",
		Parameters: parameters{
			Type:       "object",
			Required:   []string{},
			Properties: map[string]any{},
		},
	}

	listDirectory := tool{
		Type:     "function",
		Function: listDirectoryFunction,
	}
	outgoing.Tools = []tool{listDirectory}

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
		return readDirectoryHelper(&call)
	default:
		return message{}
	}
}

func readDirectoryHelper(call *toolCall) message {
	path := call.Function.Arguments["path"].(string)
	//what to do if null or not the right path need to add error handling
	//
	fileContentBytes, err := os.ReadFile(path)
	if err != nil {
		// dont need to log fatal but need to handle if we cant read from that path
		//
	}

	var fileContents string
	fileContents = string(fileContentBytes)

	outGoingMessage := message{
		Role:    "tool",
		Content: fileContents,
	}
	return outGoingMessage
}

func toolCallHelper(toolCalls []toolCall, history *[]message, model string) {
	for _, call := range toolCalls {
		toolMessage := handleToolCall(call)
		*history = append(*history, toolMessage)
	}
	postToolChatRequest := buildChatRequest(*history, model)
	toolResponse := postRequest(postToolChatRequest)

	*history = append(*history, toolResponse.Message)
	fmt.Printf("DEBUG: %+v", toolResponse.Message)
	fmt.Println(toolResponse.Message.Content)
}

func main() {
	// this can be its own function later
	scanner := bufio.NewScanner(os.Stdin)
	// get a scanner that runs on the while loop which should give us a running way to engage with the models
	// would be cool to check ollama and print out the available models

	fmt.Println(banner)
	fmt.Println("Welcome to MetCode")
	models := getModels()
	fmt.Println("Available models to choose from")

	for idx, val := range models {
		modelNum := strconv.Itoa(idx + 1)
		fmt.Println(modelNum + ": " + val)
	}
	if !scanner.Scan() {
		return
	}
	modelChosen := strings.TrimSpace(scanner.Text())

	if !slices.Contains(models, modelChosen) {
		modelNum, err := strconv.Atoi(modelChosen)
		if err != nil {
			// what error should I throw here?
			log.Fatalf("%v is not a valid model or number", err)
		}

		if 1 <= modelNum && modelNum <= len(models) {
			modelChosen = models[modelNum-1]
		}

	}
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
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())

		if input == "exit" || input == "quit" {
			fmt.Println("shutting down")
			break
		}

		if input == "" {
			continue
		}

		var newMessage message
		newMessage.Content = input
		newMessage.Role = "user"
		history = append(history, newMessage)

		outgoingChatRequest := buildChatRequest(history, modelChosen)

		done := make(chan bool)
		go startSpinner(done)
		response := postRequest(outgoingChatRequest)
		done <- true
		history = append(history, response.Message)

		if len(response.Message.ToolCalls) > 0 {
			// call a tool call which then will re prompt the AI
			toolCallHelper(response.Message.ToolCalls, &history, modelChosen)
		} else {
			fmt.Println(response.Message.Content)
		}

	}
}
