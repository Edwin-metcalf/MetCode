package agent

import "strings"

// editNudge is sent once when a turn that was expected to change a file ends
// without a successful edit_file call. It names the payload keys edit_file
// declares, so TestEditNudgeMatchesSchema fails if the schema is renamed and
// this text is left behind. The bench opts in through Agent.ExpectEdit; the
// chat client leaves it off so ordinary conversation is never nagged.
//
// It used to end with "If you have not read the file yet, call read_file first",
// which llama3.1 answered with another read_file 14 times out of 19 nudges even
// after it had just read the file. The read escape hatch is gone: content mode
// does not need a read, and snippet mode only needs the read it already did.
const editNudge = "You have not changed any file yet. Do not paste code in your reply, " +
	"and do not call read_file again. Send the edit_file call now with the path plus " +
	"either old_text + new_text to replace a snippet, or content = the complete file " +
	"(for Go, include the package clause)."

// editNudgeFor repeats the edit_file error back to the model. llama3.1 answered
// "error: missing required parameter old_text" with "the bug has been fixed" in
// all 25 cases of the WholeMessageNormalize study, without retrying, so the
// failure is quoted instead of summarised.
func editNudgeFor(lastErr string) string {
	if lastErr == "" {
		return editNudge
	}
	return editNudge + " Your last edit_file did not apply: " + lastErr
}

// clip bounds the quoted error so it cannot double the nudge. The longest
// message edit_file returns is about 250 characters.
func clip(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return strings.TrimSpace(string(runes[:n])) + "..."
}
