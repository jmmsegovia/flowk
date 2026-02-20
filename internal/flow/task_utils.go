package flow

import "strings"

// FindTaskByID searches the provided slice of tasks for the entry matching the given identifier.
// It trims leading and trailing spaces from the identifier before comparing task IDs.
func FindTaskByID(tasks []Task, id string) *Task {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return nil
	}

	for i := range tasks {
		if tasks[i].ID == trimmed {
			return &tasks[i]
		}
	}

	return nil
}
