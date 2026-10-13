package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"shiroko/cnv"
	"shiroko/core/lexer"
	"shiroko/core/lower"
	"shiroko/core/parser"
	"shiroko/core/sema"
	"shiroko/core/source"
	"shiroko/interpreter"
)

// ---------- stdlib / completion tables ----------

var replStdPkgs = []string{
	"fmt", "strings", "strconv", "os", "math", "sort",
}

var replPkgMembers = map[string][]string{
	"fmt":     {"errorf", "print", "printf", "println", "sprintf", "sprintln"},
	"strings": {"contains", "has_prefix", "has_suffix", "index", "join", "repeat", "split", "to_lower", "to_upper", "trim"},
	"strconv": {"atoi", "itoa"},
	"os":      {"args", "exit", "getenv"},
	"math":    {"abs", "ceil", "floor", "pow", "sqrt"},
	"sort":    {"ints", "strings"},
}

var replKeywords = []string{
	"break", "const", "continue", "else", "false", "for", "func",
	"if", "interface", "iter", "let", "map", "match", "nil",
	"package", "range", "return", "struct", "true",
}

var replBuiltins = []string{
	"append", "byte", "deepEqual", "errorf", "fieldsOf", "float",
	"int", "len", "make", "methodsOf", "print", "printf", "println",
	"sprintf", "sprintln", "string",
}

// replShimTypes are the fmt aliases the REPL binds at top level.
var replShimTypes = map[string]sema.Type{
	"print":    {"fn", []sema.Type{sema.TAny}, sema.TVoid, true},
	"println":  {"fn", []sema.Type{sema.TAny}, sema.TVoid, true},
	"printf":   {"fn", []sema.Type{sema.TAny}, sema.TVoid, true},
	"sprintf":  {"fn", []sema.Type{sema.TAny}, sema.TString, true},
	"sprintln": {"fn", []sema.Type{sema.TAny}, sema.TString, true},
	"errorf":   {"fn", []sema.Type{sema.TAny}, sema.TError, true},
}

// replBuiltinTypes exposes the runtime conversion builtins to sema.
var replBuiltinTypes = map[string]sema.Type{
	"int":    {"fn", []sema.Type{sema.TAny}, sema.TInt, false},
	"float":  {"fn", []sema.Type{sema.TAny}, sema.TFloat, false},
	"byte":   {"fn", []sema.Type{sema.TAny}, sema.TByte, false},
	"string": {"fn", []sema.Type{sema.TAny}, sema.TString, false},
}

// ---------- entry point ----------

func ReplCmd() error {
	printReplBanner()

	ip := interpreter.New(os.Stdout)
	for name := range replShimTypes {
		ip.BindPackageMember(name, "fmt", name)
	}

	r := &replState{ip: ip}
	ed := newLineEditor(r.completeWord)

	for {
		line, err := readInputMulti(ed)
		if err == io.EOF {
			fmt.Println()
			return nil
		}
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, ":") {
			if r.handleCommand(line) {
				return nil
			}
			continue
		}
		if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "import{") {
			fmt.Println("hint: imports are automatic — just write fmt.println(...)")
			continue
		}

		if err := r.eval(line); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}

func readInputMulti(ed *lineEditor) (string, error) {
	line, err := ed.ReadLine(">> ")
	if err != nil {
		return "", err
	}
	if line == "" {
		return "", nil
	}
	buf := line
	for braceDepth(buf) > 0 {
		next, err := ed.ReadLine("..." + strings.Repeat(" ", braceDepth(buf)))
		if err != nil {
			return buf, err
		}
		if next == "" {
			return "", nil
		}
		buf += "\n" + next
	}
	return buf, nil
}

func braceDepth(s string) int {
	depth := 0
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '/' && i+1 < len(s) && s[i+1] == '/' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '*' {
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			i += 2
			continue
		}
		if c == '"' {
			i++
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' && i+1 < len(s) {
					i += 2
				} else {
					i++
				}
			}
			if i < len(s) {
				i++
			}
			continue
		}
		if c == '\'' {
			i++
			for i < len(s) && s[i] != '\'' {
				if s[i] == '\\' && i+1 < len(s) {
					i += 2
				} else {
					i++
				}
			}
			if i < len(s) {
				i++
			}
			continue
		}
		switch c {
		case '{', '(', '[':
			depth++
		case '}', ')', ']':
			depth--
		}
		i++
	}
	return depth
}

// ---------- session state ----------

