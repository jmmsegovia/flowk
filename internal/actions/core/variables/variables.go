package variables

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"flowk/internal/actions/shared/placeholders"
	"flowk/internal/flow"
	jsonpathutil "flowk/internal/shared/jsonpathutil"
)

const (
	// ActionName identifies the Variables action in the flow definition.
	ActionName = "VARIABLES"

	scopeFlow = "flow"
)

var (
	supportedTypes = map[string]struct{}{
		"string": {},
		"number": {},
		"bool":   {},
		"array":  {},
		"object": {},
		"secret": {},
		"proxy":  {},
	}

	placeholderPattern         = regexp.MustCompile(`\{\{\s*from\.task:([^{}]+)\s*\}\}|\$\{\s*from\.task:([^{}]+)\s*\}`)
	variablePlaceholderPattern = regexp.MustCompile(`\$\{\s*([A-Za-z0-9_.-]+)\s*\}|\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)
)

// ResolveTaskPlaceholders evaluates placeholders referencing prior tasks within the provided string.
// When the full string is a single placeholder, the resolved value retains its original type. If the
// placeholder is embedded within a larger string, the rendered output is returned as a string.
func ResolveTaskPlaceholders(value string, tasks []flow.Task) (any, error) {
	return resolveTaskPlaceholders(value, tasks)
}

// Payload describes the expected configuration for a VARIABLES task.
type Payload struct {
	Scope     string           `json:"scope"`
	Overwrite bool             `json:"overwrite"`
	Vars      []VariableConfig `json:"vars"`
}

// VariableConfig contains the metadata necessary to create a runtime variable.
type VariableConfig struct {
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Value     any            `json:"value"`
	Operation *MathOperation `json:"operation"`
}

// MathOperation defines a math transformation for number variables.
type MathOperation struct {
	Operator string `json:"operator"`
	Variable string `json:"variable"`
}

// Variable represents a runtime value stored during the execution of a flow.
type Variable struct {
	Name   string
	Type   string
	Value  any
	Secret bool
}

// Validate ensures the payload is well defined.
func (p Payload) Validate() error {
	if trimmed := strings.TrimSpace(p.Scope); trimmed != "" && !strings.EqualFold(trimmed, scopeFlow) {
		return fmt.Errorf("variables task: unsupported scope %q", p.Scope)
	}
	if len(p.Vars) == 0 {
		return fmt.Errorf("variables task: vars is required")
	}

	names := make(map[string]struct{}, len(p.Vars))
	for i := range p.Vars {
		cfg := p.Vars[i]
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("variables task: vars[%d]: %w", i, err)
		}
		if _, exists := names[cfg.Name]; exists {
			return fmt.Errorf("variables task: vars[%d]: name %q is duplicated", i, cfg.Name)
		}
		names[cfg.Name] = struct{}{}
	}
	return nil
}

// Validate ensures the variable definition is correct.
func (v VariableConfig) Validate() error {
	trimmedName := strings.TrimSpace(v.Name)
	if trimmedName == "" {
		return fmt.Errorf("name is required")
	}
	for _, r := range trimmedName {
		if !(r == '_' || r == '-' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return fmt.Errorf("name %q contains invalid character %q", v.Name, r)
		}
	}
	normalizedType := strings.ToLower(strings.TrimSpace(v.Type))
	if _, ok := supportedTypes[normalizedType]; !ok {
		return fmt.Errorf("unsupported type %q", v.Type)
	}
	if v.Operation != nil {
		if normalizedType != "number" {
			return fmt.Errorf("operation requires number type")
		}
		if err := v.Operation.Validate(); err != nil {
			return fmt.Errorf("operation: %w", err)
		}
	}
	return nil
}

// Validate confirms the math operation is usable.
func (m MathOperation) Validate() error {
	operator := strings.ToLower(strings.TrimSpace(m.Operator))
	switch operator {
	case "add", "plus", "+", "subtract", "minus", "-", "multiply", "*", "times", "divide", "/":
	case "":
		return fmt.Errorf("operator is required")
	default:
		return fmt.Errorf("unsupported operator %q", m.Operator)
	}

	if strings.TrimSpace(m.Variable) == "" {
		return fmt.Errorf("variable is required")
	}
	return nil
}

// Execute evaluates the provided payload and stores the resulting variables in the supplied map.
func Execute(payload Payload, existing map[string]Variable, tasks []flow.Task) (map[string]any, flow.ResultType, error) {
	if err := payload.Validate(); err != nil {
		return nil, "", err
	}

	if payload.Scope != "" && !strings.EqualFold(payload.Scope, scopeFlow) {
		return nil, "", fmt.Errorf("variables task: unsupported scope %q", payload.Scope)
	}

	updates := make(map[string]Variable, len(payload.Vars))
	result := make(map[string]any, len(payload.Vars))

	for _, cfg := range payload.Vars {
		name := strings.TrimSpace(cfg.Name)
		varType := strings.ToLower(strings.TrimSpace(cfg.Type))

		if _, exists := existing[name]; exists && !payload.Overwrite {
			return nil, "", fmt.Errorf("variables task: variable %q already defined", name)
		}
		if _, exists := updates[name]; exists {
			return nil, "", fmt.Errorf("variables task: duplicate variable %q in payload", name)
		}

		coerced, err := resolveVariableValue(cfg, name, varType, tasks, existing, updates)
		if err != nil {
			return nil, "", err
		}

		variable := Variable{
			Name:   name,
			Type:   varType,
			Value:  coerced,
			Secret: varType == "secret",
		}

		updates[name] = variable
		result[name] = formatResultValue(variable)
	}

	for name, variable := range updates {
		existing[name] = variable
	}

	return result, flow.ResultTypeJSON, nil
}

func resolveValue(value any, tasks []flow.Task) (any, error) {
	str, ok := value.(string)
	if !ok {
		return value, nil
	}

	return resolveTaskPlaceholders(str, tasks)
}

func resolveTaskPlaceholders(str string, tasks []flow.Task) (any, error) {
	matches := placeholderPattern.FindAllStringSubmatch(str, -1)
	if len(matches) == 0 {
		return str, nil
	}

	resolved := str
	for _, match := range matches {
		full := match[0]
		expr := placeholders.SelectPlaceholderExpression(match)
		if expr == "" {
			continue
		}

		replacement, err := resolveFromTaskPlaceholder(expr, tasks)
		if err != nil {
			return nil, err
		}

		if full == str && len(matches) == 1 {
			return replacement, nil
		}

		var rendered string
		switch v := replacement.(type) {
		case string:
			rendered = v
		default:
			data, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("rendering placeholder %q: %w", expr, err)
			}
			rendered = string(data)
		}
		resolved = strings.ReplaceAll(resolved, full, rendered)
	}

	return resolved, nil
}

func resolveFromTaskPlaceholder(expr string, tasks []flow.Task) (any, error) {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return nil, fmt.Errorf("placeholder is empty")
	}

	dollarIdx := strings.Index(trimmed, "$")
	if dollarIdx <= 0 {
		return nil, fmt.Errorf("placeholder %q missing json path", expr)
	}

	taskID := strings.TrimSpace(trimmed[:dollarIdx])
	if strings.HasSuffix(taskID, ".result") {
		taskID = strings.TrimSuffix(taskID, ".result")
	}

	if taskID == "" {
		return nil, fmt.Errorf("placeholder %q missing task id", expr)
	}

	path := trimmed[dollarIdx:]
	if path == "" {
		return nil, fmt.Errorf("placeholder %q missing json path", expr)
	}

	task := flow.FindTaskByID(tasks, taskID)
	if task == nil {
		return nil, fmt.Errorf("referenced task %q not found", taskID)
	}
	if task.Status != flow.TaskStatusCompleted {
		return nil, fmt.Errorf("referenced task %q not completed", taskID)
	}

	if task.ResultType != flow.ResultTypeJSON {
		return nil, fmt.Errorf("referenced task %q does not contain json result", taskID)
	}

	normalized := normalizeJSONPath(path)
	container := jsonpathutil.NormalizeContainer(task.Result)

	value, err := jsonpathutil.Evaluate(normalized, container)
	if err != nil {
		return nil, fmt.Errorf("evaluating json path %q: %w", normalized, err)
	}
	return value, nil
}

func normalizeJSONPath(path string) string {
	expr := strings.TrimSpace(path)
	if expr == "" {
		return expr
	}
	expr = strings.ReplaceAll(expr, "'", `"`)
	if !strings.HasPrefix(expr, "$") {
		if strings.HasPrefix(expr, ".") || strings.HasPrefix(expr, "[") {
			expr = "$" + expr
		} else {
			expr = "$." + expr
		}
	} else if len(expr) > 1 {
		next := expr[1]
		if next != '.' && next != '[' {
			expr = "$." + expr[1:]
		}
	}
	return expr
}

func coerceValue(varType string, value any) (any, error) {
	switch varType {
	case "string":
		return toString(value)
	case "secret":
		val, err := toString(value)
		if err != nil {
			return nil, err
		}
		return val, nil
	case "number":
		return toNumber(value)
	case "bool":
		return toBool(value)
	case "array":
		if arr, ok := toArray(value); ok {
			return arr, nil
		}
		return nil, fmt.Errorf("expected array value, got %T", value)
	case "object":
		if obj, ok := toObject(value); ok {
			return obj, nil
		}
		return nil, fmt.Errorf("expected object value, got %T", value)
	case "proxy":
		proxies, err := toProxy(value)
		if err != nil {
			return nil, err
		}
		return proxies, nil
	default:
		return nil, fmt.Errorf("unsupported type %q", varType)
	}
}

func resolveVariableValue(cfg VariableConfig, name, varType string, tasks []flow.Task, existing, updates map[string]Variable) (any, error) {
	if cfg.Operation != nil {
		value, err := applyMathOperation(*cfg.Operation, name, existing, updates)
		if err != nil {
			return nil, fmt.Errorf("variable %q: %w", name, err)
		}
		return value, nil
	}

	value, err := resolveValue(cfg.Value, tasks)
	if err != nil {
		return nil, fmt.Errorf("variable %q: %w", name, err)
	}

	value, err = resolveVariablePlaceholders(value, existing, updates)
	if err != nil {
		return nil, fmt.Errorf("variable %q: %w", name, err)
	}

	if str, ok := value.(string); ok && placeholderPattern.MatchString(str) {
		value, err = resolveTaskPlaceholders(str, tasks)
		if err != nil {
			return nil, fmt.Errorf("variable %q: %w", name, err)
		}
	}

	coerced, err := coerceValue(varType, value)
	if err != nil {
		return nil, fmt.Errorf("variable %q: %w", name, err)
	}
	return coerced, nil
}

func resolveVariablePlaceholders(value any, existing, updates map[string]Variable) (any, error) {
	str, ok := value.(string)
	if !ok {
		return value, nil
	}

	matches := variablePlaceholderPattern.FindAllStringSubmatch(str, -1)
	if len(matches) == 0 {
		return value, nil
	}

	resolved := str
	for _, match := range matches {
		full := match[0]
		name := ""
		for i := 1; i < len(match); i++ {
			if trimmed := strings.TrimSpace(match[i]); trimmed != "" {
				name = trimmed
				break
			}
		}
		if name == "" {
			continue
		}

		variable, ok := lookupVariable(name, updates, existing)
		if !ok {
			return nil, fmt.Errorf("variable %q not defined", name)
		}

		if full == str && len(matches) == 1 {
			return variable.Value, nil
		}

		replacement, err := toString(variable.Value)
		if err != nil {
			return nil, fmt.Errorf("variable %q: %w", name, err)
		}

		resolved = strings.ReplaceAll(resolved, full, replacement)
	}

	return resolved, nil
}

func lookupVariable(name string, primary, secondary map[string]Variable) (Variable, bool) {
	if primary != nil {
		if v, ok := primary[name]; ok {
			return v, true
		}
	}
	if secondary != nil {
		if v, ok := secondary[name]; ok {
			return v, true
		}
	}
	return Variable{}, false
}

func applyMathOperation(op MathOperation, target string, existing, updates map[string]Variable) (float64, error) {
	baseVar, ok := updates[target]
	if !ok {
		baseVar, ok = existing[target]
	}
	if !ok {
		return 0, fmt.Errorf("operation requires existing variable %q", target)
	}
	if !strings.EqualFold(baseVar.Type, "number") {
		return 0, fmt.Errorf("operation requires number variable, got %q", baseVar.Type)
	}

	base, err := toNumber(baseVar.Value)
	if err != nil {
		return 0, fmt.Errorf("reading base value: %w", err)
	}

	operandName := strings.TrimSpace(op.Variable)
	operandVar, ok := updates[operandName]
	if !ok {
		operandVar, ok = existing[operandName]
	}
	if !ok {
		return 0, fmt.Errorf("referenced variable %q not found", operandName)
	}
	if !strings.EqualFold(operandVar.Type, "number") {
		return 0, fmt.Errorf("referenced variable %q must be number", operandName)
	}

	operand, err := toNumber(operandVar.Value)
	if err != nil {
		return 0, fmt.Errorf("reading operand value: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(op.Operator)) {
	case "add", "plus", "+":
		return base + operand, nil
	case "subtract", "minus", "-":
		return base - operand, nil
	case "multiply", "times", "*":
		return base * operand, nil
	case "divide", "/":
		if operand == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return base / operand, nil
	default:
		return 0, fmt.Errorf("unsupported operator %q", op.Operator)
	}
}

func toString(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	case float64, float32, int, int32, int64, int16, int8, uint, uint32, uint64, uint16, uint8:
		return fmt.Sprintf("%v", v), nil
	case bool:
		return fmt.Sprintf("%t", v), nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("unable to stringify %T", value)
		}
		return string(data), nil
	}
}

