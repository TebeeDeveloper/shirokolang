package parser

import (
	"fmt"
	"strings"

	"shiroko/internal/color"
	"shiroko/internal/lexer"
	"shiroko/internal/source"
)

// ---------- errors ----------

type ParseError struct {
	Code string
	Msg  string
	Line int
	Col  int
	Src  *source.Source
}

func (e *ParseError) String() string {
	var b strings.Builder

	line := e.Line
	loc := fmt.Sprintf("line %d:%d", line, e.Col)
	if e.Src != nil {
		line = e.Src.DisplayLine(e.Line)
		if f := e.Src.DisplayFile(e.Line); f != "" {
			loc = fmt.Sprintf("%s:%d:%d", f, line, e.Col)
		} else {
			loc = fmt.Sprintf("line %d:%d", line, e.Col)
		}
	}

	fmt.Fprintf(&b, "%s %s %s",
		color.Yellow(loc),
		color.Dim("=>"),
		color.Magenta("parse error"),
	)

	gutter := color.Dim("    |")

	if e.Src != nil {
		for _, it := range e.Src.Window(e.Line, 4, 4) {
			if it.Line == e.Line {
				fmt.Fprintf(&b, "\n%s %s", gutter, it.Text)

				col := e.Col
				if col < 1 {
					col = 1
				}
				pad := strings.Repeat(" ", col-1)
				carets := "^"
				if n := len(it.Text) - col; n > 0 {
					carets += strings.Repeat("~", n)
				}
				fmt.Fprintf(&b, "\n%s %s%s", gutter, pad, color.Red(carets))
			} else {
				fmt.Fprintf(&b, "\n%s %s", gutter, color.Dim(it.Text))
			}
		}
	}

	fmt.Fprintf(&b, "\n%s %s",
		color.Dim("=>"),
		color.Yellow("reason:")+" "+e.Msg,
	)
	return b.String()
}

func (e *ParseError) Error() string { return e.String() }

type Node = []any

// ---------- parser ----------

type Parser struct {
	toks        []lexer.Token
	src         *source.Source
	i           int
	br          int
	noStructLit int
	sync        bool
	Errors      []*ParseError
}

func New(toks []lexer.Token, src *source.Source) *Parser {
	return &Parser{toks: toks, src: src}
}

// ParseReplLine lexes and parses one REPL input in a single call.
// The REPL uses this as its front door.
//
// Lex errors are wrapped as a single-element ParseError slice so the
// caller has one shape to handle.
func ParseReplLine(src string) ([]any, []*ParseError) {
	toks, err := lexer.Lex(src)
	if err != nil {
		return nil, []*ParseError{{Msg: err.Error()}}
	}
	p := New(toks, source.New(src))
	prog := p.ParseReplLine()
	return prog, p.Errors
}

// ---------- error plumbing ----------

func (p *Parser) errAt(tok lexer.Token, msg string) {
	e := &ParseError{Msg: msg, Line: tok.Line, Col: tok.Col, Src: p.src}
	p.Errors = append(p.Errors, e)
	p.sync = true
}

func (p *Parser) errAtCode(tok lexer.Token, code, msg string) {
	e := &ParseError{
		Code: code,
		Msg:  fmt.Sprintf("at line %d:%d: %s (got %s %q)", tok.Line, tok.Col, msg, tok.Kind, tok.Value),
		Line: tok.Line, Col: tok.Col, Src: p.src,
	}
	p.Errors = append(p.Errors, e)
	p.sync = true
}

func (p *Parser) synced() bool { return p.sync }

func (p *Parser) recover(stopAtBrace bool) {
	p.br = 0
	p.noStructLit = 0
	for !p.At("EOF") {
		if p.At("NEWLINE") || p.At(";") {
			p.Advance()
			break
		}
		if stopAtBrace && p.At("}") {
			break
		}
		p.Advance()
	}
	p.sync = false
}

// ---------- position helpers ----------

func (p *Parser) pos() (int, int) {
	tok := p.Peek()
	return tok.Line, tok.Col
}

func withPos(n []any, line, col int) []any {
	return append(n, line, col)
}

func withNames(n []any, names [][2]int, line, col int) []any {
	return append(n, names, line, col)
}

// ---------- token plumbing ----------

func (p *Parser) first(j int) (int, lexer.Token) {
	for j < len(p.toks) {
		tok := p.toks[j]
		if tok.Kind == "NEWLINE" && p.br > 0 {
			j++
			continue
		}
		return j, tok
	}
	return len(p.toks) - 1, p.toks[len(p.toks)-1]
}

func (p *Parser) Peek() lexer.Token {
	_, tok := p.first(p.i)
	return tok
}

func (p *Parser) PeekN(n int) lexer.Token {
	j := p.i
	for k := 0; k < n; k++ {
		j, _ = p.first(j)
		j++
	}
	_, tok := p.first(j)
	return tok
}

func (p *Parser) Advance() lexer.Token {
	j, tok := p.first(p.i)
	p.i = j + 1
	return tok
}

func (p *Parser) At(kind string) bool { return p.Peek().Kind == kind }

func (p *Parser) Eat(kind string) lexer.Token {
	tok := p.Peek()
	if tok.Kind != kind {
		p.errAtCode(tok, "E_EXPECTED",
			fmt.Sprintf("expected %s, got %s", kind, tok.Kind))
		return tok
	}
	return p.Advance()
}

func (p *Parser) EatIdent() string {
	s, _, _ := p.EatIdentPos()
	return s
}