type replState struct {
	ip        *interpreter.Interpreter
	declLines []string
}

func (r *replState) handleCommand(line string) bool {
	switch line {
	case ":q", ":quit", ":exit":
		return true
	case ":help", ":h", ":?":
		printReplHelp()
	case ":env":
		if len(r.declLines) == 0 {
			fmt.Println("(no declarations)")
			return false
		}
		for i, d := range r.declLines {
			name, ok := declName(d)
			if !ok {
				fmt.Printf("%2d: %s\n", i, firstLine(d))
			} else {
				fmt.Printf("%2d: %s\n", i, name)
			}
		}
	case ":clear":
		ip := interpreter.New(os.Stdout)
		for name := range replShimTypes {
			ip.BindPackageMember(name, "fmt", name)
		}
		r.ip = ip
		r.declLines = nil
		fmt.Println("session cleared")
	default:
		fmt.Fprintf(os.Stderr, "unknown REPL command %q — try :help\n", line)
	}
	return false
}

// ---------- evaluation ----------

func (r *replState) eval(line string) error {
	if isDeclLine(line) {
		return r.addDecl(line)
	}
	return r.evalInline(line)
}

// evalInline evaluates a non-declaration input. Three attempts, in
// order:
//
//  1. Block wrapper, if the input is `{ ... }`.
//  2. Value wrapper: `let __v = <line>; return __v`. Handles plain
//     expressions and comprehensions/ranges. Auto-prints if __v's
//     inferred type isn't void.
//  3. Statement wrapper: `func __repl__() { <line> }`. No output.
func (r *replState) evalInline(line string) error {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") && len(trimmed) >= 2 {
		if err := r.evalBlock(trimmed); err == nil {
			return nil
		}
	}
	if err := r.tryValueWrap(line); err == nil {
		return nil
	}
	return r.execStatement(line)
}

// evalBlock handles `{ <stmts> }` as a scoped block. If the last
// statement is a non-void expression, it becomes the block's value and
// is auto-printed.
func (r *replState) evalBlock(block string) error {
	inner := strings.TrimSpace(block[1 : len(block)-1])

	// Parse as a void function to inspect the statement list.
	wrap := "func __repl__() {\n" + inner + "\n}\n"
	prog, s, fn, err := r.parseSemaWrap(wrap)
	if err != nil {
		return err
	}
	stmts := functionStmts(fn)
	if len(stmts) == 0 {
		return r.runFn(prog, s, fn, false)
	}

	last, _ := stmts[len(stmts)-1].([]any)
	if last == nil || last[0].(string) != "ExprStmt" {
		return r.runFn(prog, s, fn, false)
	}
	expr := last[1]
	ty := s.TypeOf(expr)
	if ty == nil || sema.IsVoid(ty) {
		return r.runFn(prog, s, fn, false)
	}

	// Rewrite the last ExprStmt as a return and rebuild the function
	// with return type `any`.
	newStmts := append([]any{}, stmts[:len(stmts)-1]...)
	newStmts = append(newStmts, []any{"ReturnStmt", []any{expr}})
	newFn := []any{
		"FnDecl", "__repl__",
		[]any{},
		[]any{"NamedType", "any"},
		nil,
		[]any{"Block", newStmts},
	}
	newProg := replaceFn(prog, newFn)

	// Sema the modified program from scratch. The new AST nodes we
	// built lack position info, so errors here will report line 0
	// — acceptable for an internal wrapper.
	srcObj := source.New(r.mkSrc(wrap))
	s2 := sema.New(srcObj)
	installSemaShims(s2, newProg)
	if errs := s2.Analyze(newProg); len(errs) > 0 {
		return cnv.SemaError(errs)
	}
	return r.runFn(newProg, s2, newFn, true)
}

// tryValueWrap tries `let __v = <line>; return __v`. Handles plain
// expressions and comprehensions/ranges in one go. Returns errVoidInput
// if the wrapper type-checks but __v has void type, or a parse/sema
// error if the input isn't a value at all.
func (r *replState) tryValueWrap(line string) error {
	wrap := "func __repl__() -> any {\nlet __v = " + line + "\nreturn __v\n}\n"
	prog, s, fn, err := r.parseSemaWrap(wrap)
	if err != nil {
		return err
	}
	ty := s.LocalType(fn, "__v")
	if ty == nil || sema.IsVoid(ty) {
		return errVoidInput
	}
	return r.runFn(prog, s, fn, true)
}

