package config

import (
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/pathx"
)

// Spec is one entry of outputs or inputs. clean, drift, and the overlap check
// all classify an entry through ParseSpec.
type Spec struct {
	Path string
	Glob bool
	Dir  bool
}

func ParseSpec(path string) Spec {
	return Spec{
		Path: path,
		Glob: pathx.IsGlob(path),
		Dir:  strings.HasSuffix(path, "/") || strings.HasSuffix(path, string(filepath.Separator)),
	}
}

// GlobPrefix is the literal directories before the first segment that holds a
// glob character.
func (s Spec) GlobPrefix() []string {
	var prefix []string
	for _, part := range strings.Split(filepath.ToSlash(s.Path), "/") {
		if part == "" || part == "." {
			continue
		}
		if pathx.IsGlob(part) {
			break
		}
		prefix = append(prefix, part)
	}
	return prefix
}