func (p *Parser) EatIdentPos() (string, int, int) {
	tok := p.Peek()
	if tok.Kind != "IDENT" {
		// temporary debug: show the last few tokens
		start := p.i - 5
		if start < 0 {
			start = 0
		}
		end := p.i + 2
		if end > len(p.toks) {
			end = len(p.toks)
		}
		var ctx []string
		for j := start; j < end; j++ {
			marker := ""
			if j == p.i {
				marker = " <<<"
			}
			ctx = append(ctx, fmt.Sprintf("[%s %v]%s",
				p.toks[j].Kind, p.toks[j].Value, marker))
		}
		p.errAtCode(tok, "E_EXPECTED_IDENT",
			fmt.Sprintf("expected identifier, got %s\n   context: %s",
				tok.Kind, strings.Join(ctx, " ")))
		return "", 0, 0
	}

	p.Advance()
	s, _ := tok.Value.(string)
	return s, tok.Line, tok.Col
}

func (p *Parser) EatString() string {
	tok := p.Peek()
	if tok.Kind != "STRING" {
		p.errAtCode(tok, "E_EXPECTED_STRING",
			fmt.Sprintf("expected string, got %s", tok.Kind))
		return ""
	}
	p.Advance()
	s, _ := tok.Value.(string)
	return s
}

func (p *Parser) isTerm() bool {
	return p.At("NEWLINE") || p.At(";") || p.At("}") || p.At("EOF")
}

func (p *Parser) skipNewlines() {
	for p.At("NEWLINE") || p.At(";") {
		p.Advance()
	}
}

func (p *Parser) expectTerminator() {
	if p.At("}") || p.At("EOF") {
		return
	}
	if p.At("NEWLINE") || p.At(";") {
		p.Advance()
		return
	}
	p.errAtCode(p.Peek(), "E_TERMINATOR", "expected newline or ';'")
}

func (p *Parser) peekPastNewlines() lexer.Token {
	j := p.i
	for j < len(p.toks) && p.toks[j].Kind == "NEWLINE" {
		j++
	}
	if j >= len(p.toks) {
		return lexer.Token{Kind: "EOF"}
	}
	return p.toks[j]
}

// ---------- program ----------

func (p *Parser) ParseProgram() []any {
	p.skipNewlines()
	pkg := "<error>"
	var imports []string
	var decls []any

	if p.At("PACKAGE") {
		p.Advance()
		pkg = p.EatIdent()
		if !p.synced() {
			p.expectTerminator()
		}
	}
	if p.synced() {
		p.recover(false)
	}
	p.skipNewlines()

	if p.At("IMPORT") {
		p.Advance()
		p.Eat("{")
		if !p.synced() {
			p.br++
			p.skipNewlines()
			for !p.At("}") && !p.At("EOF") && !p.synced() {
				imports = append(imports, p.EatString())
				p.skipNewlines()
			}
			p.Eat("}")
			p.br--
		}
		if !p.synced() {
			p.expectTerminator()
		}
		if p.synced() {
			p.recover(false)
		}
		p.skipNewlines()
	}

	for !p.At("EOF") {
		d := p.ParseTopDecl()
		if p.synced() {
			p.recover(false)
			p.skipNewlines()
			continue
		}
		if d != nil {
			decls = append(decls, d)
		}
		p.skipNewlines()
	}

	return []any{"Program", pkg, imports, decls}
}

// ParseReplLine parses one REPL input. It differs from ParseProgram
// in two ways:
//
//   - `package` is a hard error: the REPL is always package main.
//   - `import` uses dotted syntax: `import a.b.c` becomes the
//     import path "a/b/c". Multiple imports may appear on separate
//     lines within the same input.
//
// The return shape matches ParseProgram:
//
//	["Program", "main", imports, decls]
func (p *Parser) ParseReplLine() []any {
	p.skipNewlines()

	pkg := "main"
	var imports []string
	var decls []any

	// Hard error on `package`.
	if p.At("PACKAGE") {
		tok := p.Peek()
		p.errAtCode(tok, "E_REPL_PACKAGE",
			"`package` is not allowed in the REPL — it's always package main")
		p.recover(false)
		p.skipNewlines()
	}

	// Handle any number of `import a.b.c` lines.
	for p.At("IMPORT") {
		p.Advance()

		// At least one identifier.
		first, _, _ := p.EatIdentPos()
		if p.synced() {
			p.recover(false)
			p.skipNewlines()
			continue
		}
		path := first

		// Zero or more `.ident` segments.
		for p.At("/") {
			p.Advance()
			seg, _, _ := p.EatIdentPos()
			if p.synced() {
				break
			}
			path += "/" + seg
		}
		if p.synced() {
			p.recover(false)
			p.skipNewlines()
			continue
		}

		imports = append(imports, path)

		if !p.synced() {
			p.expectTerminator()
		}
		if p.synced() {
			p.recover(false)
		}
		p.skipNewlines()
	}

	// Everything else parses as top-level decls.
	for !p.At("EOF") {
		d := p.ParseTopDecl()
		if p.synced() {
			p.recover(false)
			p.skipNewlines()
			continue
		}
		if d != nil {
			decls = append(decls, d)
		}
		p.skipNewlines()
	}

	return []any{"Program", pkg, imports, decls}
}

func (p *Parser) ParseTopDecl() any {
	if p.At("INTERFACE") {
		return p.parseInterface()
	}
	if p.At("STRUCT") {
		return p.parseStruct()
	}
	if p.At("FUNC") {
		return p.parseFn()
	}
	if p.At("LET") {
		return p.parseTopLet()
	}
	if p.At("CONST") {
		return p.parseTopConst()
	}
	line, col := p.pos()
	st := p.parseStmt()
	if p.synced() || st == nil {
		return nil
	}
	return withPos([]any{"TopStmt", []any{st}}, line, col)
}

