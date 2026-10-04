// ollama package this holds all the stuff that deals with the ollama server
package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls"`
}

type ToolCall struct {
	Function CalledFunction `json:"function"`
}

type CalledFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ChatRequest struct {
	Messages []Message `json:"messages"`
	Model    string    `json:"model"`
	Stream   bool      `json:"stream"`
	Tools    []Tool    `json:"tools"`
	Options  Options   `json:"options"`
}

type ChatResponse struct {
	Message Message `json:"message"`
}

type Options struct {
	Temperature float64 `json:"temperature"`
	NumCtx      int     `json:"num_ctx"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  Parameters `json:"parameters"`
}

type Parameters struct {
	Type       string         `json:"type"`
	Required   []string       `json:"required"`
	Properties map[string]any `json:"properties"`
}

type models struct {
	Name string `json:"name"`
}

type modelWrapper struct {
	Models []models `json:"models"`
}

func Chat(req ChatRequest, host string) (ChatResponse, error) {
	// this is probably not needed but whatever
	b, err := json.Marshal(req)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("serializing request error %v: ", err)
	}

	body := bytes.NewBuffer(b)
	url := host + "/api/chat"

	resp, err := http.Post(url, "application/json", body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("error contacting ollama at %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return ChatResponse{}, fmt.Errorf("ollama returned %s: %s", resp.Status, msg)
	}
	var output ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
		return ChatResponse{}, fmt.Errorf("error decoding response: %w", err)
	}
	return output, nil
}

func ListModels(host string) []string {
	resp, err := http.Get(host + "/api/tags")
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
