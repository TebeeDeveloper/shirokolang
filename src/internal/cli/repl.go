package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"shiroko/internal/errors"
	"shiroko/internal/interpreter"
	"shiroko/internal/lower"
	"shiroko/internal/parser"
	"shiroko/internal/sema"
	"shiroko/internal/source"
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
	"if", "import", "interface", "iter", "let", "map", "match", "nil",
	"range", "return", "struct", "true",
}

var replBuiltins = []string{
	"append", "byte", "deepEqual", "errorf", "fieldsOf", "float",
	"input", "int", "len", "make", "methodsOf", "print", "printf",
	"println", "sprintf", "sprintln", "string",
}

// replShimTypes are the fmt aliases the REPL binds at top level.
var replShimTypes = map[string]sema.Type{
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
	"input":  {"fn", []sema.Type{sema.TString, sema.TAny}, sema.TString, false},
}

// ---------- registry (libs/shiropkgs.shiropkgs) ----------

// loadShiroPkgs reads libs/shiropkgs.shiropkgs — the package
// registry that ships with this shirocc install. Returns the set of
// import paths that are allowed in the REPL.
//
// Lookup:
//
//  1. $SHIROKO_LIBS/shiropkgs.shiropkgs, if $SHIROKO_LIBS is set.
//  2. <dir of shirocc binary>/libs/shiropkgs.shiropkgs
//
// A missing file is not an error: the returned set is empty.
func loadShiroPkgs() map[string]bool {
	out := map[string]bool{}

	path := ShiropkgsPath()
	if path == "" {
		return out
	}

	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	return out
}

// ---------- session state ----------

type replState struct {
	ip      *interpreter.Interpreter
	s       *sema.Sema
	ed      *lineEditor
	allowed map[string]bool // from libs/shiropkgs.shiropkgs
	loaded  map[string]bool // paths already imported this session
}

func newReplState(allowed map[string]bool) *replState {
	r := &replState{
		allowed: allowed,
		loaded:  map[string]bool{},
	}
	r.ip = interpreter.New(os.Stdout)
	r.s = sema.New(source.New(""))
	r.installShims()
	return r
}

// installShims binds the pre-bound fmt aliases (println, printf, …)
// and the conversion builtins (int, float, …) into the current sema
// and interpreter. Safe to call on a fresh session or after a reset.
func (r *replState) installShims() {
	for name, ty := range replBuiltinTypes {
		r.s.Globals.Symbols[name] = &sema.Symbol{
			Name: name, Kind: "func", Type: ty,
		}
	}
}

// ---------- entry point ----------

