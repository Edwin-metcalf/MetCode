//ui holds spinner logic command logic and all printing / running logic

package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Edwin-metcalf/MetCode/internal/agent"
	"github.com/Edwin-metcalf/MetCode/internal/ollama"
)

const banner = `
 __  __      _   ____          _
|  \/  | ___| |_/ ___|___   __| | ___
| |\/| |/ _ \ __| |   / _ \ / _\ |/ _ \
| |  | |  __/ |_| |__| (_) | (_| |  __/
|_|  |_|\___|\__\____\___/ \__,_|\___|
	`

type CLI struct {
	Scanner      *bufio.Scanner
	Agent        *agent.Agent
	SystemPrompt ollama.Message
	Conversation string
}

func New(a *agent.Agent, systemPrompt string) *CLI {
	a.Wait = Spinner
	return &CLI{
		Scanner:      bufio.NewScanner(os.Stdin),
		Agent:        a,
		SystemPrompt: ollama.Message{Role: "system", Content: systemPrompt},
	}
}

func (c *CLI) Run() {
	fmt.Println(banner)
	fmt.Println("Welcome to MetCode")

	c.Agent.Model = c.chooseModel()

	history := []ollama.Message{c.SystemPrompt}

	for {
		fmt.Println("enter your prompt: ")
		if !c.Scanner.Scan() {
			break
		}
		input := strings.TrimSpace(c.Scanner.Text())

		if input == "exit" || input == "quit" {
			fmt.Println("shutting down")
			break
		}

		if input == "" {
			continue
		}

		if input[0] == '/' {
			c.handleCommand(input, &history)
			continue
		}

		reply, err := c.Agent.Turn(&history, input)
		if err != nil {
			fmt.Println("error: ", err)
			continue
		}

		fmt.Println(reply)
		fmt.Printf("Estimated context window: %v \n", agent.EstimateTokens(history))
	}
}
