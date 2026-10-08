// shiroko/cli/cli.go

package cli

import (
	"fmt"
	"os"
)

type Cmd struct {
	Cli string
	Version bool
	Help bool
	Verbose bool
	Build bool
	BuildJS bool
	Input string
	Output string
	Source *string
	Error string
}

func (cmd *Cmd) RaiseError() error {
	if cmd.Error != "" {
		return fmt.Errorf("%s", cmd.Error)
	}
	return fmt.Errorf("Need args to using this compiler. See at %s help", cmd.Cli)
}

func ParseCli() (*Cmd) {
	cln := os.Args[0]
	args := os.Args[0:]
	cli := &Cmd{}
	if len(args) < 2 {
		return &Cmd{
			Error: fmt.Sprintf("Need args to use. See at \"%s help\"", cln),
		}
	}
	var c string = args[1]

	cli.Cli = cln

	switch c {
		case "version":
			cli.Version = true
		case "help":
			cli.Help = true
		case "compile":
			if len(args) <= 3 {
				cli.Error = fmt.Sprintf("Need input-file (with extension \".shrko\") and otput-file (any). See at \"%s help\"", cln)
				return cli
			}
			var inp string = args[2]
			var out string = args[3]
			cli.Build = true
			cli.Input = inp
			cli.Output = out

		case "compile-verbose":
			if len(args) <= 3 {
				cli.Error = fmt.Sprintf("Need input-file (with extension \".shrko\") and output-file (any). See at \"%s help\"", cln)
				return cli
			}
			var inp string = args[2]
			var out string = args[3]
			cli.Build = true
			cli.Verbose = true
			cli.Input = inp
			cli.Output = out
		default:
			cli.Error = fmt.Sprintf("Command \"%s\" unexpected. See at \"%s help\"", c, cln)
	}

	return cli
}
