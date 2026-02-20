package ssh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	sshclient "github.com/helloyi/go-sshclient"
	"github.com/kr/fs"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

func init() {
	registry.Register(&Action{})
}

// Action implements the SSH task handler backed by github.com/helloyi/go-sshclient.
type Action struct{}

// Name returns the registry identifier for the SSH action.
func (Action) Name() string {
	return "SSH"
}

func expandUserPath(path string) (string, error) {
	if path == "" || path[0] != '~' {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("ssh: determine user home directory: %w", err)
	}

	if len(path) == 1 {
		return home, nil
	}

	switch path[1] {
	case '/', '\\':
		return filepath.Join(home, path[2:]), nil
	default:
		return "", fmt.Errorf("ssh: expanding user-specific path %q is not supported", path)
	}
}

// Execute performs the SSH workflow described in the payload.
func (Action) Execute(ctx context.Context, payload json.RawMessage, execCtx *registry.ExecutionContext) (registry.Result, error) {
	var spec payloadSpec
	if err := json.Unmarshal(payload, &spec); err != nil {
		return registry.Result{}, fmt.Errorf("ssh: decode payload: %w", err)
	}

	if err := spec.validate(); err != nil {
		return registry.Result{}, err
	}

	client, err := spec.Connection.dial()
	if err != nil {
		return registry.Result{}, err
	}
	defer client.Close()

	state := newActionState(client, spec)
	defer state.Close()

	results := make([]stepResult, 0, len(spec.Steps))

	for idx, raw := range spec.Steps {
		select {
		case <-ctx.Done():
			return registry.Result{}, ctx.Err()
		default:
		}

		outcome, err := state.executeStep(ctx, idx, raw)
		if err != nil {
			return registry.Result{}, err
		}
		results = append(results, outcome)
	}

	return registry.Result{Value: map[string]any{
		"connection": spec.Connection.summary(),
		"steps":      results,
	}, Type: flow.ResultTypeJSON}, nil
}

// payloadSpec captures the top-level SSH payload definition.
type payloadSpec struct {
	Connection connectionSpec    `json:"connection"`
	SFTP       *sftpOptionsSpec  `json:"sftp"`
	Steps      []json.RawMessage `json:"steps"`
}

func (p *payloadSpec) validate() error {
	if err := p.Connection.validate(); err != nil {
		return err
	}
	if len(p.Steps) == 0 {
		return errors.New("ssh: at least one step must be declared")
	}
	return nil
}

// connectionSpec declares how the SSH client should be established.
type connectionSpec struct {
	Network          string      `json:"network"`
	Address          string      `json:"address"`
	Username         string      `json:"username"`
	Auth             authSpec    `json:"auth"`
	TimeoutSeconds   float64     `json:"timeoutSeconds"`
	HostKey          hostKeySpec `json:"hostKey"`
	ClientVersion    string      `json:"clientVersion"`
	PreferredCiphers []string    `json:"preferredCiphers"`
	KeepAliveSeconds float64     `json:"keepAliveSeconds"`
}

func (c *connectionSpec) validate() error {
	if strings.TrimSpace(c.Address) == "" {
		return errors.New("ssh: connection.address must be provided")
	}
	if strings.TrimSpace(c.Username) == "" {
		return errors.New("ssh: connection.username must be provided")
	}
	return nil
}

