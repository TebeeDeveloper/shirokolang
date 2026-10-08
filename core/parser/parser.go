package parser

import (
	"fmt"

	"shiroko/core/lexer"
)

type ParseError struct {
	Msg  string
	Line int
	Col  int
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("line %d, col %d: %s", e.Line, e.Col, e.Msg)
}

// Node is a generic AST node: node[0] is the kind string.
// Nodes built by parsePattern/parseMatch carry (line, col) as the
// final two elements; use nodePos in sema to read them back.
type Node = []any

type Parser struct {
	toks        []lexer.Token
	i           int
	br          int
	Errors      []*ParseError
	noStructLit int
}

func New(toks []lexer.Token) *Parser {
	return &Parser{toks: toks}
}

// pos returns the line/column of the current token.
func (p *Parser) pos() (int, int) {
	tok := p.Peek()
	return tok.Line, tok.Col
}

// withPos appends (line, col) to a freshly-built node.
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
		panic(&ParseError{fmt.Sprintf("expected %s, got %s", kind, tok.Kind), tok.Line, tok.Col})
	}
	return p.Advance()
}

func (p *Parser) EatIdent() string  { return p.Eat("IDENT").Value.(string) }
func (p *Parser) EatString() string { return p.Eat("STRING").Value.(string) }

func (p *Parser) catch(fn func()) (err *ParseError) {
	defer func() {
		if r := recover(); r != nil {
			if pe, ok := r.(*ParseError); ok {
				err = pe
			} else {
				panic(r)
			}
		}
	}()
	fn()
	return nil
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
	tok := p.Peek()
	panic(&ParseError{"expected newline or ';'", tok.Line, tok.Col})
}

