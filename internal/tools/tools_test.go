package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Edwin-metcalf/MetCode/internal/ollama"
	"github.com/Edwin-metcalf/MetCode/internal/plan"
)

func TestReadFileHelper_ValidFile(t *testing.T) {
	root := t.TempDir()
	want := "hello metcode"
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "read_file",
			Arguments: map[string]any{"path": "hello.txt"},
		},
	}

	result := readFileHelper(call, root)

	if result.Role != "tool" {
		t.Errorf("Role is not tool it is %v", result.Role)
	}
	if result.Content == "" {
		t.Errorf("expected %q, got %q", want, result.Content)
	}
}

func TestReadDirectoryHelper_NoPath(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "read_file",
			Arguments: map[string]any{},
		},
	}
	result := readFileHelper(call, ".")
	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %q", result.Content)
	}
}

func TestReadFileHelper_DirectoryGivesFriendlyError(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "read_file",
			Arguments: map[string]any{"path": "sub"},
		},
	}

	result := readFileHelper(call, root)

	want := "error: sub is a directory. Use list_directory to see what is in it."
	if result.Content != want {
		t.Errorf("expected %q, got %q", want, result.Content)
	}
}

func TestProtectedFiles(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "main.go", "old_text": "package main\ngood code:)", "new_text": "bad code:("},
		},
	}
	result := editFileHelper(call, ".")

	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %v", result.Content)
	}
}

// editing an existing file rewrites it on disk once the user confirms.
func TestEditFileValid(t *testing.T) {
	feedStdin(t, "y\n")

	root := t.TempDir()
	path := filepath.Join(root, "code.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc old() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "code.go", "old_text": "func old() {}", "new_text": "func fresh() {}"},
		},
	}
	result := editFileHelper(call, root)

	if result.Role != "tool" {
		t.Errorf("error: Role is not a tool it is: %v", result.Role)
	}
	if strings.HasPrefix(result.Content, "error") {
		t.Fatalf("expected a successful edit, got %q", result.Content)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "package main\n\nfunc fresh() {}\n"
	if string(got) != want {
		t.Errorf("file on disk = %q, want %q", got, want)
	}
}

func TestReadFileHelper_PathEscapesRoot(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "read_file",
			Arguments: map[string]any{"path": "../../../passwords"},
		},
	}

	result := readFileHelper(call, ".")
	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %q", result.Content)
	}
}

func TestEditFileHelper_PathEscapesRoot(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "../outside.go", "old_text": "foo", "new_text": "bar"},
		},
	}
	result := editFileHelper(call, ".")
	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %q", result.Content)
	}
}

func TestUpdatePlanItemHelper_ValidUpdate(t *testing.T) {
	items := []plan.Item{
		{Id: 1.0, Description: "read main.go", Status: plan.StatusPending},
		{Id: 2.0, Description: "write tests", Status: plan.StatusPending},
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name: "update_plan",
			Arguments: map[string]any{
				"id":     1.0,
				"status": "in_progress",
			},
		},
	}

	result := updatePlanItemHelper(call, &items)

	if strings.Contains(result.Content, "error") {
		t.Errorf("expected success but got %q", result.Content)
	}

	if items[0].Status != plan.StatusInProgress {
		t.Errorf("expected item 0 to be in_progress, got %v", items[0].Status)
	}

	if items[1].Status != plan.StatusPending {
		t.Errorf("item 1 should be untouched, got %v", items[1].Status)
	}
}

func TestUpdatePlanItemHelper_InvalidStatus(t *testing.T) {
	items := []plan.Item{
		{Id: 1.0, Description: "read main.go", Status: plan.StatusPending},
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name: "update_plan",
			Arguments: map[string]any{
				"id":     1.0,
				"status": "completed", // not one of the three valid values
			},
		},
	}

	result := updatePlanItemHelper(call, &items)

	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected rejection of invalid status, got %q", result.Content)
	}
	if items[0].Status != plan.StatusPending {
		t.Errorf("item should be untouched after rejected update, got %v", items[0].Status)
	}
}

func TestCreatePlanHelper_MissingDescriptions(t *testing.T) {
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "create_plan",
			Arguments: map[string]any{},
		},
	}
	var items []plan.Item
	result := createPlanHelper(call, &items)

	if !strings.Contains(result.Content, "error") {
		t.Errorf("expected error but got %q", result.Content)
	}
	if len(items) != 0 {
		t.Errorf("plan should remain empty, got %d items", len(items))
	}
}

