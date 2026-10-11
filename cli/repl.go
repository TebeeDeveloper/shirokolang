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

// replStdPkgs are the stdlib packages the REPL auto-imports when the
// accumulated source references them via "<pkg>.".
var replStdPkgs = []string{
	"fmt", "strings", "strconv", "os", "math", "sort",
}

// replPkgMembers lists the stdlib members the completer suggests
// after a dot. Kept manually in sync with interpreter.installBuiltins.
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

// replShimTypes are the fmt aliases the REPL binds at top level. Sema
// sees real signatures; the interpreter gets matching globals via
// BindPackageMember. No source-level shim is emitted — spread (`...`)
// isn't supported by the register VM.
var replShimTypes = map[string]sema.Type{
	"print":    {"fn", []sema.Type{sema.TAny}, sema.TVoid, true},
	"println":  {"fn", []sema.Type{sema.TAny}, sema.TVoid, true},
	"printf":   {"fn", []sema.Type{sema.TAny}, sema.TVoid, true},
	"sprintf":  {"fn", []sema.Type{sema.TAny}, sema.TString, true},
	"sprintln": {"fn", []sema.Type{sema.TAny}, sema.TString, true},
	"errorf":   {"fn", []sema.Type{sema.TAny}, sema.TError, true},
}

// replBuiltinTypes exposes the runtime conversion builtins to sema.
// The interpreter defines them in installBuiltins; without these
// entries sema would only know them through the conversions map,
// which handles `int(x)` but not `let f = int`.
var replBuiltinTypes = map[string]sema.Type{
	"int":    {"fn", []sema.Type{sema.TAny}, sema.TInt, false},
	"float":  {"fn", []sema.Type{sema.TAny}, sema.TFloat, false},
	"byte":   {"fn", []sema.Type{sema.TAny}, sema.TByte, false},
	"string": {"fn", []sema.Type{sema.TAny}, sema.TString, false},
}

// ---------- entry point ----------

// ReplCmd runs an interactive Shiroko session against a single
// persistent interpreter. Globals, functions and structs survive
// across inputs, like Python's REPL.
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

// readInputMulti reads one logical input using the editor, adding
// continuation prompts while brackets are unbalanced. Ctrl-C during
// the first line returns an empty string (caller loops); Ctrl-C
// during a continuation discards the whole input.
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
		next, err := ed.ReadLine("..." + strings.Repeat("    ", braceDepth(buf)))
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

// braceDepth counts ()[]{} outside strings and comments. Used to
// decide whether the REPL should keep reading input.
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

// replState pairs a live interpreter (persistent globals) with the
// source text of every accumulated top-level declaration, so each new
// input can be re-sema'd with full context.
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

// addDecl handles a top-level declaration line. If the line is a
// `let <name> = <[]T|{}T>{...comp...}` form, it's split into a
// declare step + an assign step, because the lowerer's TopLet path
// uses l.lowerExpr, which rejects comprehension/range nodes.
// AssignStmt handles them fine.
func (r *replState) addDecl(line string) error {
	if name, ty, rhs, ok := splitLetComp(line); ok {
		if err := r.addDeclPlain("let " + name + " " + ty); err != nil {
			return err
		}
		return r.evalStmt(name + " = " + rhs)
	}
	return r.addDeclPlain(line)
}

// addDeclPlain is the accumulate-and-replace logic for ordinary
// declarations.
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

// execNewDecl rebuilds the full program (accumulated decls) for
// type-checking, but Exec's only the last declaration so the
// persistent interpreter doesn't re-run earlier initializers.
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

// evalInline decides between expression and statement.
//
// Expression path (auto-print):
//
//	func __repl__() -> any {
//	    let __v = (<line>)
//	    return __v
//	}
//
// We inspect the LetStmt's expression via sema.TypeOf; if it resolves
// to void or the wrapper fails to type-check as an expression, we
// fall back to the statement wrapper.
//
// Comprehension / range input skips the paren wrap because the
// grammar only accepts those forms on the RHS of a let or assign.
func (r *replState) evalInline(line string) error {
	if isCompLike(line) {
		wrapper := "func __repl__() -> any {\nlet __v = " + line + "\nreturn __v\n}\n"
		return r.evalWrapper(wrapper, true)
	}

	exprWrapper := "func __repl__() -> any {\nlet __v = (" + line + ")\nreturn __v\n}\n"
	if err := r.evalWrapper(exprWrapper, true); err == nil {
		return nil
	}
	return r.evalStmt(line)
}

// evalStmt runs `line` as the body of a synthetic void function.
// Assignments to globals land in the live interpreter; locals inside
// the wrapper are discarded.
func (r *replState) evalStmt(line string) error {
	wrapper := "func __repl__() {\n" + line + "\n}\n"
	return r.evalWrapper(wrapper, false)
}