func ReplCmd() error {
	printReplBanner()

	allowed := loadShiroPkgs()
	r := newReplState(allowed)
	r.ed = newLineEditor(r.completeWord)

	// Share the line editor's reader with the interpreter so
	// input(...) doesn't fight the REPL over os.Stdin.
	r.ip.In = r.ed.Reader()

	for {
		line, err := readInputMulti(r.ed)
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

		if err := r.eval(line); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}

// ---------- evaluation ----------

func (r *replState) eval(line string) error {
	prog, perrs := parser.ParseReplLine(line + "\n")
	if len(perrs) > 0 {
		return errors.ParserError(perrs)
	}

	// Validate imports against libs/shiropkgs.shiropkgs.
	imports, _ := prog[2].([]string)
	for _, imp := range imports {
		if !r.allowed[imp] {
			return fmt.Errorf(
				"import %q is not available in this REPL\n"+
					"  hint: add %q to libs/shiropkgs.shiropkgs, or use one of:\n"+
					"        %s",
				imp, imp, r.availableList())
		}
		if r.loaded[imp] {
			continue
		}
		// Currently only std/* is loadable in the REPL. Non-std
		// packages listed in the registry are recognized but not yet
		// loaded — the loader + module-scope machinery for user
		// packages isn't wired into the REPL yet.
		if !strings.HasPrefix(imp, "std/") {
			return fmt.Errorf(
				"import %q: non-std packages are not yet supported in the REPL",
				imp)
		}
		r.loaded[imp] = true
	}

	errStart := len(r.s.Errors)
	r.s.Src = source.New(line + "\n")
	errs := r.s.ReplAnalyze(prog)
	if len(errs) > errStart {
		newErrs := errs[errStart:]
		r.s.Errors = r.s.Errors[:errStart]
		return errors.SemaError(newErrs)
	}

	lw := lower.New(r.s)
	ir, lerrs := lw.LowerProgram(prog)
	if len(lerrs) > 0 {
		return errors.LowerError(lerrs)
	}

	_, err := r.ip.Exec(ir)
	return err
}

// availableList renders the allowed import paths as a comma-separated
// list, for error hints.
func (r *replState) availableList() string {
	if len(r.allowed) == 0 {
		return "(none — libs/shiropkgs.shiropkgs was not found)"
	}
	var names []string
	for n := range r.allowed {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// ---------- commands ----------

func (r *replState) handleCommand(line string) bool {
	switch line {
	case ":q", ":quit", ":exit":
		return true
	case ":help", ":h", ":?":
		printReplHelp()
	case ":env":
		r.printEnv()
	case ":pkgs":
		r.printPkgs()
	case ":clear":
		r.reset()
		fmt.Println("session cleared")
	default:
		fmt.Fprintf(os.Stderr, "unknown REPL command %q — try :help\n", line)
	}
	return false
}

func (r *replState) reset() {
	r.ip = interpreter.New(os.Stdout)
	r.s = sema.New(source.New(""))
	r.loaded = map[string]bool{}
	r.installShims()
	r.ip.In = r.ed.Reader()
}

func (r *replState) printEnv() {
	if len(r.s.Globals.Symbols) == 0 {
		fmt.Println("(no bindings)")
		return
	}
	var names []string
	for name := range r.s.Globals.Symbols {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sym := r.s.Globals.Symbols[name]
		fmt.Printf("%-10s %-12s %s\n",
			sym.Kind, name, sema.TypeString(sym.Type))
	}
}

func (r *replState) printPkgs() {
	if len(r.allowed) == 0 {
		fmt.Println("(libs/shiropkgs.shiropkgs not found)")
		return
	}
	var names []string
	for n := range r.allowed {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		mark := " "
		if r.loaded[n] {
			mark = "*"
		}
		fmt.Printf("%s %s\n", mark, n)
	}
	fmt.Println("(* = imported this session)")
}

// ---------- multi-line input ----------

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
		// interpolated string — skip through, tracking holes
		if c == '#' && i+1 < len(s) && s[i+1] == '"' {
			i += 2
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' && i+1 < len(s) {
					i += 2
					continue
				}
				if s[i] == '{' {
					i++
					hd := 1
					for i < len(s) && hd > 0 {
						if s[i] == '{' {
							hd++
						} else if s[i] == '}' {
							hd--
						}
						i++
					}
					continue
				}
				i++
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
	for name := range r.s.Globals.Symbols {
		if strings.HasPrefix(name, word) {
			out = append(out, name)
		}
	}
	return sortedSet(out)
}

// ---------- banner ----------

func printReplBanner() {
	fmt.Println("Shiroko REPL")
	fmt.Println("  :help    this message")
	fmt.Println("  :env     list session bindings")
	fmt.Println("  :pkgs    list importable packages")
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
  :env             list session bindings
  :pkgs            list importable packages
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
  - Each line is parsed and evaluated on its own against the
    session's persistent globals. No mkSrc, no auto-printing.
  - Bare expressions (e.g. "1 + 2") are evaluated and discarded;
    wrap them in println(...) to see output.
  - "package" is a hard error: the REPL is always package main.
  - "import a.b.c" loads the package "a/b/c". The path must be
    listed in libs/shiropkgs.shiropkgs (next to the shirocc binary,
    or $SHIROKO_LIBS). Dots become slashes; the last segment is the
    visible name.
  - let/const/func/struct/interface declarations accumulate across
    inputs. Re-declaring the same name replaces the old binding.
  - print / println / printf / sprintf / sprintln / errorf are
    pre-bound to fmt.* — no import needed.
  - input("prompt") reads one line from stdin.`)
}
