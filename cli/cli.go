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
	Input string
	Output string
	Error string
}

func ParseJS(cln string, args []string) (cli Cmd) {
	if len(args) < 2 {
		return Cmd{
			Error: fmt.Sprintf("Need args to use. See at \"%s help\"", cln),
		}
	}
	var c string = args[0]

	cli.Cli = cln

	switch c {
	case "version":
		cli.Version = true
	case "help":
		cli.Help = true
	case "build":
		if len(args) < 3 {
			cli.Error = fmt.Sprintf("Need input-file (with extension \".shrko\") and otput-file (any). See at \"%s help\"", cln)
			return
		}
		var inp string = args[1]
		var out string = args[2]
		cli.Build = true
		cli.Input = inp
		cli.Output = out

	case "build-verbose":
		if len(args) < 3 {
			cli.Error = fmt.Sprintf("Need input-file (with extension \".shrko\") and output-file (any). See at \"%s help\"", cln)
			return
		}
		var inp string = args[1]
		var out string = args[2]
		cli.Build = true
		cli.Verbose = true
		cli.Input = inp
		cli.Output = out
	default:
		cli.Error = fmt.Sprintf("Command \"%s\" unexpected. See at \"%s help\"", c, cln)
	}

	return
}

func ParseCli() (cli Cmd) {
	cli = ParseJS(os.Args[0], os.Args[1:])
	return
}
