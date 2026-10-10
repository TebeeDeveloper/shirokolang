package cli

import (
	"fmt"
	"time"

	"shiroko/core"
)

// CheckCmd implements `shirocc check`. It runs the front-end
// (loader → lexer → parser → sema → lower) and reports success.
// No Go source is emitted.
func CheckCmd(verbose bool) error {
	p, err := loadProject()
	if err != nil {
		return err
	}

	start := time.Now()
	if _, err := core.CheckFile(p.Entry, verbose); err != nil {
		return err
	}

	if verbose {
		fmt.Printf("ok  %s  (%s)\n",
			p.Entry, time.Since(start).Round(time.Millisecond))
	} else {
		fmt.Printf("ok  %s\n", p.Entry)
	}
	return nil
}
