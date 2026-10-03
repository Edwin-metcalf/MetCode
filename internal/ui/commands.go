package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Edwin-metcalf/MetCode/internal/conversation"
	"github.com/Edwin-metcalf/MetCode/internal/ollama"
)

func (c *CLI) handleCommand(command string, history *[]ollama.Message) {
	switch command {
	case "/change-model":
		c.Agent.Model = c.chooseModel()
	case "/save":
		if err := c.handleSave(history); err != nil {
			fmt.Println("error saving:", err)
		}
	case "/load":
		loaded, err := c.handleLoad()
		if err != nil {
			fmt.Println("error loading:", err)
			return
		}
		if loaded != nil {
			*history = *loaded
		}
	case "/clear":
		*history = []ollama.Message{c.SystemPrompt}

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

func (c *CLI) chooseModel() string {
	models := ollama.ListModels(c.Agent.Host)
	fmt.Println("Available models to choose from")

	for idx, val := range models {
		modelNum := strconv.Itoa(idx + 1)
		fmt.Println(modelNum + ": " + val)
	}
	if !c.Scanner.Scan() {
		return c.Agent.Model
	}
	modelChosen := strings.TrimSpace(c.Scanner.Text())

	if !slices.Contains(models, modelChosen) {
		modelNum, err := strconv.Atoi(modelChosen)
		if err != nil {
			// what error should I throw here?
			fmt.Printf("Invalif input '%s' keeping current model", modelChosen)
			return c.Agent.Model
		}

		if 1 <= modelNum && modelNum <= len(models) {
			modelChosen = models[modelNum-1]
		} else {
			fmt.Println("No model associated with that number keeping old")
			return c.Agent.Model
		}
	}
	return modelChosen
}

func (c *CLI) handleLoad() (*[]ollama.Message, error) {
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

	if !c.Scanner.Scan() {
		return nil, fmt.Errorf("error scanning input")
	}
	choice := strings.TrimSpace(c.Scanner.Text())

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
	c.Conversation = name
	return &loaded, nil
}

func (c *CLI) handleSave(history *[]ollama.Message) error {
	dir, err := conversation.DefaultDir()
	if err != nil {
		return err
	}

	name := c.Conversation
	if name == "" {
		fmt.Println("What would you like the conversation to be named?")
		if !c.Scanner.Scan() {
			return fmt.Errorf("error scanning")
		}
		name = strings.TrimSpace(c.Scanner.Text())
	}

	saved, err := conversation.Save(dir, name, *history)
	if err != nil {
		return err
	}
	c.Conversation = saved
	fmt.Println("saved conversation as", saved)
	return nil
}
