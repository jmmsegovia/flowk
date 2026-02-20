package telnet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"

	gotelnet "github.com/reiver/go-telnet"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

func init() {
	registry.Register(&Action{})
}

// Action implements a basic TELNET client workflow.
type Action struct{}

// Name returns the registry identifier for the TELNET action.
func (a *Action) Name() string {
	return "TELNET"
}

// Execute runs the TELNET workflow described in the payload.
func (a *Action) Execute(ctx context.Context, payload json.RawMessage, execCtx *registry.ExecutionContext) (registry.Result, error) {
	var spec payloadSpec
	if err := json.Unmarshal(payload, &spec); err != nil {
		return registry.Result{}, fmt.Errorf("telnet: decode payload: %w", err)
	}

	spec.applyDefaults()
	if err := spec.validate(); err != nil {
		return registry.Result{}, err
	}

	lineEnding, err := resolveLineEnding(spec.LineEnding)
	if err != nil {
		return registry.Result{}, err
	}

	var (
		conn       net.Conn
		transcript strings.Builder
		captures   = make(map[string]string)
		connected  bool
		pending    string
	)

	closeConn := func() {
		if conn != nil {
			_ = conn.Close()
			conn = nil
		}
	}
	defer closeConn()

	for idx, step := range spec.Steps {
		select {
		case <-ctx.Done():
			return registry.Result{}, ctx.Err()
		default:
		}

		switch {
		case step.Connect != nil:
			if conn != nil {
				return registry.Result{}, fmt.Errorf("telnet: connect step encountered after connection already open (step %d)", idx)
			}

			dialTimeout := spec.TimeoutSeconds
			if dialTimeout <= 0 {
				dialTimeout = defaultTimeoutSeconds
			}

			d := &net.Dialer{}
			dialCtx, cancel := context.WithTimeout(ctx, time.Duration(dialTimeout*float64(time.Second)))
			connAddr := fmt.Sprintf("%s:%d", spec.Host, spec.Port)
			if execCtx != nil && execCtx.Logger != nil {
				execCtx.Logger.Printf("TELNET: connecting to %s", connAddr)
			}

			c, err := d.DialContext(dialCtx, "tcp", connAddr)
			cancel()
			if err != nil {
				return registry.Result{}, fmt.Errorf("telnet: connect %s: %w", connAddr, err)
			}

			conn = c

			// Establish typed handles through the go-telnet interfaces for clarity.
			var _ gotelnet.Reader = conn
			var _ gotelnet.Writer = conn

			connected = true
			transcript.WriteString(fmt.Sprintf("Connected to %s\n", connAddr))

		case step.Send != nil:
			if conn == nil {
				return registry.Result{}, fmt.Errorf("telnet: send step requires an open connection (step %d)", idx)
			}

			sendData := step.Send.Data
			appendLine := true
			if step.Send.AppendLineEnding != nil {
				appendLine = *step.Send.AppendLineEnding
			}
			if appendLine {
				sendData += lineEnding
			}

			writeDeadline := computeDeadline(ctx, spec.TimeoutSeconds)
			if err := conn.SetWriteDeadline(writeDeadline); err != nil {
				return registry.Result{}, fmt.Errorf("telnet: set write deadline: %w", err)
			}

			if execCtx != nil && execCtx.Logger != nil {
				logData := step.Send.Data
				if step.Send.Mask {
					logData = "****"
				}
				execCtx.Logger.Printf("TELNET: send %q", logData)
			}

			if _, err := conn.Write([]byte(sendData)); err != nil {
				return registry.Result{}, fmt.Errorf("telnet: write failed: %w", err)
			}

			// Clear the deadline for future operations.
			_ = conn.SetWriteDeadline(time.Time{})

			if step.Send.Mask {
				transcript.WriteString("****")
			} else {
				transcript.WriteString(step.Send.Data)
			}
			if appendLine {
				transcript.WriteString(normalizeLineEndingForTranscript(lineEnding))
			}

		case step.Expect != nil:
			if conn == nil {
				return registry.Result{}, fmt.Errorf("telnet: expect step requires an open connection (step %d)", idx)
			}

			re, err := regexp.Compile(step.Expect.Pattern)
			if err != nil {
				return registry.Result{}, fmt.Errorf("telnet: invalid pattern %q: %w", step.Expect.Pattern, err)
			}

			timeout := spec.ReadTimeoutSeconds
			if step.Expect.TimeoutSeconds != nil && *step.Expect.TimeoutSeconds > 0 {
				timeout = *step.Expect.TimeoutSeconds
			}
			if timeout <= 0 {
				timeout = defaultReadTimeoutSeconds
			}

			deadline := computeDeadline(ctx, timeout)
			if err := conn.SetReadDeadline(deadline); err != nil {
				return registry.Result{}, fmt.Errorf("telnet: set read deadline: %w", err)
			}

			buffer := &strings.Builder{}
			if pending != "" {
				buffer.WriteString(pending)
				pending = ""
			}

			matchFound := false
			applyMatch := func(buf string) {
				if matchFound {
					return
				}
				if idx := re.FindStringSubmatchIndex(buf); idx != nil {
					matchFound = true
					if step.Expect.Capture != "" && len(idx) >= 4 {
						captures[step.Expect.Capture] = buf[idx[2]:idx[3]]
					}
					pending = buf[idx[1]:]
				}
			}

			if buffer.Len() > 0 {
				applyMatch(buffer.String())
			}

			temp := make([]byte, 1024)

			for !matchFound {
				select {
				case <-ctx.Done():
					return registry.Result{}, ctx.Err()
				default:
				}

				n, err := conn.Read(temp)
				if n > 0 {
					chunk := string(temp[:n])
					buffer.WriteString(chunk)
					transcript.WriteString(chunk)
					applyMatch(buffer.String())
				}

				if matchFound {
					break
				}

				if err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						return registry.Result{}, fmt.Errorf("telnet: expect timeout waiting for %q", step.Expect.Pattern)
					}
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						return registry.Result{}, err
					}
					if errors.Is(err, net.ErrClosed) {
						return registry.Result{}, fmt.Errorf("telnet: connection closed while waiting for %q", step.Expect.Pattern)
					}
					if errors.Is(err, io.EOF) {
						return registry.Result{}, fmt.Errorf("telnet: received EOF while waiting for %q", step.Expect.Pattern)
					}
					return registry.Result{}, fmt.Errorf("telnet: read error: %w", err)
				}
			}

			// Clear read deadline for subsequent steps.
			_ = conn.SetReadDeadline(time.Time{})

			if !matchFound {
				return registry.Result{}, fmt.Errorf("telnet: pattern %q not observed", step.Expect.Pattern)
			}

		case step.Close != nil:
			if conn == nil {
				if execCtx != nil && execCtx.Logger != nil {
					execCtx.Logger.Printf("TELNET: close requested but connection already closed")
				}
				continue
			}
			if execCtx != nil && execCtx.Logger != nil {
				execCtx.Logger.Printf("TELNET: closing connection")
			}
			if err := conn.Close(); err != nil {
				return registry.Result{}, fmt.Errorf("telnet: close: %w", err)
			}
			conn = nil

		default:
			return registry.Result{}, fmt.Errorf("telnet: step %d does not define a valid operation", idx)
		}
	}

	if conn != nil {
		closeConn()
	}

	result := map[string]any{
		"connected":  connected,
		"host":       spec.Host,
		"port":       spec.Port,
		"captures":   captures,
		"transcript": transcript.String(),
	}

	return registry.Result{Value: result, Type: flow.ResultTypeJSON}, nil
}

