package bench

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// what does the task struct need a prompt
// it needs a comand if that is going to be tested, a timeout time wether or not it is supposed to use a tool
type Task struct {
	Prompt     string   `json:"prompt"`
	Name       string   `json:"name"`
	Dir        string   `json:"dir"`
	TestCmd    []string `json:"test_cmd"`
	TimeoutSec int      `json:"timeout_sec"`
	NoTools    bool     `json:"no_tools"`
}

// root will be bench/tasks must go through the multiple sup directorys /name/ which each have a task.json and a files/ directory
func LoadTasks(root string) ([]Task, error) {
	dirContents, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var taskSlice []Task

	for _, taskFolder := range dirContents {
		//all the contents should be folders of specific tasks ie bench/tasks/<name>/ task.json and files/
		if !taskFolder.IsDir() {
			continue
		}
		fullPath := filepath.Join(root, taskFolder.Name())

		data, err := os.ReadFile(filepath.Join(fullPath, "task.json"))
		if errors.Is(err, os.ErrNotExist) {
			continue //this is not a task folder
		}
		if err != nil {
			return nil, err
		}
		var task Task

		if err := json.Unmarshal(data, &task); err != nil {
			return nil, fmt.Errorf("task %s: %w", taskFolder.Name(), err)

		}
		//validation happens here as well

		if task.Prompt == "" {
			return nil, fmt.Errorf("task %s: prompt is required", taskFolder.Name())
		}

		if len(task.TestCmd) == 0 && !task.NoTools {
			return nil, fmt.Errorf("task %s: test_cmd is required unless no_tools is true", taskFolder.Name())
		}

		task.Name = taskFolder.Name()
		task.Dir = fullPath
		taskSlice = append(taskSlice, task)

	}
	return taskSlice, nil

}

// takes a task and returns the directory of the workspace a function to clean it up and an error
func NewWorkspace(t Task) (dir string, cleanup func(), err error) {
	//this should make a proper temp directory IDK WHY please explain
	tempDir, err := os.MkdirTemp("", "agent-workspace-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	cleanup = func() {
		os.RemoveAll(tempDir)
	}

	fileDir := filepath.Join(t.Dir, "files")

	err = filepath.WalkDir(fileDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(fileDir, path)
		if err != nil {
			return err
		}

		dst := filepath.Join(tempDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0755)
		}
		return copyFile(path, dst)
	})

	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to copy %s: %w", fileDir, err)
	}

	return tempDir, cleanup, nil
}

// a helper that copies a single file from src dir to destination dir
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}

	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}

	return out.Sync()
}
