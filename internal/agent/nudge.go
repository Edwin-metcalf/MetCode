package agent

// editNudge is sent once when a turn that was expected to change a file ends
// without a successful edit_file call. It names the payload keys edit_file
// declares, so TestEditNudgeMatchesSchema fails if the schema is renamed and
// this text is left behind. The bench opts in through Agent.ExpectEdit; the
// chat client leaves it off so ordinary conversation is never nagged.
const editNudge = "You have not changed any file yet. Do not paste the code in your reply. " +
	"Call the edit_file tool now with the file path plus one of: old_text + new_text " +
	"to replace a snippet, or content = the complete file (for Go, include the " +
	"package clause). If you have not read the file yet, call read_file first."
