package pkg

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Result is the loader's output: one concatenated source buffer and
// the list of files that went into it, in order.
type Result struct {
	Source string
	Files  []string
}

// Loader resolves a main file plus every package it transitively
// imports, then concatenates them into a single source buffer.
//
// Imports come exclusively from `import { ... }` blocks in .shrko
// files. There is no side-channel: the modfile holds project metadata
// only, never a dependency list.
//
// Every input file may declare any package name; the loader strips it
// and always emits `package main`. Cycles are permitted.
type Loader struct {
	state map[string]loadState
	dirs  map[string]string
	order []string
}

type loadState int

const (
	stateUnvisited loadState = iota
	stateVisiting
	stateDone
)

func NewLoader() *Loader {
	return &Loader{
		state: map[string]loadState{},
		dirs:  map[string]string{},
	}
}

// Load reads mainFile, resolves every import declared in its source
// (and, recursively, in every transitive package), and returns one
// concatenated source buffer in dependency order (each package before
// its importer, the main file last).
func (l *Loader) Load(mainFile string) (*Result, error) {
	mainSrc, err := os.ReadFile(mainFile)
	if err != nil {
		return nil, err
	}
	mainImports, err := ParseImports(string(mainSrc))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", mainFile, err)
	}

	mainDir := filepath.Dir(mainFile)
	for _, imp := range mainImports {
		if isStd(imp) {
			continue
		}
		if err := l.visit(mainDir, imp); err != nil {
			return nil, err
		}
	}

	var bodies []string
	var files []string
	var imports []string

	for _, imp := range l.order {
		srcs, err := readPackage(l.dirs[imp])
		if err != nil {
			return nil, err
		}
		for _, s := range srcs {
			imps, body, err := stripHeader(s.body)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", s.path, err)
			}
			imports = append(imports, imps...)
			bodies = append(bodies, body)
			files = append(files, s.path)
		}
	}

	mainImps, mainBody, err := stripHeader(string(mainSrc))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", mainFile, err)
	}
	imports = append(imports, mainImps...)
	bodies = append(bodies, mainBody)
	files = append(files, mainFile)
	imports = dedup(imports)

	// Emit one canonical header, then every body.
	var buf strings.Builder
	buf.WriteString("package main\n\n")
	if len(imports) > 0 {
		buf.WriteString("import {\n")
		for _, imp := range imports {
			fmt.Fprintf(&buf, "    %q\n", imp)
		}
		buf.WriteString("}\n\n")
	}
	for i, b := range bodies {
		fmt.Fprintf(&buf, "// ---- %s ----\n", files[i])
		buf.WriteString(b)
		if !strings.HasSuffix(b, "\n") {
			buf.WriteByte('\n')
		}
		buf.WriteByte('\n')
	}

	return &Result{Source: buf.String(), Files: files}, nil
}

// visit resolves one import, relative to `fromDir`, and recursively its
// own imports (each resolved relative to the package's own directory).
func (l *Loader) visit(fromDir, imp string) error {
	if isStd(imp) {
		return nil
	}
	switch l.state[imp] {
	case stateDone:
		return nil
	case stateVisiting:
		// Cycle back-edge.
		return nil
	}

	l.state[imp] = stateVisiting

	dir, err := FetchFrom(fromDir, imp)
	if err != nil {
		return err
	}
	l.dirs[imp] = dir

	deps, err := collectImports(dir)
	if err != nil {
		return fmt.Errorf("%s: %w", imp, err)
	}
	for _, sub := range deps {
		if err := l.visit(dir, sub); err != nil {
			return err
		}
	}

	l.state[imp] = stateDone
	l.order = append(l.order, imp)
	return nil
}

// collectImports returns the non-stdlib imports declared across every
// .shrko file in dir, deduplicated.
func collectImports(dir string) ([]string, error) {
	srcs, err := readPackage(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, s := range srcs {
		imps, err := ParseImports(s.body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.path, err)
		}
		for _, imp := range imps {
			if !isStd(imp) {
				out = append(out, imp)
			}
		}
	}
	return dedup(out), nil
}

type sourceFile struct {
	path string
	body string
}

// readPackage returns every .shrko file in dir, sorted by name.
// A directory with no .shrko files is not an error.
func readPackage(dir string) ([]sourceFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".shrko") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var out []sourceFile
	for _, n := range names {
		body, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return nil, err
		}
		out = append(out, sourceFile{
			path: filepath.Join(dir, n),
			body: string(body),
		})
	}
	return out, nil
}