// execStatement runs `line` as the body of a void synthetic function.
// Assignments to globals persist; locals are discarded.
func (r *replState) execStatement(line string) error {
	wrap := "func __repl__() {\n" + line + "\n}\n"
	prog, s, fn, err := r.parseSemaWrap(wrap)
	if err != nil {
		return err
	}
	return r.runFn(prog, s, fn, false)
}

// parseSemaWrap parses + semas the wrapper and returns the synthetic
// function decl along with the sema that analyzed it.
func (r *replState) parseSemaWrap(wrap string) ([]any, *sema.Sema, []any, error) {
	src := r.mkSrc(wrap)
	prog, s, err := r.parseAndSema(src)
	if err != nil {
		return nil, nil, nil, err
	}
	fn := findFn(prog, "__repl__")
	if fn == nil {
		return nil, nil, nil, errNoWrapper
	}
	return prog, s, fn, nil
}

// runFn lowers the synthetic function only, Exec's it, calls it, and
// (if print) displays the return value. The function is removed from
// globals afterwards so it doesn't leak between inputs.
func (r *replState) runFn(prog []any, s *sema.Sema, fn []any, print bool) error {
	lw := lower.New(s)
	ir, lerrs := lw.LowerProgram([]any{"Program", prog[1], prog[2], []any{fn}})
	if len(lerrs) > 0 {
		return cnv.LowerError(lerrs)
	}
	if _, err := r.ip.Exec(ir); err != nil {
		return err
	}
	defer delete(r.ip.GlobalNames, "__repl__")

	v, err := r.ip.CallGlobal("__repl__")
	if err != nil {
		return err
	}
	if print {
		fmt.Println(interpreter.Display(v))
	}
	return nil
}

var (
	errNoWrapper = fmt.Errorf("internal: wrapper function missing")
	errVoidInput = fmt.Errorf("void input")
)

// functionStmts returns the statement list of a parsed function body.
func functionStmts(fn []any) []any {
	if len(fn) < 6 {
		return nil
	}
	body, ok := fn[5].([]any)
	if !ok || len(body) < 2 {
		return nil
	}
	stmts, _ := body[1].([]any)
	return stmts
}

// replaceFn rewrites the program's decls, substituting __repl__ with
// the given node.
func replaceFn(prog []any, newFn []any) []any {
	decls, _ := prog[3].([]any)
	out := make([]any, 0, len(decls))
	for _, d := range decls {
		dd, ok := d.([]any)
		if ok && len(dd) > 1 && dd[0].(string) == "FnDecl" && dd[1].(string) == "__repl__" {
			out = append(out, newFn)
		} else {
			out = append(out, d)
		}
	}
	return []any{"Program", prog[1], prog[2], out}
}

// ---------- declarations ----------

func (r *replState) addDecl(line string) error {
	if name, ty, rhs, ok := splitLetComp(line); ok {
		if err := r.addDeclPlain("let " + name + " " + ty); err != nil {
			return err
		}
		return r.evalInline(name + " = " + rhs)
	}
	return r.addDeclPlain(line)
}

func (r *replState) addDeclPlain(line string) error {
	name, hasName := declName(line)
	replaceIdx := -1
	if hasName {
		for i, d := range r.declLines {
			if n, ok := declName(d); ok && n == name {
				replaceIdx = i
				break
			}
		}
	}

	if replaceIdx >= 0 {
		prev := r.declLines[replaceIdx]
		r.declLines[replaceIdx] = line
		if err := r.execNewDecl(); err != nil {
			r.declLines[replaceIdx] = prev
			return err
		}
		return nil
	}

	r.declLines = append(r.declLines, line)
	if err := r.execNewDecl(); err != nil {
		r.declLines = r.declLines[:len(r.declLines)-1]
		return err
	}
	return nil
}

func (r *replState) execNewDecl() error {
	if len(r.declLines) == 0 {
		return nil
	}
	src := r.mkSrc("")
	prog, s, err := r.parseAndSema(src)
	if err != nil {
		return err
	}
	decls, _ := prog[3].([]any)
	if len(decls) == 0 {
		return nil
	}
	target := decls[len(decls)-1]

	lw := lower.New(s)
	ir, lerrs := lw.LowerProgram([]any{"Program", prog[1], prog[2], []any{target}})
	if len(lerrs) > 0 {
		return cnv.LowerError(lerrs)
	}
	_, err = r.ip.Exec(ir)
	return err
}

// ---------- completion ----------

