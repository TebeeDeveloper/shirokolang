package cli

import (
	"fmt"

	"shiroko/core"
)

// TestCmd is `shirocc test` — interprets the project's test file.
func TestCmd(verbose bool) error {
	p, err := loadProject()
	if err != nil {
		return err
	}
	if p.Test == "" {
		return fmt.Errorf(
			"no test entry point: create test/main_test.shrko, " +
				"or add `test = <path>` to the shrkomod")
	}
	return core.RunInterpreted(p.Test, verbose)
}
