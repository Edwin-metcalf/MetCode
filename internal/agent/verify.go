package agent

import (
	"strings"

	"github.com/Edwin-metcalf/MetCode/internal/verify"
)

const maxVerifyFeedback = 8000

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
