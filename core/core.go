// core.go

package core

import (
	"fmt"
	"os"

	"shiroko/backend/golang"
	"shiroko/cnv"
	"shiroko/core/lexer"
	"shiroko/core/lower"
	"shiroko/core/parser"
	"shiroko/core/sema"
)

func CompileFile(input, output string, verbose bool) error {

	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}

	toks, err := lexer.Lex(string(data))
	if err != nil {
		return err
	}

	p := parser.New(toks)
	prog := p.ParseProgram()
	if len(p.Errors) > 0 {
		return cnv.ParserError(p.Errors)
	}

	s := sema.New()
	if semaErrors := s.Analyze(prog); len(semaErrors) > 0 {
		return cnv.SemaError(semaErrors)
	}

	lw := lower.New()
	ir, lowerErrors := lw.LowerProgram(prog)
	if len(lowerErrors) > 0 {
		return cnv.LowerError(lowerErrors)
	}

	goSrc := golang.EmitGo(ir)

	if err := os.WriteFile(output, []byte(goSrc), 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %s", output)

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
	%s build-verbose => build your project with verbose output
	`, cln, cln, cln, cln, cln, cln)
}
