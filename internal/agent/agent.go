// agent runs the loop between the model and the tools helper files like normalize and compact also in the package
// normalize helps normalize reponses and clean up wierd model quircks
// compact uses a transcript and a fixed prompt to shrink the conversation
package agent

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/plan"
	"github.com/Edwin-metcalf/MetCode/internal/tools"
	"github.com/Edwin-metcalf/MetCode/internal/verify"
)

const (
	maxToolRounds     = 12
	maxFailRounds     = 3
	maxVerifyAttempts = 2
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
	Wait  func() func()
	Stats func(TurnStats)
	// silences the per-call log; bench sets it so a run prints only its JSON line
	Quiet bool
	// ExpectEdit asks for one nudge when a turn ends without a successful
	// edit_file. It is opt-in because only the caller knows whether the prompt
	// was supposed to change a file; chat chit-chat must never be nudged.
	ExpectEdit bool
	// Verify runs after an applied edit; nil disables it.
	Verify verify.Func
	// VerifyReport prints each verification result to the CLI.
	VerifyReport func(verify.Result, bool)
	// Seed is sent on every request so a run can be repeated; 0 means random.
	Seed int
}

type TurnStats struct {
	Calls          int
	Rounds         int
	Nudges         int
	Recovered      int // replies whose pasted file became an edit_file call
	VerifyAttempts int
	VerifyPassed   bool
	VerifyMS       time.Duration
	PromptTokens   int
	OutputTokens   int
	PrefillTime    time.Duration
	GenTime        time.Duration
	LoadTime       time.Duration
	WallTime       time.Duration
}

// exported to bench
// constants for communication with the model
const Temperature = 0.2
const NumCTX = 8192

func addStats(s *TurnStats, r ollama.ChatResponse) {
	s.PromptTokens += r.PromptEvalCount
	s.OutputTokens += r.EvalCount
	s.PrefillTime += time.Duration(r.PromptEvalDuration)
	s.GenTime += time.Duration(r.EvalDuration)
	s.LoadTime += time.Duration(r.LoadDuration)
}

// logCall folds one model call into the turn totals and prints a line for it.
// printing lives here rather than in addStats so the accumulator stays pure
func logCall(s *TurnStats, r ollama.ChatResponse, quiet bool) {
	addStats(s, r)
	s.Calls++

	if quiet {
		return
	}

	perSec := 0.0
	if r.EvalDuration > 0 {
		perSec = float64(r.EvalCount) / time.Duration(r.EvalDuration).Seconds()
	}
	fmt.Printf("[call] %d prompt_tok=%d eval_tok=%d prefill=%s gen=%s (%.1f tok/s) load=%s\n",
		s.Calls, r.PromptEvalCount, r.EvalCount,
		time.Duration(r.PromptEvalDuration).Round(time.Millisecond),
		time.Duration(r.EvalDuration).Round(time.Millisecond), perSec,
		time.Duration(r.LoadDuration).Round(time.Millisecond))
}

func BuildChatRequest(history []ollama.Message, model string) ollama.ChatRequest {
	var outgoing ollama.ChatRequest

	outgoing.Messages = history
	outgoing.Model = model
	outgoing.Stream = false
	outgoing.Options.Temperature = Temperature
	outgoing.Options.NumCtx = NumCTX
	outgoing.Tools = tools.All

	return outgoing
}

// used for compact right now but could be for others, by not including any tool definitions we lower our context by like 600
func BuildChatRequestNoTools(history []ollama.Message, model string) ollama.ChatRequest {
	var outgoing ollama.ChatRequest

	outgoing.Messages = history
	outgoing.Model = model
	outgoing.Stream = false
	outgoing.Options.Temperature = Temperature
	outgoing.Options.NumCtx = NumCTX
	outgoing.Tools = []ollama.Tool{}

	return outgoing
}

func firstCallOnly(m ollama.Message) ollama.Message {
	if len(m.ToolCalls) > 1 {
		m.ToolCalls = m.ToolCalls[:1]
	}
	return m
}

// noToolsFlag should be true if you do not want to include tools on the call
func (a *Agent) chat(history []ollama.Message, noToolsFlag bool) (ollama.ChatResponse, error) {
	if a.Wait != nil {
		stop := a.Wait()
		defer stop()
	}
	req := BuildChatRequest(history, a.Model)
	if noToolsFlag {
		req = BuildChatRequestNoTools(history, a.Model)
	}
	req.Options.Seed = a.Seed
	return ollama.Chat(req, a.Host)
}

func (a *Agent) Turn(history *[]ollama.Message, userInput string) (string, error) {
	start := time.Now()
	var stats TurnStats

	defer func() {
		stats.WallTime = time.Since(start)
		if a.Stats != nil {
			a.Stats(stats)
		}
	}()
	return a.runTurn(history, userInput, &stats)

}

