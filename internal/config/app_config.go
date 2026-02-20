package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"gopkg.in/yaml.v3"
)

const (
	appName        = "flowk"
	configFileName = "config.yaml"
)

const (
	DefaultUIHost = "127.0.0.1"
	DefaultUIPort = 8080
	DefaultUIDir  = "ui/dist"
)

// UIConfig controls how the embedded UI server is exposed.
type UIConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	Dir  string `yaml:"dir"`
}

// Config captures the user-facing configuration stored in config.yaml.
type Config struct {
	UI UIConfig `yaml:"ui"`
}

// LoadResult reports the resolved configuration data and location.
type LoadResult struct {
	Config Config
	Path   string
	Loaded bool
}

// DefaultConfig returns the configuration values used when config.yaml is missing.
func DefaultConfig() Config {
	return Config{
		UI: UIConfig{
			Host: DefaultUIHost,
			Port: DefaultUIPort,
			Dir:  DefaultUIDir,
		},
	}
}

// ConfigPath resolves the expected config.yaml location under XDG config home.
func ConfigPath() (string, error) {
	if configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); configHome != "" {
		return filepath.Join(configHome, appName, configFileName), nil
	}
	if strings.TrimSpace(xdg.ConfigHome) == "" {
		return "", fmt.Errorf("xdg config home not set")
	}
	return filepath.Join(xdg.ConfigHome, appName, configFileName), nil
}

// Load reads config.yaml (if present) from the XDG configuration directory.
func Load() (LoadResult, error) {
	return LoadFrom("")
}

// LoadFrom reads config.yaml (if present) from the provided path.
// When path is empty, the XDG configuration location is used.
func LoadFrom(path string) (LoadResult, error) {
	resolvedPath, err := resolveConfigPath(path)
	if err != nil {
		return LoadResult{}, err
	}

	cfg := DefaultConfig()
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if writeErr := writeDefaultConfig(resolvedPath, cfg); writeErr != nil {
				return LoadResult{}, writeErr
			}
			return LoadResult{Config: cfg, Path: resolvedPath, Loaded: true}, nil
		}
		return LoadResult{}, fmt.Errorf("read config %s: %w", resolvedPath, err)
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return LoadResult{Config: cfg, Path: resolvedPath, Loaded: true}, nil
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return LoadResult{}, fmt.Errorf("parse config %s: %w", resolvedPath, err)
	}

	cfg = applyDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		return LoadResult{}, fmt.Errorf("invalid config %s: %w", resolvedPath, err)
	}

	return LoadResult{Config: cfg, Path: resolvedPath, Loaded: true}, nil
}

func resolveConfigPath(path string) (string, error) {
	if trimmed := strings.TrimSpace(path); trimmed != "" {
		return filepath.Clean(trimmed), nil
	}
	return ConfigPath()
}

func applyDefaults(cfg Config) Config {
	cfg.UI.Host = strings.TrimSpace(cfg.UI.Host)
	if cfg.UI.Host == "" {
		cfg.UI.Host = DefaultUIHost
	}

	if cfg.UI.Port == 0 {
		cfg.UI.Port = DefaultUIPort
	}

	cfg.UI.Dir = strings.TrimSpace(cfg.UI.Dir)
	if cfg.UI.Dir == "" {
		cfg.UI.Dir = DefaultUIDir
	}

	return cfg
}

func validateConfig(cfg Config) error {
	if cfg.UI.Port <= 0 || cfg.UI.Port > 65535 {
		return fmt.Errorf("ui.port must be between 1 and 65535")
	}
	return nil
}

func writeDefaultConfig(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir %s: %w", filepath.Dir(path), err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}
