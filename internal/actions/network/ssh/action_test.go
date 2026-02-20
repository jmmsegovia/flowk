package ssh

import (
	"errors"
	"testing"
)

type fakeExitError int

func (f fakeExitError) Error() string {
	return "fake exit error"
}

func (f fakeExitError) ExitStatus() int {
	return int(f)
}

func TestCommandStepAllowsExit(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if (commandStep{}).allowsExit(fakeExitError(1)) {
			t.Fatal("expected empty configuration to disallow exit codes")
		}
	})

	t.Run("nonExitError", func(t *testing.T) {
		step := commandStep{AllowedExitCodes: []int{1}}
		if step.allowsExit(errors.New("boom")) {
			t.Fatal("non-exit errors must not be treated as allowed")
		}
	})

	t.Run("matches", func(t *testing.T) {
		step := commandStep{AllowedExitCodes: []int{1, 5}}
		if !step.allowsExit(fakeExitError(5)) {
			t.Fatal("expected exit status 5 to be allowed")
		}
	})

	t.Run("noMatch", func(t *testing.T) {
		step := commandStep{AllowedExitCodes: []int{2, 4}}
		if step.allowsExit(fakeExitError(3)) {
			t.Fatal("exit status 3 should not be allowed")
		}
	})
}
