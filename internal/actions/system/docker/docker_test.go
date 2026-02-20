package docker

import (
	"strings"
	"testing"
)

func TestPayloadValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload Payload
		wantErr string
	}{
		{name: "missing operation", payload: Payload{}, wantErr: "unsupported operation"},
		{name: "run requires image", payload: Payload{Operation: OperationContainerRun}, wantErr: "image is required"},
		{name: "exec requires container", payload: Payload{Operation: OperationContainerExec, Command: []string{"ls"}}, wantErr: "container is required"},
		{name: "exec requires command", payload: Payload{Operation: OperationContainerExec, Container: "c1"}, wantErr: "command is required"},
		{name: "valid run", payload: Payload{Operation: OperationContainerRun, Image: "alpine"}},
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

func TestBuildDockerArgsAndFlags(t *testing.T) {
	t.Parallel()

	runArgs, err := buildDockerArgs(Payload{Operation: OperationContainerRun, Image: "alpine", Name: "demo", Env: []string{"A=1"}, Ports: []string{"8080:80"}, Command: []string{"echo", "ok"}, Interactive: true, TTY: true, Detach: true})
	if err != nil {
		t.Fatalf("buildDockerArgs returned error: %v", err)
	}
	joined := strings.Join(runArgs, " ")
	for _, expected := range []string{"run", "-i", "-t", "-d", "--name demo", "-e A=1", "-p 8080:80", "alpine", "echo", "ok"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected args to contain %q, got %q", expected, joined)
		}
	}

	execArgs, err := buildDockerArgs(Payload{Operation: OperationContainerExec, Container: "c1", Command: []string{"ls"}, Interactive: true})
	if err != nil {
		t.Fatalf("buildDockerArgs(exec) returned error: %v", err)
	}
	if strings.Join(execArgs, " ") != "exec -i c1 ls" {
		t.Fatalf("unexpected exec args: %v", execArgs)
	}

	if _, err := buildDockerArgs(Payload{Operation: "UNKNOWN"}); err == nil {
		t.Fatal("expected unsupported operation error")
	}

	if got := dockerFlags(true, true); strings.Join(got, " ") != "-i -t" {
		t.Fatalf("unexpected dockerFlags output: %v", got)
	}
}
