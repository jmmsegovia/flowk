package actionhelp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type actionHelpDoc struct {
	path    string
	content string
}

var (
	actionHelpOnce sync.Once
	actionHelpDocs []actionHelpDoc
	actionHelpErr  error
)

func findActionHelpMarkdown(actionName string) string {
	trimmed := strings.TrimSpace(actionName)
	if trimmed == "" {
		return ""
	}

	docs, err := loadActionHelpDocs()
	if err != nil {
		return ""
	}

	bestScore := 0
	bestContent := ""
	lowerAction := strings.ToLower(trimmed)
	tokens := actionHelpTokens(lowerAction)

	for _, doc := range docs {
		lowerContent := strings.ToLower(doc.content)
		lowerPath := strings.ToLower(doc.path)
		score := 0

		if strings.Contains(lowerContent, lowerAction) {
			score += 10
		}
		if strings.Contains(lowerPath, lowerAction) {
			score += 9
		}

		for _, token := range tokens {
			if token == "" {
				continue
			}
			if strings.Contains(lowerPath, token) {
				score += 3
			}
			if strings.Contains(lowerContent, token) {
				score++
			}
		}

		if score > bestScore {
			bestScore = score
			bestContent = doc.content
		}
	}

	if bestScore == 0 {
		return ""
	}

	return strings.TrimSpace(bestContent)
}

func loadActionHelpDocs() ([]actionHelpDoc, error) {
	actionHelpOnce.Do(func() {
		actionHelpDocs, actionHelpErr = scanActionHelpDocs()
	})

	return actionHelpDocs, actionHelpErr
}

func scanActionHelpDocs() ([]actionHelpDoc, error) {
	root, err := resolveRepoRoot()
	if err != nil {
		return nil, err
	}

	bases := []string{
		filepath.Join(root, "docs", "actions"),
		filepath.Join(root, "internal", "actions"),
	}
	var docs []actionHelpDoc

	for _, base := range bases {
		if _, statErr := os.Stat(base); statErr != nil {
			continue
		}

		err = filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".md") || strings.HasSuffix(path, "_test.md") {
				return nil
			}

			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			docs = append(docs, actionHelpDoc{path: path, content: string(content)})
			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	return docs, nil
}

func actionHelpTokens(actionName string) []string {
	parts := strings.FieldsFunc(actionName, func(r rune) bool {
		return r == '_' || r == '-' || r == ' ' || r == '\t'
	})

	stopwords := map[string]struct{}{
		"db":        {},
		"operation": {},
		"request":   {},
		"action":    {},
		"task":      {},
	}

	seen := make(map[string]struct{}, len(parts))
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if _, blocked := stopwords[trimmed]; blocked {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		tokens = append(tokens, trimmed)
	}

	if len(tokens) == 0 {
		return []string{actionName}
	}

	return tokens
}

func resolveRepoRoot() (string, error) {
	if root, ok := findRepoRootFromCurrentDir(); ok {
		return root, nil
	}

	if root, ok := findRepoRootFromCaller(); ok {
		return root, nil
	}

	return "", errors.New("unable to locate repository root")
}

func findRepoRootFromCurrentDir() (string, bool) {
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	return findRepoRoot(wd)
}

func findRepoRootFromCaller() (string, bool) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}
	return findRepoRoot(filepath.Dir(file))
}

func findRepoRoot(start string) (string, bool) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