func TestCreatePlanHelper_DoesNotClobberExistingPlan(t *testing.T) {
	items := []plan.Item{{Id: 1.0, Description: "existing step", Status: plan.StatusInProgress}}
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "create_plan",
			Arguments: map[string]any{},
		},
	}
	_ = createPlanHelper(call, &items)

	if len(items) != 1 {
		t.Errorf("expected existing plan to survive a rejected call, got %d items", len(items))
	}
	if items[0].Status != plan.StatusInProgress {
		t.Errorf("existing item should be untouched")
	}
}

func TestEditFileHelper_DoesNotTruncateExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	original := "important content"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	// an empty old_text means "create", but this file already exists. It must be
	// refused rather than treated as empty, or the content would be wiped.
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "notes.txt", "old_text": "", "new_text": "replacement"},
		},
	}
	result := editFileHelper(call, root)

	if !strings.HasPrefix(result.Content, "error") {
		t.Errorf("expected an error for an empty old_text on an existing file, got %q", result.Content)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("existing file was modified: now %q", got)
	}
}

// "content" is not an alias for new_text: models use it to mean "the whole file",
// and applying that at a snippet site splices the file into itself (llama3.1,
// baseline-3 rows 25-29 sent old_text "return a - b" plus the whole file). the
// handler must refuse it and leave the file untouched.
func TestEditFileHelper_RejectsContentAsNewText(t *testing.T) {
	root := t.TempDir()
	original := "package fixture\n\nfunc Add(a, b int) int {\n\treturn a - b\n}\n"
	if err := os.WriteFile(filepath.Join(root, "add.go"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name: "edit_file",
			Arguments: map[string]any{
				"path":     "add.go",
				"old_text": "return a - b",
				"content":  "package fixture\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n",
			},
		},
	}
	result := editFileHelper(call, root)

	if !strings.HasPrefix(result.Content, "error") {
		t.Fatalf("expected content to be rejected, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "new_text") {
		t.Errorf("error = %q, want it to name new_text", result.Content)
	}

	got, err := os.ReadFile(filepath.Join(root, "add.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("file was modified: %q", got)
	}
}

// the shape small models actually send: path plus the whole file. a complete Go
// file must replace the target instead of being rejected for a missing anchor.
func TestEditFileHelper_ContentReplacesWholeFile(t *testing.T) {
	feedStdin(t, "y\n")

	root := t.TempDir()
	path := filepath.Join(root, "add.go")
	original := "package fixture\n\nfunc Add(a, b int) int {\n\treturn a - b\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	fixed := "package fixture\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n"
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "add.go", "content": fixed},
		},
	}
	result := editFileHelper(call, root)

	if strings.HasPrefix(result.Content, "error") {
		t.Fatalf("expected a whole-file write, got %q", result.Content)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != fixed {
		t.Errorf("file = %q, want %q", got, fixed)
	}
}

// qwen sends the fixed file in new_text with no old_text. That is a whole-file
// write, not a missing anchor; baselinePlus refused 31 of these.
func TestEditFileHelper_NewTextWithoutOldTextReplacesWholeFile(t *testing.T) {
	feedStdin(t, "y\n")

	root := t.TempDir()
	path := filepath.Join(root, "add.go")
	original := "package fixture\n\nfunc Add(a, b int) int {\n\treturn a - b\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	fixed := "package fixture\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n"
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "add.go", "new_text": fixed},
		},
	}
	result := editFileHelper(call, root)

	if strings.HasPrefix(result.Content, "error") {
		t.Fatalf("expected a whole-file write, got %q", result.Content)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != fixed {
		t.Errorf("file = %q, want %q", got, fixed)
	}
}

// a fragment is not a whole file: writing it would delete the package clause
// and every other function, so it must be refused.
func TestEditFileHelper_ContentFragmentIsRejected(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "add.go")
	original := "package fixture\n\nfunc Add(a, b int) int {\n\treturn a - b\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "add.go", "content": "return a + b"},
		},
	}
	result := editFileHelper(call, root)

	if !strings.HasPrefix(result.Content, "error") {
		t.Fatalf("a fragment must not be written, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "not valid Go") {
		t.Errorf("error = %q, want it to explain content is not a whole file", result.Content)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("file was modified: %q", got)
	}
}

