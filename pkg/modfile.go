package pkg

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ShrkoModName is the project manifest that `shirocc` reads and writes.
const ShrkoModName = "shiroko.shrkomod"

// ModFile is a parsed shiroko.shrkomod. It carries project metadata
// only — imports are declared in .shrko sources via `import { ... }`
// blocks, not here.
//
// Format:
//
//	# comments start with '#'
//	name    = demo
//	version = 0.1.0
//	entry   = source/main.shrko
//	output  = build/main.go
//	test    = test/main_test.shrko
//
// Every key is optional. Unknown keys are rejected so typos surface
// early.
type ModFile struct {
	Path    string
	Name    string
	Version string
	Entry   string // relative path to entry .shrko
	Output  string // relative path to emitted .go
	Test    string // relative path to the test .shrko
}

// LoadModFile reads dir/shiroko.shrkomod. A missing file is not an
// error — it yields an empty ModFile.
func LoadModFile(dir string) (*ModFile, error) {
	path := filepath.Join(dir, ShrkoModName)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return &ModFile{Path: path}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	mf := &ModFile{Path: path}
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		i := strings.IndexByte(line, '=')
		if i < 0 {
			return nil, fmt.Errorf(
				"%s:%d: expected `key = value`, got %q",
				ShrkoModName, lineNo, line)
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])

		switch key {
			case "name":
				mf.Name = val
			case "version":
				mf.Version = val
			case "entry":
				mf.Entry = val
			case "output":
				mf.Output = val
			case "test":
				mf.Test = val
			default:
				return nil, fmt.Errorf(
					"%s:%d: unknown key %q "+
						"(want name, version, entry, output, or test)",
					ShrkoModName, lineNo, key)
		}
	}
	return mf, sc.Err()
}

// Save writes the file back with the standard header and every
// non-empty key.
func (m *ModFile) Save() error {
	var b strings.Builder
	b.WriteString("# shiroko.shrkomod — project manifest\n")
	b.WriteString("#\n")
	b.WriteString("# Imports are declared in .shrko sources, not here.\n\n")

	if m.Name != "" {
		fmt.Fprintf(&b, "name    = %s\n", m.Name)
	}
	if m.Version != "" {
		fmt.Fprintf(&b, "version = %s\n", m.Version)
	}
	if m.Entry != "" {
		fmt.Fprintf(&b, "entry   = %s\n", m.Entry)
	}
	if m.Output != "" {
		fmt.Fprintf(&b, "output  = %s\n", m.Output)
	}
	if m.Test != "" {
		fmt.Fprintf(&b, "test    = %s\n", m.Test)
	}

	return os.WriteFile(m.Path, []byte(b.String()), 0o644)
}

// ValidateImportPath accepts `<name>` or `<package>/<module>`.
func ValidateImportPath(p string) error {
	if p == "" {
		return fmt.Errorf("import path is empty")
	}
	i := strings.IndexByte(p, '/')
	if i < 0 {
		return nil
	}
	if i == 0 || i == len(p)-1 || strings.IndexByte(p[i+1:], '/') >= 0 {
		return fmt.Errorf("import path %q must be <name> or <package>/<module>", p)
	}
	return nil
}
