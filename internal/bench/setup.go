package bench

import (
	"encoding/json"
	"io"
	"os"
)

// what does the task struct need a prompt
// it needs a comand if that is going to be tested, a timeout time wether or not it is supposed to use a tool
// needs some proper answer thought right?
type Task struct {
	Prompt     string   `json:"prompt"`
	Name       string   `json:"name"`
	Dir        string   `json:"dir"`
	TestCmd    []string `json:"testCmd"`
	TimeoutSec int      `json:"timeoutSec"`
	NoTools    bool     `json:"noTools"`
}

// parse a task that comes in as a json[]bytes into a task struct
func taskParser(taskJson []byte) (*Task, error) {
	var task Task

	err := json.Unmarshal(taskJson, task)
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// takes a source path and opens the folder copy the contents to a temporary directory to do the work in
func copyFiles(sourcePath string) error {
	incomingDir, err := os.Open(sourcePath)
	if err != nil {
		return err
	}

	defer incomingDir.Close()

	tempDir, err := os.Create("testDir/")
	if err != nil {
		return err
	}

	if _, err = io.Copy(tempDir, incomingDir); err != nil {
		return err
	}

	return tempDir.Sync()
}
