// core.go

package core

import (
	"fmt"
	"shiroko/cnv"
	"shiroko/core/cio"
	"shiroko/core/compile"
	"shiroko/core/parse"
	"shiroko/core/tokenize"
)

func CompileFile(input, output string, verbose bool) error {
	
	var inp cio.Read
	inp = cio.ReadFile(input)
	if inp.Err != nil {
		return inp.Err
	}
	
	var t tokenize.Tokenizer = tokenize.Init(inp.Code)
	if verbose {
		fmt.Println("[Shiroko] initialing tokenizer...")
	}
	var p parse.Parser = parse.Init(t)
	if verbose {
		fmt.Println("[Shiroko] initialing parser...")
	}
	var c compile.Compiler = compile.Init(p, verbose)
	if verbose {
		fmt.Println("[Shiroko] initialing compiler...")
		fmt.Println("[Compiler] start to compile.")
	}
	
	c.Compile()

	if len(p.Error) != 0 {
		return cnv.ParserError(p)
	}

	if len(c.Error) != 0 {
		return cnv.CompileError(c)
	}
	
	return nil
}

func ShowVersion() string {
	return "GCCSHI | Shiroko C++ Compiler . version 1"
}

func ShowHelp(cln string) string {
	return fmt.Sprintf(`Usage:
	%s build <input.shrko> <output>
	%s build-verbose <input.shrko> <output>
	Helps:
	%s version       => show version
	%s help          => show help
	%s build         => build your project
	%s build-verbose => build your project with verbos
	`, cln, cln, cln, cln, cln, cln)
}
