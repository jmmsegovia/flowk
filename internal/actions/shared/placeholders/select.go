package placeholders

import "strings"

// SelectPlaceholderExpression returns the first non-empty placeholder expression
// found within the provided match slice. Matches typically include the full
// placeholder as the first element followed by the captured expressions.
func SelectPlaceholderExpression(match []string) string {
	for i := 1; i < len(match); i++ {
		if expr := strings.TrimSpace(match[i]); expr != "" {
			return expr
		}
	}
	return ""
}