// ---------- import parsing ----------

// ParseImports extracts every string literal from the first
// `import { ... }` block in src. Line comments (`//`) and block
// comments (`/* */`) inside the block are skipped, so a commented-out
// import is not treated as a live dependency.
func ParseImports(src string) ([]string, error) {
	i := indexKeyword(src, "import")
	if i < 0 {
		return nil, nil
	}
	j := i + len("import")
	for j < len(src) && isSpace(src[j]) {
		j++
	}
	if j >= len(src) || src[j] != '{' {
		return nil, nil
	}
	depth := 1
	j++
	var out []string
	for j < len(src) && depth > 0 {
		c := src[j]

		// Line comment.
		if c == '/' && j+1 < len(src) && src[j+1] == '/' {
			for j < len(src) && src[j] != '\n' {
				j++
			}
			continue
		}
		// Block comment.
		if c == '/' && j+1 < len(src) && src[j+1] == '*' {
			j += 2
			for j+1 < len(src) && !(src[j] == '*' && src[j+1] == '/') {
				j++
			}
			if j+1 >= len(src) {
				return nil, fmt.Errorf("unterminated block comment in import block")
			}
			j += 2
			continue
		}

		switch c {
		case '{':
			depth++
			j++
		case '}':
			depth--
			j++
		case '"':
			j++
			start := j
			for j < len(src) && src[j] != '"' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(src) {
				return nil, fmt.Errorf("unterminated string in import block")
			}
			out = append(out, src[start:j])
			j++
		default:
			j++
		}
	}
	return out, nil
}

// ---------- header stripping ----------

// stripHeader removes the leading `package X` line and optional
// `import { ... }` block from src, returning the imported paths and
// the remaining body. The package name is stripped but not inspected —
// the loader always emits `package main`, and sub-packages are free
// to name themselves.
func stripHeader(src string) (imports []string, body string, err error) {
	i := skipWS(src, 0)

	if hasKeywordAt(src, i, "package") {
		i += len("package")
		i = skipSpaces(src, i)
		for i < len(src) && isIdentByte(src[i]) {
			i++
		}
		i = skipLine(src, i)
		i = skipWS(src, i)
	}

	if hasKeywordAt(src, i, "import") {
		i += len("import")
		i = skipSpaces(src, i)
		if i < len(src) && src[i] == '{' {
			i++
			for i < len(src) && src[i] != '}' {
				c := src[i]

				// Line comment.
				if c == '/' && i+1 < len(src) && src[i+1] == '/' {
					for i < len(src) && src[i] != '\n' {
						i++
					}
					continue
				}
				// Block comment.
				if c == '/' && i+1 < len(src) && src[i+1] == '*' {
					i += 2
					for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
						i++
					}
					if i+1 >= len(src) {
						return nil, "", fmt.Errorf(
							"unterminated block comment in import block")
					}
					i += 2
					continue
				}

				if c == '"' {
					i++
					s := i
					for i < len(src) && src[i] != '"' {
						if src[i] == '\\' {
							i++
						}
						i++
					}
					imports = append(imports, src[s:i])
					i++
				} else {
					i++
				}
			}
			if i < len(src) {
				i++
			}
			i = skipLine(src, i)
			i = skipWS(src, i)
		}
	}
	return imports, src[i:], nil
}

func skipWS(s string, i int) int {
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	return i
}

func skipSpaces(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}

func skipLine(s string, i int) int {
	for i < len(s) && s[i] != '\n' {
		i++
	}
	return i
}

func hasKeywordAt(s string, i int, kw string) bool {
	if i+len(kw) > len(s) || s[i:i+len(kw)] != kw {
		return false
	}
	if i > 0 && isIdentByte(s[i-1]) {
		return false
	}
	if j := i + len(kw); j < len(s) && isIdentByte(s[j]) {
		return false
	}
	return true
}

// ---------- helpers ----------

func indexKeyword(src, kw string) int {
	for i := 0; i+len(kw) <= len(src); i++ {
		if src[i:i+len(kw)] != kw {
			continue
		}
		if i > 0 && isIdentByte(src[i-1]) {
			continue
		}
		if j := i + len(kw); j < len(src) && isIdentByte(src[j]) {
			continue
		}
		return i
	}
	return -1
}

func isIdentByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_'
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// isStd reports whether an import path refers to a builtin stdlib
// package.
func isStd(p string) bool {
	return strings.HasPrefix(p, "std/")
}

func dedup(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	return out
}
