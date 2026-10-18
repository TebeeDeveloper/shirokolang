package internal

import (
	"fmt"
	"os"

	"shiroko/internal/emitter"
	"shiroko/internal/errors"
	"shiroko/internal/interpreter"
	"shiroko/internal/lexer"
	"shiroko/internal/loader"
	"shiroko/internal/lower"
	"shiroko/internal/parser"
	"shiroko/internal/sema"
	"shiroko/internal/source"
)

// Build is the intermediate result of the frontend + lowerer.
type Build struct {
	Files   []string
	Source  *source.Source
	Program []any
	Sema    *sema.Sema
	IR      []any

	// Non-main packages loaded by the loader, keyed by package name.
	Packages map[string]*loader.Package

	// Per-package lowered IR and sema, keyed by declared name.
	PkgIR   map[string][]any
	PkgSema map[string]*sema.Sema
}

// BuildFile runs loader → lexer → parser → sema → lower and returns
// the IR plus everything in between.
func BuildFile(input string, verbose bool) (*Build, error) {
	if verbose {
		fmt.Println("Load packages...")
	}

	ldr := loader.NewLoader()
	res, err := ldr.Load(input)
	if err != nil {
		return nil, err
	}
	if verbose {
		for _, f := range res.Files {
			fmt.Println("  ", f)
		}
	}

	// Only the entry file goes through the classic pipeline.
	data, err := os.ReadFile(res.MainFile)
	if err != nil {
		return nil, err
	}
	src := source.New(string(data))

	if verbose {
		fmt.Println("Initialize Lexer...")
	}
	toks, err := lexer.Lex(string(data))
	if err != nil {
		return nil, err
	}

	if verbose {
		fmt.Println("Initialize Parser...")
	}
	p := parser.New(toks, src)
	prog := p.ParseProgram()
	if len(p.Errors) > 0 {
		return nil, errors.ParserError(p.Errors)
	}

	if verbose {
		fmt.Println("Initialize Sema...")
	}
	s := sema.New(src)
	s.SetUserPackages(semaUserPackages(res.Packages))
	if semaErrors := s.Analyze(prog); len(semaErrors) > 0 {
		return nil, errors.SemaError(semaErrors)
	}

	if verbose {
		fmt.Println("Sema no errors...")
		fmt.Println("Starting to translate to Shiroko-IR...")
	}
	lw := lower.New(s)
	ir, lowerErrors := lw.LowerProgram(prog)
	if len(lowerErrors) > 0 {
		return nil, errors.LowerError(lowerErrors)
	}

	// Run sema + lower on each loaded subpackage so the emitter has
	// real IR for their functions and structs.
	pkgIR := map[string][]any{}
	pkgSema := map[string]*sema.Sema{}
	userPkgs := semaUserPackages(res.Packages)
	for name, pkg := range res.Packages {
		ps := sema.New(pkg.Src)
		ps.SetUserPackages(userPkgs)
		if errs := ps.Analyze(pkg.Prog); len(errs) > 0 {
			return nil, errors.SemaError(errs)
		}
		plw := lower.New(ps)
		pir, lerrs := plw.LowerProgram(pkg.Prog)
		if len(lerrs) > 0 {
			return nil, errors.LowerError(lerrs)
		}
		pkgIR[name] = pir
		pkgSema[name] = ps
	}

	return &Build{
		Files:    res.Files,
		Source:   src,
		Program:  prog,
		Sema:     s,
		IR:       ir,
		Packages: res.Packages,
		PkgIR:    pkgIR,
		PkgSema:  pkgSema,
	}, nil
}

// CheckFile runs the frontend and reports whether the file type-checks.
func CheckFile(input string, verbose bool) (bool, error) {
	if _, err := BuildFile(input, verbose); err != nil {
		return false, err
	}
	return true, nil
}

