package colors

import "testing"

func TestANSIColorConstants(t *testing.T) {
	t.Parallel()

	if Reset == "" || BrightWhite == "" || Green == "" || Red == "" {
		t.Fatal("expected color constants to be non-empty")
	}
}
