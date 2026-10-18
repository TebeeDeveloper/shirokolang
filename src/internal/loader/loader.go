package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"shiroko/internal/fetch"
	"shiroko/internal/lexer"
	"shiroko/internal/parser"
	"shiroko/internal/source"
)

// Result is the loader's output: the entry program plus every
// imported package, grouped by declared package name.
type Result struct {
	// Main is the entry file's parsed program.
	Main []any

	// MainFile is the absolute path to the entry file.
	MainFile string

	// Packages maps a package name ("proc", "util", ...) to its
	// parsed program and the files that make it up. Package "main"
	// is NOT included here — it lives in Main.
	Packages map[string]*Package

	// Files is every file that was read, in load order.
	Files []string
}

// Package is one non-main package: a name, the source files that
// declared `package <name>` in the same directory, and the merged
// parsed program.
type Package struct {
	Name  string
	Dir   string
	Files []string
	Prog  []any
	Src   *source.Source
}

type Loader struct {
	state map[string]loadState
	dirs  map[string]string
	order []string

	// packages being built, keyed by name.
	packages map[string]*Package

	// current import path stack for cycle detection.
	stack []string
}

type loadState int

const (
	stateUnvisited loadState = iota
	stateVisiting
	stateDone
)

func NewLoader() *Loader {
	return &Loader{
		state:    map[string]loadState{},
		dirs:     map[string]string{},
		packages: map[string]*Package{},
	}
}

// Load reads mainFile, recursively loads every non-std import, and
// returns the entry program plus every imported package. Cycles are
// an error.
func (l *Loader) Load(mainFile string) (*Result, error) {
	mainSrc, err := os.ReadFile(mainFile)
	if err != nil {
		return nil, err
	}

	mainProg, err := parseOne(string(mainSrc))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", mainFile, err)
	}

	mainImports := importList(mainProg)
	for _, imp := range mainImports {
		if isStd(imp) {
			continue
		}
		if err := l.visit(imp); err != nil {
			return nil, err
		}
	}

	// Collect every file that was read.
	var files []string
	for _, pkg := range l.packages {
		files = append(files, pkg.Files...)
	}
	files = append(files, mainFile)
	sort.Strings(files)

	return &Result{
		Main:     mainProg,
		MainFile: mainFile,
		Packages: l.packages,
		Files:    files,
	}, nil
}

// ParseImports returns the import paths declared by an
// `import { "..." }` block at the top of a source string. It's
// independent of Load() — used by the CLI's `shirocc import` scanner
// to discover a project's declared dependencies without running the
// full pipeline.
//
// Returns nil (no error) if there is no import block.
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

		if c == '/' && j+1 < len(src) && src[j+1] == '/' {
			for j < len(src) && src[j] != '\n' {
				j++
			}
			continue
		}
		if c == '/' && j+1 < len(src) && src[j+1] == '*' {
			j += 2
			for j+1 < len(src) && !(src[j] == '*' && src[j+1] == '/') {
				j++
			}
			if j+1 >= len(src) {
				return nil, fmt.Errorf(
					"unterminated block comment in import block")
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
				return nil, fmt.Errorf(
					"unterminated string in import block")
			}
			out = append(out, src[start:j])
			j++
		default:
			j++
		}
	}
	return out, nil
}

// ---------- helpers used by ParseImports ----------

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

// visit resolves one import path, loads its package, and recurses
// into its imports. Cycles error.
func (l *Loader) visit(imp string) error {
	if isStd(imp) {
		return nil
	}
	switch l.state[imp] {
	case stateDone:
		return nil
	case stateVisiting:
		return fmt.Errorf("import cycle: %s", cyclePath(l.stack, imp))
	}

	l.state[imp] = stateVisiting
	l.stack = append(l.stack, imp)
	defer func() {
		l.stack = l.stack[:len(l.stack)-1]
	}()

	dir, err := fetch.Fetch(imp)
	if err != nil {
		return err
	}
	l.dirs[imp] = dir

	pkg, err := l.loadPackageDir(imp, dir)
	if err != nil {
		return err
	}
	l.packages[pkg.Name] = pkg

	// Recurse into the package's own imports.
	for _, sub := range importList(pkg.Prog) {
		if isStd(sub) {
			continue
		}
		if err := l.visit(sub); err != nil {
			return err
		}
	}

	l.state[imp] = stateDone
	l.order = append(l.order, imp)
	return nil
}

// loadPackageDir reads every .shrko file in dir, parses each one,
// and merges them into a single Program. All files must declare the
// same `package <name>`; that name becomes the package name.
func (l *Loader) loadPackageDir(imp, dir string) (*Package, error) {
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

	if len(names) == 0 {
		return nil, fmt.Errorf("%s: no .shrko files in %s", imp, dir)
	}

	var (
		pkgName string
		progs   [][]any
		files   []string
		bodies  []string
	)

	for _, n := range names {
		path := filepath.Join(dir, n)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		prog, err := parseOne(string(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}

		thisName, _ := prog[1].(string)
		if thisName == "" || thisName == "<error>" {
			thisName = "main"
		}
		if thisName == "main" {
			return nil, fmt.Errorf(
				"%s: imported packages must not declare `package main`", path)
		}
		if pkgName == "" {
			pkgName = thisName
		} else if pkgName != thisName {
			return nil, fmt.Errorf(
				"%s: package %q conflicts with %q in %s",
				path, thisName, pkgName, dir)
		}

		progs = append(progs, prog)
		files = append(files, path)
		bodies = append(bodies, string(data))
	}

	merged := mergePrograms(pkgName, progs)
	src := source.New(strings.Join(bodies, "\n"))

	return &Package{
		Name:  pkgName,
		Dir:   dir,
		Files: files,
		Prog:  merged,
		Src:   src,
	}, nil
}

// parseOne lexes + parses a single source string into a Program node.
func parseOne(src string) ([]any, error) {
	toks, err := lexer.Lex(src)
	if err != nil {
		return nil, err
	}
	srcObj := source.New(src)
	p := parser.New(toks, srcObj)
	prog := p.ParseProgram()
	if len(p.Errors) > 0 {
		return nil, fmt.Errorf("%s", p.Errors[0].Msg)
	}
	return prog, nil
}

// mergePrograms combines several Programs that all declare the same
// package into one. Imports are deduped; decls are concatenated.
func mergePrograms(pkg string, progs [][]any) []any {
	var imports []string
	var decls []any
	seen := map[string]bool{}

	for _, prog := range progs {
		if ims, ok := prog[2].([]string); ok {
			for _, im := range ims {
				if seen[im] {
					continue
				}
				seen[im] = true
				imports = append(imports, im)
			}
		}
		if ds, ok := prog[3].([]any); ok {
			decls = append(decls, ds...)
		}
	}

	return []any{"Program", pkg, imports, decls}
}

// importList returns prog's declared import paths.
func importList(prog []any) []string {
	if len(prog) < 3 {
		return nil
	}
	if ims, ok := prog[2].([]string); ok {
		return ims
	}
	return nil
}

// cyclePath formats the stack + the offending import as "a -> b -> a".
func cyclePath(stack []string, imp string) string {
	parts := append(append([]string{}, stack...), imp)
	return strings.Join(parts, " -> ")
}

// ---------- helpers ----------

func isStd(p string) bool {
	return strings.HasPrefix(p, "std/")
}
