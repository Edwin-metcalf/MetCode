package main

import (
	"bufio"
	_ "embed"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Edwin-metcalf/MetCode/internal/agent"
	"github.com/Edwin-metcalf/MetCode/internal/conversation"
	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/prompt"
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
	Scanner      *bufio.Scanner
	Conversation string
	SystemPrompt ollama.Message
	Agent        *agent.Agent
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
func spin() func() {
	done := make(chan bool)
	go startSpinner(done)
	return func() { done <- true }
}

func (a *App) chooseModels() string {
	models := ollama.ListModels(a.Agent.Host)
	fmt.Println("Available models to choose from")

	for idx, val := range models {
		modelNum := strconv.Itoa(idx + 1)
		fmt.Println(modelNum + ": " + val)
	}
	if !a.Scanner.Scan() {
		return a.Agent.Model
	}
	modelChosen := strings.TrimSpace(a.Scanner.Text())

	if !slices.Contains(models, modelChosen) {
		modelNum, err := strconv.Atoi(modelChosen)
		if err != nil {
			// what error should I throw here?
			fmt.Printf("Invalif input '%s' keeping current model", modelChosen)
			return a.Agent.Model
		}

		if 1 <= modelNum && modelNum <= len(models) {
			modelChosen = models[modelNum-1]
		} else {
			fmt.Println("No model associated with that number keeping old")
			return a.Agent.Model
		}
	}
	return modelChosen
}

func (a *App) handleLoad() (*[]ollama.Message, error) {
	dir, err := conversation.DefaultDir()
	if err != nil {
		return nil, err
	}

	names, err := conversation.List(dir)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		fmt.Println("No conversations saved!")
		return nil, nil
	}
	for i, n := range names {
		fmt.Printf("%d: %s\n", i+1, n)
	}

	if !a.Scanner.Scan() {
		return nil, fmt.Errorf("error scanning input")
	}
	choice := strings.TrimSpace(a.Scanner.Text())

	name := choice
	if n, err := strconv.Atoi(choice); err == nil {
		if n < 1 || n > len(names) {
			return nil, fmt.Errorf("no conversation numbered %d", n)
		}
		name = names[n-1]
	}

	loaded, err := conversation.Load(dir, name)
	if err != nil {
		return nil, err
	}
	a.Conversation = name
	return &loaded, nil
}

func (a *App) handleSave(history *[]ollama.Message) error {
	dir, err := conversation.DefaultDir()
	if err != nil {
		return err
	}

	name := a.Conversation
	if name == "" {
		fmt.Println("What would you like the conversation to be named?")
		if !a.Scanner.Scan() {
			return fmt.Errorf("error scanning")
		}
		name = strings.TrimSpace(a.Scanner.Text())
	}

	saved, err := conversation.Save(dir, name, *history)
	if err != nil {
		return err
	}
	a.Conversation = saved
	fmt.Println("saved conversation as", saved)
	return nil
}

func (a *App) handleCLICommand(command string, history *[]ollama.Message) {
	switch command {
	case "/change-model":
		a.Agent.Model = a.chooseModels()
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
		Scanner:      scanner,
		Conversation: "",
		Agent:        &agent.Agent{Host: ollamaHost, Root: workingDir, Wait: spin},
	}
	// get a scanner that runs on the while loop which should give us a running way to engage with the models

	fmt.Println(banner)
	fmt.Println("Welcome to MetCode")
	metCodeApp.Agent.Model = metCodeApp.chooseModels()

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
		reply, err := metCodeApp.Agent.Turn(&history, input)
		if err != nil {
			fmt.Println("error: ", err)
			continue
		}

		fmt.Println(reply)
		fmt.Printf("Estimated context window: %v \n", agent.EstimateTokens(history))

	}
}
