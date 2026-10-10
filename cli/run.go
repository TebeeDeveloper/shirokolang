package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"shiroko/core"
)

// RunCmd implements `shirocc run`. It compiles the project, then runs
// the generated Go file with the current process's stdio attached.
func RunCmd(verbose bool) error {
	p, err := loadProject()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.Output), 0o755); err != nil {
		return err
	}
	if err := core.CompileFile(p.Entry, p.Output, verbose); err != nil {
		return err
	}

	cmd := exec.Command("go", "run", p.Output)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	return nil
}
