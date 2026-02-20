package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"flowk/internal/actions/registry"
)

const ActionName = "SHELL"

type Payload struct {
	// Command holds the normalized command text to pass to the shell.
	Command string `json:"-"`
	// RawCommand keeps the original JSON form (array of lines) for decoding.
	RawCommand       json.RawMessage       `json:"command"`
	Shell            *ShellOptions         `json:"shell"`
	Environment      []EnvironmentVariable `json:"environment"`
	ProxyVariables   []string              `json:"proxyVariables"`
	WorkingDirectory string                `json:"workingDirectory"`
	TimeoutSeconds   float64               `json:"timeoutSeconds"`
	ContinueOnError  bool                  `json:"continueOnError"`
}

type ShellOptions struct {
	Program string   `json:"program"`
	Args    []string `json:"args"`
}

type EnvironmentVariable struct {
	Name   string `json:"name"`
	Value  any    `json:"value"`
	Secret bool   `json:"secret"`
	Proxy  bool   `json:"proxy"`
}

type EnvironmentSetting struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Secret bool   `json:"secret,omitempty"`
	Proxy  bool   `json:"proxy,omitempty"`
}

type ExecutionResult struct {
	Command          []string             `json:"command"`
	WorkingDirectory string               `json:"workingDirectory,omitempty"`
	ExitCode         int                  `json:"exitCode"`
	Stdout           string               `json:"stdout"`
	Stderr           string               `json:"stderr"`
	DurationSeconds  float64              `json:"durationSeconds"`
	Environment      []EnvironmentSetting `json:"environment,omitempty"`
}

func (p *Payload) Validate() error {
	// Normalize command from raw JSON (array only)
	if err := p.normalizeCommand(); err != nil {
		return err
	}
	trimmedCommand := strings.TrimSpace(p.Command)
	if trimmedCommand == "" {
		return fmt.Errorf("shell task: command is required")
	}
	p.Command = trimmedCommand

	if p.Shell != nil {
		p.Shell.Program = strings.TrimSpace(p.Shell.Program)
		if p.Shell.Program == "" {
			return fmt.Errorf("shell task: shell.program is required when shell is provided")
		}
	}

	for idx := range p.Environment {
		if err := p.Environment[idx].validate(idx); err != nil {
			return err
		}
	}

	for idx := range p.ProxyVariables {
		trimmed := strings.TrimSpace(p.ProxyVariables[idx])
		if trimmed == "" {
			return fmt.Errorf("shell task: proxyVariables[%d]: name is required", idx)
		}
		p.ProxyVariables[idx] = trimmed
	}

	p.WorkingDirectory = strings.TrimSpace(p.WorkingDirectory)

	if p.TimeoutSeconds < 0 {
		return fmt.Errorf("shell task: timeoutSeconds cannot be negative")
	}

	return nil
}

// normalizeCommand parses rawCommand which must be an array of strings
// and sets Command to the newline-joined script.
func (p *Payload) normalizeCommand() error {
	if len(p.RawCommand) == 0 {
		// Allow tests or programmatic callers to provide Command directly.
		p.Command = strings.TrimSpace(p.Command)
		if p.Command == "" {
			return fmt.Errorf("shell task: command is required")
		}
		return nil
	}
	// Expect array of strings
	var asArray []string
	if err := json.Unmarshal(p.RawCommand, &asArray); err != nil {
		return fmt.Errorf("shell task: command must be an array of strings")
	}
	// Join with newlines so shells like bash -lc execute sequentially
	p.Command = strings.Join(asArray, "\n")
	return nil
}

func (e *EnvironmentVariable) validate(idx int) error {
	trimmed := strings.TrimSpace(e.Name)
	if trimmed == "" {
		return fmt.Errorf("shell task: environment[%d]: name is required", idx)
	}
	if strings.Contains(trimmed, "=") {
		return fmt.Errorf("shell task: environment[%d]: name cannot contain '='", idx)
	}
	e.Name = trimmed
	return nil
}

