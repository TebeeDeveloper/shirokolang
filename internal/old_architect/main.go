package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	dumpBC := flag.Bool("bc", false, "show disassembler before exec")
	stackSize := flag.Uint64("stack", 4096, "stack size initialize")
	heapSize := flag.Uint64("heap", 4096, "heap size initialize")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Printf("Usage: %s [-bc] [-stack N] [-heap N] <file>\n", os.Args[0])
		os.Exit(1)
	}

	filePath := flag.Arg(0)
	if !strings.HasSuffix(filePath, ".shrko") {
		fmt.Fprintln(os.Stderr, "file must have extension as '.shrko'")
	}
	rawBytes, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read file %q: %v\n", filePath, err)
		os.Exit(1)
	}

	src := string(rawBytes)

	// 1. Lexer
	tokens := Tokenize(src)

	// 2. Parser
	parser := NewParser(tokens)
	parser.SetSource(src)
	prog := parser.ParseProgram()
	if errs := parser.Errors(); len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "=== syntax error (Parser) ===")
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(1)
	}

	// 3. Semantic Analyzer
	analyzer := NewAnalyzer()
	analyzer.SetSource(src)
	analyzer.Analyze(prog)
	if errs := analyzer.Errors(); len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "=== semantic error (Analyzer) ===")
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(1)
	}

	// 4. Bytecode Compiler
	compiler := NewCompiler(analyzer)
	bcProg := compiler.Compile(prog)
	if errs := compiler.Errors(); len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "=== bytecode compile fail (Compiler) ===")
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(1)
	}

	if *dumpBC && bcProg != nil && bcProg.Main != nil {
		fmt.Print(DisassembleFunction(bcProg.Main))
	}

	// 5. Virtual Machine
	vm := NewVM(*stackSize, *heapSize)
	if _, err := vm.Run(bcProg); err != nil {
		fmt.Fprintf(os.Stderr, "=== execute error (VM) ===\n%v\n", err)
		os.Exit(1)
	}
}