// ---------- types ----------

func (p *Parser) parseType() any {
	if p.synced() {
		return nil
	}
	if p.At("*") {
		p.Advance()
		inner := p.parseType()
		if p.synced() {
			return nil
		}
		return []any{"PtrType", inner}
	}
	if p.At("MAP") {
		p.Advance()
		p.Eat("[")
		key := p.parseType()
		p.Eat("]")
		val := p.parseType()
		if p.synced() {
			return nil
		}
		return []any{"MapType", key, val}
	}
	if p.At("[") {
		p.Advance()
		p.Eat("]")
		inner := p.parseType()
		if p.synced() {
			return nil
		}
		return []any{"ListType", inner}
	}
	if p.At("FUNC") {
		p.Advance()
		p.Eat("(")
		p.br++
		var params []any
		if !p.At(")") {
			params = append(params, p.parseType())
			for !p.synced() && p.At(",") {
				p.Advance()
				params = append(params, p.parseType())
			}
		}
		p.Eat(")")
		p.br--
		var ret any
		if !p.synced() && p.At("->") {
			p.Advance()
			ret = p.parseType()
		}
		if p.synced() {
			return nil
		}
		return []any{"FuncType", params, ret}
	}
	name := p.EatIdent()
	if p.synced() {
		return nil
	}
	return []any{"NamedType", name}
}

func (p *Parser) parseTupleType() any {
	p.Eat("(")
	p.br++
	var elems []any
	if !p.At(")") {
		elems = append(elems, p.parseType())
		for !p.synced() && p.At(",") {
			p.Advance()
			elems = append(elems, p.parseType())
		}
	}
	p.Eat(")")
	p.br--
	if p.synced() {
		return nil
	}
	if len(elems) == 0 {
		p.errAtCode(p.Peek(), "E_EMPTY_TUPLE", "empty tuple type")
		return nil
	}
	if len(elems) == 1 {
		return elems[0]
	}
	return []any{"TupleType", elems}
}

// ---------- declarations ----------

func (p *Parser) parseInterface() any {
	if p.synced() {
		return nil
	}
	line, col := p.pos()
	p.Eat("INTERFACE")
	name, nl, nc := p.EatIdentPos()
	p.Eat("{")
	var methods []any
	p.skipNewlines()
	for !p.At("}") && !p.At("EOF") && !p.synced() {
		mname := p.EatIdent()
		p.Eat("(")
		p.br++
		var params []any
		if !p.At(")") {
			params = append(params, p.parseParam()...)
			for !p.synced() && p.At(",") {
				p.Advance()
				params = append(params, p.parseParam()...)
			}
		}
		p.Eat(")")
		p.br--
		var ret any
		if !p.synced() && p.At("->") {
			p.Advance()
			p.skipNewlines()
			if p.At("(") {
				ret = p.parseTupleType()
			} else {
				ret = p.parseType()
			}
		}
		if p.synced() {
			break
		}
		methods = append(methods, []any{"MethodSig", mname, params, ret})
		p.expectTerminator()
		if p.synced() {
			break
		}
		p.skipNewlines()
	}
	p.Eat("}")
	if p.synced() {
		return nil
	}
	return withNames([]any{"InterfaceDecl", name, methods}, [][2]int{{nl, nc}}, line, col)
}

func (p *Parser) parseStruct() any {
	if p.synced() {
		return nil
	}
	line, col := p.pos()
	p.Eat("STRUCT")
	name, nl, nc := p.EatIdentPos()
	p.Eat("{")
	var fields []any
	p.skipNewlines()
	for !p.At("}") && !p.At("EOF") && !p.synced() {
		fname := p.EatIdent()
		fty := p.parseType()
		if p.synced() {
			break
		}
		fields = append(fields, []any{"Field", fname, fty})
		p.expectTerminator()
		if p.synced() {
			break
		}
		p.skipNewlines()
	}
	p.Eat("}")
	if p.synced() {
		return nil
	}
	return withNames([]any{"StructDecl", name, fields}, [][2]int{{nl, nc}}, line, col)
}

func (p *Parser) parseParam() []any {
	name, nl, nc := p.EatIdentPos()
	if p.synced() {
		return nil
	}
	names := []string{name}
	positions := [][2]int{{nl, nc}}

	if !p.At(",") {
		variadic := false
		if p.At("...") {
			p.Advance()
			variadic = true
		}
		ty := p.parseType()
		if p.synced() {
			return nil
		}
		return []any{[]any{"Param", names[0], ty, variadic, positions}}
	}
	for p.At(",") {
		p.Advance()
		n, l, c := p.EatIdentPos()
		names = append(names, n)
		positions = append(positions, [2]int{l, c})
		if p.synced() {
			return nil
		}
	}
	variadic := false
	if p.At("...") {
		p.Advance()
		variadic = true
	}
	ty := p.parseType()
	if p.synced() {
		return nil
	}
	params := make([]any, 0, len(names))
	for i, n := range names {
		params = append(params, []any{"Param", n, ty, variadic, [][2]int{positions[i]}})
	}
	return params
}

