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

func verificationFailureMessage(res verify.Result) string {
	return "Verification failed after the file change.\n" +
		"Command: " + verifyCommand(res) + "\n" +
		"Fix the code, not the test files, and call edit_file again.\n" +
		"Do not claim success until verification passes.\n" +
		"Output:\n" + clip(res.Output, maxVerifyFeedback)
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
