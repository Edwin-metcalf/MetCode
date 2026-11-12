package bench

import (
	"strings"
	"testing"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/plan"
	"github.com/Edwin-metcalf/MetCode/internal/tools"
)

func TestClassifyEditError_Literals(t *testing.T) {
	cases := map[string]string{
		"error: no path provided": EditFailMalformed,
		"error: missing required parameter old_text: the exact snippet to replace":                   EditFailMalformed,
		"error: missing required parameter new_text: the replacement text":                           EditFailMalformed,
		"error: old_text must be a string, not a different JSON type":                                EditFailMalformed,
		"error: new_text must be a string, not a different JSON type":                                EditFailMalformed,
		"error: edit_file takes new_text, not content. new_text is only the text":                    EditFailMalformed,
		"error: new_text contains the characters \\n instead of real line breaks":                    EditFailMalformed,
		"error: file does not exist but old_text is not empty":                                       EditFailAnchor,
		"error: old_text not found in file. Call read_file and copy old_text":                        EditFailAnchor,
		"error: old_text matches multiple locations (or is empty but the file is not). Include more": EditFailAnchor,
		"error: file is protected":          EditFailOther,
		"error: path is possibly dangerous": EditFailOther,
		"error: finding file: boom":         EditFailOther,
		"error: something nobody wrote yet": EditFailOther,
	}
	for content, want := range cases {
		if got := classifyEditError(content); got != want {
			t.Errorf("classifyEditError(%q) = %q, want %q", content, got, want)
		}
	}
}

// The literals above are a copy of the handler's messages, so they cannot catch
// a rewording in internal/tools. This drives the real handler once per bucket:
// if a message is reworded past its marker it lands in oth and fails here.
func TestClassifyEditError_DrivesRealHandlers(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"thing.go": "package fixture\n\nfunc Double() int {\n\treturn 1\n}\n",
		"dup.go":   "package fixture\n\nvar A = 1\nvar A = 1\n",
		"go.mod":   "module fixture\n",
	})

	oldQuiet, oldApprove := tools.Quiet, tools.Approve
	tools.Quiet = true
	tools.Approve = func(string) bool { return true }
	t.Cleanup(func() { tools.Quiet, tools.Approve = oldQuiet, oldApprove })

	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"no path", map[string]any{}, EditFailMalformed},
		{"no old_text", map[string]any{"path": "thing.go"}, EditFailMalformed},
		{"old_text wrong type", map[string]any{"path": "thing.go", "old_text": 5, "new_text": "y"}, EditFailMalformed},
		{"content instead of new_text", map[string]any{"path": "thing.go", "old_text": "", "content": "y"}, EditFailMalformed},
		{"literal backslash n", map[string]any{"path": "thing.go", "old_text": "return 1", "new_text": `return 2\n`}, EditFailMalformed},
		{"missing file with anchor", map[string]any{"path": "nope.go", "old_text": "abc", "new_text": "def"}, EditFailAnchor},
		{"anchor not in file", map[string]any{"path": "thing.go", "old_text": "NOT THERE", "new_text": "y"}, EditFailAnchor},
		{"anchor matches twice", map[string]any{"path": "dup.go", "old_text": "var A = 1", "new_text": "var A = 2"}, EditFailAnchor},
		{"protected file", map[string]any{"path": "go.mod", "old_text": "", "new_text": "x"}, EditFailOther},
		{"path escapes root", map[string]any{"path": "../escape.txt", "old_text": "x", "new_text": "y"}, EditFailOther},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var planItems []plan.Item
			msg := tools.Handle(ollama.ToolCall{Function: ollama.CalledFunction{
				Name: "edit_file", Arguments: tc.args,
			}}, root, &planItems)

			if !strings.HasPrefix(msg.Content, "error") {
				t.Fatalf("expected an error message, got %q", msg.Content)
			}
			if got := classifyEditError(msg.Content); got != tc.want {
				t.Errorf("classifyEditError(%q) = %q, want %q", msg.Content, got, tc.want)
			}
		})
	}
}

// editToolError builds the two messages the agent leaves behind for a failing
// edit_file call: the assistant turn with the call, then the tool reply.
func editToolError(t *testing.T, content string) []ollama.Message {
	t.Helper()
	return []ollama.Message{
		{Role: "assistant", ToolCalls: []ollama.ToolCall{{Function: ollama.CalledFunction{
			Name: "edit_file", Arguments: map[string]any{"path": "x.go"},
		}}}},
		{Role: "tool", Content: content},
	}
}

func TestEditFails_FromTranscript(t *testing.T) {
	r := mkResult("m", "t")
	r.Transcript = append(r.Transcript, editToolError(t, "error: missing required parameter old_text")...)
	r.Transcript = append(r.Transcript, editToolError(t, "error: file does not exist but old_text is not empty")...)
	r.Transcript = append(r.Transcript, editToolError(t, "error: file does not exist but old_text is not empty")...)
	// a read_file failure is a different tool, so it does not belong in the split
	r.Transcript = append(r.Transcript,
		ollama.Message{Role: "assistant", ToolCalls: []ollama.ToolCall{{Function: ollama.CalledFunction{Name: "read_file"}}}},
		ollama.Message{Role: "tool", Content: "error: no path provided"},
	)
	// a successful edit is not a failure
	r.Transcript = append(r.Transcript, editToolError(t, "x.go file editted succesfully")...)

	got := r.EditFails()

	if got[EditFailMalformed] != 1 || got[EditFailAnchor] != 2 || got[EditFailOther] != 0 {
		t.Errorf("EditFails = %v, want mal 1, anc 2, oth 0", got)
	}
}

func TestEditFails_NoErrorsIsEmpty(t *testing.T) {
	got := mkResult("m", "t").EditFails()
	if len(got) != 0 {
		t.Errorf("EditFails = %v, want nothing", got)
	}
}

// stats reads the split from the transcript, so a row written before the metric
// existed still reports real numbers.
func TestStats_EditFailsFromTranscript(t *testing.T) {
	r := mkResult("m", "t")
	r.Transcript = editToolError(t, "error: old_text not found in file.")

	s := Group{Model: "m", Task: "t", Rows: []Result{r}}.Stats()

	if s.EditFails[EditFailAnchor] != 1 {
		t.Errorf("EditFails = %v, want anc 1", s.EditFails)
	}
}

func TestStats_ToolErrorsByTool(t *testing.T) {
	rows := []Result{
		mkResult("m", "t", func(r *Result) { r.ToolErrors = map[string]int{"edit_file": 2, "read_file": 1} }),
		mkResult("m", "t", func(r *Result) { r.ToolErrors = map[string]int{"edit_file": 3, "echo": 1} }),
	}
	before := rows[0].ToolErrors["edit_file"]

	s := Group{Model: "m", Task: "t", Rows: rows}.Stats()

	want := map[string]int{"edit_file": 5, "read_file": 1, "echo": 1}
	for tool, n := range want {
		if s.ToolErrorsByTool[tool] != n {
			t.Errorf("ToolErrorsByTool[%s] = %d, want %d", tool, s.ToolErrorsByTool[tool], n)
		}
	}
	if s.ToolErrors != 7 {
		t.Errorf("ToolErrors = %d, want 7", s.ToolErrors)
	}
	// the per-row map is shared, so summing must not write into it
	if rows[0].ToolErrors["edit_file"] != before {
		t.Errorf("Stats mutated a row's ToolErrors: %v", rows[0].ToolErrors)
	}
}
