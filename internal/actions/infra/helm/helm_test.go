package helm

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPayloadValidate(t *testing.T) {

	revision := 2
	tests := []struct {
		name    string
		payload Payload
		wantErr string
	}{
		{name: "missing operation", payload: Payload{}, wantErr: "operation is required"},
		{name: "repo add requires name", payload: Payload{Operation: OperationRepoAdd, RepositoryURL: "https://charts.example.com"}, wantErr: "repository_name is required"},
		{name: "search requires query", payload: Payload{Operation: OperationSearchRepo}, wantErr: "query is required"},
		{name: "install requires chart", payload: Payload{Operation: OperationInstall, ReleaseName: "api"}, wantErr: "chart is required"},
		{name: "rollback requires revision", payload: Payload{Operation: OperationRollback, ReleaseName: "api"}, wantErr: "revision is required"},
		{name: "rollback revision min", payload: Payload{Operation: OperationRollback, ReleaseName: "api", Revision: ptrInt(0)}, wantErr: "revision must be greater than or equal to 1"},
		{name: "set value format", payload: Payload{Operation: OperationLint, Chart: "./chart", SetValues: []string{"invalid"}}, wantErr: "KEY=VALUE"},
		{name: "valid upgrade", payload: Payload{Operation: OperationUpgrade, ReleaseName: "api", Chart: "repo/api", Revision: &revision}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.payload.Validate()
			if tt.wantErr == "" && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestBuildArgs(t *testing.T) {

	args, err := buildArgs(Payload{
		Operation:       OperationUpgradeInstall,
		ReleaseName:     "api",
		Chart:           "repo/api",
		Namespace:       "demo",
		KubeContext:     "dev",
		ValuesFiles:     []string{"values.yaml"},
		SetValues:       []string{"image.tag=1.2.3"},
		Version:         "1.0.0",
		CreateNamespace: true,
		Wait:            true,
		TimeoutSeconds:  180,
		DryRun:          true,
	})
	if err != nil {
		t.Fatalf("buildArgs returned error: %v", err)
	}

	joined := strings.Join(args, " ")
	for _, expected := range []string{
		"upgrade --install api repo/api",
		"--kube-context dev",
		"--namespace demo",
		"--values values.yaml",
		"--set image.tag=1.2.3",
		"--version 1.0.0",
		"--create-namespace",
		"--wait",
		"--timeout 180s",
		"--dry-run",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected args to contain %q, got %q", expected, joined)
		}
	}
}

func TestExecuteUsesSanitizedCommand(t *testing.T) {

	original := commandRunner
	t.Cleanup(func() { commandRunner = original })

	commandRunner = func(_ context.Context, _ Payload, args []string) (commandOutput, error) {
		if strings.Join(args, " ") != "install api repo/api --set token=super-secret" {
			t.Fatalf("unexpected args: %v", args)
		}
		return commandOutput{stdout: "ok", exitCode: 0}, nil
	}

	result, err := Execute(context.Background(), Payload{
		Operation:   OperationInstall,
		ReleaseName: "api",
		Chart:       "repo/api",
		SetValues:   []string{"token=super-secret"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if strings.Join(result.Command, " ") != "helm install api repo/api --set token=****" {
		t.Fatalf("expected sanitized command, got %v", result.Command)
	}
}

func TestExecuteCommandFailure(t *testing.T) {

	original := commandRunner
	t.Cleanup(func() { commandRunner = original })

	commandRunner = func(_ context.Context, _ Payload, _ []string) (commandOutput, error) {
		return commandOutput{stderr: "boom", exitCode: 1}, errors.New("exit status 1")
	}

	_, err := Execute(context.Background(), Payload{Operation: OperationRepoUpdate})
	if err == nil || !strings.Contains(err.Error(), "command failed") {
		t.Fatalf("expected command failed error, got %v", err)
	}
}

func ptrInt(v int) *int { return &v }