func (c *connectionSpec) dial() (*sshclient.Client, error) {
	network := strings.TrimSpace(c.Network)
	if network == "" {
		network = "tcp"
	}

	timeout := time.Duration(0)
	if c.TimeoutSeconds > 0 {
		timeout = time.Duration(c.TimeoutSeconds * float64(time.Second))
	}

	config := &ssh.ClientConfig{
		User:            c.Username,
		Timeout:         timeout,
		ClientVersion:   strings.TrimSpace(c.ClientVersion),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	if len(c.PreferredCiphers) > 0 {
		config.Config = ssh.Config{Ciphers: c.PreferredCiphers}
	}

	if err := c.Auth.apply(config); err != nil {
		return nil, err
	}

	callback, cleanup, err := c.HostKey.build()
	if err != nil {
		return nil, err
	}
	if callback != nil {
		config.HostKeyCallback = callback
	}

	client, err := sshclient.Dial(network, c.Address, config)
	if err != nil {
		return nil, fmt.Errorf("ssh: dial %s %s: %w", network, c.Address, err)
	}

	if c.KeepAliveSeconds > 0 {
		go keepAliveLoop(client.UnderlyingClient(), time.Duration(c.KeepAliveSeconds*float64(time.Second)))
	}

	if cleanup != nil {
		cleanup()
	}

	return client, nil
}

func (c *connectionSpec) summary() map[string]any {
	return map[string]any{
		"address":  c.Address,
		"network":  chooseNonEmpty(c.Network, "tcp"),
		"username": c.Username,
	}
}

// hostKeySpec configures host key verification.
type hostKeySpec struct {
	Mode            string   `json:"mode"`
	KnownHostsFiles []string `json:"knownHostsFiles"`
	InlineEntries   []string `json:"inlineEntries"`
}

func (h *hostKeySpec) build() (ssh.HostKeyCallback, func(), error) {
	mode := strings.ToLower(strings.TrimSpace(h.Mode))
	switch mode {
	case "", "insecure":
		return ssh.InsecureIgnoreHostKey(), nil, nil
	case "known_hosts":
		var sources []string
		var cleanupPaths []string
		for _, file := range h.KnownHostsFiles {
			trimmed := strings.TrimSpace(file)
			if trimmed == "" {
				continue
			}
			expanded, err := expandUserPath(trimmed)
			if err != nil {
				return nil, nil, err
			}
			sources = append(sources, expanded)
		}
		if len(h.InlineEntries) > 0 {
			tempFile, err := writeTempKnownHosts(h.InlineEntries)
			if err != nil {
				return nil, nil, err
			}
			sources = append(sources, tempFile)
			cleanupPaths = append(cleanupPaths, tempFile)
		}
		if len(sources) == 0 {
			return nil, nil, errors.New("ssh: hostKey.knownHostsFiles or inlineEntries required when mode is known_hosts")
		}
		callback, err := knownhosts.New(sources...)
		if err != nil {
			return nil, nil, fmt.Errorf("ssh: known_hosts callback: %w", err)
		}
		cleanup := func() {
			for _, path := range cleanupPaths {
				_ = os.Remove(path)
			}
		}
		return callback, cleanup, nil
	default:
		return nil, nil, fmt.Errorf("ssh: unsupported hostKey.mode %q", h.Mode)
	}
}

// authSpec configures authentication.
type authSpec struct {
	Method         string `json:"method"`
	Password       string `json:"password"`
	PrivateKey     string `json:"privateKey"`
	PrivateKeyPEM  string `json:"privateKeyPEM"`
	PrivateKeyPath string `json:"privateKeyPath"`
	Passphrase     string `json:"passphrase"`
}

func (a *authSpec) apply(config *ssh.ClientConfig) error {
	method := strings.ToLower(strings.TrimSpace(a.Method))
	switch method {
	case "password":
		if a.Password == "" {
			return errors.New("ssh: auth.password must be provided when method is password")
		}
		config.Auth = []ssh.AuthMethod{ssh.Password(a.Password)}
	case "private_key", "private-key", "keyfile":
		pem, err := a.privateKeyPEM()
		if err != nil {
			return err
		}
		signer, err := signerFromPEM(pem, a.Passphrase)
		if err != nil {
			return err
		}
		config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	case "private_key_with_passphrase":
		pem, err := a.privateKeyPEM()
		if err != nil {
			return err
		}
		signer, err := signerFromPEM(pem, a.Passphrase)
		if err != nil {
			return err
		}
		config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	case "":
		return errors.New("ssh: auth.method must be supplied")
	default:
		return fmt.Errorf("ssh: unsupported auth.method %q", a.Method)
	}
	return nil
}

func (a *authSpec) privateKeyPEM() (string, error) {
	if strings.TrimSpace(a.PrivateKey) != "" {
		return a.PrivateKey, nil
	}
	if strings.TrimSpace(a.PrivateKeyPEM) != "" {
		return a.PrivateKeyPEM, nil
	}
	if strings.TrimSpace(a.PrivateKeyPath) != "" {
		path := strings.TrimSpace(a.PrivateKeyPath)
		expanded, err := expandUserPath(path)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(expanded)
		if err != nil {
			return "", fmt.Errorf("ssh: read private key %q: %w", a.PrivateKeyPath, err)
		}
		return string(data), nil
	}
	return "", errors.New("ssh: private key data or path must be provided for key authentication")
}

func signerFromPEM(pemData, passphrase string) (ssh.Signer, error) {
	key := strings.TrimSpace(pemData)
	if key == "" {
		return nil, errors.New("ssh: auth.privateKey must be provided for key authentication")
	}
	var (
		signer ssh.Signer
		err    error
	)
	keyBytes := []byte(key)
	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey(keyBytes)
	}
	if err != nil {
		return nil, fmt.Errorf("ssh: parse private key: %w", err)
	}
	return signer, nil
}