func (r *replState) completeWord(word string) []string {
	if word == "" {
		return nil
	}
	if i := strings.LastIndexByte(word, '.'); i >= 0 {
		pkg := word[:i]
		prefix := word[i+1:]
		members, ok := replPkgMembers[pkg]
		if !ok {
			return nil
		}
		var out []string
		for _, m := range members {
			if strings.HasPrefix(m, prefix) {
				out = append(out, pkg+"."+m)
			}
		}
		return sortedSet(out)
	}

	var out []string
	for _, k := range replKeywords {
		if strings.HasPrefix(k, word) {
			out = append(out, k)
		}
	}
	for _, b := range replBuiltins {
		if strings.HasPrefix(b, word) {
			out = append(out, b)
		}
	}
	for _, p := range replStdPkgs {
		if strings.HasPrefix(p, word) {
			out = append(out, p)
		}
	}
	for _, d := range r.declLines {
		if name, ok := declName(d); ok && strings.HasPrefix(name, word) {
			out = append(out, name)
		}
	}
	return sortedSet(out)
}

// ---------- pipeline helpers ----------

func (r *replState) mkSrc(extra string) string {
	var b strings.Builder
	b.WriteString("package main\n\n")

	full := strings.Join(r.declLines, "\n") + "\n" + extra
	var imports []string
	for _, pkg := range replStdPkgs {
		if strings.Contains(full, pkg+".") {
			imports = append(imports, "std/"+pkg)
		}
	}
	if len(imports) > 0 {
		b.WriteString("import {\n")
		for _, imp := range imports {
			fmt.Fprintf(&b, "    %q\n", imp)
		}
		b.WriteString("}\n\n")
	}

	for _, d := range r.declLines {
		b.WriteString(d)
		b.WriteString("\n")
	}
	b.WriteString(extra)
	return b.String()
}

func (r *replState) parseAndSema(src string) (prog []any, s *sema.Sema, err error) {
	toks, e := lexer.Lex(src)
	if e != nil {
		return nil, nil, e
	}
	srcObj := source.New(src)
	p := parser.New(toks, srcObj)
	prog = p.ParseProgram()
	if len(p.Errors) > 0 {
		return nil, nil, cnv.ParserError(p.Errors)
	}
	s = sema.New(srcObj)
	installSemaShims(s, prog)
	if errs := s.Analyze(prog); len(errs) > 0 {
		return nil, nil, cnv.SemaError(errs)
	}
	return prog, s, nil
}

func installSemaShims(s *sema.Sema, prog []any) {
	declared := map[string]bool{}
	if decls, ok := prog[3].([]any); ok {
		for _, d := range decls {
			dd, ok := d.([]any)
			if !ok || len(dd) < 2 {
				continue
			}
			switch dd[0].(string) {
			case "FnDecl", "StructDecl", "InterfaceDecl":
				if name, ok := dd[1].(string); ok {
					declared[name] = true
				}
			}
		}
	}
	for name, ty := range replShimTypes {
		if declared[name] {
			continue
		}
		s.Globals.Symbols[name] = &sema.Symbol{
			Name: name, Kind: "func", Type: ty,
		}
	}
	for name, ty := range replBuiltinTypes {
		if declared[name] {
			continue
		}
		s.Globals.Symbols[name] = &sema.Symbol{
			Name: name, Kind: "func", Type: ty,
		}
	}
}

func findFn(prog []any, name string) []any {
	decls, _ := prog[3].([]any)
	for _, d := range decls {
		dd, ok := d.([]any)
		if !ok || len(dd) < 2 {
			continue
		}
		if dd[0].(string) != "FnDecl" {
			continue
		}
		if n, ok := dd[1].(string); ok && n == name {
			return dd
		}
	}
	return nil
}

// ---------- decl / comp detection ----------

func isDeclLine(line string) bool {
	for _, kw := range []string{"func ", "struct ", "interface ", "let ", "const "} {
		if strings.HasPrefix(line, kw) {
			return true
		}
	}
	return false
}

