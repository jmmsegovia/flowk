package sleep

import (
	"encoding/json"

	"flowk/internal/actions/registry"

	_ "embed"
)

//go:embed schema.json
var schemaFragment []byte

func (action) JSONSchema() (json.RawMessage, error) {
	return registry.SchemaFromEmbedded(schemaFragment)
}

var _ registry.SchemaProvider = action{}