const (
	defaultTimeoutSeconds     = 10.0
	defaultReadTimeoutSeconds = 5.0
)

type payloadSpec struct {
	Host               string     `json:"host"`
	Port               int        `json:"port"`
	TimeoutSeconds     float64    `json:"timeoutSeconds"`
	ReadTimeoutSeconds float64    `json:"readTimeoutSeconds"`
	LineEnding         string     `json:"lineEnding"`
	Steps              []stepSpec `json:"steps"`
}

type stepSpec struct {
	Connect *struct{}   `json:"connect,omitempty"`
	Send    *sendSpec   `json:"send,omitempty"`
	Expect  *expectSpec `json:"expect,omitempty"`
	Close   *struct{}   `json:"close,omitempty"`
}

type sendSpec struct {
	Data             string `json:"data"`
	AppendLineEnding *bool  `json:"appendLineEnding"`
	Mask             bool   `json:"mask"`
}

type expectSpec struct {
	Pattern        string   `json:"pattern"`
	TimeoutSeconds *float64 `json:"timeoutSeconds"`
	Capture        string   `json:"capture"`
}

func (p *payloadSpec) applyDefaults() {
	if p.Port == 0 {
		p.Port = 23
	}
	if p.TimeoutSeconds <= 0 {
		p.TimeoutSeconds = defaultTimeoutSeconds
	}
	if p.ReadTimeoutSeconds <= 0 {
		p.ReadTimeoutSeconds = defaultReadTimeoutSeconds
	}
	if strings.TrimSpace(p.LineEnding) == "" {
		p.LineEnding = "CRLF"
	}
}

func (p *payloadSpec) validate() error {
	if strings.TrimSpace(p.Host) == "" {
		return fmt.Errorf("telnet: host is required")
	}
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("telnet: port %d out of range", p.Port)
	}
	if len(p.Steps) == 0 {
		return fmt.Errorf("telnet: steps cannot be empty")
	}
	if p.Steps[0].Connect == nil {
		return fmt.Errorf("telnet: first step must be connect")
	}
	for idx, step := range p.Steps {
		count := 0
		if step.Connect != nil {
			count++
		}
		if step.Send != nil {
			count++
		}
		if step.Expect != nil {
			count++
			if strings.TrimSpace(step.Expect.Pattern) == "" {
				return fmt.Errorf("telnet: step %d expect requires pattern", idx)
			}
		}
		if step.Close != nil {
			count++
		}
		if count != 1 {
			return fmt.Errorf("telnet: step %d must define exactly one operation", idx)
		}
	}
	return nil
}

func resolveLineEnding(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "CRLF":
		return "\r\n", nil
	case "LF":
		return "\n", nil
	case "CR":
		return "\r", nil
	default:
		return "", fmt.Errorf("telnet: unsupported line ending %q", value)
	}
}

func computeDeadline(ctx context.Context, seconds float64) time.Time {
	timeout := time.Duration(seconds * float64(time.Second))
	if timeout <= 0 {
		timeout = time.Duration(defaultTimeoutSeconds * float64(time.Second))
	}
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	return deadline
}

func normalizeLineEndingForTranscript(ending string) string {
	switch ending {
	case "\r\n":
		return "\n"
	case "\r":
		return "\r"
	default:
		return "\n"
	}
}
