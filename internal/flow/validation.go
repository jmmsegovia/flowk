package flow

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/xeipuuv/gojsonschema"
)

var (
	schemaCache        sync.Map
)

type schemaCacheKey struct {
	version uint64
}

func validateDefinitionAgainstSchema(flowPath string, content []byte) error {
	_ = flowPath

	schema, err := loadFlowSchema()
	if err != nil {
		return fmt.Errorf("validating action flow: %w", err)
	}

	result, err := schema.Validate(gojsonschema.NewBytesLoader(content))
	if err != nil {
		return fmt.Errorf("validating action flow: %w", err)
	}

	if result.Valid() {
		return nil
	}

	var messages []string
	for _, validationErr := range result.Errors() {
		messages = append(messages, validationErr.String())
	}

	return fmt.Errorf("validating action flow: schema validation failed: %s", strings.Join(messages, "; "))
}

func CombinedSchema() ([]byte, error) {
	fragments, _ := schemaFragments()
	return combineSchemaWithFragments(embeddedBaseSchema, fragments)
}

func loadFlowSchema() (*gojsonschema.Schema, error) {
	fragments, version := schemaFragments()
	key := schemaCacheKey{version: schemaCacheVersion(version)}
	if cached, ok := schemaCache.Load(key); ok {
		if schema, ok := cached.(*gojsonschema.Schema); ok && schema != nil {
			return schema, nil
		}
	}

	combined, err := combineSchemaWithFragments(embeddedBaseSchema, fragments)
	if err != nil {
		return nil, fmt.Errorf("loading flow schema: %w", err)
	}

	schema, err := gojsonschema.NewSchema(gojsonschema.NewBytesLoader(combined))
	if err != nil {
		return nil, fmt.Errorf("loading flow schema: %w", err)
	}

	schemaCache.Store(key, schema)
	return schema, nil
}

func schemaCacheVersion(version uint64) uint64 {
	if version == 0 {
		return 1
	}
	return version
}

func combineSchemaWithFragments(base []byte, fragments []json.RawMessage) ([]byte, error) {
	if len(fragments) == 0 {
		return base, nil
	}

	var document map[string]any
	if err := json.Unmarshal(base, &document); err != nil {
		return nil, fmt.Errorf("decoding base schema: %w", err)
	}

	for _, fragment := range fragments {
		if len(fragment) == 0 {
			continue
		}

		var overlay map[string]any
		if err := json.Unmarshal(fragment, &overlay); err != nil {
			return nil, fmt.Errorf("decoding action schema fragment: %w", err)
		}

		mergeSchema(document, overlay)
	}

	combined, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encoding combined schema: %w", err)
	}

	return combined, nil
}

func mergeSchema(target, overlay map[string]any) {
	for key, overlayVal := range overlay {
		if existing, ok := target[key]; ok {
			target[key] = mergeValues(existing, overlayVal)
			continue
		}

		target[key] = cloneValue(overlayVal)
	}
}

func mergeValues(existing, overlay any) any {
	existingMap, existingIsMap := existing.(map[string]any)
	overlayMap, overlayIsMap := overlay.(map[string]any)
	if existingIsMap && overlayIsMap {
		mergeSchema(existingMap, overlayMap)
		return existing
	}

	existingSlice, existingIsSlice := existing.([]any)
	overlaySlice, overlayIsSlice := overlay.([]any)
	if existingIsSlice && overlayIsSlice {
		return mergeSlices(existingSlice, overlaySlice)
	}

	return cloneValue(overlay)
}

func mergeSlices(base, extra []any) []any {
	if len(extra) == 0 {
		return base
	}
	if len(base) == 0 {
		return cloneSlice(extra)
	}

	if allStrings(base) && allStrings(extra) {
		seen := make(map[string]struct{}, len(base)+len(extra))
		merged := make([]any, 0, len(base)+len(extra))
		for _, v := range base {
			if s, ok := v.(string); ok {
				if _, exists := seen[s]; exists {
					continue
				}
				seen[s] = struct{}{}
			}
			merged = append(merged, v)
		}
		for _, v := range extra {
			if s, ok := v.(string); ok {
				if _, exists := seen[s]; exists {
					continue
				}
				seen[s] = struct{}{}
			}
			merged = append(merged, v)
		}
		return merged
	}

	merged := make([]any, 0, len(base)+len(extra))
	merged = append(merged, base...)
	merged = append(merged, extra...)
	return merged
}

func allStrings(values []any) bool {
	for _, v := range values {
		if _, ok := v.(string); !ok {
			return false
		}
	}
	return true
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copied := make(map[string]any, len(typed))
		for k, v := range typed {
			copied[k] = cloneValue(v)
		}
		return copied
	case []any:
		return cloneSlice(typed)
	default:
		return typed
	}
}

func cloneSlice(values []any) []any {
	if len(values) == 0 {
		return nil
	}
	copied := make([]any, len(values))
	for i, v := range values {
		copied[i] = cloneValue(v)
	}
	return copied
}
