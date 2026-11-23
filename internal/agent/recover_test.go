package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/verify"
)

const (
	buggyAdd = "package fixture\n\n// Add returns the sum of a and b.\nfunc Add(a, b int) int {\n\treturn a - b\n}\n"
	fixedAdd = "package fixture\n\n// Add returns the sum of a and b.\nfunc Add(a, b int) int {\n\treturn a + b\n}"
	addTest  = "package fixture\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(2, 3) != 5 {\n\t\tt.Fatal(\"bad\")\n\t}\n}"
)

// readTurn is the history of a turn where the model read each path in order.
func readTurn(paths ...string) []ollama.Message {
	h := []ollama.Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "fix add.go"}}
	for _, p := range paths {
		content := buggyAdd
		if strings.HasSuffix(p, "_test.go") {
			content = addTest
		}
		h = append(h,
			ollama.Message{Role: "assistant", ToolCalls: []ollama.ToolCall{{Function: ollama.CalledFunction{
				Name: "read_file", Arguments: map[string]any{"path": p},
			}}}},
			ollama.Message{Role: "tool", Content: content},
		)
	}
	return h
}

func TestRecoverPastedFile(t *testing.T) {
	// the shapes below are copied from qwen2.5-coder replies in verify-addbug
	fenced := "The bug is the minus sign.\n\n```go\n" + fixedAdd + "\n```\n\nPlease run the tests again."
	tagged := "<tool_response>\n" + fixedAdd + "\n\n</tool_response>"

	cases := []struct {
		name    string
		content string
		calls   []ollama.ToolCall
		history []ollama.Message
		want    bool
	}{
		{"fenced fixed file", fenced, nil, readTurn("add.go"), true},
		{"tool_response fixed file", tagged, nil, readTurn("add.go"), true},
		{"test file read too still has one target", fenced, nil, readTurn("add_test.go", "add.go"), true},
		{"echo of the original", "<tool_response>\n" + buggyAdd + "</tool_response>", nil, readTurn("add.go"), false},
		{"read failed", fenced, nil, failedRead("add.go"), false},
		{"pasted test file", "<tool_response>\n" + addTest + "\n</tool_response>", nil, readTurn("add.go"), false},
		{"two different files", fenced + "\n```go\npackage fixture\n\nfunc Sub(a, b int) int { return a - b }\n```", nil, readTurn("add.go"), false},
		{"two read paths", fenced, nil, readTurn("add.go", "sub.go"), false},
		{"nothing read", fenced, nil, readTurn(), false},
		{"only the test file read", fenced, nil, readTurn("add_test.go"), false},
		{"fragment without package", "```go\nfunc Add(a, b int) int {\n\treturn a + b\n}\n```", nil, readTurn("add.go"), false},
		{"already a tool call", fenced, []ollama.ToolCall{{Function: ollama.CalledFunction{Name: "list_directory"}}}, readTurn("add.go"), false},
		{"plain prose", "The bug is the minus sign in add.go.", nil, readTurn("add.go"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "add.go"), []byte(buggyAdd), 0o644); err != nil {
				t.Fatal(err)
			}
			m := ollama.Message{Role: "assistant", Content: tc.content, ToolCalls: tc.calls}
			got, path, ok := recoverPastedFile(m, tc.history, root)
			if ok != tc.want {
				t.Fatalf("recovered = %v, want %v", ok, tc.want)
			}
			if !ok {
				return
			}
			args := got.ToolCalls[0].Function.Arguments
			if got.ToolCalls[0].Function.Name != "edit_file" || path != "add.go" || args["path"] != "add.go" {
				t.Errorf("call = %+v, path = %q", got.ToolCalls[0].Function, path)
			}
			if args["content"] != fixedAdd+"\n" {
				t.Errorf("content = %q", args["content"])
			}
		})
	}
}

// After the fix has landed, a reply that echoes it must not write it again:
// in the replay of 20261123-174225 qwen did this after its own edit.
func TestRecoverPastedFile_SkipsEchoOfAppliedEdit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "add.go"), []byte(fixedAdd+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := ollama.Message{Role: "assistant", Content: "<tool_response>\n" + fixedAdd + "\n</tool_response>"}
	if _, _, ok := recoverPastedFile(m, readTurn("add.go"), root); ok {
		t.Error("an echo of the file already on disk was recovered")
	}
}

// failedRead is a turn whose only read_file returned an error.
func failedRead(p string) []ollama.Message {
	h := readTurn(p)
	h[len(h)-1].Content = "error reading file in path: no such file"
	return h
}

// A turn whose final reply pastes the fix ends with the file written and verified.
func TestTurn_RecoversPastedFile(t *testing.T) {
	fakeApproval(t, true)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "add.go"), []byte(buggyAdd), 0o644); err != nil {
		t.Fatal(err)
	}
	host := fakeOllama(t,
		textReply(`{"name":"read_file","arguments":{"path":"add.go"}}`),
		textReply("Here is the fix:\n```go\n"+fixedAdd+"\n```"),
		textReply("done"),
	)
	script := &verifyScript{results: []verify.Result{passResult()}}
	a := &Agent{Model: "test", Host: host, Root: root, Verify: script.run}
	var stats TurnStats
	a.Stats = func(s TurnStats) { stats = s }
	var history []ollama.Message

	if _, err := a.Turn(&history, "fix add.go"); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(root, "add.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != fixedAdd+"\n" {
		t.Errorf("add.go = %q", body)
	}
	if stats.Recovered != 1 || script.calls != 1 || !stats.VerifyPassed {
		t.Errorf("Recovered=%d verify calls=%d VerifyPassed=%v", stats.Recovered, script.calls, stats.VerifyPassed)
	}
}