func writeTempKnownHosts(entries []string) (string, error) {
	file, err := os.CreateTemp("", "flowk-known_hosts-")
	if err != nil {
		return "", fmt.Errorf("ssh: create temp known_hosts: %w", err)
	}
	for _, line := range entries {
		if _, err := fmt.Fprintln(file, line); err != nil {
			file.Close()
			os.Remove(file.Name())
			return "", fmt.Errorf("ssh: write known hosts entry: %w", err)
		}
	}
	if err := file.Close(); err != nil {
		os.Remove(file.Name())
		return "", fmt.Errorf("ssh: close known hosts temp file: %w", err)
	}
	return file.Name(), nil
}

func keepAliveLoop(client *ssh.Client, interval time.Duration) {
	if client == nil || interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
		if err != nil {
			return
		}
	}
}

type actionState struct {
	client    *sshclient.Client
	spec      payloadSpec
	sftp      *sshclient.RemoteFileSystem
	tempFiles []string
}

func newActionState(client *sshclient.Client, spec payloadSpec) *actionState {
	return &actionState{client: client, spec: spec}
}

func (s *actionState) Close() {
	if s.sftp != nil {
		_ = s.sftp.Close()
	}
	for _, file := range s.tempFiles {
		_ = os.Remove(file)
	}
}

type stepEnvelope struct {
	ID        string `json:"id"`
	Operation string `json:"operation"`
}

type stepResult struct {
	ID        string `json:"id"`
	Operation string `json:"operation"`
	Success   bool   `json:"success"`
	Output    any    `json:"output,omitempty"`
}

func (s *actionState) executeStep(ctx context.Context, idx int, raw json.RawMessage) (stepResult, error) {
	var env stepEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return stepResult{}, fmt.Errorf("ssh: decode step %d: %w", idx, err)
	}
	if env.Operation == "" {
		return stepResult{}, fmt.Errorf("ssh: step %d is missing operation", idx)
	}

	op := strings.ToUpper(env.Operation)
	switch op {
	case "RUN_COMMAND", "RUN_COMMAND_OUTPUT", "RUN_COMMAND_SMART_OUTPUT":
		res, err := s.handleCommandStep(ctx, env, raw, op)
		if err != nil {
			return stepResult{}, err
		}
		return res, nil
	case "RUN_SCRIPT", "RUN_SCRIPT_OUTPUT", "RUN_SCRIPT_SMART_OUTPUT":
		res, err := s.handleScriptStep(ctx, env, raw, op)
		if err != nil {
			return stepResult{}, err
		}
		return res, nil
	case "RUN_SCRIPT_FILE", "RUN_SCRIPT_FILE_OUTPUT", "RUN_SCRIPT_FILE_SMART_OUTPUT":
		res, err := s.handleScriptFileStep(ctx, env, raw, op)
		if err != nil {
			return stepResult{}, err
		}
		return res, nil
	case "EXECUTE_SHELL":
		res, err := s.handleShellStep(ctx, env, raw)
		if err != nil {
			return stepResult{}, err
		}
		return res, nil
	case "SFTP":
		res, err := s.handleSFTPStep(ctx, env, raw)
		if err != nil {
			return stepResult{}, err
		}
		return res, nil
	default:
		return stepResult{}, fmt.Errorf("ssh: unsupported operation %q", env.Operation)
	}
}

type commandStep struct {
	ID               string   `json:"id"`
	Commands         []string `json:"commands"`
	Append           []string `json:"append"`
	Stdout           string   `json:"stdout"`
	Stderr           string   `json:"stderr"`
	AllowedExitCodes []int    `json:"allowedExitCodes"`
}

func (s commandStep) allowsExit(err error) bool {
	if len(s.AllowedExitCodes) == 0 {
		return false
	}

	var exitErr interface{ ExitStatus() int }
	if !errors.As(err, &exitErr) {
		return false
	}

	status := exitErr.ExitStatus()
	for _, allowed := range s.AllowedExitCodes {
		if allowed == status {
			return true
		}
	}
	return false
}

