package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerPass(t *testing.T) {
	res := Runner{Command: []string{"go", "version"}}.Run(t.TempDir())
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !res.Passed {
		t.Errorf("want pass, got %+v", res)
	}
	if !strings.Contains(res.Output, "go version") {
		t.Errorf("Output = %q, want the command output", res.Output)
	}
}

func TestRunnerFail(t *testing.T) {
	res := Runner{Command: []string{"sh", "-c", "exit 3"}}.Run(t.TempDir())
	if res.Err != nil {
		t.Fatalf("a non-zero exit is a verdict, not an error: %v", res.Err)
	}
	if res.Passed {
		t.Error("want fail")
	}
}

func TestRunnerTimeout(t *testing.T) {
	res := Runner{
		Command: []string{"sh", "-c", "sleep 2"},
		Timeout: 20 * time.Millisecond,
	}.Run(t.TempDir())

	if res.Err != nil {
		t.Fatalf("a timeout is a verdict, not an error: %v", res.Err)
	}
	if res.Passed {
		t.Error("want fail")
	}
}

func TestRunnerStartError(t *testing.T) {
	res := Runner{Command: []string{"metcode-no-such-command"}}.Run(t.TempDir())
	if res.Err == nil {
		t.Fatal("want an error when the command cannot start")
	}
	if res.Passed {
		t.Error("want fail")
	}
}

func TestRunnerClipsOutput(t *testing.T) {
	res := Runner{
		Command:   []string{"sh", "-c", "printf abcdefghij"},
		MaxOutput: 4,
	}.Run(t.TempDir())

	if res.Output != "abcd" {
		t.Errorf("Output = %q, want abcd", res.Output)
	}
}

func TestRunnerUsesWorkdir(t *testing.T) {
	workdir := t.TempDir()
	res := Runner{Command: []string{"sh", "-c", "pwd -P"}}.Run(workdir)
	if res.Err != nil {
		t.Fatal(res.Err)
	}

	want, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(res.Output); got != want {
		t.Errorf("workdir = %q, want %q", got, want)
	}
}

func TestRunnerMissingCommand(t *testing.T) {
	res := Runner{}.Run(os.TempDir())
	if res.Err == nil {
		t.Fatal("want an error for an empty command")
	}
	if res.Passed {
		t.Error("want fail")
	}
}
