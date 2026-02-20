package base64

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

func TestPayloadValidate(t *testing.T) {
	t.Parallel()

	wrap := 76
	tests := []struct {
		name    string
		payload Payload
		wantErr string
	}{
		{name: "missing operation", payload: Payload{Input: "hola"}, wantErr: "unsupported operation"},
		{name: "missing input", payload: Payload{Operation: OperationEncode}, wantErr: "exactly one of input or inputFile"},
		{name: "both input and file", payload: Payload{Operation: OperationEncode, Input: "hola", InputFile: "in.txt"}, wantErr: "exactly one of input or inputFile"},
		{name: "wrap on decode", payload: Payload{Operation: OperationDecode, Input: "aG9sYQ==", Wrap: &wrap}, wantErr: "wrap is only supported"},
		{name: "ignore garbage on encode", payload: Payload{Operation: OperationEncode, Input: "hola", IgnoreGarbage: true}, wantErr: "ignoreGarbage is only supported"},
		{name: "valid encode", payload: Payload{Operation: OperationEncode, Input: "hola", Wrap: &wrap}},
		{name: "valid decode", payload: Payload{Operation: OperationDecode, Input: "aG9sYQ==", IgnoreGarbage: true}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.payload.Validate()
			if tt.wantErr == "" && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestActionExecuteEncodeDecode(t *testing.T) {
	encodePayload := map[string]any{
		"action":    ActionName,
		"operation": OperationEncode,
		"input":     "hola",
		"wrap":      0,
	}
	encodeRaw, err := json.Marshal(encodePayload)
	if err != nil {
		t.Fatalf("Marshal encode payload: %v", err)
	}

	encodedResult, err := Action{}.Execute(context.Background(), encodeRaw, &registry.ExecutionContext{})
	if err != nil {
		t.Fatalf("Execute encode: %v", err)
	}
	if encodedResult.Type != flow.ResultTypeJSON {
		t.Fatalf("unexpected result type: %s", encodedResult.Type)
	}

	enc, ok := encodedResult.Value.(ExecutionResult)
	if !ok {
		t.Fatalf("unexpected result value type: %T", encodedResult.Value)
	}
	if strings.TrimSpace(enc.Stdout) != "aG9sYQ==" {
		t.Fatalf("unexpected encoded stdout: %q", enc.Stdout)
	}

	decodePayload := map[string]any{
		"action":        ActionName,
		"operation":     OperationDecode,
		"input":         "aG9sYQ==\n#",
		"ignoreGarbage": true,
	}
	decodeRaw, err := json.Marshal(decodePayload)
	if err != nil {
		t.Fatalf("Marshal decode payload: %v", err)
	}

	decodedResult, err := Action{}.Execute(context.Background(), decodeRaw, &registry.ExecutionContext{})
	if err != nil {
		t.Fatalf("Execute decode: %v", err)
	}

	dec, ok := decodedResult.Value.(ExecutionResult)
	if !ok {
		t.Fatalf("unexpected decode result value type: %T", decodedResult.Value)
	}
	if dec.Stdout != "hola" {
		t.Fatalf("unexpected decoded stdout: %q", dec.Stdout)
	}
}

func TestExecuteUsesInputAndOutputFiles(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "in.txt")
	outputPath := filepath.Join(tmpDir, "out.b64")
	if err := os.WriteFile(inputPath, []byte("flowk"), 0o600); err != nil {
		t.Fatalf("WriteFile input: %v", err)
	}

	result, err := Execute(context.Background(), Payload{
		Operation:  OperationEncode,
		InputFile:  inputPath,
		OutputFile: outputPath,
		Wrap:       intPtr(0),
	}, &registry.ExecutionContext{})
	if err != nil {
		t.Fatalf("Execute with files: %v", err)
	}
	if result.OutputFile != outputPath {
		t.Fatalf("unexpected output path: %q", result.OutputFile)
	}

	encodedBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile output: %v", err)
	}
	if strings.TrimSpace(string(encodedBytes)) != "Zmxvd2s=" {
		t.Fatalf("unexpected output file content: %q", string(encodedBytes))
	}
}

func intPtr(v int) *int { return &v }
