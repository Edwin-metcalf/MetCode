package bench

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// quietTask is a no_tools task: Run does not grade it, so these tests stay fast.
func quietTask(t *testing.T, name string) Task {
	t.Helper()
	return loadTask(t, name, `{"prompt": "Say hi.", "no_tools": true}`,
		map[string]string{"notes.txt": "scratch"})
}

// decodeRows reads every JSON line in buf back into a Result.
func decodeRows(t *testing.T, buf *bytes.Buffer) []Result {
	t.Helper()
	var rows []Result
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	for {
		var r Result
		err := dec.Decode(&r)
		if errors.Is(err, io.EOF) {
			return rows
		}
		if err != nil {
			t.Fatalf("row %d is not a valid Result: %v", len(rows), err)
		}
		rows = append(rows, r)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestRunMatrix_ShapeAndOrder(t *testing.T) {
	srv := newFakeOllama(t, textReply("hi"))
	tasks := []Task{quietTask(t, "a"), quietTask(t, "b")}
	mc := MatrixConfig{Models: []string{"m1", "m2"}, N: 2, Host: srv.URL, GitCommit: "abc123"}

	var buf bytes.Buffer
	if err := RunMatrix(tasks, mc, &buf, nil); err != nil { // nil progress must not panic
		t.Fatal(err)
	}

	if lines := strings.Count(buf.String(), "\n"); lines != 8 {
		t.Errorf("want 8 newline-terminated rows, got %d", lines)
	}

	type key struct {
		model, task string
		run         int
	}
	var want []key
	for _, m := range mc.Models { // models outermost, so each model loads once
		for _, tk := range tasks {
			for i := 0; i < mc.N; i++ {
				want = append(want, key{m, tk.Name, i})
			}
		}
	}

	rows := decodeRows(t, &buf)
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d", len(rows), len(want))
	}
	for i, r := range rows {
		if got := (key{r.Model, r.Task, r.RunIndex}); got != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got, want[i])
		}
		if r.GitCommit != "abc123" {
			t.Errorf("row %d GitCommit = %q, want abc123", i, r.GitCommit)
		}
	}
}

func TestRunMatrix_ProgressCallback(t *testing.T) {
	srv := newFakeOllama(t, textReply("hi"))
	tasks := []Task{quietTask(t, "a")}
	mc := MatrixConfig{Models: []string{"m1", "m2"}, N: 2, Host: srv.URL}

	var dones, totals []int
	err := RunMatrix(tasks, mc, io.Discard, func(done, total int, r Result) {
		dones = append(dones, done)
		totals = append(totals, total)
		if r.Task != "a" {
			t.Errorf("progress got a result for task %q", r.Task)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(dones) != 4 {
		t.Fatalf("progress called %d times, want 4", len(dones))
	}
	for i := range dones {
		if dones[i] != i+1 || totals[i] != 4 {
			t.Errorf("call %d: done=%d total=%d, want %d and 4", i, dones[i], totals[i], i+1)
		}
	}
}

func TestRunMatrix_NothingToRunIsAnError(t *testing.T) {
	tasks := []Task{quietTask(t, "a")}
	cases := map[string]struct {
		tasks []Task
		mc    MatrixConfig
	}{
		"no models": {tasks, MatrixConfig{N: 1}},
		"n is zero": {tasks, MatrixConfig{Models: []string{"m"}, N: 0}},
		"no tasks":  {nil, MatrixConfig{Models: []string{"m"}, N: 1}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := RunMatrix(tc.tasks, tc.mc, &buf, nil); err == nil {
				t.Error("a matrix with zero runs must be an error, not a silent success")
			}
			if buf.Len() != 0 {
				t.Errorf("wrote %d bytes for a matrix that cannot run", buf.Len())
			}
		})
	}
}

func TestRunMatrix_HarnessErrorStopsButKeepsEarlierRows(t *testing.T) {
	srv := newFakeOllama(t, textReply("hi"))
	good := quietTask(t, "good")
	// no files/ folder, so NewWorkspace fails: the harness broke, the model did not
	broken := Task{Name: "broken", Dir: t.TempDir(), Prompt: "hi", NoTools: true}
	mc := MatrixConfig{Models: []string{"m1"}, N: 1, Host: srv.URL}

	var buf bytes.Buffer
	err := RunMatrix([]Task{good, broken}, mc, &buf, nil)
	if err == nil {
		t.Fatal("a harness failure must stop the matrix")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error should name the failing task, got %q", err)
	}
	rows := decodeRows(t, &buf)
	if len(rows) != 1 || rows[0].Task != "good" {
		t.Errorf("the finished run should already be on disk, got %d rows: %+v", len(rows), rows)
	}
}

func TestRunMatrix_DeadServerAbortsAfterOneRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	tasks := []Task{quietTask(t, "a"), quietTask(t, "b")}
	mc := MatrixConfig{Models: []string{"m1", "m2"}, N: 3, Host: srv.URL}

	var buf bytes.Buffer
	err := RunMatrix(tasks, mc, &buf, nil)
	if err == nil {
		t.Fatal("a server that never answers should abort the matrix")
	}
	if !strings.Contains(err.Error(), "m1") {
		t.Errorf("error should name the model, got %q", err)
	}
	rows := decodeRows(t, &buf)
	if len(rows) != 1 {
		t.Fatalf("want exactly 1 row (the evidence), got %d", len(rows))
	}
	if rows[0].Ended != "model_error" || rows[0].PromptTokens != 0 {
		t.Errorf("want model_error with 0 prompt tokens, got %q / %d", rows[0].Ended, rows[0].PromptTokens)
	}
}

// A failure AFTER the first call completed is an ordinary row: the abort check
// only fires when no model call got through at all.
func TestRunMatrix_FailureAfterFirstCallDoesNotAbort(t *testing.T) {
	var mu sync.Mutex
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		first := n%2 == 1 // each run makes two requests: the first works, the second fails
		mu.Unlock()
		if !first {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, toolReply(toolRead, map[string]any{"path": "notes.txt"}))
	}))
	defer srv.Close()

	tasks := []Task{quietTask(t, "a")}
	mc := MatrixConfig{Models: []string{"m1"}, N: 2, Host: srv.URL}

	var buf bytes.Buffer
	if err := RunMatrix(tasks, mc, &buf, nil); err != nil {
		t.Fatalf("a mid-turn failure is a data point, not an abort: %v", err)
	}
	rows := decodeRows(t, &buf)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	for i, r := range rows {
		if r.Ended != "model_error" || r.PromptTokens == 0 {
			t.Errorf("row %d: want model_error with tokens counted, got %q / %d", i, r.Ended, r.PromptTokens)
		}
	}
}

func TestRunMatrix_WriteFailureIsReturned(t *testing.T) {
	srv := newFakeOllama(t, textReply("hi"))
	tasks := []Task{quietTask(t, "a")}
	mc := MatrixConfig{Models: []string{"m1"}, N: 1, Host: srv.URL}

	err := RunMatrix(tasks, mc, failWriter{}, nil)
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("a failed write must be returned, got %v", err)
	}
}
