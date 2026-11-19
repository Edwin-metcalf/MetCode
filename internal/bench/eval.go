package bench

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Edwin-metcalf/MetCode/internal/verify"
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

// an alternative to grade workspace that grades a no tools request checking if there were unesecary tool calls
func GradeNoTools(task Task, rounds int, reply string) Grade {
	g := Grade{Passed: rounds == 0 && strings.TrimSpace(reply) != ""}
	if rounds > 0 {
		g.Output = fmt.Sprintf("made %d tool round(s) on a task that needed none", rounds)
	}
	return g
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

// verifyRunner builds the runner for both grading and in-turn verification.
// They must use the same command and timeout so feedback matches the final grade.
func verifyRunner(t Task) verify.Runner {
	timeout := defaultTimeoutSec
	if t.TimeoutSec > 0 {
		timeout = t.TimeoutSec
	}
	return verify.Runner{
		Command: t.TestCmd,
		Timeout: time.Duration(timeout) * time.Second,
	}
}

// runTestCmd runs the task's test command in the workspace.
func runTestCmd(t Task, workspace string) (passed bool, output string, err error) {
	res := verifyRunner(t).Run(workspace)
	return res.Passed, res.Output, res.Err
}
