//ui holds spinner logic command logic and all printing / running logic

package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Edwin-metcalf/MetCode/internal/agent"
	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/verify"
)

const banner = `
 __  __      _   ____          _
|  \/  | ___| |_/ ___|___   __| | ___
| |\/| |/ _ \ __| |   / _ \ / _\ |/ _ \
| |  | |  __/ |_| |__| (_) | (_| |  __/
|_|  |_|\___|\__\____\___/ \__,_|\___|
	`

type CLI struct {
	Scanner       *bufio.Scanner
	Agent         *agent.Agent
	SystemPrompt  ollama.Message
	Conversation  string
	VerifyCommand []string
}

func New(a *agent.Agent, systemPrompt string) *CLI {
	a.Wait = Spinner
	a.Stats = printStats
	a.VerifyReport = printVerify
	return &CLI{
		Scanner:      bufio.NewScanner(os.Stdin),
		Agent:        a,
		SystemPrompt: ollama.Message{Role: "system", Content: systemPrompt},
	}
}
func printStats(s agent.TurnStats) {
	perSec := 0.0

	if s.GenTime > 0 {
		perSec = float64(s.OutputTokens) / s.GenTime.Seconds()
	}

	fmt.Printf("[stats] rounds=%d verify_attempts=%d verify_passed=%t verify_ms=%s prompt_tok=%d out_tok=%d prefill=%s gen=%s (%.1f tok/s) load=%s total=%s\n",
		s.Rounds, s.VerifyAttempts, s.VerifyPassed, s.VerifyMS.Round(time.Millisecond),
		s.PromptTokens, s.OutputTokens,
		s.PrefillTime.Round(time.Millisecond),
		s.GenTime.Round(time.Millisecond), perSec,
		s.LoadTime.Round(time.Millisecond),
		s.WallTime.Round(time.Millisecond))
}

func printVerify(res verify.Result, final bool) {
	cmd := strings.Join(res.Command, " ")
	dur := res.Duration.Round(time.Millisecond)

	switch {
	case res.Err != nil:
		fmt.Printf("[verify] %s -> error (%s)\n%v\n", cmd, dur, res.Err)
	case res.Passed:
		fmt.Printf("[verify] %s -> pass (%s)\n", cmd, dur)
	case final:
		fmt.Printf("[verify] %s -> fail, retry limit reached (%s)\n", cmd, dur)
		printVerifyOutput(res.Output)
	default:
		fmt.Printf("[verify] %s -> fail, retrying (%s)\n", cmd, dur)
		printVerifyOutput(res.Output)
	}
}

func printVerifyOutput(output string) {
	if out := strings.TrimRight(output, "\n"); out != "" {
		fmt.Println(out)
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
	}
}
