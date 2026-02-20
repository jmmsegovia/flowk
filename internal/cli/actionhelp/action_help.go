package actionhelp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"flowk/internal/actions/registry"
)

var defaultFieldDescriptions = map[string]string{
	"id":             "Unique identifier for the task within the flow.",
	"description":    "Human-readable summary of what the task does.",
	"action":         "Specifies which action implementation should be executed.",
	"variable":       "Name of the loop variable that receives the current iteration value.",
	"initial":        "Starting numeric value assigned to the loop variable.",
	"condition":      "Configuration that decides whether the loop continues running.",
	"operator":       "Comparison operator used to evaluate the loop condition.",
	"value":          "Numeric value compared against the loop variable in the condition.",
	"step":           "Numeric increment or decrement applied after every iteration.",
	"tasks":          "List of tasks executed on each iteration of the loop.",
	"max_iterations": "Maximum number of times the loop is allowed to run before stopping.",
}

var defaultExampleValues = map[string]func(actionName string) any{
	"id": func(string) any {
		return "for-loop-task"
	},
	"description": func(string) any {
		return "Execute tasks for a fixed number of iterations"
	},
	"action": func(actionName string) any {
		return strings.ToUpper(strings.TrimSpace(actionName))
	},
	"variable": func(string) any {
		return "index"
	},
	"initial": func(string) any {
		return 0
	},
	"value": func(string) any {
		return 10
	},
	"step": func(string) any {
		return 1
	},
	"tasks": func(string) any {
		return exampleArray{exampleNestedTask()}
	},
	"max_iterations": func(string) any {
		return 100
	},
}

type LookupError struct {
	name string
}

func (e LookupError) Error() string {
	return fmt.Sprintf("unknown action %q", e.name)
}

type schemaDocument struct {
	Definitions map[string]schemaDefinition `json:"definitions"`
}

type schemaDefinition struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
	AllOf      []schemaConditional        `json:"allOf"`
}

type schemaConditional struct {
	If   *schemaDefinition `json:"if"`
	Then *schemaDefinition `json:"then"`
}

type actionSchemaSummary struct {
	ActionName        string
	Required          []fieldSummary
	Optional          []fieldSummary
	Properties        map[string]map[string]any
	ConditionalGroups []conditionalRequirementGroup
}

type fieldSummary struct {
	Name        string
	Description string
}

type conditionalRequirementGroup struct {
	Title            string
	Required         []fieldSummary
	Note             string
	ExampleOverrides map[string]any
}

type exampleField struct {
	Name  string
	Value any
}

type exampleObject []exampleField

type exampleArray []any

func fieldDescription(name, description string) string {
	trimmed := strings.TrimSpace(description)
	if trimmed != "" {
		return trimmed
	}

	if fallback, ok := defaultFieldDescriptions[strings.ToLower(name)]; ok {
		return fallback
	}

	return ""
}

func exampleNestedTask() exampleObject {
	return exampleObject{
		{Name: "id", Value: "nested-task-id"},
		{Name: "description", Value: "Describe the nested task to run"},
		{Name: "action", Value: "<ACTION_NAME>"},
	}
}

type schemaAccumulator struct {
	actionName    string
	requiredSet   map[string]struct{}
	requiredOrder []string
	properties    map[string]json.RawMessage
}

func newSchemaAccumulator(actionName string) *schemaAccumulator {
	return &schemaAccumulator{
		actionName:  strings.ToUpper(actionName),
		requiredSet: make(map[string]struct{}),
		properties:  make(map[string]json.RawMessage),
	}
}