func TestEditFileHelper_ContentCreatesMissingFile(t *testing.T) {
	feedStdin(t, "y\n")

	root := t.TempDir()
	content := "package sub\n"
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "sub/new.go", "content": content},
		},
	}
	result := editFileHelper(call, root)

	if strings.HasPrefix(result.Content, "error") {
		t.Fatalf("expected the file to be created, got %q", result.Content)
	}
	got, err := os.ReadFile(filepath.Join(root, "sub", "new.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("file = %q, want %q", got, content)
	}
}

// the create path uses O_EXCL so a file that appears while the approval prompt
// is waiting cannot be clobbered. The guard must stay on the create path only:
// replacing a file that already existed at read time is the common shape and
// must not trip it.
func TestEditFileHelper_CreateRaceDoesNotClobber(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "appeared.txt")
	userBytes := "written by the user\n"

	old := Approve
	Approve = func(kind string) bool {
		if err := os.WriteFile(path, []byte(userBytes), 0o644); err != nil {
			t.Errorf("seeding the race: %v", err)
		}
		return true
	}
	t.Cleanup(func() { Approve = old })

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "appeared.txt", "content": "from the model\n"},
		},
	}
	result := editFileHelper(call, root)

	if !strings.HasPrefix(result.Content, "error") {
		t.Fatalf("a file created during the prompt must not be overwritten, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "file was created after the edit was proposed") {
		t.Errorf("error = %q, want the race message", result.Content)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != userBytes {
		t.Errorf("file = %q, want the user's bytes untouched", got)
	}
}

// models also use "text" for the whole file; it must take the same path.
func TestEditFileHelper_TextIsWholeFile(t *testing.T) {
	feedStdin(t, "y\n")

	root := t.TempDir()
	content := "hello\n"
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "notes.txt", "text": content},
		},
	}
	result := editFileHelper(call, root)

	if strings.HasPrefix(result.Content, "error") {
		t.Fatalf("expected text to be accepted as a whole file, got %q", result.Content)
	}
	got, err := os.ReadFile(filepath.Join(root, "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("file = %q, want %q", got, content)
	}
}

// replacing an existing file with nothing is never intended, so it is refused.
func TestEditFileHelper_ContentEmptyOverExistingFileIsRejected(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	original := "keep me\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "notes.txt", "content": ""},
		},
	}
	result := editFileHelper(call, root)

	if !strings.HasPrefix(result.Content, "error") {
		t.Fatalf("expected an empty content to be refused, got %q", result.Content)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("file was modified: %q", got)
	}
}

// a call with no payload at all must explain both shapes, not just old_text.
func TestEditFileHelper_MissingPayloadNamesBothModes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{Name: "edit_file", Arguments: map[string]any{"path": "a.go"}},
	}
	result := editFileHelper(call, root)

	if !strings.HasPrefix(result.Content, "error") {
		t.Fatalf("expected an error, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "edit_file with only path") {
		t.Errorf("error should say a bare path does nothing, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "content") {
		t.Errorf("error should name the whole-file mode, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "Nothing was changed") {
		t.Errorf("error should say the file is untouched, got %q", result.Content)
	}
}

// a missing key and a key sent with the wrong JSON type are different mistakes,
// so the model has to be told which one it made.
func TestEditFileHelper_DistinguishesMissingFromWrongType(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "old_text missing",
			args: map[string]any{"path": "a.go", "new_text": "x"},
			want: "missing required parameter old_text",
		},
		{
			name: "old_text wrong type",
			args: map[string]any{"path": "a.go", "old_text": 1, "new_text": "x"},
			want: "old_text must be a string",
		},
		{
			name: "new_text missing",
			args: map[string]any{"path": "a.go", "old_text": "keep"},
			want: "missing required parameter new_text",
		},
		{
			name: "new_text wrong type",
			args: map[string]any{"path": "a.go", "old_text": "keep", "new_text": nil},
			want: "new_text must be a string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := &ollama.ToolCall{Function: ollama.CalledFunction{Name: "edit_file", Arguments: tt.args}}
			result := editFileHelper(call, root)

			if !strings.HasPrefix(result.Content, "error") {
				t.Fatalf("expected an error, got %q", result.Content)
			}
			if !strings.Contains(result.Content, tt.want) {
				t.Errorf("error = %q, want it to contain %q", result.Content, tt.want)
			}
		})
	}

	// none of those failures may touch the file
	got, err := os.ReadFile(filepath.Join(root, "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep me\n" {
		t.Errorf("file was modified: %q", got)
	}
}

func TestEditFileHelper_RefusesProtectedFile(t *testing.T) {
	root := t.TempDir()
	original := "module example.com/m\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "go.mod", "old_text": "", "new_text": "module replaced\n"},
		},
	}
	result := editFileHelper(call, root)

	if !strings.HasPrefix(result.Content, "error") {
		t.Errorf("edit_file should refuse a protected file, got %q", result.Content)
	}
	got, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("protected file was modified: now %q", got)
	}
}

