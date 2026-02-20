package print

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"flowk/internal/actions/core/evaluate"
	"flowk/internal/actions/core/variables"
	"flowk/internal/flow"
)

const (
	// ActionName identifies the Print action in the flow definition.
	ActionName = "PRINT"
)

// Logger matches the subset of the standard logger used by the executor.
type Logger interface {
	Printf(format string, v ...interface{})
}

// Payload describes the configuration accepted by the PRINT action.
type Payload struct {
	Entries []Entry `json:"entries"`
}

// Entry represents a single value to render.
type Entry struct {
	Message  string `json:"message,omitempty"`
	Variable string `json:"variable,omitempty"`
	TaskID   string `json:"taskId,omitempty"`
	Field    string `json:"field,omitempty"`
	Value    any    `json:"value,omitempty"`
}

var (
	variablePlaceholderPattern = regexp.MustCompile(`\$\{\s*([A-Za-z0-9_.-]+)\s*\}`)
)

// ResultEntry captures the structured information returned by Execute.
type ResultEntry struct {
	Message string `json:"message,omitempty"`
	Value   any    `json:"value,omitempty"`
}

// Validate ensures the payload is well formed.
func (p Payload) Validate() error {
	if len(p.Entries) == 0 {
		return fmt.Errorf("print task: entries is required")
	}

	for i := range p.Entries {
		if err := p.Entries[i].Validate(); err != nil {
			return fmt.Errorf("print task: entries[%d]: %w", i, err)
		}
	}
	return nil
}

// Validate ensures an entry is well formed.
func (e Entry) Validate() error {
	hasVariable := strings.TrimSpace(e.Variable) != ""
	hasTask := strings.TrimSpace(e.TaskID) != ""
	hasValue := e.Value != nil

	if !hasVariable && !hasTask && !hasValue && strings.TrimSpace(e.Message) == "" {
		return fmt.Errorf("message is required when no variable, task or value is referenced")
	}
	if hasVariable && hasTask {
		return fmt.Errorf("variable and taskId cannot be used together in the same entry")
	}
	if hasValue && (hasVariable || hasTask) {
		return fmt.Errorf("value cannot be combined with variable or taskId in the same entry")
	}
	if hasTask && strings.TrimSpace(e.TaskID) == "" {
		return fmt.Errorf("taskId is required")
	}
	if hasVariable && strings.TrimSpace(e.Variable) == "" {
		return fmt.Errorf("variable is required")
	}
	return nil
}

// Execute resolves every configured entry, logs the rendered values, and returns the
// collected output.
func Execute(payload Payload, vars map[string]variables.Variable, tasks []flow.Task, logger Logger) (any, flow.ResultType, error) {
	if err := payload.Validate(); err != nil {
		return nil, "", err
	}

	results := make([]ResultEntry, 0, len(payload.Entries))

	for idx := range payload.Entries {
		entry := payload.Entries[idx]

		value, hasValue, defaultMessage, err := resolveEntryValue(entry, vars, tasks)
		if err != nil {
			return nil, "", fmt.Errorf("entries[%d]: %w", idx, err)
		}

		message := strings.TrimSpace(entry.Message)
		if message == "" {
			message = defaultMessage
		}

		if message != "" {
			renderedMessage, err := renderMessageTemplate(message, vars, tasks)
			if err != nil {
				return nil, "", fmt.Errorf("entries[%d]: render message: %w", idx, err)
			}
			message = renderedMessage
		}

		rendered := message
		if hasValue {
			if strValue, ok := value.(string); ok {
				renderedValue, err := renderValueString(strValue, vars)
				if err != nil {
					return nil, "", fmt.Errorf("entries[%d]: render value: %w", idx, err)
				}
				value = renderedValue
			}

			formatted := formatValue(value)
			formatted, err = renderMessageTemplate(formatted, vars, tasks)
			if err != nil {
				return nil, "", fmt.Errorf("entries[%d]: render formatted value: %w", idx, err)
			}
			if message != "" {
				rendered = fmt.Sprintf("%s: %s", message, formatted)
			} else {
				rendered = formatted
			}
		}

		if logger != nil {
			if rendered != "" {
				logger.Printf("%s", rendered)
			}
		}

		results = append(results, ResultEntry{Message: message, Value: value})
	}

	return results, flow.ResultTypeJSON, nil
}

