package registry

import (
	"encoding/json"
	"fmt"
)

// SchemaFromEmbedded validates and copies an embedded JSON schema fragment.
func SchemaFromEmbedded(fragment []byte) (json.RawMessage, error) {
	if len(fragment) == 0 {
		return nil, nil
	}
	if !json.Valid(fragment) {
		return nil, fmt.Errorf("registry: embedded schema is not valid JSON")
	}
	return json.RawMessage(append([]byte(nil), fragment...)), nil
}
