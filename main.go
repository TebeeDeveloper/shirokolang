package main

import (
	"fmt"
	"os"

	"shiroko/cli"
	"shiroko/core"
)

func main() {
	cmd := cli.ParseCli()

	switch {
		case cmd.Error != "":
			fmt.Fprintln(os.Stderr, cmd.RaiseError())
			os.Exit(1)
		case cmd.Version:
			fmt.Println(core.ShowVersion())
		case cmd.Help:
			fmt.Print(core.ShowHelp(cmd.Cli))
		case cmd.Init:
			if err := cli.InitCmd(cmd.InitArgs); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case cmd.Mod:
			if err := cli.ModCmd(cmd.ModArgs); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case cmd.Build:
			if err := core.CompileFile(cmd.Input, cmd.Output, cmd.Verbose); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
	}
}
