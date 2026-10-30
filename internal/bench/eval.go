package bench

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const defaultTimeoutSec = 100

type Grade struct {
	Passed   bool
	Tampered bool
	Output   string
	//add a timed out later maybe
}

// take a task and a work directory adn run the command in a directory
func GradeWorkspace(task Task, workspace string) (Grade, error) {
	var g Grade

	if task.TestCmd == nil || task.NoTools {
		return Grade{}, fmt.Errorf("task %s has no test command to run", task.Name)
	}

	tampered, err := testFilesTampered(task, workspace)
	if err != nil {
		return Grade{}, err
	}

	//if we get tampered then just return no need to run tests
	if tampered {
		g.Tampered = true
		return g, nil
	}

	passed, output, err := runTestCmd(task, workspace)
	if err != nil {
		return Grade{}, err
	}

	g.Passed = passed
	g.Output = output

	return g, nil
}

// helper that reports wether any test file the fixture ships with is mising or changed
// only looks at top levels of files/ for nested would need to switch to a filepath.WalkDir
func testFilesTampered(t Task, workspace string) (bool, error) {
	filesPath := filepath.Join(t.Dir, "files")

	matches, err := filepath.Glob(filepath.Join(filesPath, "*_test.go"))
	if err != nil {
		return false, err
	}

	for _, match := range matches {
		original, err := os.ReadFile(match)
		if err != nil {
			return false, fmt.Errorf("reading fixture test file %s: %w", match, err)
		}

		workspaceFile := filepath.Join(workspace, filepath.Base(match))
		current, err := os.ReadFile(workspaceFile)

		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return true, nil
			}
			return false, fmt.Errorf("reading workspace test file %s: %w", workspaceFile, err)
		}

		if !bytes.Equal(original, current) {
			return true, nil
		}

	}

	return false, nil
}

// a helper that runs the test in the workspace
func runTestCmd(t Task, workspace string) (passed bool, output string, err error) {
	timeout := defaultTimeoutSec
	if t.TimeoutSec > 0 {
		timeout = t.TimeoutSec
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, t.TestCmd[0], t.TestCmd[1:]...)
	cmd.Dir = workspace

	out, err := cmd.CombinedOutput()
	output = string(out)

	if err == nil {
		return true, output, nil
	}

	//timed out case
	if ctx.Err() == context.DeadlineExceeded {
		return false, output, nil
	}

	//test failed a verdict case
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, output, nil
	}

	// could not start harness
	return false, "", err
}