func toNumber(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, fmt.Errorf("parsing number: %w", err)
		}
		return f, nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, fmt.Errorf("unable to parse empty string as number")
		}
		num := json.Number(trimmed)
		f, err := num.Float64()
		if err != nil {
			return 0, fmt.Errorf("parsing number: %w", err)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("expected numeric value, got %T", value)
	}
}

func toBool(value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(v))
		switch trimmed {
		case "true", "1":
			return true, nil
		case "false", "0":
			return false, nil
		}
		return false, fmt.Errorf("cannot parse %q as bool", v)
	default:
		return false, fmt.Errorf("expected boolean value, got %T", value)
	}
}

func toArray(value any) ([]any, bool) {
	switch v := value.(type) {
	case []any:
		return v, true
	case []string:
		out := make([]any, len(v))
		for i := range v {
			out[i] = v[i]
		}
		return out, true
	default:
		return nil, false
	}
}

func toObject(value any) (map[string]any, bool) {
	switch v := value.(type) {
	case map[string]any:
		return v, true
	default:
		return nil, false
	}
}

func toProxy(value any) (map[string]string, error) {
	switch v := value.(type) {
	case map[string]string:
		copied := make(map[string]string, len(v))
		for key, val := range v {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				return nil, fmt.Errorf("proxy map contains empty key")
			}
			copied[trimmedKey] = val
		}
		return copied, nil
	case map[string]any:
		proxies := make(map[string]string, len(v))
		for rawKey, rawValue := range v {
			trimmedKey := strings.TrimSpace(rawKey)
			if trimmedKey == "" {
				return nil, fmt.Errorf("proxy map contains empty key")
			}
			strValue, err := toString(rawValue)
			if err != nil {
				return nil, fmt.Errorf("proxy %q: %w", rawKey, err)
			}
			proxies[trimmedKey] = strValue
		}
		if len(proxies) == 0 {
			return nil, fmt.Errorf("proxy object must declare at least one entry")
		}
		return proxies, nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil, fmt.Errorf("proxy string value cannot be empty")
		}
		return map[string]string{"http": trimmed}, nil
	case json.Number:
		return toProxy(string(v))
	case nil:
		return nil, fmt.Errorf("proxy value cannot be null")
	default:
		return nil, fmt.Errorf("expected proxy value to be object or string, got %T", value)
	}
}

func formatResultValue(variable Variable) any {
	if variable.Secret {
		return "****"
	}
	return variable.Value
}
