package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/plan"
	"github.com/Edwin-metcalf/MetCode/internal/prompt"
	"github.com/Edwin-metcalf/MetCode/internal/tools"
	"github.com/joho/godotenv"
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
	OllamaHost   string
	Conversation string
	SystemPrompt ollama.Message
	ProjectRoot  string
	CurrentPlan  []plan.Item
}

func buildChatRequest(history []ollama.Message, model string) ollama.ChatRequest {
	var outgoing ollama.ChatRequest

	outgoing.Messages = history
	outgoing.Model = model
	outgoing.Stream = false
	outgoing.Options.Temperature = 0.1
	outgoing.Tools = tools.All

	return outgoing
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

func toolCallHelper(toolCalls []ollama.ToolCall, history *[]ollama.Message, depth int, failCount int, app *App) {
	if depth > 8 {
		fmt.Println("depth of tool call hit 8 breaking out")
		return
	}

	if failCount > 3 {
		fmt.Println("failed over and over again breaking out")
		return
	}
	//fmt.Printf("depth: %v\n", depth)
	//
	for _, call := range toolCalls {
		toolMessage := tools.Handle(call, app.ProjectRoot, &app.CurrentPlan)
		*history = append(*history, toolMessage)
	}

	postToolChatRequest := buildChatRequest(*history, app.CurrentModel)
	done := make(chan bool)
	go startSpinner(done)
	toolResponse, err := ollama.Chat(postToolChatRequest, app.OllamaHost)
	done <- true
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	*history = append(*history, toolResponse.Message)

	if len(toolResponse.Message.ToolCalls) > 0 {

		if strings.HasPrefix(toolResponse.Message.Content, "error") {
			failCount += 1
		} else {
			failCount = 0
		}
		toolCallHelper(toolResponse.Message.ToolCalls, history, depth+1, failCount, app)
	} else {
		//fmt.Printf("DEBUG: %+v", toolResponse.Message)
		fmt.Println(toolResponse.Message.Content)
	}
}

func (a *App) chooseModels() string {
	models := ollama.ListModels(a.OllamaHost)
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

func (a *App) handleLoad() (*[]ollama.Message, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".metcode", "conversations")
	dirSlice, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("error reading directory: %w", err)
	}

	if len(dirSlice) == 0 {
		fmt.Println("No conversations saved!")
		return nil, nil
	}

	var stringSlice []string
	for _, val := range dirSlice {
		stringSlice = append(stringSlice, val.Name())
	}

	var outputString string
	totalFileNum := strconv.Itoa(len(stringSlice))

	for idx, val := range stringSlice {
		outputString += strconv.Itoa(idx+1) + ": " + val
		if idx > 10 {
			outputString += "10 files showing, total number of files: " + totalFileNum
			break
		}
	}
	fmt.Println(outputString)

	if !a.Scanner.Scan() {
		return nil, fmt.Errorf("error scanning input")
	}
	selectedInput := strings.TrimSpace(a.Scanner.Text())
	nums := [10]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
	var filePath string

	if slices.Contains(nums[:], selectedInput) {
		idx, err := strconv.Atoi(selectedInput)
		if err != nil {
			return nil, err
		}
		filePath = filepath.Join(dir, stringSlice[idx-1])

	} else if slices.Contains(stringSlice, selectedInput) {
		filePath = filepath.Join(dir, selectedInput)
	} else {
		return nil, fmt.Errorf("bad input %q: use the number or file name shown", selectedInput)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var loaded []ollama.Message
	if err := json.Unmarshal(data, &loaded); err != nil {
		return nil, err
	}

	return &loaded, nil
}

func (a *App) handleSave(history *[]ollama.Message) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dir := filepath.Join(home, ".metcode", "conversations")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	if a.Conversation == "" {

		fmt.Println("What would you like the conversation to be named?")

		if !a.Scanner.Scan() {
			fmt.Println("error scanning")
			return fmt.Errorf("error scanning")
		}
		conversationName := strings.TrimSpace(a.Scanner.Text())

		if !strings.HasSuffix(conversationName, ".txt") {
			conversationName += ".txt"
		}

		data, err := json.MarshalIndent(*history, "", "	")
		if err != nil {
			return err
		}
		err = os.WriteFile(filepath.Join(dir, conversationName), data, 0o644)
		if err != nil {
			fmt.Println("error saving file")
			return err
		}
		a.Conversation = conversationName

	} else {
		data, err := json.MarshalIndent(*history, "", "	")
		if err != nil {
			return err
		}

		err = os.WriteFile(filepath.Join(dir, a.Conversation), data, 0o644)
		if err != nil {
			fmt.Println("error appending data")
			return err
		}
	}

	fmt.Println("saving conversation")
	return nil
}

