package bench

import "strings"

// Why an edit_file call failed. The three buckets are the reason split in the
// table: the call was the wrong shape, it pointed at text that is not there, or
// something else (refused, io, or wording this file does not know yet).
const (
	EditFailMalformed = "mal" // the call itself was the wrong shape
	EditFailAnchor    = "anc" // the text or file it pointed at is not there
	EditFailOther     = "oth" // refused, io, or an unknown message
)

// editFailMarkers are distinctive fragments of the error strings in
// internal/tools/files.go. The "error: " prefix is not matched so a reworded
// prefix does not hide a case. TestClassifyEditError_DrivesRealHandlers calls
// the real handler, so a message that changes shape here fails a test instead
// of quietly sliding into oth.
var editFailMarkers = []struct{ marker, reason string }{
	{"missing required parameter old_text", EditFailMalformed},
	{"missing required parameter new_text", EditFailMalformed},
	{"old_text must be a string", EditFailMalformed},
	{"new_text must be a string", EditFailMalformed},
	{"edit_file takes new_text, not content", EditFailMalformed},
	{"instead of real line breaks", EditFailMalformed},
	{"no path provided", EditFailMalformed},

	{"does not exist but old_text", EditFailAnchor},
	{"old_text not found", EditFailAnchor},
	{"matches multiple locations", EditFailAnchor},
}

func classifyEditError(content string) string {
	for _, m := range editFailMarkers {
		if strings.Contains(content, m.marker) {
			return m.reason
		}
	}
	return EditFailOther
}

// EditFails counts edit_file errors by reason. It walks the transcript instead
// of a stored field, so rows written before this metric existed report real
// numbers instead of zero.
func (r Result) EditFails() map[string]int {
	out := map[string]int{}
	lastCall := ""

	for _, message := range r.Transcript {
		for _, tool := range message.ToolCalls {
			lastCall = tool.Function.Name
		}
		if message.Role == "tool" && lastCall == "edit_file" && strings.HasPrefix(message.Content, "error") {
			out[classifyEditError(message.Content)] += 1
		}
	}
	return out
}
