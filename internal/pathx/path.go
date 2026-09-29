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

// RelInsideResolved is RelInside on absolute paths, retried with the root's
// symlinks resolved and then both. path may name a file that no longer exists.
func RelInsideResolved(root, path string) (string, bool) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	if rel, ok := RelInside(absRoot, absPath); ok {
		return rel, true
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", false
	}
	if rel, ok := RelInside(resolvedRoot, absPath); ok {
		return rel, true
	}
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", false
	}
	return RelInside(resolvedRoot, resolvedPath)
}