// evalWrapper parses, semas, lowers and Exec's the wrapper, then
// calls it. If wantReturn is true, the wrapper's local `__v` must
// have a non-void type — otherwise we bail so the caller can try the
// statement path.
func (r *replState) evalWrapper(wrapper string, wantReturn bool) error {
	src := r.mkSrc(wrapper)
	prog, s, err := r.parseAndSema(src)
	if err != nil {
		return err
	}
	fnDecl := findFn(prog, "__repl__")
	if fnDecl == nil {
		return fmt.Errorf("cannot parse input")
	}

	if wantReturn && !wrapperHasValueReturn(s, fnDecl) {
		return errVoidInput
	}

	lw := lower.New(s)
	ir, lerrs := lw.LowerProgram([]any{"Program", prog[1], prog[2], []any{fnDecl}})
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
	if wantReturn {
		fmt.Println(interpreter.Display(v))
	}
	return nil
}

var errVoidInput = fmt.Errorf("void input")

// wrapperHasValueReturn inspects the synthetic expression wrapper
// `let __v = (...) ; return __v` and asks sema what type __v's
// initializer got. Anything non-void means the input was a real
// expression worth auto-printing.
func wrapperHasValueReturn(s *sema.Sema, fnDecl []any) bool {
	body, ok := fnDecl[5].([]any)
	if !ok || len(body) < 2 {
		return false
	}
	stmts, ok := body[1].([]any)
	if !ok || len(stmts) < 1 {
		return false
	}
	letStmt, ok := stmts[0].([]any)
	if !ok || len(letStmt) < 4 || letStmt[0].(string) != "LetStmt" {
		return false
	}
	names, ok := letStmt[1].([]string)
	if !ok || len(names) != 1 || names[0] != "__v" {
		return false
	}
	expr := letStmt[3]
	if expr == nil {
		return false
	}
	ty := s.TypeOf(expr)
	if ty == nil {
		return false
	}
	if len(ty) == 1 && ty[0] == "void" {
		return false
	}
	return true
}

// ---------- completion ----------

// completeWord is the line editor's tab-completion hook.
//
//   - after a dot: members of the named stdlib package (fmt, strings, ...)
//   - otherwise: keywords, builtin functions, std package names, and
//     every name declared in the session
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

// installSemaShims wires fmt aliases and conversion builtins into
// sema's global scope, skipping any name the user program has
// declared itself so a user-defined `func print` isn't clobbered.
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
			Name: name,
			Kind: "func",
			Type: ty,
		}
	}
	for name, ty := range replBuiltinTypes {
		if declared[name] {
			continue
		}
		s.Globals.Symbols[name] = &sema.Symbol{
			Name: name,
			Kind: "func",
			Type: ty,
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

// isCompLike reports whether `line` starts with a list/set literal.
// Comprehension and range syntax is only valid on the RHS of a let
// or assignment, so the REPL wraps these without surrounding parens.
func isCompLike(line string) bool {
	if !strings.HasPrefix(line, "[]") && !strings.HasPrefix(line, "{}") {
		return false
	}
	return strings.IndexByte(line, '{') > 0
}

// splitLetComp detects `let <name> = <[]T|{}T>{...}` where the body
// contains comprehension or range syntax, and returns the pieces so
// the REPL can issue a two-step declare + assign.
//
// Rationale: the lowerer's TopLet path uses l.lowerExpr, which rejects
// comprehension/range nodes ("only valid on the RHS of a let/assign").
// AssignStmt handles them fine, so we route through there.
//
// Plain list/set literals like `let x = []int{1, 2, 3}` are NOT
// touched — they go through the normal TopLet path unchanged.
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
	// Fire only for comp / range syntax. Plain literals go through
	// the normal TopLet path, which handles them.
	if !strings.Contains(body, "@") && !strings.Contains(body, "..") {
		return "", "", "", false
	}

	ty = strings.TrimSpace(rest[:braceIdx])
	rhs = rest
	return name, ty, rhs, true
}

// declName extracts the name introduced by a top-level declaration
// line, so the REPL can replace a re-declaration instead of letting
// sema error on a duplicate. Methods and malformed lines return "",
// false.
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
  - anything else is evaluated as an expression and printed, or run
    as a statement if it can't be one.
  - []T{...} and {}T{...} comprehensions and ranges work at the top
    level; e.g. []int{x @ int+ | x % 2 == 0; x < 10}.
  - multi-line input is detected by unbalanced brackets; the REPL
    keeps reading with a ".." prompt.
  - print / println / printf / sprintf / sprintln / errorf are
    pre-bound to fmt.* — no import needed.
  - fmt.println(...) works without an explicit import; the REPL
    auto-adds std packages when it sees "<pkg>."`)
}