func TestEditFileHelper_CreatesNewFileInSubdirectory(t *testing.T) {
	feedStdin(t, "y\n")

	root := t.TempDir()
	content := "package sub\n"

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "sub/dir/example.go", "old_text": "", "new_text": content},
		},
	}
	result := editFileHelper(call, root)

	if strings.HasPrefix(result.Content, "error") {
		t.Fatalf("expected success, got %q", result.Content)
	}
	got, err := os.ReadFile(filepath.Join(root, "sub", "dir", "example.go"))
	if err != nil {
		t.Fatalf("expected nested file to exist: %v", err)
	}
	if string(got) != content {
		t.Errorf("file content = %q, want %q", got, content)
	}
}

func TestEditFileHelper_CreatesFileWhenOldTextEmpty(t *testing.T) {
	feedStdin(t, "y\n")

	root := t.TempDir()
	content := "hello from create\n"

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "new.txt", "old_text": "", "new_text": content},
		},
	}
	result := editFileHelper(call, root)

	if strings.HasPrefix(result.Content, "error") {
		t.Fatalf("expected success, got %q", result.Content)
	}
	got, err := os.ReadFile(filepath.Join(root, "new.txt"))
	if err != nil {
		t.Fatalf("expected file to be created: %v", err)
	}
	if string(got) != content {
		t.Errorf("file content = %q, want %q", got, content)
	}
}

func TestEditFileHelper_MissingFileWithOldTextErrors(t *testing.T) {
	root := t.TempDir()

	// the model named a substring to replace, but there is no file to hold it.
	// That is a mistake and must error, not silently create a partial file.
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "ghost.txt", "old_text": "something", "new_text": "else"},
		},
	}
	result := editFileHelper(call, root)

	if !strings.HasPrefix(result.Content, "error") {
		t.Errorf("expected an error, got %q", result.Content)
	}
	if _, err := os.Stat(filepath.Join(root, "ghost.txt")); !os.IsNotExist(err) {
		t.Errorf("no file should have been created, stat err = %v", err)
	}
}

func TestEditFileHelper_CreateCanceledByUser(t *testing.T) {
	emptyStdin(t) // an empty stdin declines the y/n confirm

	root := t.TempDir()
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "canceled.txt", "old_text": "", "new_text": "nope"},
		},
	}
	result := editFileHelper(call, root)

	if result.Content != "Edit canceled by user" {
		t.Errorf("expected cancel message, got %q", result.Content)
	}
	if _, err := os.Stat(filepath.Join(root, "canceled.txt")); !os.IsNotExist(err) {
		t.Errorf("a declined create must not write the file, stat err = %v", err)
	}
}

func TestEditFileHelper_RejectsBadOldText(t *testing.T) {
	cases := map[string]struct {
		fileContent string
		oldText     string
	}{
		"old_text not found":        {fileContent: "keep me\n", oldText: "not present"},
		"old_text matches multiple": {fileContent: "dup\ndup\n", oldText: "dup"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "notes.txt")
			if err := os.WriteFile(path, []byte(tc.fileContent), 0o644); err != nil {
				t.Fatal(err)
			}

			call := &ollama.ToolCall{
				Function: ollama.CalledFunction{
					Name:      "edit_file",
					Arguments: map[string]any{"path": "notes.txt", "old_text": tc.oldText, "new_text": "x"},
				},
			}
			result := editFileHelper(call, root)

			if !strings.HasPrefix(result.Content, "error") {
				t.Errorf("expected an error prefix, got %q", result.Content)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.fileContent {
				t.Errorf("file should be untouched, got %q", got)
			}
		})
	}
}

