package cli

import (
	"shiroko/internal"
)

// BuildCmd is `shirocc build` — compiles the current project's entry
// file to a .go file next to it.
func BuildCmd(verbose bool) error {
	p, err := loadProject()
	if err != nil {
		return err
	}
	return internal.CompileFile(p.Entry, p.Output, verbose)
}