func (s *actionState) handleCommandStep(ctx context.Context, env stepEnvelope, raw json.RawMessage, op string) (stepResult, error) {
	var step commandStep
	if err := json.Unmarshal(raw, &step); err != nil {
		return stepResult{}, fmt.Errorf("ssh: decode command step %q: %w", env.ID, err)
	}
	if len(step.Commands) == 0 {
		return stepResult{}, fmt.Errorf("ssh: step %q commands cannot be empty", env.ID)
	}

	rs := s.client.Cmd(step.Commands[0])
	for _, cmd := range step.Append {
		rs = rs.Cmd(cmd)
	}
	for _, cmd := range step.Commands[1:] {
		rs = rs.Cmd(cmd)
	}

	captureStdout := strings.EqualFold(step.Stdout, "capture")
	captureStderr := strings.EqualFold(step.Stderr, "capture")
	var stdoutBuf, stderrBuf bytes.Buffer
	if captureStdout || captureStderr {
		var stdout io.Writer
		var stderr io.Writer
		if captureStdout {
			stdout = &stdoutBuf
		}
		if captureStderr {
			stderr = &stderrBuf
		}
		rs = rs.SetStdio(stdout, stderr)
	}

	result := stepResult{ID: env.ID, Operation: env.Operation, Success: true}

	switch op {
	case "RUN_COMMAND":
		err := rs.Run()
		if err != nil && !step.allowsExit(err) {
			return stepResult{}, fmt.Errorf("ssh: command run %q failed: %w", env.ID, err)
		}
		if captureStdout || captureStderr {
			result.Output = map[string]any{"stdout": stdoutBuf.String(), "stderr": stderrBuf.String()}
		}
	case "RUN_COMMAND_OUTPUT":
		output, err := rs.Output()
		if err != nil && !step.allowsExit(err) {
			return stepResult{}, fmt.Errorf("ssh: command output %q failed: %w", env.ID, err)
		}
		result.Output = string(output)
	case "RUN_COMMAND_SMART_OUTPUT":
		output, err := rs.SmartOutput()
		if err != nil && !step.allowsExit(err) {
			return stepResult{}, fmt.Errorf("ssh: command smart output %q failed: %w", env.ID, err)
		}
		result.Output = string(output)
	}

	return result, nil
}

