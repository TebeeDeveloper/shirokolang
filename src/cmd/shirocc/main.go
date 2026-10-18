package main

import (
	"fmt"
	"os"

	"shiroko/internal"
	"shiroko/internal/cli"
	"shiroko/internal/errors"
)

func main() {
	cmd := cli.ParseCli()

	if cmd.Error != "" {
		fmt.Fprintln(os.Stderr, cmd.Error)
		os.Exit(1)
	}

	var err error

	switch {
	case cmd.Version:
		fmt.Println(internal.ShowVersion())

	case cmd.Help:
		fmt.Println(internal.ShowHelp(cmd.Cli))

	case cmd.Init:
		err = cli.InitCmd(cmd.InitArgs)

	case cmd.Mod:
		err = cli.ModCmd(cmd.ModArgs)

	case cmd.Build:
		err = cli.BuildCmd(cmd.Verbose)

	case cmd.Check:
		err = cli.CheckCmd(cmd.Verbose)

	case cmd.Run:
		err = cli.RunCmd(cmd.Verbose)

	case cmd.Test:
		err = cli.TestCmd(cmd.Verbose)

	case cmd.Repl:
		err = cli.ReplCmd()

	case cmd.Import:
		err = cli.ImportCmd()

	case cmd.RemovePackage:
		for _, name := range cmd.RemovePackageArgs {
			if err := cli.RemovePackageCmd(name); err != nil {
				fmt.Fprintln(os.Stderr, errors.PrintError(err))
				os.Exit(1)
			}
		}

	case cmd.Compile:
		err = internal.CompileFile(cmd.Input, cmd.Output, cmd.Verbose)

	default:
		err = cmd.RaiseError()
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