// recordResponse runs a model message through the cleanup every response needs
// and appends it to history. Every response site (first, tool round, verify
// retry, nudge) goes through here, or a call shape recovered at one site is lost
// at another.
func (a *Agent) recordResponse(history *[]ollama.Message, resp ollama.ChatResponse, stats *TurnStats) ollama.ChatResponse {
	resp.Message = firstCallOnly(resp.Message)
	resp.Message = canonicalizeNativeCalls(resp.Message)
	resp.Message = a.normalizeMessage(resp.Message)
	if recovered, path, ok := recoverPastedFile(resp.Message, *history, a.Root); ok {
		resp.Message = recovered
		stats.Recovered++
		if !a.Quiet {
			fmt.Printf("[recover] pasted file -> edit_file %s\n", path)
		}
	}
	*history = append(*history, resp.Message)
	return resp
}

func (a *Agent) runTurn(history *[]ollama.Message, userInput string, stats *TurnStats) (string, error) {
	//turns user input to history and runs the model tool loop until we have an anser without a tool call

	*history = append(*history, ollama.Message{Role: "user", Content: userInput})

	resp, err := a.chat(*history, false)
	if err != nil {
		//dont add empty
		*history = (*history)[:len(*history)-1]
		return "", err
	}
	logCall(stats, resp, a.Quiet)

	resp = a.recordResponse(history, resp, stats)

	failRounds := 0
	edited := false
	pendingVerify := false
	verifyAttempts := 0
	nudged := false
	// the last edit_file failure, quoted back in the nudge. Empty when the model
	// never called edit_file at all (or only called it successfully).
	lastEditErr := ""
	// one shared budget: a nudged attempt cannot double the tool-call allowance
	rounds := 0

	for {
		for len(resp.Message.ToolCalls) > 0 {
			stats.Rounds += 1
			if rounds >= maxToolRounds {
				return "", ErrMaxRounds
			}
			rounds++

			allFailed := true

			for _, call := range resp.Message.ToolCalls {
				result, refused := a.refuseTestEdit(call)
				if !refused {
					result = tools.Handle(call, a.Root, &a.Plan)
				}

				if !strings.HasPrefix(result.Content, "error") {
					allFailed = false
					// A user refusal ("Edit canceled by user") is not an error and
					// also not a reason to nag the model into trying again.
					if call.Function.Name == "edit_file" {
						edited = true
						if result.Content != "Edit canceled by user" {
							pendingVerify = true
						}
					}
				} else if call.Function.Name == "edit_file" {
					lastEditErr = clip(result.Content, 240)
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

			resp, err = a.chat(*history, false)
			if err != nil {
				return "", err
			}

			logCall(stats, resp, a.Quiet)
			resp = a.recordResponse(history, resp, stats)
		}

		// Verify only after a real edit, and only when a new edit needs checking.
		if a.Verify != nil && pendingVerify && verifyAttempts < maxVerifyAttempts {
			verifyAttempts++
			pendingVerify = false

			res := a.Verify(a.Root)
			final := verifyAttempts >= maxVerifyAttempts
			stats.VerifyAttempts++
			stats.VerifyMS += res.Duration
			if a.VerifyReport != nil {
				a.VerifyReport(res, final)
			}

			if res.Err != nil {
				*history = append(*history, ollama.Message{Role: "system", Content: verificationErrorMessage(res)})
				break
			}
			if res.Passed {
				stats.VerifyPassed = true
				break
			}

			if final {
				*history = append(*history, ollama.Message{Role: "system", Content: verificationLimitMessage(res)})
				break
			}
			*history = append(*history, ollama.Message{Role: "system", Content: verificationFailureMessage(res)})
			if rounds >= maxToolRounds {
				return "", ErrMaxRounds
			}
			rounds++
			resp, err = a.chat(*history, false)
			if err != nil {
				return "", err
			}
			logCall(stats, resp, a.Quiet)
			resp = a.recordResponse(history, resp, stats)
			continue
		}

		// the model is about to end the turn: nudge once if the caller expected
		// a file change and none landed. A successful edit ends the turn as usual.
		if !a.ExpectEdit || edited || nudged {
			break
		}
		nudged = true
		stats.Nudges += 1
		*history = append(*history, ollama.Message{Role: "system", Content: editNudgeFor(lastEditErr)})

		resp, err = a.chat(*history, false)
		if err != nil {
			return "", err
		}
		logCall(stats, resp, a.Quiet)
		resp = a.recordResponse(history, resp, stats)
	}
	return resp.Message.Content, nil

}