func (p *Parser) synchronize() {
	for !p.At("EOF") {
		if p.At("NEWLINE") || p.At(";") {
			p.Advance()
			return
		}
		if p.At("}") {
			p.Advance()
			return
		}
		p.Advance()
	}
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

	if err := p.catch(func() {
		p.Eat("PACKAGE")
		pkg = p.EatIdent()
		p.expectTerminator()
		p.skipNewlines()
	}); err != nil {
		p.Errors = append(p.Errors, err)
		return []any{"Program", pkg, []string{}, []any{}}
	}

	if p.At("IMPORT") {
		if err := p.catch(func() {
			p.Advance()
			p.Eat("{")
			p.br++
			p.skipNewlines()
			for !p.At("}") {
				imports = append(imports, p.EatString())
				p.skipNewlines()
			}
			p.Eat("}")
			p.br--
			p.expectTerminator()
			p.skipNewlines()
		}); err != nil {
			p.Errors = append(p.Errors, err)
			p.synchronize()
			p.skipNewlines()
		}
	}

	for !p.At("EOF") {
		if err := p.catch(func() {
			decls = append(decls, p.ParseTopDecl())
		}); err != nil {
			p.Errors = append(p.Errors, err)
			p.synchronize()
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
	if p.At("FN") {
		return p.parseFn()
	}
	tok := p.Peek()
	panic(&ParseError{fmt.Sprintf("expected declaration, got %s", tok.Kind), tok.Line, tok.Col})
}

// ---------- types ----------

func (p *Parser) parseType() any {
	if p.At("[") {
		p.Advance()
		p.Eat("]")
		return []any{"ListType", p.parseType()}
	}
	if p.At("{") && p.PeekN(1).Kind == "}" {
		p.Advance()
		p.Advance()
		return []any{"SetType", p.parseType()}
	}
	return []any{"NamedType", p.EatIdent()}
}

// ---------- declarations ----------

func (p *Parser) parseInterface() any {
	p.Eat("INTERFACE")
	name := p.EatIdent()
	p.Eat("{")
	var methods []any
	p.skipNewlines()
	for !p.At("}") {
		mname := p.EatIdent()
		p.Eat("(")
		p.br++
		var params []any
		if !p.At(")") {
			params = append(params, p.parseParam()...)
			for p.At(",") {
				p.Advance()
				params = append(params, p.parseParam()...)
			}
		}
		p.Eat(")")
		p.br--
		var ret any
		if !p.isTerm() {
			ret = p.parseType()
		}
		methods = append(methods, []any{"MethodSig", mname, params, ret})
		p.expectTerminator()
		p.skipNewlines()
	}
	p.Eat("}")
	return []any{"InterfaceDecl", name, methods}
}

func (p *Parser) parseStruct() any {
	p.Eat("STRUCT")
	name := p.EatIdent()
	p.Eat("{")
	var fields []any
	p.skipNewlines()
	for !p.At("}") {
		fname := p.EatIdent()
		fty := p.parseType()
		fields = append(fields, []any{"Field", fname, fty})
		p.expectTerminator()
		p.skipNewlines()
	}
	p.Eat("}")
	return []any{"StructDecl", name, fields}
}

// parseParam parses one parameter group. Normally this is a single
// "name type" (or "name ...type") parameter, but the grouped form
// "a, b int" is also accepted and expands to one Param per name, all
// sharing the same type and variadic flag.
func (p *Parser) parseParam() []any {
	names := []string{p.EatIdent()}

	// Not a grouped form: this name owns its type.
	if !p.At(",") {
		variadic := false
		if p.At("...") {
			p.Advance()
			variadic = true
		}
		ty := p.parseType()
		return []any{[]any{"Param", names[0], ty, variadic}}
	}

	// Grouped form: collect names up to the shared type.
	for p.At(",") {
		p.Advance()
		names = append(names, p.EatIdent())
	}
	variadic := false
	if p.At("...") {
		p.Advance()
		variadic = true
	}
	ty := p.parseType()

	params := make([]any, 0, len(names))
	for _, n := range names {
		params = append(params, []any{"Param", n, ty, variadic})
	}
	return params
}

func (p *Parser) parseFn() any {
	p.Eat("FN")
	recv := ""
	if p.At("(") {
		p.Advance()
		p.br++
		recv = p.EatIdent()
		p.Eat(")")
		p.br--
	}
	name := p.EatIdent()
	p.Eat("(")
	p.br++
	var params []any
	if !p.At(")") {
		params = append(params, p.parseParam()...)
		for p.At(",") {
			p.Advance()
			params = append(params, p.parseParam()...)
		}
	}
	p.Eat(")")
	p.br--
	p.skipNewlines()

	var ret any
	if !p.At("{") {
		ret = p.parseType()
		p.skipNewlines()
	}
	body := p.parseBlock()
	if recv != "" {
		return []any{"MethodDecl", recv, name, params, ret, body}
	}
	return []any{"FnDecl", name, params, ret, body}
}

// ---------- statements ----------

func (p *Parser) parseBlock() any {
	p.Eat("{")
	var stmts []any
	p.skipNewlines()
	for !p.At("}") {
		if err := p.catch(func() {
			stmts = append(stmts, p.parseStmt())
			p.expectTerminator()
		}); err != nil {
			p.Errors = append(p.Errors, err)
			p.synchronize()
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
		name := p.EatIdent()
		op := p.Advance().Kind
		return []any{"IncDecStmt", name, op}
	}
	e := p.parseExpr()
	if p.At("=") {
		p.Advance()
		return []any{"AssignStmt", e, p.parseExpr()}
	}
	return []any{"ExprStmt", e}
}

func (p *Parser) parseLet() any {
	p.Eat("LET")
	name := p.EatIdent()
	var ty, expr any
	if p.At("=") {
		p.Advance()
		expr = p.parseExpr()
	} else if !p.isTerm() {
		ty = p.parseType()
		if p.At("=") {
			p.Advance()
			expr = p.parseExpr()
		}
	}
	return []any{"LetStmt", name, ty, expr}
}

func (p *Parser) parseConst() any {
	p.Eat("CONST")
	name := p.EatIdent()
	p.Eat("=")
	expr := p.parseExpr()
	return []any{"ConstStmt", name, expr}
}

func (p *Parser) parseReturn() any {
	p.Eat("RETURN")
	var expr any
	if !p.isTerm() {
		expr = p.parseExpr()
	}
	return []any{"ReturnStmt", expr}
}

func (p *Parser) parseIf() any {
	p.Eat("IF")
	p.noStructLit++
	cond := p.parseExpr()
	p.noStructLit--
	p.skipNewlines()
	thenBlock := p.parseBlock()

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
	}
	return []any{"IfStmt", cond, thenBlock, els}
}

// ---------- for loops ----------

func (p *Parser) parseFor() any {
	p.Eat("FOR")

	if p.At("RANGE") {
		p.Advance()
		p.noStructLit++
		count := p.parseExpr()
		p.noStructLit--
		p.skipNewlines()
		body := p.parseBlock()
		return []any{"ForRangeStmt", count, body}
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
				p.skipNewlines()
				body := p.parseBlock()
				return []any{"ForIterStmt", name, nil, src, body}
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
			p.skipNewlines()
			body := p.parseBlock()
			return []any{"ForCStmt", name, init, cond, post, body}
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
			p.skipNewlines()
			body := p.parseBlock()
			return []any{"ForIterStmt", val, idx, src, body}
		}

		p.noStructLit++
		cond := p.parseExpr()
		p.noStructLit--
		p.skipNewlines()
		body := p.parseBlock()
		return []any{"ForCondStmt", cond, body}
	}

	tok := p.Peek()
	panic(&ParseError{"malformed for-loop header", tok.Line, tok.Col})
}

func (p *Parser) parseForPost() any {
	if p.At("IDENT") && (p.PeekN(1).Kind == "++" || p.PeekN(1).Kind == "--") {
		name := p.EatIdent()
		op := p.Advance().Kind
		return []any{"IncDecStmt", name, op}
	}
	e := p.parseExpr()
	if p.At("=") {
		p.Advance()
		return []any{"AssignStmt", e, p.parseExpr()}
	}
	return []any{"ExprStmt", e}
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
	if lvl >= len(binOps) {
		return p.parseUnary()
	}
	left := p.parseBin(lvl + 1)
	for {
		save := p.i
		p.skipNewlines()
		if !binOps[lvl][p.Peek().Kind] {
			p.i = save
			break
		}
		op := p.Advance().Kind
		p.skipNewlines()
		left = []any{"BinaryExpr", op, left, p.parseBin(lvl+1)}
	}
	return left
}

func (p *Parser) parseUnary() any {
	if p.At("-") || p.At("!") {
		op := p.Advance().Kind
		return []any{"UnaryExpr", op, p.parseUnary()}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() any {
	e := p.parsePrimary()
	for {
		switch {
			case p.At("("):
				p.Eat("(")
				p.br++
				var args []any
				if !p.At(")") {
					args = append(args, p.parseCallArg())
					for p.At(",") {
						p.Advance()
						args = append(args, p.parseCallArg())
					}
				}
				p.Eat(")")
				p.br--
				e = []any{"CallExpr", e, args}
			case p.At("."):
				p.Advance()
				e = []any{"SelectorExpr", e, p.EatIdent()}
			case p.At("["):
				p.Advance()
				p.br++
				var lo any
				if !p.At(":") {
					lo = p.parseExpr()
				}
				if p.At(":") {
					p.Advance()
					var hi any
					if !p.At("]") {
						hi = p.parseExpr()
					}
					p.Eat("]")
					p.br--
					e = []any{"SliceExpr", e, lo, hi}
				} else {
					p.Eat("]")
					p.br--
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
	e := p.parseExpr()
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
	p.skipNewlines()
	p.Eat("{")
	p.br++
	p.skipNewlines()
	var arms []any
	for !p.At("}") {
		pat := p.parsePattern()
		p.Eat("=>")
		p.skipNewlines()
		var body any
		if p.At("{") {
			body = p.parseBlock()
		} else {
			body = p.parseExpr()
		}
		arms = append(arms, []any{"MatchArm", pat, body})
		if p.At(",") {
			p.Advance()
		}
		p.skipNewlines()
	}
	p.Eat("}")
	p.br--
	return withPos([]any{"MatchExpr", scrutinee, arms}, line, col)
}

func (p *Parser) parsePattern() any {
	tok := p.Peek()
	line, col := tok.Line, tok.Col

	if tok.Kind == "IDENT" && tok.Value.(string) == "_" {
		p.Advance()
		return withPos([]any{"WildcardPattern"}, line, col)
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
			name := tok.Value.(string)
			if name == "true" || name == "false" {
				p.Advance()
				lo = (name == "true")
			} else {
				panic(&ParseError{fmt.Sprintf("expected pattern, got %s", name), tok.Line, tok.Col})
			}
		default:
			panic(&ParseError{fmt.Sprintf("expected pattern, got %s", tok.Kind), tok.Line, tok.Col})
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
				panic(&ParseError{fmt.Sprintf("expected range end, got %s", tok.Kind), tok.Line, tok.Col})
		}
		return withPos([]any{"RangePattern", lo, hi}, line, col)
	}
	return withPos([]any{"LitPattern", lo}, line, col)
}

func (p *Parser) parsePrimary() any {
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
			return e
		case "[":
			return p.parseListOrComp()
		case "{":
			return p.parseSetOrComp()
		case "IDENT":
			name := tok.Value.(string)
			p.Advance()
			if name == "true" {
				return []any{"BoolExpr", true}
			}
			if name == "false" {
				return []any{"BoolExpr", false}
			}
			if p.At("{") && p.noStructLit == 0 {
				return p.parseStructLit(name)
			}
			return []any{"IdentExpr", name}
		case "FN":
			return p.parseFnLit()
		case "MATCH":
			return p.parseMatch()
	}
	panic(&ParseError{fmt.Sprintf("unexpected token %s", tok.Kind), tok.Line, tok.Col})
}

// ---------- literals & comprehensions ----------

func (p *Parser) parseListOrComp() any {
	p.Eat("[")
	p.Eat("]")
	elemTy := p.parseType()
	p.Eat("{")
	p.br++

	if p.At("}") {
		p.Eat("}")
		p.br--
		return []any{"ListLitExpr", []any{"ListType", elemTy}, []any{}}
	}

	first := p.parseExpr()
	if p.At("@") {
		return p.parseCompAfterFirst(
			"ListCompExpr", []any{"ListType", elemTy}, first)
	}

	if p.At("..") {
		p.Advance()
		hi := p.parseExpr()
		p.Eat("}")
		p.br--
		return []any{"RangeLitExpr", []any{"ListType", elemTy}, first, hi}
	}

	elems := []any{first}
	for p.At(",") {
		p.Advance()
		elems = append(elems, p.parseExpr())
	}
	p.Eat("}")
	p.br--
	return []any{"ListLitExpr", []any{"ListType", elemTy}, elems}
}

func (p *Parser) parseSetOrComp() any {
	p.Eat("{")
	p.Eat("}")
	elemTy := p.parseType()
	p.Eat("{")
	p.br++

	if p.At("}") {
		p.Eat("}")
		p.br--
		return []any{"SetLitExpr", []any{"SetType", elemTy}, []any{}}
	}

	first := p.parseExpr()
	if p.At("@") {
		return p.parseCompAfterFirst(
			"SetCompExpr", []any{"SetType", elemTy}, first)
	}

	if p.At("..") {
		p.Advance()
		hi := p.parseExpr()
		p.Eat("}")
		p.br--
		return []any{"RangeLitExpr", []any{"SetType", elemTy}, first, hi}
	}

	elems := []any{first}
	for p.At(",") {
		p.Advance()
		elems = append(elems, p.parseExpr())
	}
	p.Eat("}")
	p.br--
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
		tok := p.Peek()
		panic(&ParseError{"comprehension element must reference a loop variable",
			tok.Line, tok.Col})
	}

	var src any
	if p.At("IDENT") && p.PeekN(1).Kind == "+" {
		n2 := p.PeekN(2).Kind
		if n2 == "|" || n2 == ";" || n2 == "}" {
			v := p.Advance().Value.(string)
			p.Advance()
			src = []any{"SeqExpr", v, "+"}
		} else {
			src = p.parseExpr()
		}
	} else {
		src = p.parseExpr()
	}

	var filt, cond any
	if p.At("|") {
		p.Advance()
		filt = p.parseExpr()
	}
	if p.At(";") {
		p.Advance()
		cond = p.parseExpr()
	}
	p.Eat("}")
	p.br--
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
	return []any{"StructLitExpr", name, fields}
}

func (p *Parser) parseFnLit() any {
	p.Eat("FN")
	p.Eat("(")
	p.br++
	var params []any
	if !p.At(")") {
		params = append(params, p.parseParam()...)
		for p.At(",") {
			p.Advance()
			params = append(params, p.parseParam()...)
		}
	}
	p.Eat(")")
	p.br--
	p.skipNewlines()

	var ret any
	if !p.At("{") {
		ret = p.parseType()
		p.skipNewlines()
	}
	body := p.parseBlock()
	return []any{"FnLitExpr", params, ret, body}
}