func (p *Parser) parseFn() any {
	if p.synced() {
		return nil
	}
	line, col := p.pos()
	p.Eat("FUNC")

	recv := ""
	var recvPos, namePos [2]int

	first, fl, fc := p.EatIdentPos()
	namePos = [2]int{fl, fc}
	name := first

	if p.At(".") {
		p.Advance()
		recv = first
		recvPos = [2]int{fl, fc}
		n, nl, nc := p.EatIdentPos()
		name = n
		namePos = [2]int{nl, nc}
	}
	if p.synced() {
		return nil
	}

	p.Eat("(")
	p.br++
	var params []any
	if !p.At(")") {
		params = append(params, p.parseParam()...)
		for !p.synced() && p.At(",") {
			p.Advance()
			params = append(params, p.parseParam()...)
		}
	}
	p.Eat(")")
	p.br--
	p.skipNewlines()

	var ret, errTy any
	if !p.synced() && p.At("->") {
		p.Advance()
		p.skipNewlines()
		if p.At("(") {
			ret = p.parseTupleType()
		} else {
			ret = p.parseType()
		}
		p.skipNewlines()
		if !p.synced() && p.At("!") {
			p.Advance()
			p.skipNewlines()
			errTy = p.parseType()
			p.skipNewlines()
		}
	}
	if p.synced() {
		return nil
	}
	body := p.parseBlock()
	if p.synced() {
		return nil
	}
	if recv != "" {
		return withNames(
			[]any{"MethodDecl", recv, name, params, ret, errTy, body},
			[][2]int{namePos, recvPos}, line, col)
	}
	return withNames(
		[]any{"FnDecl", name, params, ret, errTy, body},
		[][2]int{namePos}, line, col)
}

func (p *Parser) parseTopLet() any {
	line, col := p.pos()
	p.Eat("LET")

	names := []string{}
	positions := [][2]int{}
	n, nl, nc := p.EatIdentPos()
	names = append(names, n)
	positions = append(positions, [2]int{nl, nc})
	for !p.synced() && p.At(",") {
		p.Advance()
		n, nl, nc := p.EatIdentPos()
		names = append(names, n)
		positions = append(positions, [2]int{nl, nc})
	}
	if p.synced() {
		return nil
	}

	var expr, els any

	parseElse := func() {
		if p.synced() || !p.At("ELSE") {
			return
		}
		p.Advance()
		p.skipNewlines()
		els = p.parseBlock()
	}

	if !p.At("=") {
		p.errAtCode(p.Peek(), "E_LET_EQ",
			"let requires `= <expr>`; type annotations are not supported")
		return nil
	}
	p.Advance()
	expr = p.parseExpr()
	parseElse()
	if p.synced() {
		return nil
	}

	if els != nil {
		if len(names) != 1 {
			p.errAtCode(p.Peek(), "E_LET_ELSE_MULTI",
				"let-else does not support multiple names")
			return nil
		}
		inner := withNames(
			[]any{"LetStmt", names, nil, expr, els},
			positions, line, col)
		return withPos([]any{"TopStmt", []any{inner}}, line, col)
	}

	return withNames(
		[]any{"TopLet", names, nil, expr},
		positions, line, col)
}

func (p *Parser) parseTopConst() any {
	line, col := p.pos()
	p.Eat("CONST")
	name, nl, nc := p.EatIdentPos()
	p.Eat("=")
	expr := p.parseExpr()
	if p.synced() {
		return nil
	}
	return withNames(
		[]any{"TopConst", name, expr},
		[][2]int{{nl, nc}}, line, col)
}

// ---------- statements ----------

func (p *Parser) parseBlock() any {
	savedBr := p.br
	p.br = 0
	defer func() { p.br = savedBr }()

	p.Eat("{")
	var stmts []any
	p.skipNewlines()
	for !p.At("}") && !p.At("EOF") {
		st := p.parseStmt()
		if p.synced() {
			p.recover(true)
			continue
		}
		if st != nil {
			stmts = append(stmts, st)
		}
		p.expectTerminator()
		if p.synced() {
			p.recover(true)
			continue
		}
		p.skipNewlines()
	}
	p.Eat("}")
	return []any{"Block", stmts}
}

func (p *Parser) parseStmt() any {
	if p.At("LET") {
		return p.parseLet()
	}
	if p.At("CONST") {
		return p.parseConst()
	}
	if p.At("FOR") {
		return p.parseFor()
	}
	if p.At("IF") {
		return p.parseIf()
	}
	if p.At("RETURN") {
		return p.parseReturn()
	}
	if p.At("BREAK") {
		line, col := p.pos()
		p.Advance()
		if p.synced() {
			return nil
		}
		return withPos([]any{"BreakStmt"}, line, col)
	}
	if p.At("CONTINUE") {
		line, col := p.pos()
		p.Advance()
		if p.synced() {
			return nil
		}
		return withPos([]any{"ContinueStmt"}, line, col)
	}
	if p.At("IDENT") && (p.PeekN(1).Kind == "++" || p.PeekN(1).Kind == "--") {
		line, col := p.pos()
		name := p.EatIdent()
		op := p.Advance().Kind
		if p.synced() {
			return nil
		}
		return withPos([]any{"IncDecStmt", name, op}, line, col)
	}
	line, col := p.pos()
	e := p.parseExpr()
	if p.synced() {
		return nil
	}
	if p.At("=") {
		p.Advance()
		rhs := p.parseExpr()
		if p.synced() {
			return nil
		}
		return withPos([]any{"AssignStmt", e, rhs}, line, col)
	}
	return withPos([]any{"ExprStmt", e}, line, col)
}

func (p *Parser) parseLet() any {
	line, col := p.pos()
	p.Eat("LET")

	names := []string{}
	positions := [][2]int{}
	n, nl, nc := p.EatIdentPos()
	names = append(names, n)
	positions = append(positions, [2]int{nl, nc})
	for !p.synced() && p.At(",") {
		p.Advance()
		n, nl, nc := p.EatIdentPos()
		names = append(names, n)
		positions = append(positions, [2]int{nl, nc})
	}

	var expr, els any

	parseElse := func() {
		if p.synced() || !p.At("ELSE") {
			return
		}
		p.Advance()
		p.skipNewlines()
		els = p.parseBlock()
	}

	if !p.At("=") {
		p.errAtCode(p.Peek(), "E_LET_EQ",
			"let requires `= <expr>`; type annotations are not supported")
		return nil
	}
	p.Advance()
	expr = p.parseExpr()
	parseElse()
	if p.synced() {
		return nil
	}
	if els != nil && len(names) > 1 {
		p.errAtCode(p.Peek(), "E_LET_ELSE_MULTI",
			"let-else does not support multiple names")
		return nil
	}
	return withNames(
		[]any{"LetStmt", names, nil, expr, els},
		positions, line, col)
}

