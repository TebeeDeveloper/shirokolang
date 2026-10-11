package cli

import (
	"fmt"

	"shiroko/core"
)

// CheckCmd is `shirocc check` — runs the frontend and reports whether
// the current project type-checks.
func CheckCmd(verbose bool) error {
	p, err := loadProject()
	if err != nil {
		return err
	}
	ok, err := core.CheckFile(p.Entry, verbose)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("check failed: %s", p.Entry)
	}
	fmt.Printf("OK %s\n", p.Entry)
	return nil
}