func resolveEntryValue(entry Entry, vars map[string]variables.Variable, tasks []flow.Task) (value any, hasValue bool, defaultMessage string, err error) {
	if trimmedVar := strings.TrimSpace(entry.Variable); trimmedVar != "" {
		if vars == nil {
			return nil, false, "", fmt.Errorf("variable %q not defined", trimmedVar)
		}

		variable, ok := vars[trimmedVar]
		if !ok {
			return nil, false, "", fmt.Errorf("variable %q not defined", trimmedVar)
		}

		hasValue = true
		defaultMessage = fmt.Sprintf("variable %s", trimmedVar)
		if variable.Secret {
			value = "****"
		} else {
			value = variable.Value
		}
		return
	}

	if entry.Value != nil {
		value = entry.Value
		if str, ok := entry.Value.(string); ok {
			resolved, err := renderValueTemplate(str, vars, tasks)
			if err != nil {
				return nil, false, "", err
			}
			value = resolved
		}

		hasValue = true
		defaultMessage = ""
		return value, hasValue, defaultMessage, nil
	}

	if trimmedTask := strings.TrimSpace(entry.TaskID); trimmedTask != "" {
		task := flow.FindTaskByID(tasks, trimmedTask)
		if task == nil {
			return nil, false, "", fmt.Errorf("referenced task %q not found", trimmedTask)
		}
		if task.Status != flow.TaskStatusCompleted {
			return nil, false, "", fmt.Errorf("referenced task %q not completed (status: %s)", trimmedTask, task.Status)
		}

		field := strings.TrimSpace(entry.Field)
		if field == "" {
			field = "result"
		}

		resolved, err := evaluate.ResolveFieldValue(task, field)
		if err != nil {
			return nil, false, "", fmt.Errorf("resolve field %q: %w", field, err)
		}

		hasValue = true
		value = resolved
		defaultMessage = fmt.Sprintf("task %s %s", trimmedTask, field)
		return value, hasValue, defaultMessage, nil
	}

	return nil, false, "", nil
}

func renderMessageTemplate(template string, vars map[string]variables.Variable, tasks []flow.Task) (string, error) {
	if strings.TrimSpace(template) == "" {
		return template, nil
	}

	resolved, err := variables.ResolveTaskPlaceholders(template, tasks)
	if err != nil {
		return "", err
	}

	var rendered string
	switch v := resolved.(type) {
	case string:
		rendered = v
	default:
		rendered = formatValue(v)
	}

	if !variablePlaceholderPattern.MatchString(rendered) {
		return rendered, nil
	}

	return replaceVariablePlaceholders(rendered, vars)
}

func renderValueTemplate(template string, vars map[string]variables.Variable, tasks []flow.Task) (any, error) {
	resolved, err := variables.ResolveTaskPlaceholders(template, tasks)
	if err != nil {
		return nil, err
	}

	str, ok := resolved.(string)
	if !ok {
		return resolved, nil
	}

	trimmed := strings.TrimSpace(str)
	matches := variablePlaceholderPattern.FindStringSubmatch(trimmed)
	if len(matches) == 2 && trimmed == matches[0] {
		name := strings.TrimSpace(matches[1])
		if vars == nil {
			return nil, fmt.Errorf("variable %q not defined", name)
		}

		variable, ok := vars[name]
		if !ok {
			return nil, fmt.Errorf("variable %q not defined", name)
		}
		if variable.Secret {
			return "****", nil
		}
		return variable.Value, nil
	}

	return replaceVariablePlaceholders(str, vars)
}

func renderValueString(value string, vars map[string]variables.Variable) (any, error) {
	if !variablePlaceholderPattern.MatchString(value) {
		return value, nil
	}
	return replaceVariablePlaceholders(value, vars)
}

func replaceVariablePlaceholders(value string, vars map[string]variables.Variable) (string, error) {
	if strings.TrimSpace(value) == "" {
		return value, nil
	}

	var renderErr error
	rendered := variablePlaceholderPattern.ReplaceAllStringFunc(value, func(match string) string {
		if renderErr != nil {
			return ""
		}

		submatches := variablePlaceholderPattern.FindStringSubmatch(match)
		if len(submatches) != 2 {
			return match
		}

		name := strings.TrimSpace(submatches[1])
		if vars == nil {
			renderErr = fmt.Errorf("variable %q not defined", name)
			return ""
		}

		variable, ok := vars[name]
		if !ok {
			renderErr = fmt.Errorf("variable %q not defined", name)
			return ""
		}

		if variable.Secret {
			return "****"
		}

		return formatValue(variable.Value)
	})

	if renderErr != nil {
		return "", renderErr
	}

	return rendered, nil
}

func formatValue(value any) string {
	if value == nil {
		return "null"
	}

	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case []byte:
		return string(v)
	}

	data, err := json.Marshal(value)
	if err == nil {
		return string(data)
	}

	return fmt.Sprintf("%v", value)
}
