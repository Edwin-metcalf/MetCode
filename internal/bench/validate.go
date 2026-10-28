package bench

import "fmt"

func ValidateTask(t Task) error {
	//if there is no tools there is nothing to test
	if t.NoTools {
		return nil
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
