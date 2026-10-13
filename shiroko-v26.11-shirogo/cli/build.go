package cli

import (
	"shiroko/core"
)

// BuildCmd is `shirocc build` — compiles the current project's entry
// file to a .go file next to it.
func BuildCmd(verbose bool) error {
	p, err := loadProject()
	if err != nil {
		return err
	}
	return core.CompileFile(p.Entry, p.Output, verbose)
}
