package config

import (
	"path/filepath"
	"strings"
)

// A pattern is the set of absolute paths one output spec names, the way git
// reads a pathspec. A glob is one pattern over the whole path, and *, ? and
// [...] also match "/". A literal names itself, and a directory also names
// everything under it. Two patterns overlap when one path could match both.
type pattern [][]atom

// A literal without a trailing slash is a directory when isDir says so.
func compileOutput(configDir, spec string, isDir func(string) bool) pattern {
	parsed := ParseSpec(spec)
	rel := strings.TrimRight(filepath.ToSlash(spec), "/")
	if rel == "" {
		rel = "."
	}
	base := filepath.ToSlash(filepath.Clean(configDir))
	if parsed.Glob && !parsed.Dir {
		full := filepath.ToSlash(filepath.Clean(filepath.Join(configDir, filepath.FromSlash(rel))))
		// The config directory stays literal even when its name holds glob characters.
		literal := ""
		if strings.HasPrefix(full, base+"/") {
			literal = base + "/"
		}
		return pattern{append(literalAtoms(literal), parseGlob(strings.TrimPrefix(full, literal))...)}
	}
	if parsed.Glob {
		rel = globDir(parsed)
	}
	full := filepath.ToSlash(filepath.Clean(filepath.Join(configDir, filepath.FromSlash(rel))))
	if !parsed.Dir && !isDir(filepath.FromSlash(full)) {
		return pattern{literalAtoms(full)}
	}
	under := append(literalAtoms(strings.TrimSuffix(full, "/")+"/"), atom{kind: atomStar})
	return pattern{literalAtoms(full), under}
}

func globDir(spec Spec) string {
	dir := spec.GlobPrefix()
	if len(dir) == 0 {
		return "."
	}
	return strings.Join(dir, "/")
}

func literalAtoms(s string) []atom {
	out := make([]atom, 0, len(s))
	for _, r := range s {
		out = append(out, atom{kind: atomLit, lit: r})
	}
	return out
}

func patternsOverlap(a, b pattern) bool {
	for _, x := range a {
		for _, y := range b {
			if atomsOverlap(x, y) {
				return true
			}
		}
	}
	return false
}

type atomKind int

const (
	atomLit atomKind = iota
	atomAny
	atomStar
	atomClass
)

type atom struct {
	kind atomKind
	lit  rune
	neg  bool
	wide bool
	set  map[rune]struct{}
}

func globsOverlap(a, b string) bool {
	return atomsOverlap(parseGlob(a), parseGlob(b))
}

func parseGlob(s string) []atom {
	r := []rune(s)
	out := make([]atom, 0, len(r))
	for i := 0; i < len(r); i++ {
		switch r[i] {
		case '*':
			if len(out) > 0 && out[len(out)-1].kind == atomStar {
				continue
			}
			out = append(out, atom{kind: atomStar})
		case '?':
			out = append(out, atom{kind: atomAny})
		case '\\':
			if i+1 >= len(r) {
				out = append(out, atom{kind: atomLit, lit: '\\'})
				continue
			}
			i++
			out = append(out, atom{kind: atomLit, lit: r[i]})
		case '[':
			class, next, ok := parseClass(r, i)
			if !ok {
				out = append(out, atom{kind: atomLit, lit: '['})
				continue
			}
			out = append(out, class)
			i = next - 1
		default:
			out = append(out, atom{kind: atomLit, lit: r[i]})
		}
	}
	return out
}

func parseClass(r []rune, start int) (atom, int, bool) {
	i := start + 1
	if i >= len(r) {
		return atom{}, 0, false
	}
	neg := false
	if r[i] == '!' || r[i] == '^' {
		neg = true
		i++
	}
	set := make(map[rune]struct{})
	wide := false
	if i < len(r) && r[i] == ']' {
		set[']'] = struct{}{}
		i++
	}
	for i < len(r) && r[i] != ']' {
		if r[i] == '\\' && i+1 < len(r) {
			i++
			set[r[i]] = struct{}{}
			i++
			continue
		}
		if i+2 < len(r) && r[i+1] == '-' && r[i+2] != ']' {
			lo, hi := r[i], r[i+2]
			if lo > hi {
				lo, hi = hi, lo
			}
			if int(hi-lo) > 256 {
				wide = true
			} else {
				for c := lo; c <= hi; c++ {
					set[c] = struct{}{}
				}
			}
			i += 3
			continue
		}
		set[r[i]] = struct{}{}
		i++
	}
	if i >= len(r) || r[i] != ']' {
		return atom{}, 0, false
	}
	if !neg && !wide && len(set) == 0 {
		return atom{}, 0, false
	}
	return atom{kind: atomClass, neg: neg, wide: wide, set: set}, i + 1, true
}

type globState struct{ i, j int }

func atomsOverlap(a, b []atom) bool {
	seen := make(map[globState]bool)
	q := []globState{{0, 0}}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		if cur.i == len(a) && cur.j == len(b) {
			return true
		}
		q = append(q, atomSteps(a, b, cur)...)
	}
	return false
}

func atomSteps(a, b []atom, cur globState) []globState {
	i, j := cur.i, cur.j
	var out []globState
	if i < len(a) && a[i].kind == atomStar {
		out = append(out, globState{i + 1, j})
		if j < len(b) && b[j].kind == atomStar {
			out = append(out, globState{i, j + 1})
			return out
		}
		if nj, ok := advanceOne(b, j); ok {
			out = append(out, globState{i, nj})
		}
		return out
	}
	if j < len(b) && b[j].kind == atomStar {
		out = append(out, globState{i, j + 1})
		if ni, ok := advanceOne(a, i); ok {
			out = append(out, globState{ni, j})
		}
		return out
	}
	if i < len(a) && j < len(b) && oneCharOverlap(a[i], b[j]) {
		out = append(out, globState{i + 1, j + 1})
	}
	return out
}

func advanceOne(p []atom, i int) (int, bool) {
	if i >= len(p) || p[i].kind == atomStar {
		return 0, false
	}
	if p[i].kind == atomClass && !classNonEmpty(p[i]) {
		return 0, false
	}
	return i + 1, true
}

func oneCharOverlap(a, b atom) bool {
	switch {
	case a.kind == atomAny:
		return classNonEmpty(b)
	case b.kind == atomAny:
		return classNonEmpty(a)
	case a.kind == atomLit && b.kind == atomLit:
		return a.lit == b.lit
	case a.kind == atomLit && b.kind == atomClass:
		return classContains(b, a.lit)
	case a.kind == atomClass && b.kind == atomLit:
		return classContains(a, b.lit)
	case a.kind == atomClass && b.kind == atomClass:
		return classesIntersect(a, b)
	default:
		return false
	}
}

func classNonEmpty(c atom) bool {
	switch c.kind {
	case atomAny, atomLit:
		return true
	case atomClass:
		if c.wide || c.neg {
			return true
		}
		return len(c.set) > 0
	default:
		return false
	}
}

func classContains(c atom, r rune) bool {
	if c.wide {
		return true
	}
	_, ok := c.set[r]
	if c.neg {
		return !ok
	}
	return ok
}

func classesIntersect(a, b atom) bool {
	if a.wide || b.wide {
		return true
	}
	if a.neg && b.neg {
		return true
	}
	if !a.neg && !b.neg {
		for r := range a.set {
			if _, ok := b.set[r]; ok {
				return true
			}
		}
		return false
	}
	pos, neg := a, b
	if a.neg {
		pos, neg = b, a
	}
	for r := range pos.set {
		if _, banned := neg.set[r]; !banned {
			return true
		}
	}
	return false
}
