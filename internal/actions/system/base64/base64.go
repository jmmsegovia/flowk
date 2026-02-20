package base64

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"flowk/internal/actions/registry"
)

const (
	ActionName = "BASE64"

	OperationEncode = "ENCODE"
	OperationDecode = "DECODE"
)

type Payload struct {
	Operation     string `json:"operation"`
	Input         string `json:"input"`
	InputFile     string `json:"inputFile"`
	OutputFile    string `json:"outputFile"`
	Wrap          *int   `json:"wrap"`
	IgnoreGarbage bool   `json:"ignoreGarbage"`
	URLSafe       bool   `json:"urlSafe"`
}

type ExecutionResult struct {
	Command         []string `json:"command"`
	Operation       string   `json:"operation"`
	InputFile       string   `json:"inputFile,omitempty"`
	OutputFile      string   `json:"outputFile,omitempty"`
	ExitCode        int      `json:"exitCode"`
	Stdout          string   `json:"stdout"`
	Stderr          string   `json:"stderr"`
	DurationSeconds float64  `json:"durationSeconds"`
}

const defaultWrapWidth = 76

func (p *Payload) Validate() error {
	p.Operation = strings.ToUpper(strings.TrimSpace(p.Operation))
	p.InputFile = strings.TrimSpace(p.InputFile)
	p.OutputFile = strings.TrimSpace(p.OutputFile)

	switch p.Operation {
	case OperationEncode, OperationDecode:
	default:
		return fmt.Errorf("base64 task: unsupported operation %q", p.Operation)
	}

	hasInput := p.Input != ""
	hasInputFile := p.InputFile != ""
	if hasInput == hasInputFile {
		return fmt.Errorf("base64 task: exactly one of input or inputFile is required")
	}

	if p.Wrap != nil {
		if *p.Wrap < 0 || *p.Wrap > 4096 {
			return fmt.Errorf("base64 task: wrap must be between 0 and 4096")
		}
		if p.Operation == OperationDecode {
			return fmt.Errorf("base64 task: wrap is only supported for ENCODE operation")
		}
	}

	if p.IgnoreGarbage && p.Operation != OperationDecode {
		return fmt.Errorf("base64 task: ignoreGarbage is only supported for DECODE operation")
	}
	if p.URLSafe && p.Operation != OperationEncode {
		return fmt.Errorf("base64 task: urlSafe is only supported for ENCODE operation")
	}

	return nil
}

func Execute(ctx context.Context, spec Payload, execCtx *registry.ExecutionContext) (ExecutionResult, error) {
	result := ExecutionResult{
		Command:    []string{"encoding/base64", strings.ToLower(spec.Operation)},
		Operation:  spec.Operation,
		InputFile:  spec.InputFile,
		OutputFile: spec.OutputFile,
	}
	started := time.Now()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, fmt.Errorf("base64: operation interrupted: %w", ctxErr)
	}

	inputBytes, err := loadInputBytes(spec)
	if err != nil {
		return result, err
	}

	var output string
	switch spec.Operation {
	case OperationEncode:
		output = encodeBase64(inputBytes, spec.Wrap)
		if spec.URLSafe {
			output = toBase64URL(output)
		}
	case OperationDecode:
		decoded, decodeErr := decodeBase64(inputBytes, spec.IgnoreGarbage)
		if decodeErr != nil {
			result.ExitCode = 1
			result.Stderr = decodeErr.Error()
			result.DurationSeconds = time.Since(started).Seconds()
			return result, decodeErr
		}
		output = string(decoded)
	default:
		return result, fmt.Errorf("base64: unsupported operation %q", spec.Operation)
	}

	result.Stdout = output
	result.ExitCode = 0
	result.DurationSeconds = time.Since(started).Seconds()

	if spec.OutputFile != "" {
		if err := os.WriteFile(spec.OutputFile, []byte(result.Stdout), 0o600); err != nil {
			return result, fmt.Errorf("base64: writing output file: %w", err)
		}
	}

	return result, nil
}

func toBase64URL(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.ReplaceAll(trimmed, "\n", "")
	trimmed = strings.ReplaceAll(trimmed, "\r", "")
	trimmed = strings.ReplaceAll(trimmed, "+", "-")
	trimmed = strings.ReplaceAll(trimmed, "/", "_")
	trimmed = strings.TrimRight(trimmed, "=")
	return trimmed
}

func filterBase64Input(value []byte) []byte {
	if len(value) == 0 {
		return value
	}
	filtered := make([]byte, 0, len(value))
	for _, b := range value {
		switch {
		case b >= 'A' && b <= 'Z':
			filtered = append(filtered, b)
		case b >= 'a' && b <= 'z':
			filtered = append(filtered, b)
		case b >= '0' && b <= '9':
			filtered = append(filtered, b)
		case b == '+', b == '/', b == '=':
			filtered = append(filtered, b)
		}
	}
	return filtered
}

func loadInputBytes(spec Payload) ([]byte, error) {
	if spec.Input != "" {
		return []byte(spec.Input), nil
	}
	if spec.InputFile == "" {
		return nil, fmt.Errorf("base64: input or inputFile is required")
	}
	data, err := os.ReadFile(spec.InputFile)
	if err != nil {
		return nil, fmt.Errorf("base64: reading input file: %w", err)
	}
	return data, nil
}

func encodeBase64(input []byte, wrap *int) string {
	encoded := base64.StdEncoding.EncodeToString(input)
	width := defaultWrapWidth
	if wrap != nil {
		width = *wrap
	}
	if width <= 0 {
		return encoded
	}
	return wrapLines(encoded, width)
}

func wrapLines(value string, width int) string {
	if width <= 0 || value == "" {
		return value
	}
	var builder strings.Builder
	builder.Grow(len(value) + (len(value)/width) + 1)
	for i := 0; i < len(value); i += width {
		end := i + width
		if end > len(value) {
			end = len(value)
		}
		builder.WriteString(value[i:end])
		builder.WriteByte('\n')
	}
	return builder.String()
}

func decodeBase64(input []byte, ignoreGarbage bool) ([]byte, error) {
	payload := strings.ReplaceAll(string(input), "\n", "")
	payload = strings.ReplaceAll(payload, "\r", "")
	if ignoreGarbage {
		payload = string(filterBase64Input([]byte(payload)))
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("base64: decode input: %w", err)
	}
	return decoded, nil
}
