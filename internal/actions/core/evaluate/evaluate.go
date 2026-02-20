package evaluate

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"flowk/internal/actions/shared/placeholders"
	"flowk/internal/flow"
	"flowk/internal/logging/colors"
	jsonpathutil "flowk/internal/shared/jsonpathutil"
)

const (
	// ActionName identifies the Evaluate action in the flow definition.
	ActionName = "EVALUATE"

	jsonResultFieldPrefix = "result$"
)

var (
	taskPlaceholderPattern     = regexp.MustCompile(`^(?:\{\{\s*from\.task:([^{}]+)\s*\}\}|\$\{\s*from\.task:([^{}]+)\s*\})$`)
	variablePlaceholderPattern = regexp.MustCompile(`^\$\{\s*([A-Za-z0-9_.-]+)\s*\}$`)
	legacyVariablePattern      = regexp.MustCompile(`^\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}$`)
)

// Condition represents a single validation that must be satisfied by the
// referenced task.
type Condition struct {
	Left      string `json:"left"`
	Operation string `json:"operation"`
	Right     any    `json:"right"`

	Field    string `json:"field"`
	Expected any    `json:"expected"`
}

func (c Condition) usingLeftAlias() bool {
	return strings.TrimSpace(c.Left) != ""
}

func (c Condition) leftOperand() string {
	if c.usingLeftAlias() {
		return c.Left
	}
	return c.Field
}

func (c Condition) rightOperand() any {
	if c.usingLeftAlias() {
		return c.Right
	}
	return c.Expected
}

type Logger interface {
	Printf(format string, v ...interface{})
}

type coloredLogger interface {
	PrintColored(string, string)
}

// Validate ensures the condition is well defined.
func (c Condition) Validate() error {
	if strings.TrimSpace(c.leftOperand()) == "" {
		return fmt.Errorf("field is required")
	}
	operation := strings.TrimSpace(c.Operation)
	if operation == "" {
		return fmt.Errorf("operation is required")
	}
	switch operation {
	case "=", "!=", ">", "<", ">=", "<=":
	case "STARTS_WITH", "ENDS_WITH", "CONTAINS", "NOT_CONTAINS", "MATCHES":
	case "IN", "NOT_IN":
	default:
		return fmt.Errorf("unsupported operation %q", c.Operation)
	}
	return nil
}

// Execute validates the provided conditions against the referenced task. The
// returned boolean indicates whether all conditions were satisfied.
func Execute(task *flow.Task, tasks []flow.Task, variables map[string]any, conditions []Condition, logger Logger) (bool, flow.ResultType, error) {
	for idx, condition := range conditions {
		if err := condition.Validate(); err != nil {
			return false, "", fmt.Errorf("validate condition %d: %w", idx, err)
		}

		leftValue, err := resolveOperandValue(task, tasks, variables, condition.leftOperand(), true)
		if err != nil {
			return false, "", fmt.Errorf("conditions[%d]: %w", idx, err)
		}

		if slice, ok := leftValue.([]any); ok && len(slice) == 0 {
			switch condition.rightOperand().(type) {
			case []any, map[string]any:
				// allow comparing empty collections
			default:
				// specific behavior for some operators
				op := strings.TrimSpace(condition.Operation)
				if op == "NOT_IN" || op == "!=" {
					// continue to evaluation
				} else {
					logger.Printf("conditions[%d]: field %q did not return any results", idx, strings.TrimSpace(condition.leftOperand()))
					return false, flow.ResultTypeBool, nil
				}
			}
		}

		rightValue, err := resolveOperandValue(task, tasks, variables, condition.rightOperand(), false)
		if err != nil {
			return false, "", fmt.Errorf("conditions[%d]: %w", idx, err)
		}

		operation := strings.TrimSpace(condition.Operation)

		var (
			matches    bool
			compareErr error
		)
		switch operation {
		case "=":
			matches, compareErr = evaluateEqual(leftValue, rightValue)
		case "!=":
			matches, compareErr = evaluateEqual(leftValue, rightValue)
			matches = !matches
		case ">", "<", ">=", "<=":
			matches, compareErr = evaluateComparison(leftValue, rightValue, operation)
		case "STARTS_WITH", "ENDS_WITH", "MATCHES":
			matches, compareErr = evaluateStringOp(leftValue, rightValue, operation)
		case "CONTAINS":
			matches, compareErr = evaluateContains(leftValue, rightValue)
		case "NOT_CONTAINS":
			matches, compareErr = evaluateContains(leftValue, rightValue)
			matches = !matches
		case "IN", "NOT_IN":
			matches, compareErr = evaluateCollectionOp(leftValue, rightValue, operation)
		default:
			return false, "", fmt.Errorf("conditions[%d]: unsupported operation %q", idx, operation)
		}
		if compareErr != nil {
			return false, "", fmt.Errorf("conditions[%d]: %w", idx, compareErr)
		}

		plain, colored := conditionLogMessages(idx, operation, rightValue, leftValue, matches)
		if coloredLogger, ok := logger.(coloredLogger); ok {
			coloredLogger.PrintColored(plain, colored)
		} else {
			logger.Printf("%s", plain)
		}

		if !matches {
			return false, flow.ResultTypeBool, nil
		}
	}

	return true, flow.ResultTypeBool, nil
}

