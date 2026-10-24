package bench

import (
	"fmt"
	"os/exec"
	"time"
)

// take a task and a work directory adn run the command in a directory
func runTest(task Task) error {
	commandList := task.TestCmd

	if len(commandList) == 0 {
		return fmt.Errorf("command list cannot be empty")
	}

	cmd := exec.Command(commandList[0], commandList[1:]...)

	_, err := cmd.Output()
	if err != nil {
		return err
	}
	timer := time.NewTimer(time.Duration(task.TimeoutSec) * time.Second)
	//something with the output and check if it si right or record or what not.

	<-timer.C
	return fmt.Errorf("Time out error")

}
