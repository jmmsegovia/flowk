package jsonpathutil

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/PaesslerAG/gval"
	"github.com/PaesslerAG/jsonpath"
)

const lengthSuffix = ".length()"

var extendedLanguage = gval.NewLanguage(
	jsonpath.Language(),
	gval.Arithmetic(),
	gval.Text(),
	gval.PropositionalLogic(),
)

// Evaluate resolves the provided JSONPath expression against the given container.
// It supports chained extensions such as .length() that can be appended to the
// JSONPath expression and applied after evaluating the base expression.
func Evaluate(expr string, container any) (any, error) {
	base := expr
	var lengthCount int

	for strings.HasSuffix(base, lengthSuffix) {
		base = strings.TrimSuffix(base, lengthSuffix)
		lengthCount++
	}

	value, err := evaluateJSONPath(base, container)
	if err != nil {
		return nil, err
	}

	for i := 0; i < lengthCount; i++ {
		value, err = applyLength(value)
		if err != nil {
			return nil, err
		}
	}

	return value, nil
}

func evaluateJSONPath(expr string, container any) (any, error) {
	eval, err := extendedLanguage.NewEvaluable(expr)
	if err != nil {
		return nil, err
	}
	return eval(context.Background(), container)
}

func applyLength(value any) (any, error) {
	if value == nil {
		return nil, fmt.Errorf("length() unsupported for <nil>")
	}

	switch v := value.(type) {
	case string:
		return float64(len(v)), nil
	case []byte:
		return float64(len(v)), nil
	case map[string]any:
		return float64(len(v)), nil
	case []any:
		return float64(len(v)), nil
	}

	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return nil, fmt.Errorf("length() unsupported for type %T", value)
	}

	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.String:
		return float64(rv.Len()), nil
	}

	return nil, fmt.Errorf("length() unsupported for type %T", value)
}

// NormalizeContainer recursively converts container values into structures
// compatible with the jsonpath engine by decoding embedded JSON blobs when
// possible.
func NormalizeContainer(value any) any {
	switch v := value.(type) {
	case json.RawMessage:
		var decoded any
		if err := json.Unmarshal(v, &decoded); err == nil {
			return NormalizeContainer(decoded)
		}
		return string(v)
	case []byte:
		var decoded any
		if err := json.Unmarshal(v, &decoded); err == nil {
			return NormalizeContainer(decoded)
		}
		return string(v)
	case map[string]any:
		normalized := make(map[string]any, len(v))
		for key, val := range v {
			normalized[key] = NormalizeContainer(val)
		}
		return normalized
	case []any:
		normalized := make([]any, len(v))
		for i, item := range v {
			normalized[i] = NormalizeContainer(item)
		}
		return normalized
	}

	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return value
	}

	switch rv.Kind() {
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return value
		}

		normalized := make(map[string]any, rv.Len())
		for _, key := range rv.MapKeys() {
			normalized[key.String()] = NormalizeContainer(rv.MapIndex(key).Interface())
		}
		return normalized
	case reflect.Slice, reflect.Array:
		normalized := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			normalized[i] = NormalizeContainer(rv.Index(i).Interface())
		}
		return normalized
	case reflect.Struct:
		data, err := json.Marshal(value)
		if err != nil {
			return value
		}
		var decoded any
		if err := json.Unmarshal(data, &decoded); err != nil {
			return value
		}
		return NormalizeContainer(decoded)
	}

	return value
}