func (a *App) handleCLICommand(command string, history *[]ollama.Message) {
	switch command {
	case "/change-model":
		a.CurrentModel = a.chooseModels()
	case "/save":
		if err := a.handleSave(history); err != nil {
			fmt.Println("error saving:", err)
		}
	case "/load":
		loaded, err := a.handleLoad()
		if err != nil {
			fmt.Println("error loading:", err)
			return
		}
		if loaded != nil {
			*history = *loaded
		}
	case "/clear":
		*history = []ollama.Message{a.SystemPrompt}

	case "/help":
		fmt.Println(`Available Commands
	- /help - Displays help menu
	- /change-model - Changes the model your using
	- /save - Save the current conversation to a new conversation or an old one
	- /load - Load a past conversation
			`)
	default:
		fmt.Println("That is not a command I currently have try /help")
	}
}

func estimateTokens(messages []ollama.Message) int {
	totalTokens := 0
	for _, val := range messages {
		text := val.Content
		totalTokens += len(text) / 4
	}
	return totalTokens
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, using default local host")
	}

	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}

	scanner := bufio.NewScanner(os.Stdin)

	workingDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get working directory")
	}

	metCodeApp := &App{
		CurrentModel: "",
		Scanner:      scanner,
		OllamaHost:   ollamaHost,
		Conversation: "",
		ProjectRoot:  workingDir,
	}
	// get a scanner that runs on the while loop which should give us a running way to engage with the models

	fmt.Println(banner)
	fmt.Println("Welcome to MetCode")
	metCodeApp.CurrentModel = metCodeApp.chooseModels()

	var history []ollama.Message

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("could not determine home directory: %v", err)
	}

	sysContent, err := prompt.LoadOrCreateSystemPrompt(home)
	if err != nil {
		log.Fatalf("failed to load system prompt: %v", err)
	}

	systemMessage := ollama.Message{
		Role:    "system",
		Content: sysContent,
	}
	history = append(history, systemMessage)
	metCodeApp.SystemPrompt = systemMessage
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

		if input == "" {
			continue
		}
		if input[0] == '/' {
			metCodeApp.handleCLICommand(input, &history)
			continue
		}
		var newMessage ollama.Message
		newMessage.Content = input
		newMessage.Role = "user"
		history = append(history, newMessage)

		outgoingChatRequest := buildChatRequest(history, metCodeApp.CurrentModel)

		done := make(chan bool)
		go startSpinner(done)
		response, err := ollama.Chat(outgoingChatRequest, ollamaHost)
		done <- true
		if err != nil {
			fmt.Println("error", err)
			history = history[:len(history)-1]
			continue
		}
		history = append(history, response.Message)

		if len(response.Message.ToolCalls) > 0 {
			// call a tool call which then will re prompt the AI
			toolCallHelper(response.Message.ToolCalls, &history, 0, 0, metCodeApp)
		} else {
			fmt.Println(response.Message.Content)
		}

		contextWindowlen := estimateTokens(history)
		fmt.Printf("Estimated context window: %v \n", contextWindowlen)

	}
}