func (a *schemaAccumulator) collect(def *schemaDefinition) error {
	if def == nil {
		return nil
	}

	for _, name := range def.Required {
		if _, exists := a.requiredSet[name]; !exists {
			a.requiredSet[name] = struct{}{}
			a.requiredOrder = append(a.requiredOrder, name)
		}
	}

	for name, raw := range def.Properties {
		if _, exists := a.properties[name]; !exists {
			a.properties[name] = raw
		}
	}

	for _, cond := range def.AllOf {
		if cond.Then == nil {
			continue
		}

		matches, err := cond.If.matchesAction(a.actionName)
		if err != nil {
			return err
		}
		if matches {
			if err := a.collect(cond.Then); err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *schemaDefinition) matchesAction(action string) (bool, error) {
	if d == nil {
		return true, nil
	}
	if len(d.Properties) == 0 {
		return true, nil
	}

	raw, hasAction := d.Properties["action"]
	if !hasAction {
		return true, nil
	}

	var descriptor struct {
		Const string   `json:"const"`
		Enum  []string `json:"enum"`
	}
	if err := json.Unmarshal(raw, &descriptor); err != nil {
		return false, err
	}

	if descriptor.Const != "" {
		return strings.EqualFold(descriptor.Const, action), nil
	}
	if len(descriptor.Enum) > 0 {
		for _, candidate := range descriptor.Enum {
			if strings.EqualFold(candidate, action) {
				return true, nil
			}
		}
		return false, nil
	}
	return true, nil
}

func Build(actionName string) (string, error) {
	summary, err := loadActionSchemaSummary(actionName)
	if err != nil {
		return "", err
	}

	return formatActionHelp(summary), nil
}

func loadActionSchemaSummary(actionName string) (actionSchemaSummary, error) {
	trimmed := strings.TrimSpace(actionName)
	if trimmed == "" {
		return actionSchemaSummary{}, errors.New("action name is required")
	}

	action, found := registry.Lookup(trimmed)
	if !found {
		return actionSchemaSummary{}, LookupError{name: trimmed}
	}

	provider, ok := action.(registry.SchemaProvider)
	if !ok {
		return actionSchemaSummary{}, fmt.Errorf("action %q does not expose a schema", trimmed)
	}

	fragment, err := provider.JSONSchema()
	if err != nil {
		return actionSchemaSummary{}, fmt.Errorf("retrieving schema: %w", err)
	}
	if len(fragment) == 0 {
		return actionSchemaSummary{}, fmt.Errorf("action %q returned an empty schema", trimmed)
	}

	summary, err := summarizeActionSchema(trimmed, fragment)
	if err != nil {
		return actionSchemaSummary{}, err
	}
	return summary, nil
}

func summarizeActionSchema(actionName string, fragment []byte) (actionSchemaSummary, error) {
	var doc schemaDocument
	if err := json.Unmarshal(fragment, &doc); err != nil {
		return actionSchemaSummary{}, fmt.Errorf("decoding schema: %w", err)
	}

	taskDef, ok := doc.Definitions["task"]
	if !ok {
		return actionSchemaSummary{}, errors.New("schema does not define a task section")
	}

	accumulator := newSchemaAccumulator(actionName)
	if err := accumulator.collect(&taskDef); err != nil {
		return actionSchemaSummary{}, err
	}

	descriptions := make(map[string]string, len(accumulator.properties))
	propertyDetails := make(map[string]map[string]any, len(accumulator.properties))
	for name, raw := range accumulator.properties {
		descriptions[name] = describeSchemaProperty(raw)
		propertyDetails[name] = decodeSchemaProperty(raw)
	}

	required := make([]fieldSummary, 0, len(accumulator.requiredOrder))
	for _, name := range accumulator.requiredOrder {
		required = append(required, fieldSummary{Name: name, Description: fieldDescription(name, descriptions[name])})
	}

	optionalNames := make([]string, 0, len(accumulator.properties))
	for name := range accumulator.properties {
		if _, isRequired := accumulator.requiredSet[name]; isRequired {
			continue
		}
		optionalNames = append(optionalNames, name)
	}
	sort.Strings(optionalNames)

	optional := make([]fieldSummary, 0, len(optionalNames))
	for _, name := range optionalNames {
		optional = append(optional, fieldSummary{Name: name, Description: fieldDescription(name, descriptions[name])})
	}

	conditional, err := buildConditionalRequirementGroups(actionName, &taskDef, propertyDetails)
	if err != nil {
		return actionSchemaSummary{}, err
	}

	return actionSchemaSummary{
		ActionName:        strings.ToUpper(actionName),
		Required:          required,
		Optional:          optional,
		Properties:        propertyDetails,
		ConditionalGroups: conditional,
	}, nil
}

func describeSchemaProperty(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var property map[string]any
	if err := json.Unmarshal(raw, &property); err != nil {
		return ""
	}

	return describeProperty(property)
}

func decodeSchemaProperty(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}

	var property map[string]any
	if err := json.Unmarshal(raw, &property); err != nil {
		return nil
	}

	return property
}

func describeProperty(property map[string]any) string {
	if property == nil {
		return ""
	}

	var parts []string

	if typeDescription := describeType(property); typeDescription != "" {
		parts = append(parts, typeDescription)
	}

	if constValue, ok := stringValue(property["const"]); ok {
		parts = append(parts, fmt.Sprintf("must be %q", constValue))
	}

	if enumValues := stringSlice(property["enum"]); len(enumValues) > 0 {
		quoted := make([]string, 0, len(enumValues))
		for _, val := range enumValues {
			quoted = append(quoted, fmt.Sprintf("%q", val))
		}
		parts = append(parts, fmt.Sprintf("allowed values: %s", strings.Join(quoted, ", ")))
	}

	if minimum, ok := numberValue(property["minimum"]); ok {
		parts = append(parts, fmt.Sprintf("minimum %s", formatNumber(minimum)))
	}
	if maximum, ok := numberValue(property["maximum"]); ok {
		parts = append(parts, fmt.Sprintf("maximum %s", formatNumber(maximum)))
	}
	if minLength, ok := numberValue(property["minLength"]); ok {
		parts = append(parts, fmt.Sprintf("minimum length %s", formatNumber(minLength)))
	}
	if maxLength, ok := numberValue(property["maxLength"]); ok {
		parts = append(parts, fmt.Sprintf("maximum length %s", formatNumber(maxLength)))
	}
	if minItems, ok := numberValue(property["minItems"]); ok {
		parts = append(parts, fmt.Sprintf("minimum items %s", formatNumber(minItems)))
	}
	if maxItems, ok := numberValue(property["maxItems"]); ok {
		parts = append(parts, fmt.Sprintf("maximum items %s", formatNumber(maxItems)))
	}

	if typeValue, ok := stringValue(property["type"]); ok && typeValue == "object" {
		if required := stringSlice(property["required"]); len(required) > 0 {
			parts = append(parts, fmt.Sprintf("requires fields: %s", strings.Join(required, ", ")))
		}
		if additional, ok := property["additionalProperties"].(bool); ok && !additional {
			parts = append(parts, "does not allow unspecified fields")
		}
	}

	if description, ok := stringValue(property["description"]); ok {
		trimmed := strings.TrimSpace(description)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}

	return strings.Join(parts, "; ")
}

func describeType(property map[string]any) string {
	if ref, ok := stringValue(property["$ref"]); ok && ref != "" {
		return fmt.Sprintf("reference %s", ref)
	}

	typeValue, hasType := stringValue(property["type"])
	if !hasType {
		return ""
	}

	switch typeValue {
	case "array":
		if itemsMap, ok := mapValue(property["items"]); ok {
			if itemDescription := describeType(itemsMap); itemDescription != "" {
				return fmt.Sprintf("array of %s", itemDescription)
			}
		}
		return "array"
	default:
		return typeValue
	}
}

func stringValue(value any) (string, bool) {
	str, ok := value.(string)
	return str, ok
}

func stringSlice(value any) []string {
	switch v := value.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	case []string:
		return append([]string(nil), v...)
	default:
		return nil
	}
}

