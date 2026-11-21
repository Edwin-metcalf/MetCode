package agent

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/verify"
)

const maxVerifyFeedback = 8000

// refuseTestEdit keeps Go test files read-only while verify is on, because the
// tests are what verify trusts. In the verify-two-bugs-msgUpdate study llama
// answered a failing test by rewriting scores_test.go in 11 of 20 runs (9 applied).
// It lives here and not in tools.Handle because only the agent knows whether
// verify is on. Aliases like write_file are already canonicalized to edit_file.
func (a *Agent) refuseTestEdit(call ollama.ToolCall) (ollama.Message, bool) {
	if a.Verify == nil || call.Function.Name != "edit_file" {
		return ollama.Message{}, false
	}
	path, _ := call.Function.Arguments["path"].(string)
	if !strings.HasSuffix(filepath.Base(path), "_test.go") {
		return ollama.Message{}, false
	}
	if !a.Quiet {
		fmt.Printf("[tool] edit_file %s refused: test files are read-only while verify is on\n", path)
	}
	return ollama.Message{Role: "tool", Content: fmt.Sprintf("error: %s is a test file and is read-only. Do not change the tests; edit the code they test instead.", path)}, true
}

func verifyCommand(res verify.Result) string {
	if len(res.Command) == 0 {
		return "unknown command"
	}
	return strings.Join(res.Command, " ")
}

// verificationFailureMessage does not name the command. In the verify-two-bugs
// study llama answered a "Command: go test ..." line by calling a tool named
// "go test" in 18 of 20 runs instead of editing; the CLI still shows the
// command through VerifyReport. The "Verification failed" prefix is kept so
// tests and transcript queries still find it.
func verificationFailureMessage(res verify.Result) string {
	return "Verification failed: the tests still fail after your edit. MetCode already ran them for you; do not run them yourself.\n" +
		"Test output:\n" + clip(res.Output, maxVerifyFeedback) + "\n" +
		"Work out which line of the code produces that wrong result, then call edit_file with the fix. " +
		"Fix the code, not the test files. Do not claim success until the tests pass."
}

func verificationLimitMessage(res verify.Result) string {
	return "Verification failed after the file change; retry limit reached.\n" +
		"Command: " + verifyCommand(res) + "\n" +
		"Output:\n" + clip(res.Output, maxVerifyFeedback)
}

func verificationErrorMessage(res verify.Result) string {
	return "Verification could not run. Command: " + verifyCommand(res) +
		". Error: " + res.Err.Error()
}
