package bench

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

const defaultTimeoutSec = 0

type Grade struct {
	Passed   bool
	Tampered bool
	Output   string
	//add a timed out later maybe
}

// take a task and a work directory adn run the command in a directory
func GradeWorkspace(task Task, workspace string) (Grade, error) {
	var g Grade

	passed, output, err := runTestCmd(task, workspace)
	if err != nil {
		return Grade{}, err
	}

	g.Passed = passed
	g.Output = output

	return g, nil
}

func runTestCmd(t Task, workspace string) (passed bool, output string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, t.TestCmd[0], t.TestCmd[1:]...)

	err = cmd.Run()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Println("Command timed out!")
			return false, "Command Timed out", err
		}
		fmt.Printf("Command failed: %v\n", err)
		return false, "Command failed", err
	}

	return true, "output", nil
}