type scriptStep struct {
	ID     string `json:"id"`
	Script string `json:"script"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

func (s *actionState) handleScriptStep(ctx context.Context, env stepEnvelope, raw json.RawMessage, op string) (stepResult, error) {
	var step scriptStep
	if err := json.Unmarshal(raw, &step); err != nil {
		return stepResult{}, fmt.Errorf("ssh: decode script step %q: %w", env.ID, err)
	}
	if strings.TrimSpace(step.Script) == "" {
		return stepResult{}, fmt.Errorf("ssh: script step %q requires script content", env.ID)
	}

	rs := s.client.Script(step.Script)
	captureStdout := strings.EqualFold(step.Stdout, "capture")
	captureStderr := strings.EqualFold(step.Stderr, "capture")
	var stdoutBuf, stderrBuf bytes.Buffer
	if captureStdout || captureStderr {
		var stdout io.Writer
		var stderr io.Writer
		if captureStdout {
			stdout = &stdoutBuf
		}
		if captureStderr {
			stderr = &stderrBuf
		}
		rs = rs.SetStdio(stdout, stderr)
	}

	result := stepResult{ID: env.ID, Operation: env.Operation, Success: true}
	switch op {
	case "RUN_SCRIPT":
		if err := rs.Run(); err != nil {
			return stepResult{}, fmt.Errorf("ssh: script run %q failed: %w", env.ID, err)
		}
		if captureStdout || captureStderr {
			result.Output = map[string]any{"stdout": stdoutBuf.String(), "stderr": stderrBuf.String()}
		}
	case "RUN_SCRIPT_OUTPUT":
		output, err := rs.Output()
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: script output %q failed: %w", env.ID, err)
		}
		result.Output = string(output)
	case "RUN_SCRIPT_SMART_OUTPUT":
		output, err := rs.SmartOutput()
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: script smart output %q failed: %w", env.ID, err)
		}
		result.Output = string(output)
	}
	return result, nil
}

type scriptFileStep struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

func (s *actionState) handleScriptFileStep(ctx context.Context, env stepEnvelope, raw json.RawMessage, op string) (stepResult, error) {
	var step scriptFileStep
	if err := json.Unmarshal(raw, &step); err != nil {
		return stepResult{}, fmt.Errorf("ssh: decode script file step %q: %w", env.ID, err)
	}
	if strings.TrimSpace(step.Path) == "" {
		return stepResult{}, fmt.Errorf("ssh: script file step %q requires path", env.ID)
	}

	abs, err := filepath.Abs(step.Path)
	if err != nil {
		return stepResult{}, fmt.Errorf("ssh: resolve path %q: %w", step.Path, err)
	}

	rs := s.client.ScriptFile(abs)
	result := stepResult{ID: env.ID, Operation: env.Operation, Success: true}
	switch op {
	case "RUN_SCRIPT_FILE":
		if err := rs.Run(); err != nil {
			return stepResult{}, fmt.Errorf("ssh: script file run %q failed: %w", env.ID, err)
		}
	case "RUN_SCRIPT_FILE_OUTPUT":
		output, err := rs.Output()
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: script file output %q failed: %w", env.ID, err)
		}
		result.Output = string(output)
	case "RUN_SCRIPT_FILE_SMART_OUTPUT":
		output, err := rs.SmartOutput()
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: script file smart output %q failed: %w", env.ID, err)
		}
		result.Output = string(output)
	}
	return result, nil
}

type shellStep struct {
	ID            string        `json:"id"`
	Input         string        `json:"input"`
	RequestPTY    bool          `json:"requestPty"`
	Terminal      *terminalSpec `json:"terminal"`
	CaptureStdout bool          `json:"captureStdout"`
	CaptureStderr bool          `json:"captureStderr"`
}

type terminalSpec struct {
	Term   string            `json:"term"`
	Height int               `json:"height"`
	Width  int               `json:"width"`
	Modes  map[string]uint32 `json:"modes"`
}

func (s *actionState) handleShellStep(ctx context.Context, env stepEnvelope, raw json.RawMessage) (stepResult, error) {
	var step shellStep
	if err := json.Unmarshal(raw, &step); err != nil {
		return stepResult{}, fmt.Errorf("ssh: decode shell step %q: %w", env.ID, err)
	}

	var shell *sshclient.RemoteShell
	if step.RequestPTY {
		shell = s.client.Terminal(step.Terminal.toConfig())
	} else {
		shell = s.client.Shell()
	}

	var (
		stdin     io.Reader
		stdoutBuf bytes.Buffer
		stderrBuf bytes.Buffer
		stdout    io.Writer
		stderr    io.Writer
	)

	if step.Input != "" {
		stdin = strings.NewReader(step.Input)
	}
	if step.CaptureStdout {
		stdout = &stdoutBuf
	}
	if step.CaptureStderr {
		stderr = &stderrBuf
	}
	shell = shell.SetStdio(stdin, stdout, stderr)

	result := stepResult{ID: env.ID, Operation: env.Operation, Success: true}
	if err := shell.Start(); err != nil {
		return stepResult{}, fmt.Errorf("ssh: shell step %q failed: %w", env.ID, err)
	}
	if step.CaptureStdout || step.CaptureStderr {
		result.Output = map[string]any{
			"stdout": stdoutBuf.String(),
			"stderr": stderrBuf.String(),
		}
	}
	return result, nil
}

func (t *terminalSpec) toConfig() *sshclient.TerminalConfig {
	if t == nil {
		return nil
	}
	modes := ssh.TerminalModes{}
	for key, value := range t.Modes {
		parsed, err := strconv.ParseUint(key, 0, 8)
		if err != nil {
			continue
		}
		modes[uint8(parsed)] = value
	}
	return &sshclient.TerminalConfig{
		Term:   chooseNonEmpty(t.Term, "xterm"),
		Height: choosePositive(t.Height, 40),
		Weight: choosePositive(t.Width, 80),
		Modes:  modes,
	}
}

type sftpOptionsSpec struct {
	MaxConcurrentRequestsPerFile int   `json:"maxConcurrentRequestsPerFile"`
	MaxPacket                    int   `json:"maxPacket"`
	ConcurrentReads              *bool `json:"concurrentReads"`
	ConcurrentWrites             *bool `json:"concurrentWrites"`
	UseFstat                     *bool `json:"useFstat"`
}

type sftpStep struct {
	ID     string         `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

func (s *actionState) handleSFTPStep(ctx context.Context, env stepEnvelope, raw json.RawMessage) (stepResult, error) {
	var step sftpStep
	if err := json.Unmarshal(raw, &step); err != nil {
		return stepResult{}, fmt.Errorf("ssh: decode sftp step %q: %w", env.ID, err)
	}
	if step.Method == "" {
		return stepResult{}, fmt.Errorf("ssh: sftp step %q requires method", env.ID)
	}

	rfs, err := s.ensureSFTP()
	if err != nil {
		return stepResult{}, err
	}

	method := strings.ToUpper(step.Method)
	result := stepResult{ID: env.ID, Operation: env.Operation, Success: true}
	switch method {
	case "CHMOD":
		path := stringValue(step.Params, "path")
		modeStr := stringValue(step.Params, "mode")
		if path == "" || modeStr == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp chmod %q requires path and mode", env.ID)
		}
		mode, err := parseFileMode(modeStr)
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: parse mode: %w", err)
		}
		if err := rfs.Chmod(path, mode); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp chmod %q failed: %w", env.ID, err)
		}
	case "CHOWN":
		path := stringValue(step.Params, "path")
		uid := intValue(step.Params, "uid")
		gid := intValue(step.Params, "gid")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp chown %q requires path", env.ID)
		}
		if err := rfs.Chown(path, uid, gid); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp chown %q failed: %w", env.ID, err)
		}
	case "CHTIMES":
		path := stringValue(step.Params, "path")
		atimeStr := stringValue(step.Params, "atime")
		mtimeStr := stringValue(step.Params, "mtime")
		if path == "" || atimeStr == "" || mtimeStr == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp chtimes %q requires path, atime, mtime", env.ID)
		}
		atime, err := time.Parse(time.RFC3339, atimeStr)
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: parse atime: %w", err)
		}
		mtime, err := time.Parse(time.RFC3339, mtimeStr)
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: parse mtime: %w", err)
		}
		if err := rfs.Chtimes(path, atime, mtime); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp chtimes %q failed: %w", env.ID, err)
		}
	case "CREATE":
		path := stringValue(step.Params, "path")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp create %q requires path", env.ID)
		}
		file, err := rfs.Create(path)
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp create %q failed: %w", env.ID, err)
		}
		defer file.Close()
		data := stringValue(step.Params, "content")
		if data != "" {
			if _, err := file.Write([]byte(data)); err != nil {
				return stepResult{}, fmt.Errorf("ssh: write created file: %w", err)
			}
		}
	case "DOWNLOAD":
		remote := stringValue(step.Params, "remotePath")
		local := stringValue(step.Params, "localPath")
		if remote == "" || local == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp download %q requires remotePath and localPath", env.ID)
		}
		if err := rfs.Download(remote, local); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp download %q failed: %w", env.ID, err)
		}
	case "GETWD":
		wd, err := rfs.Getwd()
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp getwd %q failed: %w", env.ID, err)
		}
		result.Output = wd
	case "GLOB":
		pattern := stringValue(step.Params, "pattern")
		if pattern == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp glob %q requires pattern", env.ID)
		}
		matches, err := rfs.Glob(pattern)
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp glob %q failed: %w", env.ID, err)
		}
		result.Output = matches
	case "LINK":
		oldname := stringValue(step.Params, "old")
		newname := stringValue(step.Params, "new")
		if oldname == "" || newname == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp link %q requires old and new", env.ID)
		}
		if err := rfs.Link(oldname, newname); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp link %q failed: %w", env.ID, err)
		}
	case "LSTAT":
		path := stringValue(step.Params, "path")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp lstat %q requires path", env.ID)
		}
		info, err := rfs.Lstat(path)
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp lstat %q failed: %w", env.ID, err)
		}
		result.Output = fileInfoToMap(info)
	case "MKDIR":
		path := stringValue(step.Params, "path")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp mkdir %q requires path", env.ID)
		}
		if err := rfs.Mkdir(path); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp mkdir %q failed: %w", env.ID, err)
		}
	case "MKDIR_ALL":
		path := stringValue(step.Params, "path")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp mkdir_all %q requires path", env.ID)
		}
		if err := rfs.MkdirAll(path); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp mkdir_all %q failed: %w", env.ID, err)
		}
	case "OPEN":
		path := stringValue(step.Params, "path")
		mode := strings.ToLower(stringValue(step.Params, "mode"))
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp open %q requires path", env.ID)
		}
		file, err := rfs.Open(path)
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp open %q failed: %w", env.ID, err)
		}
		defer file.Close()
		switch mode {
		case "", "stat":
			info, err := file.Stat()
			if err != nil {
				return stepResult{}, fmt.Errorf("ssh: sftp open stat %q failed: %w", env.ID, err)
			}
			result.Output = fileInfoToMap(info)
		case "read":
			data, err := io.ReadAll(file)
			if err != nil {
				return stepResult{}, fmt.Errorf("ssh: sftp open read %q failed: %w", env.ID, err)
			}
			result.Output = string(data)
		}
	case "OPEN_FILE":
		path := stringValue(step.Params, "path")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp open_file %q requires path", env.ID)
		}
		flags := parseFileFlags(step.Params["flags"])
		file, err := rfs.OpenFile(path, flags)
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp open_file %q failed: %w", env.ID, err)
		}
		defer file.Close()
		data := stringValue(step.Params, "write")
		if data != "" {
			if _, err := file.Write([]byte(data)); err != nil {
				return stepResult{}, fmt.Errorf("ssh: sftp open_file write %q failed: %w", env.ID, err)
			}
		}
		if readMode := strings.ToLower(stringValue(step.Params, "read")); readMode == "all" {
			payload, err := io.ReadAll(file)
			if err != nil {
				return stepResult{}, fmt.Errorf("ssh: sftp open_file read %q failed: %w", env.ID, err)
			}
			result.Output = string(payload)
		}
	case "POSIX_RENAME":
		oldname := stringValue(step.Params, "old")
		newname := stringValue(step.Params, "new")
		if oldname == "" || newname == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp posix_rename %q requires old and new", env.ID)
		}
		if err := rfs.PosixRename(oldname, newname); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp posix_rename %q failed: %w", env.ID, err)
		}
	case "READ_DIR":
		path := stringValue(step.Params, "path")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp read_dir %q requires path", env.ID)
		}
		entries, err := rfs.ReadDir(path)
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp read_dir %q failed: %w", env.ID, err)
		}
		result.Output = fileInfosToSlice(entries)
	case "READ_FILE":
		path := stringValue(step.Params, "path")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp read_file %q requires path", env.ID)
		}
		data, err := rfs.ReadFile(path)
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp read_file %q failed: %w", env.ID, err)
		}
		result.Output = string(data)
	case "READ_LINK":
		path := stringValue(step.Params, "path")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp read_link %q requires path", env.ID)
		}
		link, err := rfs.ReadLink(path)
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp read_link %q failed: %w", env.ID, err)
		}
		result.Output = link
	case "REAL_PATH":
		path := stringValue(step.Params, "path")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp real_path %q requires path", env.ID)
		}
		resolved, err := rfs.RealPath(path)
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp real_path %q failed: %w", env.ID, err)
		}
		result.Output = resolved
	case "REMOVE":
		path := stringValue(step.Params, "path")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp remove %q requires path", env.ID)
		}
		if err := rfs.Remove(path); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp remove %q failed: %w", env.ID, err)
		}
	case "REMOVE_DIRECTORY":
		path := stringValue(step.Params, "path")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp remove_directory %q requires path", env.ID)
		}
		if err := rfs.RemoveDirectory(path); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp remove_directory %q failed: %w", env.ID, err)
		}
	case "RENAME":
		oldname := stringValue(step.Params, "old")
		newname := stringValue(step.Params, "new")
		if oldname == "" || newname == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp rename %q requires old and new", env.ID)
		}
		if err := rfs.Rename(oldname, newname); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp rename %q failed: %w", env.ID, err)
		}
	case "STAT":
		path := stringValue(step.Params, "path")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp stat %q requires path", env.ID)
		}
		info, err := rfs.Stat(path)
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp stat %q failed: %w", env.ID, err)
		}
		result.Output = fileInfoToMap(info)
	case "STAT_VFS":
		path := stringValue(step.Params, "path")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp stat_vfs %q requires path", env.ID)
		}
		stat, err := rfs.StatVFS(path)
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp stat_vfs %q failed: %w", env.ID, err)
		}
		result.Output = statVFSMap(stat)
	case "SYMLINK":
		oldname := stringValue(step.Params, "old")
		newname := stringValue(step.Params, "new")
		if oldname == "" || newname == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp symlink %q requires old and new", env.ID)
		}
		if err := rfs.Symlink(oldname, newname); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp symlink %q failed: %w", env.ID, err)
		}
	case "TRUNCATE":
		path := stringValue(step.Params, "path")
		size := int64Value(step.Params, "size")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp truncate %q requires path", env.ID)
		}
		if err := rfs.Truncate(path, size); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp truncate %q failed: %w", env.ID, err)
		}
	case "UPLOAD":
		local := stringValue(step.Params, "localPath")
		remote := stringValue(step.Params, "remotePath")
		if remote == "" || local == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp upload %q requires remotePath and localPath", env.ID)
		}
		if err := rfs.Upload(local, remote); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp upload %q failed: %w", env.ID, err)
		}
	case "WAIT":
		if err := rfs.Wait(); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp wait %q failed: %w", env.ID, err)
		}
	case "WALK":
		root := stringValue(step.Params, "root")
		if root == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp walk %q requires root", env.ID)
		}
		walker, err := rfs.Walk(root)
		if err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp walk %q failed: %w", env.ID, err)
		}
		result.Output = walkToSlice(walker)
	case "WRITE_FILE":
		path := stringValue(step.Params, "path")
		data := stringValue(step.Params, "content")
		modeStr := stringValue(step.Params, "mode")
		if path == "" {
			return stepResult{}, fmt.Errorf("ssh: sftp write_file %q requires path", env.ID)
		}
		perm := os.FileMode(0644)
		if modeStr != "" {
			mode, err := parseFileMode(modeStr)
			if err != nil {
				return stepResult{}, fmt.Errorf("ssh: parse mode: %w", err)
			}
			perm = mode
		}
		if err := rfs.WriteFile(path, []byte(data), perm); err != nil {
			return stepResult{}, fmt.Errorf("ssh: sftp write_file %q failed: %w", env.ID, err)
		}
	default:
		return stepResult{}, fmt.Errorf("ssh: unsupported sftp method %q", step.Method)
	}

	return result, nil
}

