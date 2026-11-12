package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A bad -n used to print and keep going: the run created a results file and
// only failed later with "nothing to run". It now stops with the flag error.
func TestRunRejectsRepeatCountBelowOne(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-task-dir")
	code := run([]string{"run", "-models", "m", "-n", "0", "-tasks", missing, "-host", "http://127.0.0.1:1"})
	if code != 2 {
		t.Fatalf("run with -n 0 returned %d, want 2 (flag error)", code)
	}
}

// A task directory that cannot be loaded used to print the error and continue
// with zero tasks, creating the results file first. It now stops before that.
func TestRunStopsWhenTasksCannotBeLoaded(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-task-dir")
	out := filepath.Join(t.TempDir(), "results.jsonl")
	code := run([]string{"run", "-models", "m", "-n", "1", "-tasks", missing, "-out", out, "-host", "http://127.0.0.1:1"})
	if code != 1 {
		t.Fatalf("run with an unreadable task dir returned %d, want 1 (load error)", code)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatalf("results file %s was created even though loading tasks failed", out)
	}
}
