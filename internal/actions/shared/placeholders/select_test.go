package placeholders

import "testing"

func TestSelectPlaceholderExpression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		match []string
		want  string
	}{
		{name: "selects first capture", match: []string{"${a}", " a ", "b"}, want: "a"},
		{name: "skips empty captures", match: []string{"${a}", " ", "\t", " b "}, want: "b"},
		{name: "no captures", match: []string{"${a}"}, want: ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := SelectPlaceholderExpression(tt.match); got != tt.want {
				t.Fatalf("SelectPlaceholderExpression(%v) = %q, want %q", tt.match, got, tt.want)
			}
		})
	}
}