func evaluateStringOp(actual, expected any, operation string) (bool, error) {
	actualStr, ok := actual.(string)
	if !ok {
		return false, fmt.Errorf("expected string value on left side, got %T", actual)
	}
	expectedStr, ok := expected.(string)
	if !ok {
		return false, fmt.Errorf("expected string value on right side, got %T", expected)
	}

	switch operation {
	case "STARTS_WITH":
		return strings.HasPrefix(actualStr, expectedStr), nil
	case "ENDS_WITH":
		return strings.HasSuffix(actualStr, expectedStr), nil
	case "MATCHES":
		matched, err := regexp.MatchString(expectedStr, actualStr)
		if err != nil {
			return false, fmt.Errorf("invalid regex %q: %w", expectedStr, err)
		}
		return matched, nil
	default:
		return false, fmt.Errorf("unsupported string operation %q", operation)
	}
}

func evaluateContains(left, right any) (bool, error) {
	// 1. String contains String
	if leftStr, ok := left.(string); ok {
		if rightStr, ok := right.(string); ok {
			return strings.Contains(leftStr, rightStr), nil
		}
		return false, fmt.Errorf("expected string on right side for string CONTAINS, got %T", right)
	}

	// 2. Collection contains Element
	// Treat left as the collection
	found, err := evaluateCollectionOp(right, left, "IN")
	if err != nil {
		// If evaluateCollectionOp fails due to type issues (e.g. left is not collection), return that error
		return false, fmt.Errorf("CONTAINS check failed: %w", err)
	}
	return found, nil
}

func evaluateCollectionOp(element, collection any, operation string) (bool, error) {
	// Normalize collection to []any
	var list []any
	switch c := collection.(type) {
	case []any:
		list = c
	default:
		// Attempt to convert other slice types using reflection if necessary,
		// but standard JSON unmarshal result is []any.
		rv := reflect.ValueOf(collection)
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			for i := 0; i < rv.Len(); i++ {
				list = append(list, rv.Index(i).Interface())
			}
		} else {
			return false, fmt.Errorf("expected collection on right side, got %T", collection)
		}
	}

	found := false
	for _, item := range list {
		eq, err := evaluateEqual(element, item)
		if err != nil {
			return false, err
		}
		if eq {
			found = true
			break
		}
	}

	if operation == "IN" {
		return found, nil
	}
	return !found, nil // NOT_IN
}

func conditionLogMessages(idx int, operation string, expected, actual any, matches bool) (string, string) {
	op := strings.TrimSpace(operation)
	status := "OK"
	statusColor := colors.Green
	if !matches {
		status = "KO"
		statusColor = colors.Red
	}

	formattedExpected := formatValue(expected)
	formattedActual := formatValue(actual)
	plain := fmt.Sprintf("Condition [%02d] - %s %s %s %s", idx+1, formattedExpected, op, formattedActual, status)
	colored := fmt.Sprintf("Condition [%02d] - %s %s %s %s%s%s", idx+1, formattedExpected, op, formattedActual, statusColor, status, colors.Reset)

	return plain, colored
}

func formatValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case string:
		return fmt.Sprintf("%q", v)
	case json.Number:
		return v.String()
	case fmt.Stringer:
		return v.String()
	case bool:
		return fmt.Sprintf("%t", v)
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return fmt.Sprintf("%v", v)
	}

	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return "null"
	}

	switch rv.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array, reflect.Struct:
		if data, err := json.Marshal(value); err == nil {
			return string(data)
		}
	}

	return fmt.Sprintf("%v", value)
}

func resolveFieldValue(task *flow.Task, field string) (any, error) {
	trimmed := strings.TrimSpace(field)
	switch {
	case strings.EqualFold(trimmed, "status"):
		return string(task.Status), nil
	case strings.EqualFold(trimmed, "success"):
		return task.Success, nil
	case strings.EqualFold(trimmed, "resulttype"):
		return string(task.ResultType), nil
	case strings.EqualFold(trimmed, "result"):
		return task.Result, nil
	case strings.HasPrefix(strings.ToLower(trimmed), jsonResultFieldPrefix):
		if task.ResultType != flow.ResultTypeJSON {
			return nil, fmt.Errorf("field %q requires json result type, got %s", field, task.ResultType)
		}

		path := strings.TrimSpace(trimmed[len(jsonResultFieldPrefix):])
		if path == "" {
			return nil, fmt.Errorf("field %q: json path is empty", field)
		}

		expr := normalizeJSONPathExpression(path)
		if !strings.HasPrefix(expr, "$") {
			if strings.HasPrefix(expr, ".") || strings.HasPrefix(expr, "[") {
				expr = "$" + expr
			} else {
				expr = "$." + expr
			}
		}

		normalized := jsonpathutil.NormalizeContainer(task.Result)

		result, err := jsonpathutil.Evaluate(expr, normalized)
		if err != nil {
			return nil, fmt.Errorf("evaluating json path %q: %w", expr, err)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported field %q", field)
	}
}

func resolveConditionFieldValue(target *flow.Task, tasks []flow.Task, field string) (any, error) {
	if strings.TrimSpace(field) == "" {
		return nil, fmt.Errorf("field is required")
	}

	return resolveStringOperand(target, tasks, nil, field, true)
}

func resolveOperandValue(target *flow.Task, tasks []flow.Task, variables map[string]any, operand any, allowFieldResolution bool) (any, error) {
	switch v := operand.(type) {
	case string:
		return resolveStringOperand(target, tasks, variables, v, allowFieldResolution)
	default:
		return operand, nil
	}
}

func resolveStringOperand(target *flow.Task, tasks []flow.Task, variables map[string]any, raw string, allowFieldResolution bool) (any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		if allowFieldResolution {
			return nil, fmt.Errorf("value is required")
		}
		return raw, nil
	}

	if matches := taskPlaceholderPattern.FindStringSubmatch(trimmed); len(matches) == 3 {
		expr := placeholders.SelectPlaceholderExpression(matches)
		if expr == "" {
			return nil, fmt.Errorf("placeholder is empty")
		}
		return resolveTaskPlaceholder(tasks, expr)
	}

	if matches := variablePlaceholderPattern.FindStringSubmatch(trimmed); len(matches) == 2 {
		return resolveVariablePlaceholder(matches[1], variables)
	}

	if matches := legacyVariablePattern.FindStringSubmatch(trimmed); len(matches) == 2 {
		return resolveVariablePlaceholder(matches[1], variables)
	}

	if allowFieldResolution {
		if target == nil {
			return nil, fmt.Errorf("unsupported field %q", raw)
		}
		return resolveFieldValue(target, trimmed)
	}

	return raw, nil
}

func resolveTaskPlaceholder(tasks []flow.Task, expr string) (any, error) {
	taskID, fieldName, err := parsePlaceholder(expr)
	if err != nil {
		return nil, err
	}

	task := flow.FindTaskByID(tasks, taskID)
	if task == nil {
		return nil, fmt.Errorf("referenced task %q not found", taskID)
	}
	if task.Status != flow.TaskStatusCompleted {
		return nil, fmt.Errorf("referenced task %q not completed", taskID)
	}

	return resolveFieldValue(task, fieldName)
}

func resolveVariablePlaceholder(name string, variables map[string]any) (any, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, fmt.Errorf("variable placeholder is empty")
	}
	if len(variables) == 0 {
		return nil, fmt.Errorf("variable %q is not defined", trimmed)
	}
	value, ok := variables[trimmed]
	if !ok {
		return nil, fmt.Errorf("variable %q is not defined", trimmed)
	}
	return value, nil
}

