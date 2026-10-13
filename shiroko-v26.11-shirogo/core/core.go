// core/core.go

package core

import (
	"fmt"
	"os"

	"shiroko/cnv"
	"shiroko/core/emitter"
	"shiroko/core/lexer"
	"shiroko/core/lower"
	"shiroko/core/parser"
	"shiroko/core/sema"
	"shiroko/core/source"
	"shiroko/interpreter"
	"shiroko/pkg"
)

// Build is the intermediate result of the frontend + lowerer.
type Build struct {
	Files   []string
	Source  *source.Source
	Program []any
	Sema    *sema.Sema
	IR      []any
}

// BuildFile runs loader → lexer → parser → sema → lower and returns
// the IR plus everything in between.
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
	src := source.NewWithFiles(data, res.Spans)
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
	lw := lower.New(s)
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

// CheckFile runs the frontend (loader → lexer → parser → sema) and
// reports whether the file type-checks cleanly.
func CheckFile(input string, verbose bool) (bool, error) {
	if _, err := BuildFile(input, verbose); err != nil {
		return false, err
	}
	return true, nil
}

// CompileFile transpiles Shiroko source to Go source and writes the
// result to `output`. This is the `shirocc build` / `shirocc compile`
// path: the emitter produces a Go file that you then `go build`.
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
	if verbose {
		fmt.Printf("Finished %s\n", output)
	}
	return nil
}

// RunInterpreted executes the program in a tree-walking interpreter.
// No Go is emitted; no `go run` subprocess.
func RunInterpreted(input string, verbose bool) error {
	b, err := BuildFile(input, verbose)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Println("Starting interpreter...")
	}
	ip := interpreter.New(os.Stdout)
	return ip.Run(b.IR)
}

func ShowVersion() string {
	return "GCCSHI | Shiroko C++ Compiler . version 26.10"
}

func ShowHelp(cln string) string {
	return fmt.Sprintf(`Usage:
	%s init <name>                      create a new project in ./<name>
	%s mod <name>                       create a library package in ./<name>
	%s mod <package>/<module>           create a nested library package
	%s build                            compile the current project to Go
	%s check                            type-check the current project
	%s run                              interpret the current project
	%s test                             interpret the project's tests
	%s compile <input.shrko> <output>   compile to Go
	%s compile-verbose <input.shrko> <output>
	                                    compile with verbose output

Other:
	%s version                          show version
	%s help                             show help

Environment:
	SHIROKO_PATH      colon-separated dirs to search for packages
	`, cln, cln, cln, cln, cln, cln, cln, cln, cln, cln, cln)
}
