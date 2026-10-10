package parser

import (
	"fmt"
	"strings"

	"shiroko/core/color"
	"shiroko/core/lexer"
	"shiroko/core/source"
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

	fmt.Fprintf(&b, "%s %s %s",
		    color.Yellow(fmt.Sprintf("line %d:%d", e.Line, e.Col)),
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

// ---------- error plumbing ----------

func (p *Parser) errAt(tok lexer.Token, msg string) {
	e := &ParseError{Msg: msg, Line: tok.Line, Col: tok.Col, Src: p.src}
	p.Errors = append(p.Errors, e)
	p.sync = true
}

func (p *Parser) errAtCode(tok lexer.Token, code, msg string) {
	e := &ParseError{Code: code, Msg: msg, Line: tok.Line, Col: tok.Col, Src: p.src}
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
	tok := p.Peek()
	if tok.Kind != "IDENT" {
		p.errAtCode(tok, "E_EXPECTED_IDENT",
			    fmt.Sprintf("expected identifier, got %s", tok.Kind))
		return ""
	}
	p.Advance()
	s, _ := tok.Value.(string)
	return s
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
	for p.At("NEWLINE") {
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
	p.errAtCode(p.Peek(), "E_DECL",
		    fmt.Sprintf("expected declaration, got %s", p.Peek().Kind))
	return nil
}

// ---------- types ----------

func (p *Parser) parseType() any {
	if p.synced() {
		return nil
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
	if p.At("{") && p.PeekN(1).Kind == "}" {
		p.Advance()
		p.Advance()
		inner := p.parseType()
		if p.synced() {
			return nil
		}
		return []any{"SetType", inner}
	}
	name := p.EatIdent()
	if p.synced() {
		return nil
	}
	return []any{"NamedType", name}
}

// parseTupleType parses "(T1, T2, ...)". A single-element form "(T)"
// normalises to T, so `func f() (int)` is identical to `func f() int`.
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
	p.Eat("INTERFACE")
	name := p.EatIdent()
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
	return []any{"InterfaceDecl", name, methods}
}

func (p *Parser) parseStruct() any {
	if p.synced() {
		return nil
	}
	p.Eat("STRUCT")
	name := p.EatIdent()
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
	return []any{"StructDecl", name, fields}
}

func (p *Parser) parseParam() []any {
	names := []string{p.EatIdent()}
	if p.synced() {
		return nil
	}
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
		return []any{[]any{"Param", names[0], ty, variadic}}
	}
	for p.At(",") {
		p.Advance()
		names = append(names, p.EatIdent())
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
	for _, n := range names {
		params = append(params, []any{"Param", n, ty, variadic})
	}
	return params
}

func (p *Parser) parseFn() any {
	if p.synced() {
		return nil
	}
	p.Eat("FUNC")

	// Receiver is now: `Type.name`, i.e. `func User.f() ...`
	recv := ""
	first := p.EatIdent()
	name := first
	if p.At(".") {
		p.Advance()
		recv = first
		name = p.EatIdent()
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

	// Return type: `-> T`, `-> (T1, T2)`, or `-> T ! E`.
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
		return []any{"MethodDecl", recv, name, params, ret, errTy, body}
	}
	return []any{"FnDecl", name, params, ret, errTy, body}
}

// ---------- statements ----------

func (p *Parser) parseBlock() any {
	// A block is a statement context: newlines are significant again,
	// regardless of how many enclosing expressions we are nested in
	// (e.g. a `{ ... }` match-arm body while `p.br > 0`).
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

	names := []string{p.EatIdent()}
	for !p.synced() && p.At(",") {
		p.Advance()
		names = append(names, p.EatIdent())
	}

	var ty, expr, els any

	parseElse := func() {
		if p.synced() || !p.At("ELSE") {
			return
		}
		p.Advance()
		p.skipNewlines()
		els = p.parseBlock()
	}

	if p.At("=") {
		p.Advance()
		expr = p.parseExpr()
		parseElse()
	} else if len(names) == 1 && !p.isTerm() {
		ty = p.parseType()
		if p.At("=") {
			p.Advance()
			expr = p.parseExpr()
			parseElse()
		}
	}
	if p.synced() {
		return nil
	}
	if len(names) > 1 && expr == nil {
		p.errAtCode(p.Peek(), "E_LET_MULTI",
			    "multiple-name let requires an initializer")
		return nil
	}
	if els != nil && len(names) > 1 {
		p.errAtCode(p.Peek(), "E_LET_ELSE_MULTI",
			    "let-else does not support multiple names")
		return nil
	}
	return withPos([]any{"LetStmt", names, ty, expr, els}, line, col)
}

func (p *Parser) parseConst() any {
	line, col := p.pos()
	p.Eat("CONST")
	name := p.EatIdent()
	p.Eat("=")
	expr := p.parseExpr()
	if p.synced() {
		return nil
	}
	return withPos([]any{"ConstStmt", name, expr}, line, col)
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

	p.errAtCode(p.Peek(), "E_FOR_HEADER", "malformed for-loop header")
	return nil
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
		op := p.Advance().Kind
		p.skipNewlines()
		right := p.parseBin(lvl + 1)
		if p.synced() {
			return nil
		}
		left = []any{"BinaryExpr", op, left, right}
	}
	return left
}

func (p *Parser) parseUnary() any {
	if p.synced() {
		return nil
	}
	if p.At("-") || p.At("!") {
		op := p.Advance().Kind
		inner := p.parseUnary()
		if p.synced() {
			return nil
		}
		return []any{"UnaryExpr", op, inner}
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
				e = []any{"CallExpr", e, args}
			case p.At("."):
				p.Advance()
				name := p.EatIdent()
				if p.synced() {
					return nil
				}
				e = []any{"SelectorExpr", e, name}
			case p.At("["):
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
					e = []any{"SliceExpr", e, lo, hi}
				} else {
					p.Eat("]")
					p.br--
					if p.synced() {
						return nil
					}
					e = []any{"IndexExpr", e, lo}
				}
			case p.At("++") || p.At("--"):
				op := p.Advance().Kind
				e = []any{"PostfixExpr", op, e}
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
		p.Advance()
		return []any{"SpreadExpr", e}
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

	if tok.Kind == "IDENT" {
		if s, _ := tok.Value.(string); s == "_" {
			p.Advance()
			return withPos([]any{"WildcardPattern"}, line, col)
		}
	}

	var lo any
	switch tok.Kind {
		case "INT", "BYTE":
			p.Advance()
			lo = tok.Value
		case "FLOAT":
			p.Advance()
			lo = tok.Value
		case "STRING":
			p.Advance()
			lo = tok.Value
		case "IDENT":
			name, _ := tok.Value.(string)
			if name == "true" || name == "false" {
				p.Advance()
				lo = (name == "true")
			} else {
				p.errAtCode(tok, "E_PATTERN",
					    fmt.Sprintf("expected pattern, got %s", name))
				return nil
			}
		default:
			p.errAtCode(tok, "E_PATTERN",
				    fmt.Sprintf("expected pattern, got %s", tok.Kind))
			return nil
	}

	if p.At("..") {
		p.Advance()
		tok = p.Peek()
		var hi any
		switch tok.Kind {
			case "INT", "BYTE":
				p.Advance()
				hi = tok.Value
			case "FLOAT":
				p.Advance()
				hi = tok.Value
			default:
				p.errAtCode(tok, "E_RANGE_END",
					    fmt.Sprintf("expected range end, got %s", tok.Kind))
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
			return []any{"IntExpr", tok.Value}
		case "FLOAT":
			p.Advance()
			return []any{"FloatExpr", tok.Value}
		case "BYTE":
			p.Advance()
			return []any{"ByteExpr", tok.Value}
		case "STRING":
			p.Advance()
			return []any{"StrExpr", tok.Value}
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
		case "{":
			return p.parseSetOrComp()
		case "IDENT":
			name, _ := tok.Value.(string)
			p.Advance()
			if name == "true" {
				return []any{"BoolExpr", true}
			}
			if name == "false" {
				return []any{"BoolExpr", false}
			}
			if name == "nil" {
				return []any{"NilExpr"}
			}
			if p.At("{") && p.noStructLit == 0 {
				return p.parseStructLit(name)
			}
			return []any{"IdentExpr", name}
		case "FUNC":
			return p.parseFnLit()
		case "MATCH":
			return p.parseMatch()
	}
	p.errAtCode(tok, "E_PRIMARY",
		    fmt.Sprintf("unexpected token %s", tok.Kind))
	return nil
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

func (p *Parser) parseSetOrComp() any {
	p.Eat("{")
	p.Eat("}")
	elemTy := p.parseType()
	if p.synced() {
		return nil
	}
	p.Eat("{")
	p.br++

	if p.At("}") {
		p.Eat("}")
		p.br--
		return []any{"SetLitExpr", []any{"SetType", elemTy}, []any{}}
	}

	first := p.parseExpr()
	if p.synced() {
		return nil
	}
	if p.At("@") {
		return p.parseCompAfterFirst("SetCompExpr", []any{"SetType", elemTy}, first)
	}

	if p.At("..") {
		p.Advance()
		hi := p.parseExpr()
		p.Eat("}")
		p.br--
		if p.synced() {
			return nil
		}
		return []any{"RangeLitExpr", []any{"SetType", elemTy}, first, hi}
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
	return []any{"SetLitExpr", []any{"SetType", elemTy}, elems}
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
	p.Advance() // @
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

func (p *Parser) parseStructLit(name string) any {
	p.Eat("{")
	p.br++
	var fields []any
	if !p.At("}") {
		for {
			fname := p.EatIdent()
			p.Eat(":")
			fval := p.parseExpr()
			if p.synced() {
				return nil
			}
			fields = append(fields, []any{"FieldInit", fname, fval})
			if p.At(",") {
				p.Advance()
				continue
			}
			break
		}
	}
	p.Eat("}")
	p.br--
	if p.synced() {
		return nil
	}
	return []any{"StructLitExpr", name, fields}
}

func (p *Parser) parseFnLit() any {
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
		// Bổ sung phần bắt lỗi `! E` cho function literal nếu có
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
	// Đảm bảo trả về đủ 5 phần tử khớp với sema.go
	return []any{"FnLitExpr", params, ret, errTy, body}
}