func parsePlaceholder(expr string) (string, string, error) {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return "", "", fmt.Errorf("placeholder is empty")
	}

	if dollarIdx := strings.Index(trimmed, "$"); dollarIdx > 0 {
		prefix := strings.TrimSpace(trimmed[:dollarIdx])
		if strings.HasSuffix(prefix, ".result") {
			prefix = strings.TrimSuffix(prefix, ".result")
		}
		if prefix == "" {
			return "", "", fmt.Errorf("placeholder %q missing task id", expr)
		}
		field := "result" + trimmed[dollarIdx:]
		return prefix, field, nil
	}

	dotIdx := strings.LastIndex(trimmed, ".")
	if dotIdx <= 0 {
		return "", "", fmt.Errorf("placeholder %q missing field reference", expr)
	}

	taskID := strings.TrimSpace(trimmed[:dotIdx])
	if taskID == "" {
		return "", "", fmt.Errorf("placeholder %q missing task id", expr)
	}

	field := strings.TrimSpace(trimmed[dotIdx+1:])
	if field == "" {
		return "", "", fmt.Errorf("placeholder %q missing field reference", expr)
	}

	return taskID, field, nil
}

// ResolveFieldValue exposes the field resolution logic so other actions can reuse
// the same semantics when inspecting task metadata or JSON results.
func ResolveFieldValue(task *flow.Task, field string) (any, error) {
	return resolveFieldValue(task, field)
}

func evaluateEqual(actual, expected any) (bool, error) {
	actual = unwrapSingleElement(actual)

	switch exp := expected.(type) {
	case nil:
		return actual == nil, nil
	case bool:
		value, ok := actual.(bool)
		if !ok {
			return false, fmt.Errorf("expected boolean value, got %T", actual)
		}
		return value == exp, nil
	case string:
		value, ok := actual.(string)
		if !ok {
			return false, fmt.Errorf("expected string value, got %T", actual)
		}
		return value == exp, nil
	case float64:
		actualNumber, ok := toFloat64(actual)
		if !ok {
			return false, fmt.Errorf("expected numeric value, got %T", actual)
		}
		return actualNumber == exp, nil
	default:
		if number, ok := expected.(json.Number); ok {
			parsed, err := number.Float64()
			if err != nil {
				return false, fmt.Errorf("parsing expected number: %w", err)
			}
			actualNumber, ok := toFloat64(actual)
			if !ok {
				return false, fmt.Errorf("expected numeric value, got %T", actual)
			}
			return actualNumber == parsed, nil
		}
		return reflect.DeepEqual(actual, expected), nil
	}
}

func evaluateComparison(actual, expected any, operation string) (bool, error) {
	actual = unwrapSingleElement(actual)

	actualNumber, ok := toFloat64(actual)
	if !ok {
		return false, fmt.Errorf("expected numeric value, got %T", actual)
	}

	expectedNumber, ok := toFloat64(expected)
	if !ok {
		if number, ok := expected.(json.Number); ok {
			parsed, err := number.Float64()
			if err != nil {
				return false, fmt.Errorf("parsing expected number: %w", err)
			}
			expectedNumber = parsed
		} else {
			return false, fmt.Errorf("expected numeric value, got %T", expected)
		}
	}

	switch operation {
	case ">":
		return actualNumber > expectedNumber, nil
	case "<":
		return actualNumber < expectedNumber, nil
	case ">=":
		return actualNumber >= expectedNumber, nil
	case "<=":
		return actualNumber <= expectedNumber, nil
	default:
		return false, fmt.Errorf("unsupported operation %q", operation)
	}
}

func toFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func unwrapSingleElement(value any) any {
	switch v := value.(type) {
	case []any:
		if len(v) == 1 {
			return unwrapSingleElement(v[0])
		}
	}
	return value
}

func normalizeJSONPathExpression(expr string) string {
	var builder strings.Builder
	builder.Grow(len(expr))

	for _, r := range expr {
		if r == '\'' {
			builder.WriteRune('"')
			continue
		}
		builder.WriteRune(r)
	}

	return builder.String()
}