func (s *actionState) ensureSFTP() (*sshclient.RemoteFileSystem, error) {
	if s.sftp != nil {
		return s.sftp, nil
	}

	var opts []sshclient.SftpOption
	if s.spec.SFTP != nil {
		if s.spec.SFTP.MaxConcurrentRequestsPerFile > 0 {
			opts = append(opts, sshclient.SftpMaxConcurrentRequestsPerFile(s.spec.SFTP.MaxConcurrentRequestsPerFile))
		}
		if s.spec.SFTP.MaxPacket > 0 {
			opts = append(opts, sshclient.SftpMaxPacket(s.spec.SFTP.MaxPacket))
		}
		if s.spec.SFTP.ConcurrentReads != nil {
			opts = append(opts, sshclient.SftpUseConcurrentReads(*s.spec.SFTP.ConcurrentReads))
		}
		if s.spec.SFTP.ConcurrentWrites != nil {
			opts = append(opts, sshclient.SftpUseConcurrentWrites(*s.spec.SFTP.ConcurrentWrites))
		}
		if s.spec.SFTP.UseFstat != nil {
			opts = append(opts, sshclient.SftpUseFstat(*s.spec.SFTP.UseFstat))
		}
	}

	sftp := s.client.Sftp(opts...)
	s.sftp = sftp
	return sftp, nil
}

