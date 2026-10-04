// agent runs the loop between the model and the tools
package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/plan"
	"github.com/Edwin-metcalf/MetCode/internal/tools"
)

const (
	maxToolRounds = 12
	maxFailRounds = 3
)

var (
	ErrMaxRounds       = errors.New("too many tool-call rounds in one turn")
	ErrTooManyFailures = errors.New("tool calls kept failing")
)

type Agent struct {
	Model string
	Host  string
	Root  string
	Plan  []plan.Item
	// returns runs when the request finishes (the UI uses it for the spinner).
	Wait func() func()
}

func BuildChatRequest(history []ollama.Message, model string) ollama.ChatRequest {
	var outgoing ollama.ChatRequest

	outgoing.Messages = history
	outgoing.Model = model
	outgoing.Stream = false
	outgoing.Options.Temperature = 0.2
	outgoing.Options.NumCtx = 32000
	outgoing.Tools = tools.All

	return outgoing
}

func EstimateTokens(messages []ollama.Message) int {
	totalTokens := 0
	for _, val := range messages {
		text := val.Content
		totalTokens += len(text) / 4
	}
	return totalTokens
}

func firstCallOnly(m ollama.Message) ollama.Message {
	if len(m.ToolCalls) > 1 {
		m.ToolCalls = m.ToolCalls[:1]
	}
	return m
}

func (a *Agent) chat(history []ollama.Message) (ollama.ChatResponse, error) {
	if a.Wait != nil {
		stop := a.Wait()
		defer stop()
	}
	return ollama.Chat(BuildChatRequest(history, a.Model), a.Host)
}

func (a *Agent) Turn(history *[]ollama.Message, userInput string) (string, error) {
	//turns user input to history and runs the model tool loop until we have an anser without a tool call
	*history = append(*history, ollama.Message{Role: "user", Content: userInput})

	resp, err := a.chat(*history)
	if err != nil {
		//dont add empty
		*history = (*history)[:len(*history)-1]
		return "", err
	}

	resp.Message = firstCallOnly(resp.Message)
	*history = append(*history, resp.Message)
	fmt.Printf("[debug] toolcalls=%d content=%q\n", len(resp.Message.ToolCalls), resp.Message.Content)

	failRounds := 0

	for round := 0; len(resp.Message.ToolCalls) > 0; round++ {
		if round >= maxToolRounds {
			return "", ErrMaxRounds
		}

		allFailed := true

		for _, call := range resp.Message.ToolCalls {
			result := tools.Handle(call, a.Root, &a.Plan)

			if !strings.HasPrefix(result.Content, "error") {
				allFailed = false
			}
			*history = append(*history, result)
		}

		if allFailed {
			failRounds++
		} else {
			failRounds = 0
		}

		if failRounds > maxFailRounds {
			return "", ErrTooManyFailures
		}

		resp, err = a.chat(*history)
		if err != nil {
			return "", err
		}
		resp.Message = firstCallOnly(resp.Message)
		*history = append(*history, resp.Message)
	}
	return resp.Message.Content, nil

}
