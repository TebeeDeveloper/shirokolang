// shiroko/cli/cli.go

package cli

import (
	"fmt"
	"os"
	"strings"
)

type Cmd struct {
	Cli string

	Version bool
	Help    bool

	Init     bool
	InitArgs []string

	Mod     bool
	ModArgs []string

	Build   bool
	Check   bool
	Run     bool
	Test    bool
	Repl    bool
	Verbose bool

	Compile bool
	Input   string
	Output  string

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
	rest := args[2:]

	// Split flags from positional arguments. Anything starting with
	// `-` is a flag; everything else is positional.
	var flags []string
	var positional []string
	for _, a := range rest {
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
		} else {
			positional = append(positional, a)
		}
	}
	for _, f := range flags {
		switch f {
		case "-v", "--verbose":
			cli.Verbose = true
		default:
			cli.Error = fmt.Sprintf("unknown flag %q", f)
			return cli
		}
	}

	switch c {
	case "version":
		cli.Version = true

	case "help":
		cli.Help = true

	case "init":
		if len(positional) < 1 {
			cli.Error = fmt.Sprintf(
				"Need a project name. See at \"%s help\"", cln)
			return cli
		}
		cli.Init = true
		cli.InitArgs = positional

	case "mod":
		if len(positional) < 1 {
			cli.Error = fmt.Sprintf(
				"Need <name> or <package>/<module>. See at \"%s help\"", cln)
			return cli
		}
		cli.Mod = true
		cli.ModArgs = positional

	case "build":
		cli.Build = true

	case "check":
		cli.Check = true

	case "run":
		cli.Run = true

	case "test":
		cli.Test = true

	case "repl":
		cli.Repl = true

	case "compile", "compile-verbose":
		if len(positional) < 2 {
			cli.Error = fmt.Sprintf(
				"Need input-file (with extension \".shrko\") and output-file (any). See at \"%s help\"",
				cln)
			return cli
		}
		cli.Compile = true
		cli.Input = positional[0]
		cli.Output = positional[1]
		if c == "compile-verbose" {
			cli.Verbose = true
		}

	default:
		cli.Error = fmt.Sprintf(
			"Command \"%s\" unexpected. See at \"%s help\"", c, cln)
	}

	return cli
}
