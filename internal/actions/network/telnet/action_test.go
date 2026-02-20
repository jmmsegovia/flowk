package telnet

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPayloadSpecApplyDefaults(t *testing.T) {
	t.Parallel()

	spec := payloadSpec{}
	spec.applyDefaults()

	if spec.Port != 23 || spec.TimeoutSeconds != defaultTimeoutSeconds || spec.ReadTimeoutSeconds != defaultReadTimeoutSeconds || spec.LineEnding != "CRLF" {
		t.Fatalf("defaults not applied correctly: %+v", spec)
	}
}

func TestPayloadSpecValidate(t *testing.T) {
	t.Parallel()

	connect := &struct{}{}
	send := &sendSpec{Data: "hello"}

	tests := []struct {
		name    string
		spec    payloadSpec
		wantErr string
	}{
		{name: "missing host", spec: payloadSpec{Port: 23, Steps: []stepSpec{{Connect: connect}}}, wantErr: "host is required"},
		{name: "invalid port", spec: payloadSpec{Host: "localhost", Port: 70000, Steps: []stepSpec{{Connect: connect}}}, wantErr: "out of range"},
		{name: "empty steps", spec: payloadSpec{Host: "localhost", Port: 23}, wantErr: "steps cannot be empty"},
		{name: "first step must be connect", spec: payloadSpec{Host: "localhost", Port: 23, Steps: []stepSpec{{Send: send}}}, wantErr: "first step must be connect"},
		{name: "multiple operations in step", spec: payloadSpec{Host: "localhost", Port: 23, Steps: []stepSpec{{Connect: connect}, {Connect: connect, Send: send}}}, wantErr: "exactly one operation"},
		{name: "valid", spec: payloadSpec{Host: "localhost", Port: 23, Steps: []stepSpec{{Connect: connect}, {Send: send}}}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.spec.validate()
			if tt.wantErr == "" && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestLineEndingAndDeadlineHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default", input: "", want: "\r\n"},
		{name: "lf", input: "LF", want: "\n"},
		{name: "cr", input: "CR", want: "\r"},
		{name: "invalid", input: "bad", wantErr: true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveLineEnding(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("resolveLineEnding(%q) = %q, %v", tt.input, got, err)
			}
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	deadline := computeDeadline(ctx, 5)
	if deadline.After(time.Now().Add(100 * time.Millisecond)) {
		t.Fatalf("deadline did not honor context deadline: %v", deadline)
	}

	if got := normalizeLineEndingForTranscript("\r\n"); got != "\n" {
		t.Fatalf("unexpected transcript normalization: %q", got)
	}
}
