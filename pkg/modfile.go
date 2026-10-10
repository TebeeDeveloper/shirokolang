package pkg

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ShrkoModName is the project manifest filename.
const ShrkoModName = "shiroko.shrkomod"

// ModFile is a parsed shiroko.shrkomod.
//
// The file is a small INI-flavoured config holding project metadata:
//
//	# comment (also //)
//	name    = myapp
//	version = 0.1.0
//	entry   = source/main.shrko
//	output  = build/main.go
//
// It does NOT list dependencies. Imports are declared in `.shrko`
// source files, one place only — see ParseImports in loader.go.
type ModFile struct {
	Path    string
	Name    string
	Version string
	Entry   string
	Output  string
}

// LoadModFile reads dir/shiroko.shrkomod. Missing file is not an
// error — it just yields an empty ModFile.
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
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" ||
			strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "//") {
			continue
		}

		// Inline comment.
		if i := strings.Index(line, " #"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		} else if i := strings.Index(line, " //"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}

		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue // no key; ignore
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		val := strings.TrimSpace(line[eq+1:])
		val = strings.Trim(val, `"'`)

		switch key {
		case "name":
			mf.Name = val
		case "version":
			mf.Version = val
		case "entry":
			mf.Entry = val
		case "output":
			mf.Output = val
		}
	}
	return mf, sc.Err()
}

// Save writes the file back.
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
	return os.WriteFile(m.Path, []byte(b.String()), 0o644)
}

// ValidateImportPath accepts `<name>` or `<package>/<module>`.
// Rejects empty, leading/trailing slash, or more than one slash.
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
