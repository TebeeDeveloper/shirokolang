// shiroko/thread/thread.go

package thread

import (
	"shiroko/cnv"
	"shiroko/cli"
	"shiroko/core"
)

type Thread struct {
	Version string
	Help string
	Error error
}

func Run(cmd cli.Cmd) Thread {

	if cmd.Error != "" {
		return Thread{Error: cnv.CliError(cmd)}
	}

	if cmd.Version {
		return Thread{Version: core.ShowVersion()}

	}

	if cmd.Help {
		return Thread{Help: core.ShowHelp(cmd.Cli)}
	}

	if cmd.Build {
		var err error
		if cmd.Verbose {
			err = core.CompileFile(cmd.Input, cmd.Output, true)
			return Thread{Error: err}
		}
		err = core.CompileFile(cmd.Input, cmd.Output, false)
		return Thread{Error: err}
	}

	return Thread{}
}

func CliRun() Thread {
	var cmd cli.Cmd = cli.ParseCli()

	if cmd.Error != "" {
		return Thread{Error: cnv.CliError(cmd)}
	}
	if cmd.Version {
		return Thread{Version: core.ShowVersion()}

	}

	if cmd.Help {
		return Thread{Help: core.ShowHelp(cmd.Cli)}
	}

	if cmd.Build {
		var err error
		if cmd.Verbose {
			err = core.CompileFile(cmd.Input, cmd.Output, true)
			return Thread{Error: err}
		}
		err = core.CompileFile(cmd.Input, cmd.Output, false)
		return Thread{Error: err}
	}

	return Thread{}
}