func (p *Parser) parseConst() any {
	line, col := p.pos()
	p.Eat("CONST")
	name, nl, nc := p.EatIdentPos()
	p.Eat("=")
	expr := p.parseExpr()
	if p.synced() {
		return nil
	}
	return withNames(
		[]any{"ConstStmt", name, expr},
		[][2]int{{nl, nc}}, line, col)
}

func (p *Parser) parseReturn() any {
	line, col := p.pos()
	p.Eat("RETURN")

	var exprs []any
	if !p.synced() && !p.isTerm() {
		exprs = append(exprs, p.parseExpr())
		for !p.synced() && p.At(",") {
			p.Advance()
			exprs = append(exprs, p.parseExpr())
		}
	}
	if p.synced() {
		return nil
	}
	return withPos([]any{"ReturnStmt", exprs}, line, col)
}

func (p *Parser) parseIf() any {
	line, col := p.pos()
	p.Eat("IF")
	p.noStructLit++
	cond := p.parseExpr()
	p.noStructLit--
	if p.synced() {
		return nil
	}
	p.skipNewlines()
	thenBlock := p.parseBlock()
	if p.synced() {
		return nil
	}

	var els any
	if p.peekPastNewlines().Kind == "ELSE" {
		p.skipNewlines()
		p.Eat("ELSE")
		p.skipNewlines()
		if p.At("IF") {
			els = p.parseIf()
		} else {
			els = p.parseBlock()
		}
		if p.synced() {
			return nil
		}
	}
	return withPos([]any{"IfStmt", cond, thenBlock, els}, line, col)
}

// ---------- for loops ----------

func (p *Parser) parseFor() any {
	line, col := p.pos()
	p.Eat("FOR")

	if p.At("RANGE") {
		p.Advance()
		p.noStructLit++
		count := p.parseExpr()
		p.noStructLit--
		if p.synced() {
			return nil
		}
		p.skipNewlines()
		body := p.parseBlock()
		if p.synced() {
			return nil
		}
		return withPos([]any{"ForRangeStmt", count, body}, line, col)
	}

	if p.At("{") {
		p.skipNewlines()
		body := p.parseBlock()
		if p.synced() {
			return nil
		}
		return withPos([]any{"ForCondStmt", []any{"BoolExpr", true, line, col}, body}, line, col)
	}

	if p.At("IDENT") {
		n1 := p.PeekN(1).Kind

		if n1 == "=" {
			if p.PeekN(2).Kind == "ITER" {
				name := p.EatIdent()
				p.Eat("=")
				p.Eat("ITER")
				p.noStructLit++
				src := p.parseExpr()
				p.noStructLit--
				if p.synced() {
					return nil
				}
				p.skipNewlines()
				body := p.parseBlock()
				if p.synced() {
					return nil
				}
				return withPos([]any{"ForIterStmt", name, nil, src, body}, line, col)
			}
			name := p.EatIdent()
			p.Eat("=")
			p.noStructLit++
			init := p.parseExpr()
			p.Eat(",")
			cond := p.parseExpr()
			p.Eat(",")
			post := p.parseForPost()
			p.noStructLit--
			if p.synced() {
				return nil
			}
			p.skipNewlines()
			body := p.parseBlock()
			if p.synced() {
				return nil
			}
			return withPos([]any{"ForCStmt", name, init, cond, post, body}, line, col)
		}

		if n1 == "," {
			idx := p.EatIdent()
			p.Eat(",")
			val := p.EatIdent()
			p.Eat("=")
			p.Eat("ITER")
			p.noStructLit++
			src := p.parseExpr()
			p.noStructLit--
			if p.synced() {
				return nil
			}
			p.skipNewlines()
			body := p.parseBlock()
			if p.synced() {
				return nil
			}
			return withPos([]any{"ForIterStmt", val, idx, src, body}, line, col)
		}
	}

	p.noStructLit++
	cond := p.parseExpr()
	p.noStructLit--
	if p.synced() {
		return nil
	}
	p.skipNewlines()
	body := p.parseBlock()
	if p.synced() {
		return nil
	}
	return withPos([]any{"ForCondStmt", cond, body}, line, col)
}

func (p *Parser) parseForPost() any {
	line, col := p.pos()
	if p.At("IDENT") && (p.PeekN(1).Kind == "++" || p.PeekN(1).Kind == "--") {
		name := p.EatIdent()
		op := p.Advance().Kind
		if p.synced() {
			return nil
		}
		return withPos([]any{"IncDecStmt", name, op}, line, col)
	}
	e := p.parseExpr()
	if p.synced() {
		return nil
	}
	if p.At("=") {
		p.Advance()
		rhs := p.parseExpr()
		if p.synced() {
			return nil
		}
		return withPos([]any{"AssignStmt", e, rhs}, line, col)
	}
	return withPos([]any{"ExprStmt", e}, line, col)
}

// ---------- expressions ----------

var binOps = []map[string]bool{
	{"||": true},
	{"&&": true},
	{"==": true, "!=": true},
	{"<": true, ">": true, "<=": true, ">=": true},
	{"+": true, "-": true},
	{"*": true, "/": true, "%": true},
}

