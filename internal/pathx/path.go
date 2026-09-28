package pathx

import (
	"path/filepath"
	"strings"
)

const GlobChars = "*?[]"

func IsGlob(spec string) bool {
	return strings.ContainsAny(spec, GlobChars)
}

func RelEscapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func RelInside(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || RelEscapes(rel) {
		return "", false
	}
	return rel, true
}
