// core/core.go

package core

import (
	"fmt"
	"os"

	"shiroko/core/emitter"
	"shiroko/cnv"
	"shiroko/core/lexer"
	"shiroko/core/lower"
	"shiroko/core/parser"
	"shiroko/core/sema"
	"shiroko/core/source"
	"shiroko/pkg"
)

// Build is what every compiler command runs first: fetch deps, merge
// them with the main file into one source buffer, then run the whole
// frontend + lowerer. It returns everything later stages might need.
type Build struct {
	// Files lists every source file that went into the buffer, in
	// dependency order (main file last).
	Files []string

	// Source is the merged buffer as a *source.Source, so error
	// printers can show windows around a line.
	Source *source.Source

	// Program is the parsed AST.
	Program []any

	// Sema holds the symbol tables built during analysis.
	Sema *sema.Sema

	// IR is the lowered Shiroko-IR, ready for a backend.
	IR []any
}

// BuildFile runs loader → lexer → parser → sema → lower and returns
// the IR plus everything in between. Verbose prints each stage.
func BuildFile(input string, verbose bool) (*Build, error) {
	if verbose {
		fmt.Println("Load packages...")
	}

	ldr := pkg.NewLoader()
	res, err := ldr.Load(input)
	if err != nil {
		return nil, err
	}
	if verbose {
		for _, f := range res.Files {
			fmt.Println("  ", f)
		}
	}

	data := res.Source

	if verbose {
		fmt.Println("Initialize Lexer...")
	}
	toks, err := lexer.Lex(data)
	if err != nil {
		return nil, err
	}

	if verbose {
		fmt.Println("Initialize Parser...")
	}
	src := source.New(data)
	p := parser.New(toks, src)
	prog := p.ParseProgram()
	if len(p.Errors) > 0 {
		return nil, cnv.ParserError(p.Errors)
	}

	if verbose {
		fmt.Println("Initialize Sema...")
	}
	s := sema.New(src)
	if semaErrors := s.Analyze(prog); len(semaErrors) > 0 {
		return nil, cnv.SemaError(semaErrors)
	}

	if verbose {
		fmt.Println("Sema no errors...")
		fmt.Println("Starting to translate to Shiroko-IR...")
	}
	lw := lower.New()
	ir, lowerErrors := lw.LowerProgram(prog)
	if len(lowerErrors) > 0 {
		return nil, cnv.LowerError(lowerErrors)
	}

	return &Build{
		Files:   res.Files,
		Source:  src,
		Program: prog,
		Sema:    s,
		IR:      ir,
	}, nil
}

// CheckFile runs the front-end (loader → lexer → parser → sema →
// lower) and returns the IR. Same pipeline as BuildFile; the alias
// exists so `shirocc check` reads naturally and so the two can diverge
// later if we decide check should stop before lowering.
func CheckFile(input string, verbose bool) (*Build, error) {
	return BuildFile(input, verbose)
}

// CompileFile builds the IR, transpiles it to Go, and writes the result
// to `output`.
func CompileFile(input, output string, verbose bool) error {
	b, err := BuildFile(input, verbose)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Println("Starting to transpile IR to Golang...")
	}
	goSrc, cerr := emitter.Generate(b.IR, b.Source)
	if len(cerr) > 0 {
		return cnv.EmitterError(cerr)
	}

	if err := os.WriteFile(output, []byte(goSrc), 0644); err != nil {
		return err
	}
	fmt.Printf("Finished %s\n", output)
	return nil
}

func ShowVersion() string {
	return "shirocc | Shiroko Compact Compiler . version 26.11"
}

func ShowHelp(cln string) string {
	return fmt.Sprintf(`shirocc — Shiroko Compact Compiler

Usage:
  %s init <name>              create a new project
  %s mod <name>               add a library package (in ./source)
  %s mod <package>/<module>   add a nested library package

  %s build [-v]               compile the project to Go
  %s run   [-v]               compile and execute the project
  %s check [-v]               parse + type-check only

  %s version
  %s help

The entry file and output path come from the project's
shiroko.shrkomod. Defaults: source/main.shrko → build/main.go.

Environment:
  SHIROKO_PATH   colon-separated dirs to search for packages
  NO_COLOR       disable ANSI colors when set
`,
		cln, cln, cln, cln, cln, cln, cln, cln)
}
