package expansion

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"flowk/internal/actions/core/variables"
	"flowk/internal/flow"
)

// Variable describes a runtime value consumed during expansion operations.
type Variable = variables.Variable

var (
	variablePattern    = regexp.MustCompile(`\$\{([^{}]+)\}`)
	rawVariablePattern = regexp.MustCompile(`^\$\{\s*([A-Za-z0-9_.-]+)\s*\}$`)
)

func ExpandTaskPayload(raw json.RawMessage, vars map[string]Variable, tasks []flow.Task) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decoding task payload for expansion: %w", err)
	}

	expanded, err := expandVarsWithTasks(decoded, vars, tasks)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(expanded)
	if err != nil {
		return nil, fmt.Errorf("encoding expanded task payload: %w", err)
	}

	return json.RawMessage(data), nil
}

// ExpandParallelTaskPayload performs variable interpolation on PARALLEL task payloads
// while preserving the nested task definitions so they can be expanded at runtime
// with the execution context specific to each branch. This prevents placeholders
// that reference iteration variables from failing during the initial expansion
// phase.
func ExpandParallelTaskPayload(raw json.RawMessage, vars map[string]Variable, tasks []flow.Task) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decoding parallel task payload for expansion: %w", err)
	}

	nestedTasks, hasTasks := payload["tasks"]
	if hasTasks {
		delete(payload, "tasks")
	}

	expandedAny, err := expandVarsWithTasks(payload, vars, tasks)
	if err != nil {
		return nil, err
	}

	expanded, ok := expandedAny.(map[string]any)
	if !ok {
		expanded = make(map[string]any)
	}

	if hasTasks {
		expanded["tasks"] = nestedTasks
	}

	data, err := json.Marshal(expanded)
	if err != nil {
		return nil, fmt.Errorf("encoding expanded parallel task payload: %w", err)
	}

	return json.RawMessage(data), nil
}

func ExpandEvaluateTaskPayload(raw json.RawMessage, vars map[string]Variable, tasks []flow.Task) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decoding evaluate task payload for expansion: %w", err)
	}

	conditions, hasConditions := payload["if_conditions"]
	if hasConditions {
		delete(payload, "if_conditions")
	}

	expandedAny, err := expandVarsWithTasks(payload, vars, tasks)
	if err != nil {
		return nil, err
	}

	expanded, ok := expandedAny.(map[string]any)
	if !ok {
		expanded = make(map[string]any)
	}

	if hasConditions {
		expanded["if_conditions"] = conditions
	}

	data, err := json.Marshal(expanded)
	if err != nil {
		return nil, fmt.Errorf("encoding expanded evaluate task payload: %w", err)
	}

	return json.RawMessage(data), nil
}

func expandVars(value any, vars map[string]Variable) (any, error) {
	return expandVarsWithStack(value, vars, nil, nil)
}

// ExpandValue walks the provided value performing variable interpolation using the supplied map.
func ExpandValue(value any, vars map[string]Variable) (any, error) {
	return expandVarsWithStack(value, vars, nil, nil)
}

func expandVarsWithTasks(value any, vars map[string]Variable, tasks []flow.Task) (any, error) {
	return expandVarsWithStack(value, vars, tasks, nil)
}

func expandVarsWithStack(value any, vars map[string]Variable, tasks []flow.Task, stack map[string]struct{}) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		expanded := make(map[string]any, len(v))
		for key, val := range v {
			expandedVal, err := expandVarsWithStack(val, vars, tasks, stack)
			if err != nil {
				return nil, err
			}
			expanded[key] = expandedVal
		}
		return expanded, nil
	case []any:
		expanded := make([]any, len(v))
		for i, item := range v {
			expandedVal, err := expandVarsWithStack(item, vars, tasks, stack)
			if err != nil {
				return nil, err
			}
			expanded[i] = expandedVal
		}
		return expanded, nil
	case string:
		return expandStringValueWithStack(v, vars, tasks, stack)
	default:
		return value, nil
	}
}

func ExpandStringValue(value string, vars map[string]Variable) (any, error) {
	return expandStringValueWithStack(value, vars, nil, nil)
}

func expandStringValueWithStack(value string, vars map[string]Variable, tasks []flow.Task, stack map[string]struct{}) (any, error) {
	if value == "" {
		return value, nil
	}

	if matches := rawVariablePattern.FindStringSubmatch(value); len(matches) == 2 {
		name := strings.TrimSpace(matches[1])
		variable, ok := vars[name]
		if !ok {
			return nil, fmt.Errorf("variable %q is not defined", name)
		}

		if stack == nil {
			stack = make(map[string]struct{})
		}
		if _, seen := stack[name]; seen {
			return nil, fmt.Errorf("variable %q: circular reference detected", name)
		}

		stack[name] = struct{}{}
		expanded, err := expandVarsWithStack(variable.Value, vars, tasks, stack)
		delete(stack, name)
		if err != nil {
			return nil, fmt.Errorf("variable %q: %w", name, err)
		}

		return expanded, nil
	}

	expanded, err := expandStringWithStack(value, vars, stack)
	if err != nil {
		return nil, err
	}
	if tasks == nil {
		return expanded, nil
	}
	resolved, err := variables.ResolveTaskPlaceholders(expanded, tasks)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

func ExpandString(value string, vars map[string]Variable) (string, error) {
	return expandStringWithStack(value, vars, nil)
}

func expandStringWithStack(value string, vars map[string]Variable, stack map[string]struct{}) (string, error) {
	if value == "" {
		return value, nil
	}

	var expandErr error
	replaced := variablePattern.ReplaceAllStringFunc(value, func(match string) string {
		if expandErr != nil {
			return ""
		}

		if strings.HasPrefix(match, "${") {
			name := strings.TrimSpace(match[2 : len(match)-1])
			if strings.HasPrefix(name, "from.task:") {
				return match
			}
			return replaceVariable(name, vars, &expandErr, stack)
		}

		return match
	})

	if expandErr != nil {
		return "", expandErr
	}
	return replaced, nil
}

func replaceVariable(name string, vars map[string]Variable, errRef *error, stack map[string]struct{}) string {
	variable, ok := vars[name]
	if !ok {
		*errRef = fmt.Errorf("variable %q is not defined", name)
		return ""
	}

	if stack == nil {
		stack = make(map[string]struct{})
	}
	if _, seen := stack[name]; seen {
		*errRef = fmt.Errorf("variable %q: circular reference detected", name)
		return ""
	}

	stack[name] = struct{}{}
	expanded, err := expandVarsWithStack(variable.Value, vars, nil, stack)
	delete(stack, name)
	if err != nil {
		*errRef = fmt.Errorf("variable %q: %w", name, err)
		return ""
	}

	value, err := stringifyVariable(expanded)
	if err != nil {
		*errRef = fmt.Errorf("variable %q: %w", name, err)
		return ""
	}
	return value
}

func stringifyVariable(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case fmt.Stringer:
		return v.String(), nil
	case []any, map[string]any:
		data, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("encoding value: %w", err)
		}
		return string(data), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}