func Execute(ctx context.Context, spec Payload, execCtx *registry.ExecutionContext) (ExecutionResult, error) {
	if execCtx == nil {
		execCtx = &registry.ExecutionContext{}
	}

	builder := newEnvironmentBuilder(os.Environ())

	for idx := range spec.Environment {
		entry := spec.Environment[idx]
		value, err := stringify(entry.Value)
		if err != nil {
			return ExecutionResult{}, fmt.Errorf("shell task: environment[%d]: %w", idx, err)
		}
		if entry.Proxy {
			if err := builder.applyProxy(entry.Name, value, entry.Secret); err != nil {
				return ExecutionResult{}, fmt.Errorf("shell task: environment[%d]: %w", idx, err)
			}
			continue
		}
		if err := builder.apply(entry.Name, value, entry.Secret, false); err != nil {
			return ExecutionResult{}, fmt.Errorf("shell task: environment[%d]: %w", idx, err)
		}
	}

	for _, name := range spec.ProxyVariables {
		if execCtx.Variables == nil {
			return ExecutionResult{}, fmt.Errorf("shell task: proxy variable %q not defined", name)
		}
		variable, ok := execCtx.Variables[name]
		if !ok {
			return ExecutionResult{}, fmt.Errorf("shell task: proxy variable %q not defined", name)
		}
		if !strings.EqualFold(variable.Type, "proxy") {
			return ExecutionResult{}, fmt.Errorf("shell task: proxy variable %q must have type proxy", name)
		}
		proxies, err := normalizeProxyValue(variable.Value)
		if err != nil {
			return ExecutionResult{}, fmt.Errorf("shell task: proxy variable %q: %w", name, err)
		}
		for proxyName, proxyValue := range proxies {
			if err := builder.applyProxy(proxyName, proxyValue, false); err != nil {
				return ExecutionResult{}, fmt.Errorf("shell task: proxy variable %q: %w", name, err)
			}
		}
	}

	envSlice, envSettings := builder.finalize()

	program, args := resolveShell(spec)
	command := exec.CommandContext(ctx, program, args...)
	command.Env = envSlice
	if spec.WorkingDirectory != "" {
		command.Dir = spec.WorkingDirectory
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	command.Stdout = &stdoutBuf
	command.Stderr = &stderrBuf

	logCommand(execCtx.Logger, spec, program, args, envSettings)

	start := time.Now()
	runErr := command.Run()
	duration := time.Since(start)

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ExecutionResult{}, fmt.Errorf("shell: command interrupted: %w", ctxErr)
			}
			return ExecutionResult{}, fmt.Errorf("shell: executing command: %w", runErr)
		}
	}

	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	logCommandOutcome(execCtx.Logger, exitCode, stdout, stderr, duration)

	result := ExecutionResult{
		Command:          append([]string{program}, args...),
		WorkingDirectory: spec.WorkingDirectory,
		ExitCode:         exitCode,
		Stdout:           stdout,
		Stderr:           stderr,
		DurationSeconds:  duration.Seconds(),
		Environment:      envSettings,
	}

	if runErr != nil && exitCode != 0 && !spec.ContinueOnError {
		return result, fmt.Errorf("shell: command exited with code %d", exitCode)
	}

	return result, nil
}

func resolveShell(spec Payload) (string, []string) {
	program := ""
	args := []string{}

	if spec.Shell != nil {
		program = spec.Shell.Program
		if len(spec.Shell.Args) > 0 {
			args = append(args, spec.Shell.Args...)
		}
	}

	if program == "" {
		program, args = defaultShell()
	}

	args = append(args, spec.Command)
	return program, args
}

func defaultShell() (string, []string) {
	switch runtime.GOOS {
	case "windows":
		program := os.Getenv("COMSPEC")
		if strings.TrimSpace(program) == "" {
			program = "cmd.exe"
		}
		return program, []string{"/C"}
	default:
		return "/bin/sh", []string{"-c"}
	}
}

type environmentBuilder struct {
	values  map[string]string
	applied map[string]envApplication
}

type envApplication struct {
	value  string
	secret bool
	proxy  bool
}

func newEnvironmentBuilder(base []string) *environmentBuilder {
	values := make(map[string]string, len(base))
	for _, entry := range base {
		if idx := strings.Index(entry, "="); idx >= 0 {
			name := entry[:idx]
			values[name] = entry[idx+1:]
		}
	}
	return &environmentBuilder{
		values:  values,
		applied: make(map[string]envApplication),
	}
}

func (b *environmentBuilder) apply(name, value string, secret, proxy bool) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("environment variable name is required")
	}
	if strings.Contains(trimmed, "=") {
		return fmt.Errorf("environment variable name %q cannot contain '='", trimmed)
	}
	b.values[trimmed] = value
	b.applied[trimmed] = envApplication{value: value, secret: secret, proxy: proxy}
	return nil
}

