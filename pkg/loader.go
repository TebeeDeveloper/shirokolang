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
// Every input file must be `package main` (or have no package line at
// all — the loader treats that as `package main`). Any other package
// name is rejected. Cycles are permitted: since everything collapses
// into one program and sema hoists all top-level declarations, the
// order in which cycle members are emitted doesn't affect correctness.
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

// Load reads mainFile, merges the imports declared in its source with
// the ones listed in the sibling shiroko.shrkomod, recursively
// resolves every transitive import, and returns one concatenated
// source buffer in dependency order (each package before its
// importer, the main file last).
func (l *Loader) Load(mainFile string) (*Result, error) {
	mainSrc, err := os.ReadFile(mainFile)
	if err != nil {
		return nil, err
	}
	mainImports, err := ParseImports(string(mainSrc))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", mainFile, err)
	}
	mf, err := LoadModFile(filepath.Dir(mainFile))
	if err != nil {
		return nil, err
	}
	all := dedup(append(append([]string{}, mf.Imports...), mainImports...))

	for _, imp := range all {
		if isStd(imp) {
			continue
		}
		if err := l.visit(imp); err != nil {
			return nil, err
		}
	}

	var bodies []string
	var files []string
	var imports []string

	// Dependencies first, main file last.
	for _, imp := range l.order {
		srcs, err := readPackage(l.dirs[imp])
		if err != nil {
			return nil, err
		}
		for _, s := range srcs {
			_, imps, body, err := stripHeader(s.body)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", s.path, err)
			}
			imports = append(imports, imps...)
			bodies = append(bodies, body)
			files = append(files, s.path)
		}
	}

	// Main file.
	_, mainImps, mainBody, err := stripHeader(string(mainSrc))
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

// visit resolves one import and, recursively, its own imports.
func (l *Loader) visit(imp string) error {
	if isStd(imp) {
		return nil
	}
	switch l.state[imp] {
		case stateDone:
			return nil
		case stateVisiting:
			// Cycle back-edge: the frame above us will append this
			// package to order once its own recursion returns. No
			// action needed — cycles are fine in a merged program.
			return nil
	}

	l.state[imp] = stateVisiting

	dir, err := Fetch(imp)
	if err != nil {
		return err
	}
	l.dirs[imp] = dir

	deps, err := collectImports(dir)
	if err != nil {
		return fmt.Errorf("%s: %w", imp, err)
	}
	for _, sub := range deps {
		if err := l.visit(sub); err != nil {
			return err
		}
	}

	l.state[imp] = stateDone
	l.order = append(l.order, imp)
	return nil
}

// collectImports merges the import block of every .shrko file in dir
// with the entries in dir/shiroko.shrkomod.
func collectImports(dir string) ([]string, error) {
	mf, err := LoadModFile(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, imp := range mf.Imports {
		if !isStd(imp) {
			out = append(out, imp)
		}
	}

	srcs, err := readPackage(dir)
	if err != nil {
		return nil, err
	}
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
// `import { ... }` block in src. Returns nil if there is none.
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
		switch src[j] {
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
// the remaining body. The package name is validated: it must be
// empty or `main`.
func stripHeader(src string) (imports []string, body string, err error) {
	i := skipWS(src, 0)

	if hasKeywordAt(src, i, "package") {
		i += len("package")
		i = skipSpaces(src, i)
		start := i
		for i < len(src) && isIdentByte(src[i]) {
			i++
		}
		name := src[start:i]
		if name != "main" {
			return nil, "", fmt.Errorf(
				"package %q not allowed: only `package main` is supported",
			      name)
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
				if src[i] == '"' {
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
// package. The emitter maps these to Go imports; the loader must not
// try to fetch them.
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