// CompileFile transpiles Shiroko source to Go source and writes the
// result to `output`.
func CompileFile(input, output string, verbose bool) error {
	b, err := BuildFile(input, verbose)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Println("Starting to transpile IR to Golang...")
	}
	goSrc, cerr := emitter.GenerateWithPackages(
		b.IR, b.PkgIR, b.Packages, b.Source)
	if len(cerr) > 0 {
		return errors.EmitterError(cerr)
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
	return "ShiroCC | Shiroko Compact Compiler . version 26.12"
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
	%s repl                             open REPL
	%s version                          show version
	%s help                             show help

Environment:
	SHIROKO_PATH      colon-separated dirs to search for packages
	`, cln, cln, cln, cln, cln, cln, cln, cln, cln, cln, cln, cln)
}

// ---------- package → sema bridging ----------

func semaUserPackages(pkgs map[string]*loader.Package) map[string]*sema.UserPackage {
	out := make(map[string]*sema.UserPackage, len(pkgs))
	for name, p := range pkgs {
		up := &sema.UserPackage{
			Name:    name,
			Members: map[string]sema.Type{},
		}
		if len(p.Prog) < 4 {
			out[name] = up
			continue
		}
		decls, _ := p.Prog[3].([]any)
		for _, d := range decls {
			dd, ok := d.([]any)
			if !ok || len(dd) < 2 {
				continue
			}
			switch dd[0].(string) {
			case "FnDecl":
				up.Members[dd[1].(string)] = fnTypeOfDecl(dd)
			case "MethodDecl":
				// methods are reached via the receiver, not as
				// package members; skip
			case "StructDecl":
				up.Members[dd[1].(string)] = sema.Type{"struct", dd[1].(string)}
			case "InterfaceDecl":
				up.Members[dd[1].(string)] = sema.Type{"interface", dd[1].(string)}
			case "TopLet":
				for _, n := range dd[1].([]string) {
					up.Members[n] = sema.TAny
				}
			case "TopConst":
				up.Members[dd[1].(string)] = sema.TAny
			}
		}
		out[name] = up
	}
	return out
}

func fnTypeOfDecl(dd []any) sema.Type {
	params := dd[2].([]any)
	var ps []sema.Type
	variadic := false
	for _, p := range params {
		pp := p.([]any)
		ps = append(ps, typeNodeToSema(pp[2]))
		if v, ok := pp[3].(bool); ok && v {
			variadic = true
		}
	}
	ret := sema.TVoid
	if dd[3] != nil {
		ret = typeNodeToSema(dd[3])
	}
	var errTy sema.Type
	if len(dd) > 4 && dd[4] != nil {
		errTy = typeNodeToSema(dd[4])
	}
	if errTy != nil {
		return sema.Type{"fn", ps,
			sema.Type{"tuple", []sema.Type{ret, sema.TError}}, variadic}
	}
	return sema.Type{"fn", ps, ret, variadic}
}

func typeNodeToSema(node any) sema.Type {
	if node == nil {
		return sema.TVoid
	}
	n, ok := node.([]any)
	if !ok || len(n) == 0 {
		return sema.TAny
	}
	switch n[0].(string) {
	case "NamedType":
		switch n[1].(string) {
		case "int":
			return sema.TInt
		case "float":
			return sema.TFloat
		case "string":
			return sema.TString
		case "byte":
			return sema.TByte
		case "bool":
			return sema.TBool
		case "any":
			return sema.TAny
		case "error":
			return sema.TError
		}
		return sema.Type{"struct", n[1].(string)}
	case "PtrType":
		return sema.Type{"ptr", typeNodeToSema(n[1])}
	case "ListType":
		return sema.Type{"list", typeNodeToSema(n[1])}
	case "SetType":
		return sema.Type{"set", typeNodeToSema(n[1])}
	case "MapType":
		return sema.Type{"map", typeNodeToSema(n[1]), typeNodeToSema(n[2])}
	case "TupleType":
		var elems []sema.Type
		for _, e := range n[1].([]any) {
			elems = append(elems, typeNodeToSema(e))
		}
		return sema.Type{"tuple", elems}
	}
	return sema.TAny
}