// a payload a model double-encoded gets decoded, but only when the shape is
// unambiguous: no real line break anywhere and at least one \n escape.
func TestRepairEscapes(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"escaped whole file": {
			in:   `package fixture\n\nfunc F() {}\n`,
			want: "package fixture\n\nfunc F() {}\n",
		},
		"escaped indentation": {
			in:   `func F() {\n\treturn 1\n}`,
			want: "func F() {\n\treturn 1\n}",
		},
		"escaped quotes": {
			in:   `func F() {\n\tfmt.Println(\"hi\")\n}`,
			want: "func F() {\n\tfmt.Println(\"hi\")\n}",
		},
		"already real newlines": {
			in:   "package fixture\n\nfunc F() {}\n",
			want: "package fixture\n\nfunc F() {}\n",
		},
		"one line snippet without escapes": {
			in:   "return a + b",
			want: "return a + b",
		},
		// mixed shape is left alone on purpose: real newlines mean the text is
		// genuine, so a remaining \n is more likely to belong there.
		"mixed shape": {
			in:   "func F() {\n\ts := `a\\nb`\n}\n",
			want: "func F() {\n\ts := `a\\nb`\n}\n",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := repairEscapes(tc.in); got != tc.want {
				t.Errorf("repairEscapes(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// the over-escaped shape llama3.1 sends must apply as a real snippet edit.
// old_text has to be decoded too or the anchor never matches the file.
func TestEditFileHelper_RepairsEscapedSnippet(t *testing.T) {
	feedStdin(t, "y\n")

	root := t.TempDir()
	path := filepath.Join(root, "add.go")
	original := "package fixture\n\nfunc Add(a, b int) int {\n\treturn a - b\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name: "edit_file",
			Arguments: map[string]any{
				"path":     "add.go",
				"old_text": `package fixture\n\nfunc Add(a, b int) int {\n\treturn a - b\n}\n`,
				"new_text": `package fixture\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n`,
			},
		},
	}
	result := editFileHelper(call, root)

	if strings.HasPrefix(result.Content, "error") {
		t.Fatalf("expected the escaped payload to be repaired, got %q", result.Content)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n"
	if string(got) != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestEditFileHelper_RepairsEscapedContent(t *testing.T) {
	feedStdin(t, "y\n")

	root := t.TempDir()
	path := filepath.Join(root, "add.go")
	original := "package fixture\n\nfunc Add(a, b int) int {\n\treturn a - b\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name: "edit_file",
			Arguments: map[string]any{
				"path":    "add.go",
				"content": `package fixture\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n`,
			},
		},
	}
	result := editFileHelper(call, root)

	if strings.HasPrefix(result.Content, "error") {
		t.Fatalf("expected the escaped whole file to be repaired, got %q", result.Content)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n"
	if string(got) != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

// a genuine multi-line payload keeps a literal backslash-n inside a Go string
// literal. Repair must not touch text that already has real newlines.
func TestEditFileHelper_KeepsNewlineInsideGoString(t *testing.T) {
	feedStdin(t, "y\n")

	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	original := "package fixture\n\nfunc A() {}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name: "edit_file",
			Arguments: map[string]any{
				"path":     "a.go",
				"old_text": "func A() {}",
				"new_text": "func A() {\n\ts := \"a\\nb\"\n\t_ = s\n}",
			},
		},
	}
	result := editFileHelper(call, root)

	if strings.HasPrefix(result.Content, "error") {
		t.Fatalf("expected a successful edit, got %q", result.Content)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `s := "a\nb"`) {
		t.Errorf("literal backslash-n inside the string was decoded, file = %q", got)
	}
}

func TestRunCommandHelper_RejectsPathEscapingRoot(t *testing.T) {
	cases := map[string]string{
		"parent escape":    "../outside",
		"absolute path":    "/etc",
		"rooted backslash": `\etc`,
		"interior dots":    "a/../../etc",
		"nested escape":    "sub/../../secrets",
	}

	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			call := &ollama.ToolCall{
				Function: ollama.CalledFunction{
					Name:      "run_command",
					Arguments: map[string]any{"command": "ls", "path": path},
				},
			}

			// returns before the y/n confirm, so this never touches stdin
			result := runCommandHelper(call, t.TempDir())

			if !strings.HasPrefix(result.Content, "error") {
				t.Errorf("path %q should be rejected, got %q", path, result.Content)
			}
		})
	}
}

// feedStdin points os.Stdin at a pipe pre-loaded with input so a y/n confirm
// reads the answer instead of blocking on the real terminal. Closing the write
// end means any further reads return EOF, which a confirm treats as "no".
func feedStdin(t *testing.T, input string) {
	t.Helper()
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatal(err)
	}
	w.Close()
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		r.Close()
	})
}

