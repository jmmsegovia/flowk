package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"flowk/internal/actions/core/forloop"
	"flowk/internal/actions/core/parallel"
	"flowk/internal/flow"
)

type subtaskMatch struct {
	task   flow.Task
	parent *flow.Task
	root   *flow.Task
	path   string
}

func findSubtaskForRun(tasks []flow.Task, subtaskID string) (*subtaskMatch, error) {
	trimmed := strings.TrimSpace(subtaskID)
	if trimmed == "" {
		return nil, fmt.Errorf("run-subtask: subtask id is required")
	}

	matches := make([]subtaskMatch, 0)
	for i := range tasks {
		parent := &tasks[i]
		found, err := findSubtasksInTask(parent, trimmed, parent.ID, parent)
		if err != nil {
			return nil, err
		}
		matches = append(matches, found...)
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("run-subtask: subtask id %q not found in flow definition", trimmed)
	}
	if len(matches) > 1 {
		paths := make([]string, 0, len(matches))
		for _, match := range matches {
			paths = append(paths, match.path)
		}
		return nil, fmt.Errorf("run-subtask: subtask id %q is ambiguous (matches: %s)", trimmed, strings.Join(paths, ", "))
	}

	match := matches[0]
	return &match, nil
}

func findSubtasksInTask(parent *flow.Task, targetID, path string, root *flow.Task) ([]subtaskMatch, error) {
	if parent == nil || !isCompositeAction(parent.Action) {
		return nil, nil
	}

	if root == nil {
		root = parent
	}

	children, err := extractSubtasks(parent)
	if err != nil {
		return nil, err
	}

	matches := make([]subtaskMatch, 0)
	for i := range children {
		child := children[i]
		if strings.TrimSpace(child.FlowID) == "" {
			child.FlowID = parent.FlowID
			children[i] = child
		}
		childPath := fmt.Sprintf("%s > %s", path, child.ID)
		if strings.TrimSpace(child.ID) == targetID {
			matches = append(matches, subtaskMatch{task: child, parent: parent, root: root, path: childPath})
		}

		nested, err := findSubtasksInTask(&children[i], targetID, childPath, root)
		if err != nil {
			return nil, err
		}
		matches = append(matches, nested...)
	}

	return matches, nil
}

func isCompositeAction(action string) bool {
	trimmed := strings.TrimSpace(action)
	return strings.EqualFold(trimmed, parallel.ActionName) || strings.EqualFold(trimmed, forloop.ActionName)
}

func extractSubtasks(parent *flow.Task) ([]flow.Task, error) {
	if parent == nil || !isCompositeAction(parent.Action) {
		return nil, nil
	}

	var payload struct {
		Tasks  []flow.Task `json:"tasks"`
		Fields struct {
			Tasks []flow.Task `json:"tasks"`
		} `json:"fields"`
	}

	if err := json.Unmarshal(parent.Payload, &payload); err != nil {
		return nil, fmt.Errorf("%s action: decoding payload: %w", strings.ToLower(parent.Action), err)
	}

	if len(payload.Tasks) > 0 {
		return payload.Tasks, nil
	}
	if len(payload.Fields.Tasks) > 0 {
		return payload.Fields.Tasks, nil
	}

	return nil, fmt.Errorf("%s action: tasks is required", strings.ToLower(parent.Action))
}