func (p *Parser) parseExpr() any { return p.parseBin(0) }

func (p *Parser) parseBin(lvl int) any {
	if p.synced() {
		return nil
	}
	if lvl >= len(binOps) {
		return p.parseUnary()
	}
	left := p.parseBin(lvl + 1)
	if p.synced() {
		return nil
	}
	for {
		save := p.i
		p.skipNewlines()
		if !binOps[lvl][p.Peek().Kind] {
			p.i = save
			break
		}
		line, col := p.pos()
		op := p.Advance().Kind
		p.skipNewlines()
		right := p.parseBin(lvl + 1)
		if p.synced() {
			return nil
		}
		left = withPos([]any{"BinaryExpr", op, left, right}, line, col)
	}
	return left
}

func (p *Parser) parseUnary() any {
	if p.synced() {
		return nil
	}
	if p.At("-") || p.At("!") || p.At("&") || p.At("*") {
		line, col := p.pos()
		op := p.Advance().Kind
		inner := p.parseUnary()
		if p.synced() {
			return nil
		}
		return withPos([]any{"UnaryExpr", op, inner}, line, col)
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() any {
	if p.synced() {
		return nil
	}
	e := p.parsePrimary()
	if p.synced() {
		return nil
	}
	for {
		switch {
		case p.At("("):
			line, col := p.pos()
			p.Eat("(")
			p.br++
			var args []any
			if !p.At(")") {
				args = append(args, p.parseCallArg())
				for !p.synced() && p.At(",") {
					p.Advance()
					args = append(args, p.parseCallArg())
				}
			}
			p.Eat(")")
			p.br--
			if p.synced() {
				return nil
			}
			e = withPos([]any{"CallExpr", e, args}, line, col)
		case p.At("."):
			line, col := p.pos()
			p.Advance()
			name, nl, nc := p.EatIdentPos()
			if p.synced() {
				return nil
			}
			e = withNames(
				[]any{"SelectorExpr", e, name},
				[][2]int{{nl, nc}}, line, col)
		case p.At("["):
			line, col := p.pos()
			p.Advance()
			p.br++
			var lo any
			if !p.At(":") {
				lo = p.parseExpr()
			}
			if p.synced() {
				p.br--
				return nil
			}
			if p.At(":") {
				p.Advance()
				var hi any
				if !p.At("]") {
					hi = p.parseExpr()
				}
				p.Eat("]")
				p.br--
				if p.synced() {
					return nil
				}
				e = withPos([]any{"SliceExpr", e, lo, hi}, line, col)
			} else {
				p.Eat("]")
				p.br--
				if p.synced() {
					return nil
				}
				e = withPos([]any{"IndexExpr", e, lo}, line, col)
			}
		case p.At("++") || p.At("--"):
			line, col := p.pos()
			op := p.Advance().Kind
			e = withPos([]any{"PostfixExpr", op, e}, line, col)
		default:
			return e
		}
	}
}

func (p *Parser) parseCallArg() any {
	if p.synced() {
		return nil
	}
	e := p.parseExpr()
	if p.synced() {
		return nil
	}
	if p.At("...") {
		line, col := p.pos()
		p.Advance()
		return withPos([]any{"SpreadExpr", e}, line, col)
	}
	return e
}

func (p *Parser) parseMatch() any {
	p.Eat("MATCH")
	line, col := p.pos()
	p.noStructLit++
	scrutinee := p.parseExpr()
	p.noStructLit--
	if p.synced() {
		return nil
	}
	p.skipNewlines()
	p.Eat("{")
	p.br++
	p.skipNewlines()
	var arms []any
	for !p.At("}") && !p.At("EOF") {
		pat := p.parsePattern()
		if p.synced() {
			p.recover(true)
			continue
		}
		p.Eat("=>")
		p.skipNewlines()
		var body any
		if p.At("{") {
			body = p.parseBlock()
		} else {
			body = p.parseExpr()
		}
		if p.synced() {
			p.recover(true)
			continue
		}
		arms = append(arms, []any{"MatchArm", pat, body})
		if p.At(",") {
			p.Advance()
		}
		p.skipNewlines()
	}
	p.Eat("}")
	p.br--
	if p.synced() {
		return nil
	}
	return withPos([]any{"MatchExpr", scrutinee, arms}, line, col)
}

func (p *Parser) parsePattern() any {
	if p.synced() {
		return nil
	}
	tok := p.Peek()
	line, col := tok.Line, tok.Col

	// wildcard
	if tok.Kind == "IDENT" {
		if s, _ := tok.Value.(string); s == "_" {
			// but only if the next token is => — `_` could also be
			// an identifier expression in some positions
			if p.PeekN(1).Kind == "=>" {
				p.Advance()
				return withPos([]any{"WildcardPattern"}, line, col)
			}
		}
	}

	// parse any expression
	lo := p.parseExpr()
	if p.synced() {
		return nil
	}

	// range pattern?
	if p.At("..") {
		p.Advance()
		hi := p.parseExpr()
		if p.synced() {
			return nil
		}
		return withPos([]any{"RangePattern", lo, hi}, line, col)
	}

	return withPos([]any{"LitPattern", lo}, line, col)
}

func (p *Parser) parsePrimary() any {
	if p.synced() {
		return nil
	}
	tok := p.Peek()
	switch tok.Kind {
	case "INT":
		p.Advance()
		return withPos([]any{"IntExpr", tok.Value}, tok.Line, tok.Col)
	case "FLOAT":
		p.Advance()
		return withPos([]any{"FloatExpr", tok.Value}, tok.Line, tok.Col)
	case "BYTE":
		p.Advance()
		return withPos([]any{"ByteExpr", tok.Value}, tok.Line, tok.Col)
	case "STRING":
		p.Advance()
		return withPos([]any{"StrExpr", tok.Value}, tok.Line, tok.Col)
	// ★ interpolated string literal
	case "ISTRING":
		p.Advance()
		return p.parseInterpString(tok)
	case "(":
		p.Eat("(")
		p.br++
		e := p.parseExpr()
		p.Eat(")")
		p.br--
		if p.synced() {
			return nil
		}
		return e
	case "[":
		return p.parseListOrComp()
	case "MAP":
		return p.parseMapLit()
	case "IDENT":
		name, _ := tok.Value.(string)
		p.Advance()
		if name == "true" {
			return withPos([]any{"BoolExpr", true}, tok.Line, tok.Col)
		}
		if name == "false" {
			return withPos([]any{"BoolExpr", false}, tok.Line, tok.Col)
		}
		if name == "nil" {
			return withPos([]any{"NilExpr"}, tok.Line, tok.Col)
		}
		if p.At("{") && p.noStructLit == 0 {
			return p.parseStructLit(name, tok.Line, tok.Col)
		}
		return withPos([]any{"IdentExpr", name}, tok.Line, tok.Col)
	case "FUNC":
		return p.parseFnLit()
	case "MATCH":
		return p.parseMatch()
	}
	p.errAtCode(tok, "E_PRIMARY",
		fmt.Sprintf("unexpected token %s", tok.Kind))
	return nil
}

// ★ parseInterpString turns an ISTRING token into an InterpStrExpr AST
// node. Each "expr" segment is re-lexed and re-parsed on its own so
// the hole's grammar is just "any expression".
//
// AST shape:
//
//	InterpStrExpr{parts, line, col}
//
// where parts is []any and each element is either:
//
//	[]any{"StrPart", text}
//	[]any{"ExprPart", expr}
func (p *Parser) parseInterpString(tok lexer.Token) any {
	segs, _ := tok.Value.([]lexer.InterpSeg)

	var parts []any
	for _, seg := range segs {
		switch seg.Kind {
		case "text":
			parts = append(parts, []any{"StrPart", seg.Text})
		case "expr":
			expr, err := p.relexHole(seg)
			if err != nil {
				p.errAtCode(tok, "E_INTERP_HOLE", err.Error())
				return nil
			}
			if expr == nil {
				p.errAtCode(tok, "E_INTERP_HOLE",
					fmt.Sprintf("empty expression in interpolated string hole at line %d, col %d",
						seg.Line, seg.Col))
				return nil
			}
			parts = append(parts, []any{"ExprPart", expr, seg.Fmt})
		}
	}
	return withPos([]any{"InterpStrExpr", parts}, tok.Line, tok.Col)
}

// ★ relexHole runs a fresh lex+parse over the raw source of one {...}
// hole. Errors are returned as plain messages (not ParseErrors) so
// parseInterpString can attach them to the ISTRING token's position.
func (p *Parser) relexHole(seg lexer.InterpSeg) (any, error) {
	src := seg.Src
	if strings.TrimSpace(src) == "" {
		return nil, nil
	}

	toks, err := lexer.Lex(src)
	if err != nil {
		return nil, fmt.Errorf(
			"in interpolated-string hole at line %d, col %d: %w",
			seg.Line, seg.Col, err)
	}

	// Drop EOF and any trailing NEWLINEs so the mini-parser sees only
	// the expression tokens.
	if len(toks) > 0 && toks[len(toks)-1].Kind == "EOF" {
		toks = toks[:len(toks)-1]
	}
	for len(toks) > 0 && toks[len(toks)-1].Kind == "NEWLINE" {
		toks = toks[:len(toks)-1]
	}
	if len(toks) == 0 {
		return nil, nil
	}

	sub := New(toks, p.src)

	// ★ Skip any leading NEWLINEs before parsing — parseExpr does not
	// do this itself.
	sub.skipNewlines()
	if sub.synced() {
		return nil, fmt.Errorf(
			"in interpolated-string hole at line %d, col %d: invalid expression",
			seg.Line, seg.Col)
	}

	e := sub.parseExpr()

	// Any leftover tokens after parseExpr means the hole has trailing
	// garbage — a real error.
	if !sub.synced() {
		// consume trailing NEWLINEs the parser may have stopped on
		sub.skipNewlines()
		if sub.i < len(sub.toks) {
			// trailing tokens remain
			return nil, fmt.Errorf(
				"in interpolated-string hole at line %d, col %d: "+
					"unexpected trailing tokens",
				seg.Line, seg.Col)
		}
	}

	if sub.synced() || len(sub.Errors) > 0 {
		msg := "invalid expression in interpolated-string hole"
		if len(sub.Errors) > 0 {
			msg = sub.Errors[0].Msg
		}
		return nil, fmt.Errorf(
			"in interpolated-string hole at line %d, col %d: %s",
			seg.Line, seg.Col, msg)
	}

	if e == nil {
		return nil, nil
	}
	return e, nil
}

// ---------- literals & comprehensions ----------

func (p *Parser) parseListOrComp() any {
	p.Eat("[")
	p.Eat("]")
	elemTy := p.parseType()
	if p.synced() {
		return nil
	}
	p.Eat("{")
	p.br++

	if p.At("}") {
		p.Eat("}")
		p.br--
		return []any{"ListLitExpr", []any{"ListType", elemTy}, []any{}}
	}

	first := p.parseExpr()
	if p.synced() {
		return nil
	}
	if p.At("@") {
		return p.parseCompAfterFirst("ListCompExpr", []any{"ListType", elemTy}, first)
	}

	if p.At("..") {
		p.Advance()
		hi := p.parseExpr()
		p.Eat("}")
		p.br--
		if p.synced() {
			return nil
		}
		return []any{"RangeLitExpr", []any{"ListType", elemTy}, first, hi}
	}

	elems := []any{first}
	for !p.synced() && p.At(",") {
		p.Advance()
		elems = append(elems, p.parseExpr())
	}
	p.Eat("}")
	p.br--
	if p.synced() {
		return nil
	}
	return []any{"ListLitExpr", []any{"ListType", elemTy}, elems}
}

func (p *Parser) parseMapLit() any {
	p.Eat("MAP")
	p.Eat("[")
	keyTy := p.parseType()
	p.Eat("]")
	valTy := p.parseType()
	if p.synced() {
		return nil
	}
	fullTy := []any{"MapType", keyTy, valTy}

	p.Eat("{")
	p.br++
	if p.At("}") {
		p.Eat("}")
		p.br--
		return []any{"MapLitExpr", fullTy, []any{}}
	}
	var entries []any
	for !p.At("}") && !p.At("EOF") && !p.synced() {
		k := p.parseExpr()
		if p.synced() {
			return nil
		}
		p.Eat(":")
		v := p.parseExpr()
		if p.synced() {
			return nil
		}
		entries = append(entries, []any{"MapEntry", k, v})
		if p.At(",") {
			p.Advance()
		}
		p.skipNewlines()
	}
	p.Eat("}")
	p.br--
	if p.synced() {
		return nil
	}
	return []any{"MapLitExpr", fullTy, entries}
}

func (p *Parser) compVar(e any) string {
	n, ok := e.([]any)
	if !ok {
		return ""
	}
	switch n[0].(string) {
	case "IdentExpr":
		return n[1].(string)
	case "BinaryExpr":
		if r := p.compVar(n[2]); r != "" {
			return r
		}
		return p.compVar(n[3])
	case "UnaryExpr":
		return p.compVar(n[2])
	case "CallExpr":
		if r := p.compVar(n[1]); r != "" {
			return r
		}
		for _, a := range n[2].([]any) {
			if r := p.compVar(a); r != "" {
				return r
			}
		}
	case "IndexExpr":
		if r := p.compVar(n[1]); r != "" {
			return r
		}
		return p.compVar(n[2])
	case "SliceExpr":
		if r := p.compVar(n[1]); r != "" {
			return r
		}
		if r := p.compVar(n[2]); r != "" {
			return r
		}
		return p.compVar(n[3])
	case "SelectorExpr":
		return p.compVar(n[1])
	}
	return ""
}

func (p *Parser) parseCompAfterFirst(nodeKind string, fullTy any, elem any) any {
	p.Advance()
	varName := p.compVar(elem)
	if varName == "" {
		p.errAtCode(p.Peek(), "E_COMP_VAR",
			"comprehension element must reference a loop variable")
		return nil
	}

	var src any
	if p.At("IDENT") && p.PeekN(1).Kind == "+" {
		n2 := p.PeekN(2).Kind
		if n2 == "|" || n2 == ";" || n2 == "}" {
			v, _ := p.Advance().Value.(string)
			p.Advance()
			src = []any{"SeqExpr", v, "+"}
		} else {
			src = p.parseExpr()
		}
	} else {
		src = p.parseExpr()
	}
	if p.synced() {
		return nil
	}

	var filt, cond any
	if p.At("|") {
		p.Advance()
		filt = p.parseExpr()
	}
	if p.synced() {
		return nil
	}
	if p.At(";") {
		p.Advance()
		cond = p.parseExpr()
	}
	if p.synced() {
		return nil
	}
	p.Eat("}")
	p.br--
	if p.synced() {
		return nil
	}
	return []any{nodeKind, fullTy, varName, src, filt, cond, elem}
}

func (p *Parser) parseStructLit(name string, line, col int) any {
	p.Eat("{")
	p.br++
	var fields []any
	p.skipNewlines()
	for !p.At("}") && !p.At("EOF") && !p.synced() {
		fname := p.EatIdent()
		p.Eat(":")
		p.skipNewlines()
		fval := p.parseExpr()
		if p.synced() {
			return nil
		}
		fields = append(fields, []any{"FieldInit", fname, fval})
		p.skipNewlines()
		if p.At(",") {
			p.Advance()
			p.skipNewlines()
			continue
		}
		break
	}
	p.skipNewlines()
	p.Eat("}")
	p.br--
	if p.synced() {
		return nil
	}
	return withPos([]any{"StructLitExpr", name, fields}, line, col)
}

func (p *Parser) parseFnLit() any {
	line, col := p.pos()
	p.Eat("FUNC")
	p.Eat("(")
	p.br++
	var params []any
	if !p.At(")") {
		params = append(params, p.parseParam()...)
		for !p.synced() && p.At(",") {
			p.Advance()
			params = append(params, p.parseParam()...)
		}
	}
	p.Eat(")")
	p.br--
	p.skipNewlines()

	var ret, errTy any
	if !p.synced() && p.At("->") {
		p.Advance()
		p.skipNewlines()
		if p.At("(") {
			ret = p.parseTupleType()
		} else {
			ret = p.parseType()
		}
		p.skipNewlines()
		if !p.synced() && p.At("!") {
			p.Advance()
			p.skipNewlines()
			errTy = p.parseType()
			p.skipNewlines()
		}
	}
	if p.synced() {
		return nil
	}
	body := p.parseBlock()
	if p.synced() {
		return nil
	}
	return withPos([]any{"FnLitExpr", params, ret, errTy, body}, line, col)
}