// emptyStdin is the "decline the confirm" case used by the run_command tests.
func emptyStdin(t *testing.T) {
	t.Helper()
	feedStdin(t, "")
}

func TestRunCommandHelper_AcceptsPathInsideRoot(t *testing.T) {
	emptyStdin(t)

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "run_command",
			Arguments: map[string]any{"command": "ls", "path": "sub"},
		},
	}

	// a path inside root passes validation and reaches the confirm, which the
	// empty stdin declines
	result := runCommandHelper(call, root)

	if result.Content != "command canceled by user" {
		t.Errorf("expected the confirm to be reached, got %q", result.Content)
	}
}

func TestRunCommandHelper_DefaultsToRoot(t *testing.T) {
	root := t.TempDir()

	dir, err := resolveSafePath(root, "")
	if err != nil {
		t.Fatalf("empty path should resolve to root: %v", err)
	}
	if dir != root {
		t.Errorf("default dir = %q, want root %q", dir, root)
	}
}

func TestUpdatePlanItemHelper_UnknownIDIsAFailure(t *testing.T) {
	items := []plan.Item{
		{Id: 1.0, Description: "read main.go", Status: plan.StatusPending},
	}

	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name: "update_plan",
			Arguments: map[string]any{
				"id":     99.0,
				"status": "in_progress",
			},
		},
	}

	result := updatePlanItemHelper(call, &items)

	// the agent loop counts failures by this exact prefix (agent.go)
	if !strings.HasPrefix(result.Content, "error") {
		t.Errorf("unknown id must report as an error prefix, got %q", result.Content)
	}
	if len(items) != 1 || items[0].Status != plan.StatusPending {
		t.Errorf("plan should be untouched, got %+v", items)
	}
}

func TestResolveSafePath_RejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	if err := os.WriteFile(
		filepath.Join(outside, "secret.txt"),
		[]byte("secret"),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	err := os.Symlink(outside, filepath.Join(root, "escape"))
	if err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err = resolveSafePath(root, "escape/secret.txt")
	if err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}

func TestResolveSafePath_RejectsBrokenSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	// the target does not exist, so EvalSymlinks reports ErrNotExist. A fallback
	// to the parent would let edit_file create this file outside the root.
	err := os.Symlink(
		filepath.Join(outside, "missing"),
		filepath.Join(root, "escape"),
	)
	if err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := resolveSafePath(root, "escape/new.txt"); err == nil {
		t.Fatal("expected broken symlink escape to be rejected")
	}
}

func TestResolveSafePath_AllowsSymlinkInsideRoot(t *testing.T) {
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(
		filepath.Join(root, "real"),
		filepath.Join(root, "alias"),
	); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := resolveSafePath(root, "alias/file.txt")
	if err != nil {
		t.Fatalf("symlink inside root should be allowed: %v", err)
	}
	want := filepath.Join(root, "real", "file.txt")
	if got != want {
		t.Errorf("resolved path = %q, want %q", got, want)
	}
}

func TestReadFileHelper_OutsideRootNamesReason(t *testing.T) {
	root := t.TempDir()
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "read_file",
			Arguments: map[string]any{"path": "../secret.txt"},
		},
	}
	msg := readFileHelper(call, root)
	if !strings.Contains(msg.Content, "is outside the workspace root") {
		t.Errorf("expected message to name the reason, got %q", msg.Content)
	}
}

func TestEditFileHelper_OutsideRootNamesReason(t *testing.T) {
	root := t.TempDir()
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "edit_file",
			Arguments: map[string]any{"path": "../outside.go", "old_text": "foo", "new_text": "bar"},
		},
	}
	msg := editFileHelper(call, root)
	if !strings.Contains(msg.Content, "is outside the workspace root") {
		t.Errorf("expected message to name the reason, got %q", msg.Content)
	}
}

func TestReadFileHelper_SymlinkEscapeNamesReason(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	call := &ollama.ToolCall{
		Function: ollama.CalledFunction{
			Name:      "read_file",
			Arguments: map[string]any{"path": "escape/secret.txt"},
		},
	}
	msg := readFileHelper(call, root)
	if !strings.Contains(msg.Content, "is outside the workspace root") {
		t.Errorf("expected symlink escape to name the reason, got %q", msg.Content)
	}
}
