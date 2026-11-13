package bench

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// promptFileRef matches a file name a prompt refers to. The narrow extension
// list stops ordinary words ("go", "tests") from counting as file references.
var promptFileRef = regexp.MustCompile(`[A-Za-z0-9_][A-Za-z0-9_./-]*\.(?:go|mod|sum|txt|md|json|ya?ml|toml|sh)\b`)

// checkPromptFiles catches a fixture whose prompt names a file the fixture does
// not ship. change-constant told the model to edit change-constant.go while the
// files were gravity.go and gravity_test.go, so the task could never pass.
func checkPromptFiles(t Task) error {
	filesDir := filepath.Join(t.Dir, "files")
	have := map[string]bool{}

	err := filepath.WalkDir(filesDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			have[d.Name()] = true
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		// no files/ folder: a no_tools-style task, nothing to compare against
		return nil
	}
	if err != nil {
		return fmt.Errorf("task %s: reading fixture files: %w", t.Name, err)
	}

	for _, ref := range promptFileRef.FindAllString(t.Prompt, -1) {
		if have[filepath.Base(ref)] {
			continue
		}
		names := make([]string, 0, len(have))
		for name := range have {
			names = append(names, name)
		}
		sort.Strings(names)
		return fmt.Errorf("task %s: prompt mentions %q but files/ does not contain it (have: %s)",
			t.Name, ref, strings.Join(names, ", "))
	}

	return nil
}

func ValidateTask(t Task) error {
	//if there is no tools there is nothing to test
	if t.NoTools {
		return nil
	}

	if err := checkPromptFiles(t); err != nil {
		return err
	}

	dir, cleanup, err := NewWorkspace(t)
	if err != nil {
		return fmt.Errorf("task %s: creating workspace: %w", t.Name, err)
	}
	defer cleanup()

	grade, err := GradeWorkspace(t, dir)
	if err != nil {
		return fmt.Errorf("error task %s: causing error %w", t.Name, err)
	}

	if grade.Passed {
		return fmt.Errorf("error task %s: already passed", t.Name)
	}

	if grade.Tampered {
		return fmt.Errorf("error grade tampered this should not be able to happem task: %s", t.Name)
	}

	return nil
}

func ValidateAll(tasks []Task) []error {
	var problems []error

	//only append the issue tests
	for _, task := range tasks {
		if err := ValidateTask(task); err != nil {
			problems = append(problems, err)
		}
	}

	return problems
}
