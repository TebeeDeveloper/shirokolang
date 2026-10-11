package cli

import (
	"shiroko/core"
)

// RunCmd is `shirocc run` — interprets the current project's entry
// file. No Go emission, no `go run` subprocess.
func RunCmd(verbose bool) error {
	p, err := loadProject()
	if err != nil {
		return err
	}
	return core.RunInterpreted(p.Entry, verbose)
}