func (b *environmentBuilder) applyProxy(name, value string, secret bool) error {
	names := canonicalProxyNames(name)
	if len(names) == 0 {
		return fmt.Errorf("proxy name is required")
	}
	for _, candidate := range names {
		if err := b.apply(candidate, value, secret, true); err != nil {
			return err
		}
	}
	return nil
}

func (b *environmentBuilder) finalize() ([]string, []EnvironmentSetting) {
	keys := make([]string, 0, len(b.values))
	for key := range b.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	envSlice := make([]string, len(keys))
	for i, key := range keys {
		envSlice[i] = fmt.Sprintf("%s=%s", key, b.values[key])
	}

	appliedKeys := make([]string, 0, len(b.applied))
	for key := range b.applied {
		appliedKeys = append(appliedKeys, key)
	}
	sort.Strings(appliedKeys)

	settings := make([]EnvironmentSetting, len(appliedKeys))
	for i, key := range appliedKeys {
		entry := b.applied[key]
		value := entry.value
		if entry.secret {
			value = "****"
		}
		settings[i] = EnvironmentSetting{
			Name:   key,
			Value:  value,
			Secret: entry.secret,
			Proxy:  entry.proxy,
		}
	}

	return envSlice, settings
}

func canonicalProxyNames(name string) []string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil
	}
	lower := strings.ToLower(trimmed)
	switch lower {
	case "http", "http_proxy":
		return []string{"HTTP_PROXY", "http_proxy"}
	case "https", "https_proxy":
		return []string{"HTTPS_PROXY", "https_proxy"}
	case "no", "no_proxy":
		return []string{"NO_PROXY", "no_proxy"}
	default:
		return []string{trimmed}
	}
}

func normalizeProxyValue(value any) (map[string]string, error) {
	switch v := value.(type) {
	case map[string]string:
		proxies := make(map[string]string, len(v))
		for key, val := range v {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				return nil, fmt.Errorf("proxy map contains empty key")
			}
			proxies[trimmedKey] = val
		}
		return proxies, nil
	case map[string]any:
		proxies := make(map[string]string, len(v))
		for key, raw := range v {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				return nil, fmt.Errorf("proxy map contains empty key")
			}
			str, err := stringify(raw)
			if err != nil {
				return nil, fmt.Errorf("proxy %q: %w", key, err)
			}
			proxies[trimmedKey] = str
		}
		return proxies, nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil, fmt.Errorf("proxy value cannot be empty")
		}
		return map[string]string{"http": trimmed}, nil
	case json.Number:
		return normalizeProxyValue(string(v))
	case nil:
		return nil, fmt.Errorf("proxy value cannot be null")
	default:
		return nil, fmt.Errorf("unsupported proxy value type %T", value)
	}
}

func stringify(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case fmt.Stringer:
		return v.String(), nil
	case json.Number:
		return v.String(), nil
	case float64, float32, int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
		return fmt.Sprintf("%v", v), nil
	case bool:
		return fmt.Sprintf("%t", v), nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("stringify %T: %w", value, err)
		}
		return string(data), nil
	}
}

func logCommand(logger registry.Logger, spec Payload, program string, args []string, env []EnvironmentSetting) {
	if logger == nil {
		return
	}

	commandLine := append([]string{program}, args...)
	logger.Printf("SHELL: executing %s", strings.Join(commandLine, " "))
	if spec.WorkingDirectory != "" {
		logger.Printf("SHELL: working directory %s", spec.WorkingDirectory)
	}
	if len(env) > 0 {
		logger.Printf("SHELL: environment overrides:")
		for _, setting := range env {
			logger.Printf("  %s=%s", setting.Name, setting.Value)
		}
	}
}

func logCommandOutcome(logger registry.Logger, exitCode int, stdout, stderr string, duration time.Duration) {
	if logger == nil {
		return
	}

	logger.Printf("SHELL: exit code %d (duration %s)", exitCode, duration.Round(time.Millisecond))
	if strings.TrimSpace(stdout) != "" {
		logger.Printf("SHELL stdout:\n%s", stdout)
	}
	if strings.TrimSpace(stderr) != "" {
		logger.Printf("SHELL stderr:\n%s", stderr)
	}
}
