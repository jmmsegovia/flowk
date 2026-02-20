package flow

import (
	"encoding/json"
	"sync"
)

// SchemaFragmentsProvider exposes the schema fragments contributed by registered actions.
type SchemaFragmentsProvider func() ([]json.RawMessage, uint64)

var (
	schemaProviderMu sync.RWMutex
	schemaProvider   SchemaFragmentsProvider
)

// RegisterSchemaProvider configures the function used to retrieve schema fragments contributed by actions.
func RegisterSchemaProvider(provider SchemaFragmentsProvider) {
	schemaProviderMu.Lock()
	defer schemaProviderMu.Unlock()
	schemaProvider = provider
}

func schemaFragments() ([]json.RawMessage, uint64) {
	schemaProviderMu.RLock()
	provider := schemaProvider
	schemaProviderMu.RUnlock()
	if provider == nil {
		return nil, 0
	}
	return provider()
}
