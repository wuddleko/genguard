package config

import (
	"path/filepath"
	"strings"
)

// A pattern is the set of paths one output spec can name, relative to the
// config directory and then cleaned against that directory. A trailing slash
// is the whole directory. A glob with no slash matches that name in any
// directory under the config. Two patterns overlap when one path could match
// both.
type pattern struct {
	parts []segment
	dir   bool
}

type segment struct {
	any  bool
	glob string
}

func compileOutput(configDir, spec string) pattern {
	parsed := ParseSpec(spec)
	rel := strings.TrimRight(filepath.ToSlash(spec), "/")
	if rel == "" {
		rel = "."
	}
	if parsed.Dir {
		if parsed.Glob {
			rel = globDir(parsed)
		}
		cleaned := filepath.ToSlash(filepath.Clean(filepath.Join(configDir, filepath.FromSlash(rel))))
		return pattern{parts: splitPath(cleaned), dir: true}
	}
	if parsed.Glob && !strings.Contains(rel, "/") && rel != "**" {
		root := filepath.ToSlash(filepath.Clean(configDir))
		parts := splitPath(root)
		parts = append(parts, segment{any: true}, segment{glob: rel})
		return pattern{parts: parts}
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.Join(configDir, filepath.FromSlash(rel))))
	return pattern{parts: expandTrailingAny(splitPath(cleaned))}
}

func globDir(spec Spec) string {
	dir := spec.GlobPrefix()
	if len(dir) == 0 {
		return "."
	}
	return strings.Join(dir, "/")
}

func splitPath(cleaned string) []segment {
	raw := strings.Split(cleaned, "/")
	parts := make([]segment, 0, len(raw))
	for _, part := range raw {
		if part == "**" {
			parts = append(parts, segment{any: true})
			continue
		}
		parts = append(parts, segment{glob: part})
	}
	return parts
}

// A trailing ** matches files inside, so it needs one segment and may continue.
func expandTrailingAny(parts []segment) []segment {
	if len(parts) == 0 || !parts[len(parts)-1].any {
		return parts
	}
	out := append([]segment(nil), parts[:len(parts)-1]...)
	out = append(out, segment{glob: "*"}, segment{any: true})
	return out
}

func patternsOverlap(a, b pattern) bool {
	type key struct{ i, j int }
	memo := make(map[key]bool)
	visiting := make(map[key]bool)
	var walk func(i, j int) bool
	walk = func(i, j int) bool {
		k := key{i, j}
		if v, ok := memo[k]; ok {
			return v
		}
		if visiting[k] {
			return false
		}
		visiting[k] = true
		ok := overlapAt(a, b, i, j, walk)
		visiting[k] = false
		memo[k] = ok
		return ok
	}
	return walk(0, 0)
}

func overlapAt(a, b pattern, i, j int, walk func(int, int) bool) bool {
	aDone := i >= len(a.parts)
	bDone := j >= len(b.parts)
	if aDone && bDone {
		return true
	}
	if !aDone && a.parts[i].any && bDone {
		return walk(i+1, j)
	}
	if !bDone && b.parts[j].any && aDone {
		return walk(i, j+1)
	}
	if aDone && a.dir {
		return bDone || satisfiable(b.parts[j:])
	}
	if bDone && b.dir {
		return satisfiable(a.parts[i:])
	}
	if aDone || bDone {
		return false
	}
	if a.parts[i].any {
		if walk(i+1, j) {
			return true
		}
		if !b.parts[j].any && satisfiable(b.parts[j:j+1]) {
			return walk(i, j+1)
		}
		return false
	}
	if b.parts[j].any {
		if walk(i, j+1) {
			return true
		}
		if satisfiable(a.parts[i : i+1]) {
			return walk(i+1, j)
		}
		return false
	}
	if globsOverlap(a.parts[i].glob, b.parts[j].glob) {
		return walk(i+1, j+1)
	}
	return false
}

func satisfiable(parts []segment) bool {
	for _, part := range parts {
		if part.any {
			continue
		}
		if !globsOverlap(part.glob, "*") {
			return false
		}
	}
	return len(parts) > 0
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
