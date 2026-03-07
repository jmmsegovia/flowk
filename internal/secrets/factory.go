package secrets

import (
	"fmt"
	"strings"
)

// Config describes secret provider settings used by the runtime.
type Config struct {
	Provider string
	Vault    VaultConfig
}

// BuildResolver builds a resolver from runtime configuration.
func BuildResolver(cfg Config) (Resolver, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" || provider == "none" {
		return nil, nil
	}

	switch provider {
	case providerVault:
		return NewVaultResolver(cfg.Vault)
	default:
		return nil, fmt.Errorf("unsupported secrets provider %q", cfg.Provider)
	}
}
