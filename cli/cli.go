// shiroko/cli/cli.go

package cli

import (
	"fmt"
	"os"
)

// Cmd is the parsed command line.
//
// The compiler verbs are Build, Run and Check. None of them take
// positional arguments — the project's shiroko.shrkomod supplies the
// entry file and output path.
type Cmd struct {
	Cli string

	Version bool
	Help    bool

	Init     bool
	InitArgs []string

	Mod     bool
	ModArgs []string

	Build   bool
	Run     bool
	Check   bool
	Verbose bool

	Error string
}

// RaiseError returns the parsed error, or a generic hint if none was
// set. Kept for callers that used to rely on it.
func (cmd *Cmd) RaiseError() error {
	if cmd.Error != "" {
		return fmt.Errorf("%s", cmd.Error)
	}
	return fmt.Errorf("need args. See `%s help`", cmd.Cli)
}

func ParseCli() *Cmd {
	cln := os.Args[0]
	args := os.Args[1:]
	cli := &Cmd{Cli: cln}

	if len(args) == 0 {
		cli.Error = fmt.Sprintf("need args. See `%s help`", cln)
		return cli
	}

	c := args[0]
	rest := args[1:]

	switch c {
	case "version":
		cli.Version = true

	case "help":
		cli.Help = true

	case "init":
		if len(rest) == 0 {
			cli.Error = fmt.Sprintf("`init` needs a project name. See `%s help`", cln)
			return cli
		}
		cli.Init = true
		cli.InitArgs = rest

	case "mod":
		if len(rest) == 0 {
			cli.Error = fmt.Sprintf(
				"`mod` needs <name> or <package>/<module>. See `%s help`", cln)
			return cli
		}
		cli.Mod = true
		cli.ModArgs = rest

	case "build", "run", "check":
		for _, a := range rest {
			switch a {
			case "-v", "--verbose":
				cli.Verbose = true
			default:
				cli.Error = fmt.Sprintf(
					"`%s` takes no arguments (got %q). See `%s help`",
					c, a, cln)
				return cli
			}
		}
		switch c {
		case "build":
			cli.Build = true
		case "run":
			cli.Run = true
		case "check":
			cli.Check = true
		}

	default:
		cli.Error = fmt.Sprintf("unknown command %q. See `%s help`", c, cln)
	}

	return cli
}