func numberValue(value any) (float64, bool) {
	num, ok := value.(float64)
	return num, ok
}

func mapValue(value any) (map[string]any, bool) {
	m, ok := value.(map[string]any)
	return m, ok
}

func formatNumber(value float64) string {
	if math.Trunc(value) == value {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("%g", value)
}

func formatActionHelp(summary actionSchemaSummary) string {
	title := fmt.Sprintf("Action %s", summary.ActionName)
	underline := strings.Repeat("=", len(title))

	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(underline)
	b.WriteString("\n\n")

	if len(summary.ConditionalGroups) == 0 {
		b.WriteString("Required fields:\n")
		writeFieldSummaries(&b, summary.Required)
		b.WriteString("\n")

		b.WriteString("Optional fields:\n")
		writeFieldSummaries(&b, summary.Optional)

		if example := buildActionExample(summary); example != "" {
			b.WriteString("\n")
			b.WriteString("Example:\n")
			writeIndentedBlock(&b, example)
		}
		return b.String()
	}

	writeAllowedValues(&b, summary)
	b.WriteString("\n")

	b.WriteString("Required fields (depending on \"operation\"):\n\n")
	for idx, group := range summary.ConditionalGroups {
		fmt.Fprintf(&b, "%d) %s\n", idx+1, group.Title)
		b.WriteString("   Required:\n")
		if len(group.Required) == 0 {
			b.WriteString("     - None\n")
		} else {
			for _, field := range group.Required {
				if trimmed := strings.TrimSpace(field.Description); trimmed != "" {
					fmt.Fprintf(&b, "     - %s\n", field.Name)
				} else {
					fmt.Fprintf(&b, "     - %s\n", field.Name)
				}
			}
		}
		if note := strings.TrimSpace(group.Note); note != "" {
			fmt.Fprintf(&b, "   %s\n", note)
		}
		b.WriteString("\n")
	}

	b.WriteString("Optional fields:\n")
	writeFieldSummaries(&b, summary.Optional)

	b.WriteString("\nExamples:\n\n")
	for idx, group := range summary.ConditionalGroups {
		fmt.Fprintf(&b, "%d) %s\n", idx+1, group.Title)
		if example := buildConditionalExample(summary, group); example != "" {
			writeIndentedBlockWithIndent(&b, example, "   ")
		}
		if idx < len(summary.ConditionalGroups)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func writeFieldSummaries(b *strings.Builder, fields []fieldSummary) {
	if len(fields) == 0 {
		b.WriteString("  - None\n")
		return
	}

	for _, field := range fields {
		if trimmed := strings.TrimSpace(field.Description); trimmed != "" {
			fmt.Fprintf(b, "  - %s — %s\n", field.Name, trimmed)
		} else {
			fmt.Fprintf(b, "  - %s\n", field.Name)
		}
	}
}

func writeAllowedValues(b *strings.Builder, summary actionSchemaSummary) {
	allowed := extractAllowedValues(summary)
	if len(allowed) == 0 {
		return
	}

	b.WriteString("Allowed values:\n")
	for _, field := range allowed {
		fmt.Fprintf(b, "  - %s — %s\n", field.Name, strings.Join(field.Description, ", "))
	}
	b.WriteString("\n")
}

func writeIndentedBlock(b *strings.Builder, block string) {
	if block == "" {
		return
	}

	lines := strings.Split(block, "\n")
	for _, line := range lines {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteString("\n")
	}
}

func writeIndentedBlockWithIndent(b *strings.Builder, block, indent string) {
	if block == "" {
		return
	}

	lines := strings.Split(block, "\n")
	for _, line := range lines {
		b.WriteString(indent)
		b.WriteString(line)
		b.WriteString("\n")
	}
}

func buildActionExample(summary actionSchemaSummary) string {
	fields := make([]exampleField, 0, len(summary.Required)+len(summary.Optional))

	for _, field := range summary.Required {
		value := exampleValueForField(field.Name, summary.Properties[field.Name], summary.ActionName)
		fields = append(fields, exampleField{Name: field.Name, Value: value})
	}

	for _, field := range summary.Optional {
		value := exampleValueForField(field.Name, summary.Properties[field.Name], summary.ActionName)
		fields = append(fields, exampleField{Name: field.Name, Value: value})
	}

	if len(fields) == 0 {
		return ""
	}

	var b strings.Builder
	writeExampleObject(&b, exampleObject(fields), 0)
	return b.String()
}

func exampleValueForField(name string, property map[string]any, actionName string) any {
	if value, ok := exampleValueFromSchema(name, property, actionName); ok {
		return value
	}

	if value, ok := exampleValueFromDefaults(name, actionName); ok {
		return value
	}

	switch strings.ToLower(propertyType(property)) {
	case "integer", "number":
		return 0
	case "boolean":
		return true
	case "array":
		return exampleArray{}
	case "object":
		return exampleObject{}
	default:
		return fmt.Sprintf("<%s>", sanitizePlaceholderName(name))
	}
}

func exampleValueFromSchema(name string, property map[string]any, actionName string) (any, bool) {
	if property == nil {
		return nil, false
	}

	if ref, ok := stringValue(property["$ref"]); ok {
		if ref == "#/definitions/task" {
			return exampleNestedTask(), true
		}
		return map[string]string{"$ref": ref}, true
	}

	if value, ok := property["const"]; ok {
		return value, true
	}

	if enumValues := stringSlice(property["enum"]); len(enumValues) > 0 {
		return enumValues[0], true
	}

	switch propertyType(property) {
	case "boolean":
		return true, true
	case "array":
		if items, ok := mapValue(property["items"]); ok {
			itemValue := exampleValueForField(name, items, actionName)
			switch v := itemValue.(type) {
			case exampleArray:
				if len(v) > 0 {
					return v, true
				}
				return exampleArray{exampleObject{}}, true
			default:
				return exampleArray{itemValue}, true
			}
		}
		return exampleArray{}, true
	case "object":
		props, _ := mapValue(property["properties"])
		if len(props) == 0 {
			return exampleObject{}, true
		}

		requiredNames := stringSlice(property["required"])
		requiredSet := make(map[string]struct{}, len(requiredNames))
		for _, r := range requiredNames {
			requiredSet[r] = struct{}{}
		}

		ordered := append([]string(nil), requiredNames...)
		optional := make([]string, 0)
		for key := range props {
			if _, ok := requiredSet[key]; ok {
				continue
			}
			optional = append(optional, key)
		}
		sort.Strings(optional)
		ordered = append(ordered, optional...)

		fields := make([]exampleField, 0, len(ordered))
		for _, key := range ordered {
			childProperty, _ := mapValue(props[key])
			value := exampleValueForField(key, childProperty, actionName)
			fields = append(fields, exampleField{Name: key, Value: value})
		}
		return exampleObject(fields), true
	}

	return nil, false
}

func exampleValueFromDefaults(name, actionName string) (any, bool) {
	builder, ok := defaultExampleValues[strings.ToLower(name)]
	if !ok {
		return nil, false
	}
	return builder(actionName), true
}

func propertyType(property map[string]any) string {
	if property == nil {
		return ""
	}
	if t, ok := stringValue(property["type"]); ok {
		return t
	}
	return ""
}

func sanitizePlaceholderName(name string) string {
	sanitized := strings.ReplaceAll(name, "_", " ")
	sanitized = strings.TrimSpace(sanitized)
	if sanitized == "" {
		return "value"
	}
	return sanitized
}

func writeExampleObject(b *strings.Builder, obj exampleObject, indent int) {
	b.WriteString("{\n")
	for i, field := range obj {
		b.WriteString(strings.Repeat("  ", indent+1))
		fmt.Fprintf(b, "\"%s\": ", field.Name)
		writeExampleValue(b, field.Value, indent+1)
		if i < len(obj)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(strings.Repeat("  ", indent))
	b.WriteString("}")
}

func writeExampleArray(b *strings.Builder, arr exampleArray, indent int) {
	b.WriteString("[\n")
	for i, item := range arr {
		b.WriteString(strings.Repeat("  ", indent+1))
		writeExampleValue(b, item, indent+1)
		if i < len(arr)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(strings.Repeat("  ", indent))
	b.WriteString("]")
}

func writeExampleValue(b *strings.Builder, value any, indent int) {
	switch v := value.(type) {
	case exampleObject:
		writeExampleObject(b, v, indent)
	case exampleArray:
		writeExampleArray(b, v, indent)
	case []any:
		writeExampleArray(b, exampleArray(v), indent)
	default:
		data, err := marshalExampleValue(v)
		if err != nil {
			fmt.Fprintf(b, "\"%v\"", v)
			return
		}
		b.Write(data)
	}
}

func marshalExampleValue(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}

	data := buf.Bytes()
	if len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}

	return data, nil
}

func buildConditionalRequirementGroups(actionName string, def *schemaDefinition, properties map[string]map[string]any) ([]conditionalRequirementGroup, error) {
	if def == nil {
		return nil, nil
	}

	normalizedAction := strings.ToUpper(strings.TrimSpace(actionName))
	baseValues := map[string]string{"action": normalizedAction}
	baseRequiredNames, err := collectRequiredFields(def, baseValues)
	if err != nil {
		return nil, err
	}

	baseSet := make(map[string]struct{}, len(baseRequiredNames))
	for _, name := range baseRequiredNames {
		baseSet[name] = struct{}{}
	}

	operations := enumerateOperationValues(def)
	if len(operations) == 0 {
		return nil, nil
	}

	groups := make([]conditionalRequirementGroup, 0, len(operations)+1)
	for _, op := range operations {
		values := map[string]string{
			"action":    normalizedAction,
			"operation": op,
		}
		requiredNames, err := collectRequiredFields(def, values)
		if err != nil {
			return nil, err
		}

		missing := difference(baseSet, requiredNames)
		note := formatMissingNote(missing)

		overrides := map[string]any{
			"action":    normalizedAction,
			"operation": op,
		}
		overrides = mergeExampleOverrides(overrides, kubernetesExampleOverrides(normalizedAction, op))

		group := conditionalRequirementGroup{
			Title:            fmt.Sprintf("operation = %q", op),
			Required:         buildFieldSummaries(requiredNames, properties),
			Note:             note,
			ExampleOverrides: overrides,
		}
		groups = append(groups, group)
	}

	defaultOverrides := map[string]any{
		"action":    normalizedAction,
		"operation": "<operation>",
	}
	defaultOverrides = mergeExampleOverrides(defaultOverrides, kubernetesExampleOverrides(normalizedAction, ""))

	defaultGroup := conditionalRequirementGroup{
		Title:            "operation = any other value (default case)",
		Required:         buildFieldSummaries(baseRequiredNames, properties),
		ExampleOverrides: defaultOverrides,
	}
	groups = append(groups, defaultGroup)

	return groups, nil
}

func buildFieldSummaries(names []string, properties map[string]map[string]any) []fieldSummary {
	summaries := make([]fieldSummary, 0, len(names))
	for _, name := range names {
		summaries = append(summaries, fieldSummary{Name: name, Description: fieldDescription(name, describeSchemaPropertyFromMap(properties[name]))})
	}
	return summaries
}

func describeSchemaPropertyFromMap(property map[string]any) string {
	if property == nil {
		return ""
	}
	return describeProperty(property)
}

func difference(baseSet map[string]struct{}, names []string) []string {
	missing := make([]string, 0)
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		seen[name] = struct{}{}
	}
	for name := range baseSet {
		if _, ok := seen[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

func formatMissingNote(missing []string) string {
	if len(missing) == 0 {
		return ""
	}

	if len(missing) == 1 {
		return fmt.Sprintf("(note: %q is NOT required)", missing[0])
	}

	quoted := make([]string, 0, len(missing))
	for _, name := range missing {
		quoted = append(quoted, fmt.Sprintf("%q", name))
	}
	return fmt.Sprintf("(note: %s are NOT required)", strings.Join(quoted, ", "))
}

func collectRequiredFields(def *schemaDefinition, values map[string]string) ([]string, error) {
	acc := &conditionalAccumulator{requiredSet: make(map[string]struct{})}
	if err := acc.collect(def, values); err != nil {
		return nil, err
	}
	return acc.requiredOrder, nil
}

type conditionalAccumulator struct {
	requiredSet   map[string]struct{}
	requiredOrder []string
}

func (a *conditionalAccumulator) collect(def *schemaDefinition, values map[string]string) error {
	if def == nil {
		return nil
	}

	for _, name := range def.Required {
		if _, exists := a.requiredSet[name]; !exists {
			a.requiredSet[name] = struct{}{}
			a.requiredOrder = append(a.requiredOrder, name)
		}
	}

	for _, cond := range def.AllOf {
		if cond.Then == nil {
			continue
		}

		matches, err := matchesCondition(cond.If, values)
		if err != nil {
			return err
		}
		if matches {
			if err := a.collect(cond.Then, values); err != nil {
				return err
			}
		}
	}

	return nil
}

func matchesCondition(def *schemaDefinition, values map[string]string) (bool, error) {
	if def == nil {
		return true, nil
	}

	for _, name := range def.Required {
		if _, ok := values[name]; !ok {
			return false, nil
		}
	}

	for name, raw := range def.Properties {
		var property map[string]any
		if err := json.Unmarshal(raw, &property); err != nil {
			return false, err
		}

		value, hasValue := values[name]
		if ok := propertyMatches(property, value, hasValue); !ok {
			return false, nil
		}
	}

	return true, nil
}

func propertyMatches(property map[string]any, value string, hasValue bool) bool {
	if property == nil {
		return true
	}

	if constValue, ok := stringValue(property["const"]); ok {
		if !hasValue {
			return false
		}
		return strings.EqualFold(constValue, value)
	}

	if enumValues := stringSlice(property["enum"]); len(enumValues) > 0 {
		if !hasValue {
			return false
		}
		for _, candidate := range enumValues {
			if strings.EqualFold(candidate, value) {
				return true
			}
		}
		return false
	}

	if notValue, ok := mapValue(property["not"]); ok {
		if constValue, ok := stringValue(notValue["const"]); ok {
			if !hasValue {
				return true
			}
			return !strings.EqualFold(constValue, value)
		}
	}

	return true
}

func enumerateOperationValues(def *schemaDefinition) []string {
	values := make([]string, 0)
	seen := make(map[string]struct{})

	var walk func(*schemaDefinition)
	walk = func(d *schemaDefinition) {
		if d == nil {
			return
		}

		if raw, ok := d.Properties["operation"]; ok {
			var property map[string]any
			if err := json.Unmarshal(raw, &property); err == nil {
				if constValue, ok := stringValue(property["const"]); ok {
					if _, exists := seen[constValue]; !exists {
						seen[constValue] = struct{}{}
						values = append(values, constValue)
					}
				}
				if enumValues := stringSlice(property["enum"]); len(enumValues) > 0 {
					for _, enumValue := range enumValues {
						if _, exists := seen[enumValue]; exists {
							continue
						}
						seen[enumValue] = struct{}{}
						values = append(values, enumValue)
					}
				}
			}
		}

		for _, cond := range d.AllOf {
			walk(cond.If)
			walk(cond.Then)
		}
	}

	walk(def)

	preferred := []string{"PORT_FORWARD", "STOP_PORT_FORWARD", "SCALE"}
	ordered := make([]string, 0, len(values))
	seenOrdered := make(map[string]struct{})
	for _, candidate := range preferred {
		if _, ok := seen[candidate]; ok {
			ordered = append(ordered, candidate)
			seenOrdered[candidate] = struct{}{}
		}
	}

	for _, candidate := range values {
		if _, ok := seenOrdered[candidate]; ok {
			continue
		}
		ordered = append(ordered, candidate)
	}

	return ordered
}

func extractAllowedValues(summary actionSchemaSummary) []struct {
	Name        string
	Description []string
} {
	allowed := make([]struct {
		Name        string
		Description []string
	}, 0)

	for name, property := range summary.Properties {
		constVal, hasConst := stringValue(property["const"])
		enumVals := stringSlice(property["enum"])

		if hasConst || len(enumVals) > 0 {
			descriptions := make([]string, 0)
			if hasConst {
				descriptions = append(descriptions, fmt.Sprintf("%q", constVal))
			}
			if len(enumVals) > 0 {
				quoted := make([]string, 0, len(enumVals))
				for _, val := range enumVals {
					quoted = append(quoted, fmt.Sprintf("%q", val))
				}
				descriptions = append(descriptions, strings.Join(quoted, ", "))
			}
			allowed = append(allowed, struct {
				Name        string
				Description []string
			}{Name: name, Description: descriptions})
		}
	}

	sort.SliceStable(allowed, func(i, j int) bool {
		return allowed[i].Name < allowed[j].Name
	})

	return allowed
}

func buildConditionalExample(summary actionSchemaSummary, group conditionalRequirementGroup) string {
	if len(group.Required) == 0 {
		return ""
	}

	overrides := group.ExampleOverrides
	fields := make([]exampleField, 0, len(group.Required))
	for _, field := range group.Required {
		value := exampleValueForField(field.Name, summary.Properties[field.Name], summary.ActionName)
		if overrides != nil {
			if override, ok := overrides[field.Name]; ok {
				value = override
			}
		}
		fields = append(fields, exampleField{Name: field.Name, Value: value})
	}

	var b strings.Builder
	writeExampleObject(&b, exampleObject(fields), 0)
	return b.String()
}

func kubernetesExampleOverrides(actionName, operation string) map[string]any {
	if !strings.EqualFold(actionName, "KUBERNETES") {
		return nil
	}

	switch strings.ToUpper(operation) {
	case "PORT_FORWARD":
		return map[string]any{
			"id":           "port-forward-task",
			"description":  "Forward local port to service in cluster",
			"service":      "<service-name>",
			"local_port":   "<local-port>",
			"service_port": "<service-port>",
		}
	case "STOP_PORT_FORWARD":
		return map[string]any{
			"id":          "stop-port-forward-task",
			"description": "Stop port forwarding",
			"local_port":  "<local-port>",
		}
	case "SCALE":
		return map[string]any{
			"id":          "scale-deployment-task",
			"description": "Scale a deployment",
			"namespace":   "<namespace>",
			"deployments": "<deployment-names>",
			"replicas":    "<replica-count>",
		}
	case "WAIT_FOR_POD_READINESS":
		return map[string]any{
			"id":                    "wait-for-pod-readiness",
			"description":           "Wait for deployments to become ready",
			"namespace":             "<namespace>",
			"deployments":           "<deployment-names>",
			"max_wait_seconds":      "<max-wait-seconds>",
			"poll_interval_seconds": "<poll-interval-seconds>",
		}
	case "":
		return map[string]any{
			"id":          "generic-k8s-task",
			"description": "Perform a Kubernetes operation",
		}
	default:
		return map[string]any{
			"id":          "generic-k8s-task",
			"description": "Perform a Kubernetes operation",
		}
	}
}

func mergeExampleOverrides(base map[string]any, overrides map[string]any) map[string]any {
	if len(overrides) == 0 {
		return base
	}

	if base == nil {
		base = make(map[string]any, len(overrides))
	}

	for key, value := range overrides {
		base[key] = value
	}
	return base
}

func Usage(program string) string {
	return fmt.Sprintf("Usage:\n  %s help action [action_name]\n\nLists every available action or displays the fields required to configure the specified action.", program)
}

func Index(program string) string {
	names := registry.Names()

	var b strings.Builder
	b.WriteString("Available actions:\n")
	if len(names) == 0 {
		b.WriteString("  - None registered\n")
	} else {
		for _, name := range names {
			fmt.Fprintf(&b, "  - %s\n", name)
		}
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "Use \"%s help action <name>\" to view details for a specific action.", program)
	return b.String()
}
