package thread

import (
	"fmt"

	"shiroko/cli"
	"shiroko/core"
)

func RunJS(cmd *cli.Cmd) error {
	if cmd.Error != "" {
		return cmd.RaiseError()
	}
	return core.CompileFile(cmd.Input, cmd.Output, cmd.Verbose)
}

func RunCLI(cmd *cli.Cmd) string {
	if cmd.Error != "" {
		return fmt.Sprintf("%v\n", cmd.RaiseError())
	}
	if cmd.Version {
		return core.ShowVersion()
	}
	if cmd.Help {
		return core.ShowHelp(cmd.Cli)
	}
	err := core.CompileFile(cmd.Input, cmd.Output, cmd.Verbose)
	if err != nil {
		return fmt.Sprintf("%v", err)
	}
	return ""
}
