package bench

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Edwin-metcalf/MetCode/internal/agent"
	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/prompt"
	"github.com/Edwin-metcalf/MetCode/internal/tools"
)

// config is what varies between runs
type Config struct {
	Model     string
	Host      string
	RunIndex  int
	GitCommit string
}

// big ol return
type Result struct {
	//identity from the config
	Task        string  `json:"task"`
	Model       string  `json:"model"`
	RunIndex    int     `json:"run_index"`
	NumCtx      int     `json:"num_ctx"`
	Temperature float64 `json:"temperature"`
	PromptHash  string  `json:"prompt_hash"` //sha256 the prompt.default
	GitCommit   string  `json:"git_commit"`

	//outcome from Grade and the error Turn returned
	Passed      bool   `json:"passed"`
	Tampered    bool   `json:"tampered"`
	Ended       string `json:"ended"` //how the round ended could be ok max_rounds model_error etc
	EditApplied bool   `json:"edit_applied"`

	//behavior from summarize Approve counter and workspace
	ToolCalls        map[string]int `json:"tool_calls"`
	ToolErrors       map[string]int `json:"tool_errors"`
	UnnecessaryCalls int            `json:"unnecessary_calls"`
	DeclinedCommands int            `json:"declined_commands"`

	//cost from stats and timer
	Rounds       int   `json:"rounds"`
	PromptTokens int   `json:"prompt_tokens"`
	OutputTokens int   `json:"output_tokens"`
	WallMS       int64 `json:"wall_ms"`
}

// summarizes history to find the tool calls and tool_calls that errored
func summarize(history []ollama.Message) (calls map[string]int, errs map[string]int) {
	calls = map[string]int{}
	errs = map[string]int{}
	lastCall := ""

	for _, message := range history {
		for _, tool := range message.ToolCalls {
			calls[tool.Function.Name] += 1
			lastCall = tool.Function.Name
		}
		if message.Role == "tool" && strings.HasPrefix(message.Content, "error") {
			errs[lastCall] += 1
		}
	}
	return calls, errs
}

// editApplied reports wether the workspace differes from the task's files/
func editApplied(t Task, workspace string) (bool, error) {
	fixtureDir := filepath.Join(t.Dir, "files")
	inFixture := make(map[string]bool)
	changed := false

	err := filepath.WalkDir(fixtureDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(fixtureDir, path)
		if err != nil {
			return err
		}
		inFixture[rel] = true
		original, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		current, err := os.ReadFile(filepath.Join(workspace, rel))
		if errors.Is(err, os.ErrNotExist) {
			changed = true
			return filepath.SkipAll
		}
		if err != nil {
			return err
		}
		if !bytes.Equal(original, current) {
			changed = true
			return filepath.SkipAll
		}
		return nil

	})
	if err != nil {
		return false, err
	}
	if changed {
		return true, nil
	}

	//need a second pass for a workspace file wiht no fixture counterpart was created by model
	err = filepath.WalkDir(workspace, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		if !inFixture[rel] {
			changed = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}

// takes a task and config and returns a result
func Run(task Task, cfg Config) (Result, error) {
	//will need a ollama connection for the bench marking
	host := cfg.Host
	if host == "" {
		host = os.Getenv("OLLAMA_HOST")
	}
	if host == "" {
		host = "http://localhost:11434"
	}

	workDir, cleanup, err := NewWorkspace(task)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()

	declined := 0
	old := tools.Approve
	tools.Approve = func(kind string) bool {
		if kind == "command" {
			declined += 1
			return false
		}
		return true
	}

	defer func() { tools.Approve = old }()

	var stats agent.TurnStats
	a := agent.Agent{Model: cfg.Model, Host: host, Root: workDir, Stats: func(s agent.TurnStats) { stats = s }}

	sys := prompt.Default()
	history := []ollama.Message{{Role: "system", Content: sys}}

	ended := "ok"
	_, turnErr := a.Turn(&history, task.Prompt)
	if turnErr != nil {
		//TODO an un reachable ollama server is not a model_error
		//be able to tell them apart would be good to keep garbage rows out
		if errors.Is(turnErr, agent.ErrMaxRounds) {
			ended = "max_rounds"
		} else if errors.Is(turnErr, agent.ErrTooManyFailures) {
			ended = "too_many_failures"
		} else {
			ended = "model_error"
		}
	}

	calls, errs := summarize(history)

	//before we grade
	edited, err := editApplied(task, workDir)
	if err != nil {
		return Result{}, err
	}

	var grade Grade
	if !task.NoTools {
		grade, err = GradeWorkspace(task, workDir)
		if err != nil {
			return Result{}, err
		}
	}

	unnecessary := 0
	if task.NoTools {
		for _, n := range calls {
			unnecessary += n
		}
	}
	sum := sha256.Sum256([]byte(sys))

	return Result{
		Task: task.Name, Model: cfg.Model, RunIndex: cfg.RunIndex,
		NumCtx: agent.NumCTX, Temperature: agent.Temperature,
		PromptHash: hex.EncodeToString(sum[:]), GitCommit: cfg.GitCommit,
		Passed: grade.Passed, Tampered: grade.Tampered, Ended: ended, EditApplied: edited,
		ToolCalls: calls, ToolErrors: errs,
		UnnecessaryCalls: unnecessary, DeclinedCommands: declined,
		Rounds: stats.Rounds, PromptTokens: stats.PromptTokens, OutputTokens: stats.OutputTokens,
		WallMS: stats.WallTime.Milliseconds(),
	}, nil

}