func stringValue(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case string:
			return v
		case fmt.Stringer:
			return v.String()
		default:
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

func intValue(params map[string]any, key string) int {
	if params == nil {
		return 0
	}
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case float64:
			return int(v)
		case int:
			return v
		case json.Number:
			n, _ := v.Int64()
			return int(n)
		case string:
			i, _ := strconv.Atoi(v)
			return i
		}
	}
	return 0
}

func int64Value(params map[string]any, key string) int64 {
	if params == nil {
		return 0
	}
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case float64:
			return int64(v)
		case int64:
			return v
		case int:
			return int64(v)
		case json.Number:
			n, _ := v.Int64()
			return n
		case string:
			i, _ := strconv.ParseInt(v, 10, 64)
			return i
		}
	}
	return 0
}

func parseFileMode(value string) (os.FileMode, error) {
	val := strings.TrimSpace(value)
	if val == "" {
		return 0, errors.New("empty mode")
	}
	if strings.HasPrefix(val, "0") {
		parsed, err := strconv.ParseUint(val, 8, 32)
		if err != nil {
			return 0, err
		}
		return os.FileMode(parsed), nil
	}
	parsed, err := strconv.ParseUint(val, 10, 32)
	if err != nil {
		return 0, err
	}
	return os.FileMode(parsed), nil
}

