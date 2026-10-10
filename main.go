package main

import (
	"fmt"
	"os"

	"shiroko/cli"
	"shiroko/cnv"
	"shiroko/core"
)

func main() {
	cmd := cli.ParseCli()

	if cmd.Error != "" {
		fmt.Fprintln(os.Stderr, cnv.CliError(cmd.Error))
		os.Exit(2)
	}

	if cmd.Help {
		fmt.Println(core.ShowHelp(cmd.Cli))
		return
	}
	if cmd.Version {
		fmt.Println(core.ShowVersion())
		return
	}

	var err error
	switch {
	case cmd.Init:
		err = cli.InitCmd(cmd.InitArgs)
	case cmd.Mod:
		err = cli.ModCmd(cmd.ModArgs)
	case cmd.Build:
		err = cli.BuildCmd(cmd.Verbose)
	case cmd.Run:
		err = cli.RunCmd(cmd.Verbose)
	case cmd.Check:
		err = cli.CheckCmd(cmd.Verbose)
	default:
		fmt.Fprintln(os.Stderr, core.ShowHelp(cmd.Cli))
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, cnv.PrintError(err))
		os.Exit(1)
	}
}
