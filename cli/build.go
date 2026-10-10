package cli

import (
	"os"
	"path/filepath"

	"shiroko/core"
)

// BuildCmd implements `shirocc build`. It loads the project, compiles
// the entry file, and writes the Go output to the path resolved from
// the modfile.
func BuildCmd(verbose bool) error {
	p, err := loadProject()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.Output), 0o755); err != nil {
		return err
	}
	return core.CompileFile(p.Entry, p.Output, verbose)
}