func parseFileFlags(raw any) int {
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case string:
		parts := strings.Split(v, "|")
		flags := 0
		for _, part := range parts {
			switch strings.TrimSpace(strings.ToUpper(part)) {
			case "O_RDONLY":
				flags |= os.O_RDONLY
			case "O_WRONLY":
				flags |= os.O_WRONLY
			case "O_RDWR":
				flags |= os.O_RDWR
			case "O_CREATE":
				flags |= os.O_CREATE
			case "O_APPEND":
				flags |= os.O_APPEND
			case "O_TRUNC":
				flags |= os.O_TRUNC
			case "O_EXCL":
				flags |= os.O_EXCL
			case "O_SYNC":
				flags |= os.O_SYNC
			}
		}
		return flags
	case []any:
		flags := 0
		for _, item := range v {
			flags |= parseFileFlags(item)
		}
		return flags
	default:
		return 0
	}
}

func fileInfoToMap(info os.FileInfo) map[string]any {
	if info == nil {
		return nil
	}
	return map[string]any{
		"name":    info.Name(),
		"size":    info.Size(),
		"mode":    info.Mode().String(),
		"perm":    fmt.Sprintf("%#o", info.Mode().Perm()),
		"modTime": info.ModTime().Format(time.RFC3339),
		"isDir":   info.IsDir(),
	}
}

func fileInfosToSlice(infos []os.FileInfo) []map[string]any {
	result := make([]map[string]any, 0, len(infos))
	for _, info := range infos {
		result = append(result, fileInfoToMap(info))
	}
	return result
}

func walkToSlice(walker *fs.Walker) []map[string]any {
	if walker == nil {
		return nil
	}
	var result []map[string]any
	for walker.Step() {
		entry := map[string]any{
			"path": walker.Path(),
		}
		if err := walker.Err(); err != nil {
			entry["error"] = err.Error()
		}
		entry["info"] = fileInfoToMap(walker.Stat())
		result = append(result, entry)
	}
	return result
}

func statVFSMap(stat *sshclient.StatVFS) map[string]any {
	if stat == nil || stat.StatVFS == nil {
		return nil
	}
	s := stat.StatVFS
	return map[string]any{
		"bsize":   s.Bsize,
		"frsize":  s.Frsize,
		"blocks":  s.Blocks,
		"bfree":   s.Bfree,
		"bavail":  s.Bavail,
		"files":   s.Files,
		"ffree":   s.Ffree,
		"favail":  s.Favail,
		"fsid":    s.Fsid,
		"flag":    s.Flag,
		"namemax": s.Namemax,
	}
}

func chooseNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func choosePositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