// splitLetComp detects `let <name> = <[]T|{}T>{...}` where the body
// contains comprehension or range syntax, and returns the pieces so
// the REPL can issue a two-step declare + assign.
//
// The lowerer's TopLet path uses l.lowerExpr, which rejects comp
// nodes; AssignStmt handles them fine. So we route through there.
func splitLetComp(line string) (name, ty, rhs string, ok bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "let ") {
		return "", "", "", false
	}
	rest := strings.TrimSpace(t[4:])

	end := strings.IndexAny(rest, " \t=")
	if end <= 0 {
		return "", "", "", false
	}
	name = rest[:end]
	rest = strings.TrimSpace(rest[end:])
	if !strings.HasPrefix(rest, "=") {
		return "", "", "", false
	}
	rest = strings.TrimSpace(rest[1:])

	if !(strings.HasPrefix(rest, "[]") || strings.HasPrefix(rest, "{}")) {
		return "", "", "", false
	}
	braceIdx := strings.IndexByte(rest, '{')
	if braceIdx < 0 {
		return "", "", "", false
	}
	body := rest[braceIdx:]
	if !strings.Contains(body, "@") && !strings.Contains(body, "..") {
		return "", "", "", false
	}

	ty = strings.TrimSpace(rest[:braceIdx])
	rhs = rest
	return name, ty, rhs, true
}

func declName(line string) (string, bool) {
	t := strings.TrimSpace(line)

	for _, e := range []struct {
		prefix string
		plen   int
	}{
		{"let ", 4},
		{"const ", 6},
	} {
		if strings.HasPrefix(t, e.prefix) {
			rest := t[e.plen:]
			end := strings.IndexAny(rest, " \t,=")
			if end < 0 {
				return rest, true
			}
			return rest[:end], true
		}
	}

	if strings.HasPrefix(t, "func ") {
		rest := t[5:]
		if strings.HasPrefix(rest, "(") {
			close := strings.IndexByte(rest, ')')
			if close < 0 {
				return "", false
			}
			after := strings.TrimSpace(rest[close+1:])
			end := strings.IndexByte(after, '(')
			if end < 0 {
				return "", false
			}
			return strings.TrimSpace(after[:end]), true
		}
		end := strings.IndexByte(rest, '(')
		if end < 0 {
			return "", false
		}
		return strings.TrimSpace(rest[:end]), true
	}

	if strings.HasPrefix(t, "struct ") {
		rest := t[7:]
		end := strings.IndexAny(rest, " {")
		if end < 0 {
			return rest, true
		}
		return rest[:end], true
	}

	if strings.HasPrefix(t, "interface ") {
		rest := t[10:]
		end := strings.IndexAny(rest, " {")
		if end < 0 {
			return rest, true
		}
		return rest[:end], true
	}

	return "", false
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ---------- banner ----------

func printReplBanner() {
	fmt.Println("Shiroko REPL")
	fmt.Println("  :help    this message")
	fmt.Println("  :env     list accumulated declarations")
	fmt.Println("  :clear   reset the session")
	fmt.Println("  :q       quit")
	fmt.Println()
	fmt.Println("Keys: arrows · Ctrl-A/E (home/end) · Ctrl-K/U/W (kill) ·")
	fmt.Println("      Ctrl-L (clear) · Ctrl-C (cancel) · Ctrl-D (exit) · Tab (complete)")
	fmt.Println()
	fmt.Println("print / println / printf / sprintf / sprintln / errorf")
	fmt.Println("are pre-bound to fmt.* — no import needed.")
	fmt.Println()
}

func printReplHelp() {
	fmt.Println(`REPL commands:
  :help, :h, :?    show this help
  :env             list accumulated declarations
  :clear           reset the session
  :q, :quit, :exit exit

Editing:
  ← →              move cursor
  ↑ ↓              history
  Home / Ctrl-A    start of line
  End  / Ctrl-E    end of line
  Ctrl-K           kill to end of line
  Ctrl-U           kill to start of line
  Ctrl-W           kill previous word
  Ctrl-L           clear screen
  Ctrl-C           cancel current input
  Ctrl-D           exit (empty line) / delete char
  Tab              autocomplete

Input rules:
  - let x = 1, func f(...) ..., struct S ..., interface I ...
    accumulate across inputs. Re-declaring the same name replaces it.
  - bare expressions are evaluated and printed; statements run
    without output. Assignments to globals persist.
  - { ... } blocks scope their locals and auto-print the last
    non-void expression if there is one.
  - []T{...} and {}T{...} comprehensions and ranges work at the top
    level; e.g. []int{x @ int+ | x % 2 == 0; x < 10}.
  - multi-line input is detected by unbalanced brackets; the REPL
    keeps reading with a ".." prompt.
  - print / println / printf / sprintf / sprintln / errorf are
    pre-bound to fmt.* — no import needed.
  - fmt.println(...) works without an explicit import; the REPL
    auto-adds std packages when it sees "<pkg>."`)
}
