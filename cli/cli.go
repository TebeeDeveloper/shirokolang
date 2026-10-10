// shiroko/cli/cli.go

package cli

import (
	"fmt"
	"os"
)

type Cmd struct {
	Cli string

	Version bool
	Help    bool

	Init     bool
	InitArgs []string

	Build   bool
	Verbose bool
	Input   string
	Output  string

	Mod     bool
	ModArgs []string

	Error string
}

func (cmd *Cmd) RaiseError() error {
	if cmd.Error != "" {
		return fmt.Errorf("%s", cmd.Error)
	}
	return fmt.Errorf("Need args to using this compiler. See at %s help", cmd.Cli)
}

func ParseCli() *Cmd {
	cln := os.Args[0]
	args := os.Args[0:]
	cli := &Cmd{Cli: cln}

	if len(args) < 2 {
		return &Cmd{
			Cli:   cln,
			Error: fmt.Sprintf("Need args to use. See at \"%s help\"", cln),
		}
	}

	c := args[1]

	switch c {
		case "version":
			cli.Version = true

		case "help":
			cli.Help = true

		case "mod":
			if len(args) <= 2 {
				cli.Error = fmt.Sprintf(
					"Need <package>/<module>. See at \"%s help\"", cln)
				return cli
			}
			cli.Mod = true
			cli.ModArgs = args[2:]

		case "init":
			if len(args) <= 2 {
				cli.Error = fmt.Sprintf(
					"Need a project name. See at \"%s help\"", cln)
				return cli
			}
			cli.Init = true
			cli.InitArgs = args[2:]

		case "compile":
			if len(args) <= 3 {
				cli.Error = fmt.Sprintf(
					"Need input-file (with extension \".shrko\") and output-file (any). See at \"%s help\"",
							cln)
				return cli
			}
			cli.Build = true
			cli.Input = args[2]
			cli.Output = args[3]

		case "compile-verbose":
			if len(args) <= 3 {
				cli.Error = fmt.Sprintf(
					"Need input-file (with extension \".shrko\") and output-file (any). See at \"%s help\"",
							cln)
				return cli
			}
			cli.Build = true
			cli.Verbose = true
			cli.Input = args[2]
			cli.Output = args[3]

		default:
			cli.Error = fmt.Sprintf(
				"Command \"%s\" unexpected. See at \"%s help\"", c, cln)
	}

	return cli
}
