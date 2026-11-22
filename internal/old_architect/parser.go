package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
	"reflect"
)

type Parser struct {
	tokens         []Token
	pos            int
	errors         []string
	allowStructLit bool
	sourceLines    []string
}

func NewParser(tokens []Token) *Parser {
	p := &Parser{
		tokens:         tokens,
		pos:            0,
		allowStructLit: true,
	}
	if len(p.tokens) == 0 || p.tokens[len(p.tokens)-1].Type != EOF {
		line, col := 1, 1
		if len(p.tokens) > 0 {
			last := p.tokens[len(p.tokens)-1]
			line, col = last.Line, last.Column
		}
		p.tokens = append(p.tokens, Token{Type: EOF, Line: line, Column: col})
	}
	return p
}

func posFromToken(tok Token) BasePos {
	return BasePos{TokLine: tok.Line, TokCol: tok.Column}
}

func (p *Parser) SetSource(src string) {
	if src == "" {
		p.sourceLines = nil
		return
	}
	p.sourceLines = strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) cur() Token {
	if p.pos >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[p.pos]
}

func (p *Parser) peek() Token {
	if p.pos+1 >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[p.pos+1]
}

func (p *Parser) advance() {
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
}

func (p *Parser) consumeOptionalSemicolon() {
	if p.cur().Type == SEMICOLON {
		p.advance()
	}
}

func contextHint(tok Token) string {
	switch tok.Type {
	case ILLEGAL:
		return fmt.Sprintf("character %q is not supported by the lexer", tok.Literal)
	case RBRACE:
		return "unexpected '}' or missing ';' on the previous statement"
	case SEMICOLON:
		return "missing ';'"
	case EOF:
		return "unexpected end of file (check unclosed '{', '(', '[' or missing ';')"
	default:
		return ""
	}
}

func tokenSpanOnLine(lineText string, tok Token) (int, int) {
	lineRunes := []rune(lineText)
	lineLen := len(lineRunes)
	if lineLen == 0 {
		return 1, 1
	}

	rawLen := utf8.RuneCountInString(tok.Literal)
	if rawLen < 1 {
		rawLen = 1
	}

	col := tok.Column
	if col < 1 {
		col = 1
	}
	if col > lineLen+1 {
		col = lineLen + 1
	}

	startCol := col
	if rawLen > 1 && col-rawLen >= 0 && col <= lineLen && string(lineRunes[col-rawLen:col]) == tok.Literal {
		startCol = col - rawLen + 1
	}

	if startCol < 1 {
		startCol = 1
	}
	if startCol > lineLen {
		startCol = lineLen
	}

	spanLen := rawLen
	if startCol-1+spanLen > lineLen {
		spanLen = lineLen - startCol + 1
	}
	if spanLen < 1 {
		spanLen = 1
	}
	return startCol, spanLen
}

func formatPointerLine(lineText string, startCol int, spanLen int) string {
	lineRunes := []rune(lineText)
	var prefix strings.Builder
	w := 0
	for i := 0; i < startCol-1 && i < len(lineRunes); i++ {
		r := lineRunes[i]
		if r == '\t' {
			step := 4 - (w % 4)
			prefix.WriteString(strings.Repeat(" ", step))
			w += step
		} else {
			prefix.WriteByte(' ')
			w++
		}
	}

	var mark strings.Builder
	for i := 0; i < spanLen; i++ {
		idx := startCol - 1 + i
		r := rune(' ')
		if idx < len(lineRunes) {
			r = lineRunes[idx]
		}
		step := 1
		if r == '\t' {
			step = 4 - (w % 4)
		}
		if i == 0 {
			mark.WriteByte('^')
			if step > 1 {
				mark.WriteString(strings.Repeat("~", step-1))
			}
		} else {
			mark.WriteString(strings.Repeat("~", step))
		}
		w += step
	}

	return prefix.String() + mark.String()
}

func (p *Parser) formatError(tok Token, msg string) string {
	var b strings.Builder

	if tok.Line > 0 && tok.Line <= len(p.sourceLines) {
		lineText := p.sourceLines[tok.Line-1]
		startCol, spanLen := tokenSpanOnLine(lineText, tok)

		fmt.Fprintf(&b, "error[line %d:%d]: %s", tok.Line, startCol, msg)
		if hint := contextHint(tok); hint != "" {
			fmt.Fprintf(&b, " (%s)", hint)
		}

		numWidth := len(fmt.Sprintf("%d", tok.Line))
		if numWidth < 3 {
			numWidth = 3
		}
		gutterPad := strings.Repeat(" ", numWidth)

		if tok.Line-2 >= 0 && tok.Line-2 < len(p.sourceLines) {
			prevLine := strings.ReplaceAll(p.sourceLines[tok.Line-2], "\t", "    ")
			fmt.Fprintf(&b, "\n  %*d | %s", numWidth, tok.Line-1, prevLine)
		}

		displayLine := strings.ReplaceAll(lineText, "\t", "    ")
		pointer := formatPointerLine(lineText, startCol, spanLen)

		fmt.Fprintf(&b, "\n  %*d | %s", numWidth, tok.Line, displayLine)
		fmt.Fprintf(&b, "\n  %s | %s", gutterPad, pointer)
		return b.String()
	}

	fmt.Fprintf(&b, "error[line %d:%d]: %s", tok.Line, tok.Column, msg)
	if hint := contextHint(tok); hint != "" {
		fmt.Fprintf(&b, " (%s)", hint)
	}
	return b.String()
}

func (p *Parser) addError(tok Token, msg string) {
	p.errors = append(p.errors, p.formatError(tok, msg))
}

func (p *Parser) expect(tokType TokenType) (Token, bool) {
	if p.cur().Type == tokType {
		tok := p.cur()
		p.advance()
		return tok, true
	}
	p.addError(p.cur(), fmt.Sprintf("expected %s, got %s (value=%q)", tokType, p.cur().Type, p.cur().Literal))
	return Token{}, false
}

func (p *Parser) synchronize() {
	for p.cur().Type != EOF {
		switch p.cur().Type {
		case SEMICOLON:
			p.advance()
			return
		case RBRACE:
			return
		case STRUCT, FN, LET, CONST, RETURN, IF, FOR, USE, CONTINUE, BREAK:
			return
		}
		p.advance()
	}
}

func (p *Parser) ParseProgram() *Program {
	firstTok := p.cur()
	prog := &Program{BasePos: posFromToken(firstTok)}
	for p.cur().Type != EOF {
		startPos := p.pos
		stmt := p.parseTopOrStmt()
		if stmt != nil {
			prog.Items = append(prog.Items, stmt)
		} else {
			p.synchronize()
			if p.pos == startPos {
				p.advance()
			}
		}
	}
	return prog
}

func (p *Parser) parseTopOrStmt() Stmt {
	switch p.cur().Type {
	case USE:
		return p.parseUseStmt()
	case STRUCT:
		return p.parseStructDecl()
	case FN:
		return p.parseFuncDecl()
	default:
		return p.parseStatement()
	}
}

func (p *Parser) parseUseStmt() *UseStmt {
	startTok := p.cur()
	p.advance()
	modTok, ok := p.expect(IDENT)
	if !ok {
		return nil
	}
	p.consumeOptionalSemicolon()
	return &UseStmt{BasePos: posFromToken(startTok), Module: modTok.Literal}
}

func (p *Parser) parseStructDecl() *StructDecl {
	startTok := p.cur()
	p.advance()
	nameTok, ok := p.expect(IDENT)
	if !ok {
		return nil
	}
	if _, ok := p.expect(LBRACE); !ok {
		return nil
	}

	var fields []*FieldDecl
	for p.cur().Type != RBRACE && p.cur().Type != EOF {
		fieldNameTok, ok := p.expect(IDENT)
		if !ok {
			return nil
		}
		fieldType, ok := p.parseType()
		if !ok {
			return nil
		}
		p.consumeOptionalSemicolon()
		fields = append(fields, &FieldDecl{
			BasePos: posFromToken(fieldNameTok),
			Name:    fieldNameTok.Literal,
			Type:    fieldType,
		})
	}

	if _, ok := p.expect(RBRACE); !ok {
		return nil
	}

	return &StructDecl{
		BasePos: posFromToken(startTok),
		Name:    nameTok.Literal,
		Fields:  fields,
	}
}

func (p *Parser) parseReturnType() (string, bool) {
	if p.cur().Type == LPAREN {
		p.advance()
		var types []string
		if p.cur().Type != RPAREN {
			for {
				t, ok := p.parseType()
				if !ok {
					return "", false
				}
				types = append(types, t)
				if p.cur().Type == COMMA {
					p.advance()
					if p.cur().Type == RPAREN {
						break
					}
					continue
				}
				break
			}
		}
		if _, ok := p.expect(RPAREN); !ok {
			return "", false
		}
		if len(types) == 0 {
			return "unit", true
		}
		return "(" + joinStrings(types, ", ") + ")", true
	}

	first, ok := p.parseType()
	if !ok {
		return "", false
	}
	if p.cur().Type != COMMA {
		return first, true
	}

	types := []string{first}
	for p.cur().Type == COMMA {
		p.advance()
		nextType, ok := p.parseType()
		if !ok {
			return "", false
		}
		types = append(types, nextType)
	}
	return "(" + joinStrings(types, ", ") + ")", true
}

func (p *Parser) parseFuncDecl() *FuncDecl {
	startTok := p.cur()
	p.advance()
	nameTok, ok := p.expect(IDENT)
	if !ok {
		return nil
	}
	params, ok := p.parseParamGroup()
	if !ok {
		return nil
	}

	var retType string
	if p.cur().Type == ARROW {
		p.advance()
		retType, ok = p.parseReturnType()
		if !ok {
			return nil
		}
	}

	body := p.parseBlock()
	if body == nil {
		return nil
	}

	return &FuncDecl{
		BasePos:    posFromToken(startTok),
		Name:       nameTok.Literal,
		Params:     params,
		ReturnType: retType,
		Body:       body,
	}
}

func (p *Parser) parseParamGroup() ([]*Param, bool) {
	if _, ok := p.expect(LPAREN); !ok {
		return nil, false
	}

	var params []*Param
	if p.cur().Type != RPAREN {
		for {
			nameTok, ok := p.expect(IDENT)
			if !ok {
				return nil, false
			}
			if p.cur().Type == COLON {
				p.advance()
			}

			isVariadic := false
			if p.cur().Type == ELLIPSIS {
				p.advance()
				isVariadic = true
			}

			typ, ok := p.parseType()
			if !ok {
				return nil, false
			}

			params = append(params, &Param{
				BasePos:  posFromToken(nameTok),
				Name:     nameTok.Literal,
				Type:     typ,
				Variadic: isVariadic,
			})

			if p.cur().Type == COMMA {
				if isVariadic {
					p.addError(p.cur(), "variadic parameter must be the last parameter")
					return nil, false
				}
				p.advance()
				continue
			}
			break
		}
	}

	if _, ok := p.expect(RPAREN); !ok {
		return nil, false
	}
	return params, true
}

func (p *Parser) parseType() (string, bool) {
	if p.cur().Type == LBRACKET {
		tok := p.cur()
		p.advance()
		if _, ok := p.expect(RBRACKET); !ok {
			p.addError(tok, "expected ']' in list type")
			return "", false
		}
		elemType, ok := p.parseType()
		if !ok {
			return "", false
		}
		return "[]" + elemType, true
	}

	if p.cur().Type == FN {
		p.advance()
		if _, ok := p.expect(LPAREN); !ok {
			return "", false
		}
		var paramTypes []string
		if p.cur().Type != RPAREN {
			for {
				prefix := ""
				if p.cur().Type == ELLIPSIS {
					p.advance()
					prefix = "..."
				}
				pt, ok := p.parseType()
				if !ok {
					return "", false
				}
				paramTypes = append(paramTypes, prefix+pt)
				if p.cur().Type == COMMA {
					if prefix != "" {
						p.addError(p.cur(), "variadic parameter must be the last parameter")
						return "", false
					}
					p.advance()
					continue
				}
				break
			}
		}
		if _, ok := p.expect(RPAREN); !ok {
			return "", false
		}
		retType := "unit"
		if p.cur().Type == ARROW {
			p.advance()
			rt, ok := p.parseReturnType()
			if !ok {
				return "", false
			}
			retType = rt
		}
		return "fn(" + joinStrings(paramTypes, ", ") + ") -> " + retType, true
	}

	tok, ok := p.expect(IDENT)
	if !ok {
		return "", false
	}
	return tok.Literal, true
}

func joinStrings(items []string, sep string) string {
	if len(items) == 0 {
		return ""
	}
	out := items[0]
	for i := 1; i < len(items); i++ {
		out += sep + items[i]
	}
	return out
}

func (p *Parser) parseStatement() Stmt {
	switch p.cur().Type {
	case USE:
		return p.parseUseStmt()
	case LET, CONST:
		return p.parseLetStmt()
	case RETURN:
		return p.parseReturnStmt()
	case IF:
		return p.parseIfStmt()
	case FOR:
		return p.parseForStmt()
	case CONTINUE:
		tok := p.cur()
		p.advance()
		p.consumeOptionalSemicolon()
		return &ContinueStmt{BasePos: posFromToken(tok)}
	case BREAK:
		tok := p.cur()
		p.advance()
		p.consumeOptionalSemicolon()
		return &BreakStmt{BasePos: posFromToken(tok)}
	case LBRACE:
		return p.parseBlock()
	default:
		return p.parseAssignOrExprStmt()
	}
}

func (p *Parser) parseLetStmt() *LetStmt {
	startTok := p.cur()
	isConst := p.cur().Type == CONST
	p.advance()
	firstTok, ok := p.expect(IDENT)
	if !ok {
		return nil
	}

	if p.cur().Type == DOT {
		kw := "let"
		if isConst {
			kw = "const"
		}
		p.addError(p.cur(), fmt.Sprintf("cannot declare field with '%s' (use assignment without '%s', e.g. 'obj.field = value')", kw, kw))
		return nil
	}

	names := []string{firstTok.Literal}
	var types []string

	var firstType string
	if p.cur().Type == COLON {
		p.advance()
		firstType, ok = p.parseType()
		if !ok {
			return nil
		}
	}

	if p.cur().Type == COMMA {
		types = make([]string, len(names))
		if firstType != "" {
			types[0] = firstType
		}
		for p.cur().Type == COMMA {
			p.advance()
			nextTok, ok := p.expect(IDENT)
			if !ok {
				return nil
			}
			names = append(names, nextTok.Literal)
			var nextTyp string
			if p.cur().Type == COLON {
				p.advance()
				nextTyp, ok = p.parseType()
				if !ok {
					return nil
				}
			}
			types = append(types, nextTyp)
		}
	}

	if _, ok := p.expect(ASSIGN); !ok {
		return nil
	}

	initExpr := p.parseExpr()
	if initExpr == nil {
		return nil
	}

	p.consumeOptionalSemicolon()

	return &LetStmt{
		BasePos: posFromToken(startTok),
		IsConst: isConst,
		Name:    names[0],
		Names:   names,
		Type:    firstType,
		Types:   types,
		Init:    initExpr,
	}
}

func (p *Parser) parseAssignOrExprStmt() Stmt {
	lhs := p.parseExpr()
	if isNilExpr(lhs) {
		return nil
	}

	if p.cur().Type == ASSIGN {
		p.advance()
		rhs := p.parseExpr()
		if isNilExpr(rhs) {
			return nil
		}
		p.consumeOptionalSemicolon()
		return &AssignStmt{
			BasePos: BasePos{TokLine: lhs.Line(), TokCol: lhs.Column()},
			Target:  lhs,
			Value:   rhs,
		}
	}

	p.consumeOptionalSemicolon()
	return &ExprStmt{
		BasePos: BasePos{TokLine: lhs.Line(), TokCol: lhs.Column()},
		Expr:    lhs,
	}
}

func (p *Parser) parseReturnStmt() *ReturnStmt {
	startTok := p.cur()
	p.advance()
	if p.cur().Type == SEMICOLON {
		p.advance()
		return &ReturnStmt{BasePos: posFromToken(startTok)}
	}

	first := p.parseExpr()
	if first == nil {
		return nil
	}

	values := []Expr{first}
	for p.cur().Type == COMMA {
		p.advance()
		nextVal := p.parseExpr()
		if nextVal == nil {
			return nil
		}
		values = append(values, nextVal)
	}

	p.consumeOptionalSemicolon()

	return &ReturnStmt{
		BasePos: posFromToken(startTok),
		Value:   first,
		Values:  values,
	}
}

func (p *Parser) parseCondWithoutStructLit() Expr {
	prev := p.allowStructLit
	p.allowStructLit = false
	cond := p.parseExpr()
	p.allowStructLit = prev
	return cond
}

func (p *Parser) parseIfStmt() *IfStmt {
	startTok := p.cur()
	p.advance()
	cond := p.parseCondWithoutStructLit()
	if cond == nil {
		return nil
	}
	body := p.parseBlock()
	if body == nil {
		return nil
	}

	stmt := &IfStmt{
		BasePos:  posFromToken(startTok),
		Branches: []IfBranch{{BasePos: posFromToken(startTok), Cond: cond, Body: body}},
	}

	for p.cur().Type == ELSE {
		elseTok := p.cur()
		p.advance()
		if p.cur().Type == IF {
			p.advance()
			elseIfCond := p.parseCondWithoutStructLit()
			if elseIfCond == nil {
				return nil
			}
			elseIfBody := p.parseBlock()
			if elseIfBody == nil {
				return nil
			}
			stmt.Branches = append(stmt.Branches, IfBranch{
				BasePos: posFromToken(elseTok),
				Cond:    elseIfCond,
				Body:    elseIfBody,
			})
		} else {
			elseBody := p.parseBlock()
			if elseBody == nil {
				return nil
			}
			stmt.ElseBody = elseBody
			break
		}
	}

	return stmt
}

func (p *Parser) parseForStmt() Stmt {
	startTok := p.cur()
	p.advance()

	if p.cur().Type == LET && p.peek().Type == COLON {
		p.advance()
		nameTok, ok := p.expect(IDENT)
		if !ok {
			return nil
		}
		if _, ok := p.expect(COLON); !ok {
			return nil
		}
		prev := p.allowStructLit
		p.allowStructLit = false
		iterable := p.parseExpr()
		p.allowStructLit = prev
		if iterable == nil {
			return nil
		}
		body := p.parseBlock()
		if body == nil {
			return nil
		}
		return &ForEachStmt{
			BasePos:  posFromToken(startTok),
			VarName:  nameTok.Literal,
			Iterable: iterable,
			Body:     body,
		}
	}

	if p.cur().Type == LET || p.hasForClauseSemicolon() {
		var initStmt Stmt
		if p.cur().Type != SEMICOLON {
			initStmt = p.parseForInit()
			if initStmt == nil {
				return nil
			}
		}
		if _, ok := p.expect(SEMICOLON); !ok {
			return nil
		}

		var cond Expr
		if p.cur().Type != SEMICOLON {
			cond = p.parseCondWithoutStructLit()
			if cond == nil {
				return nil
			}
		}
		if _, ok := p.expect(SEMICOLON); !ok {
			return nil
		}

		var postStmt Stmt
		if p.cur().Type != LBRACE {
			postStmt = p.parseForPost()
			if postStmt == nil {
				return nil
			}
		}

		body := p.parseBlock()
		if body == nil {
			return nil
		}

		return &ForStmt{
			BasePos: posFromToken(startTok),
			Init:    initStmt,
			Cond:    cond,
			Post:    postStmt,
			Body:    body,
		}
	}

	cond := p.parseCondWithoutStructLit()
	if cond == nil {
		return nil
	}
	body := p.parseBlock()
	if body == nil {
		return nil
	}
	return &ForStmt{BasePos: posFromToken(startTok), Cond: cond, Body: body}
}

func (p *Parser) hasForClauseSemicolon() bool {
	for i := p.pos; i < len(p.tokens); i++ {
		switch p.tokens[i].Type {
		case SEMICOLON:
			return true
		case LBRACE, RBRACE, EOF:
			return false
		}
	}
	return false
}

func (p *Parser) parseForInit() Stmt {
	if p.cur().Type == LET {
		startTok := p.cur()
		p.advance()
		nameTok, ok := p.expect(IDENT)
		if !ok {
			return nil
		}

		names := []string{nameTok.Literal}
		var types []string
		var firstType string

		if p.cur().Type == COLON {
			p.advance()
			firstType, ok = p.parseType()
			if !ok {
				return nil
			}
		}

		if _, ok := p.expect(ASSIGN); !ok {
			return nil
		}

		initExpr := p.parseExpr()
		if initExpr == nil {
			return nil
		}

		return &LetStmt{
			BasePos: posFromToken(startTok),
			Name:    names[0],
			Names:   names,
			Type:    firstType,
			Types:   types,
			Init:    initExpr,
		}
	}

	return p.parseForPost()
}

func (p *Parser) parseForPost() Stmt {
	lhs := p.parseExpr()
	if lhs == nil {
		return nil
	}

	if p.cur().Type == ASSIGN {
		p.advance()
		rhs := p.parseExpr()
		if rhs == nil {
			return nil
		}
		return &AssignStmt{
			BasePos: BasePos{TokLine: lhs.Line(), TokCol: lhs.Column()},
			Target:  lhs,
			Value:   rhs,
		}
	}

	if p.cur().Type == INCREMENT || p.cur().Type == DECREMENT {
		opTok := p.cur()
		p.advance()
		return &ExprStmt{
			BasePos: BasePos{TokLine: lhs.Line(), TokCol: lhs.Column()},
			Expr: &UnaryExpr{
				BasePos: BasePos{TokLine: lhs.Line(), TokCol: lhs.Column()},
				Op:      opTok.Literal,
				Right:   lhs,
			},
		}
	}

	return &ExprStmt{
		BasePos: BasePos{TokLine: lhs.Line(), TokCol: lhs.Column()},
		Expr:    lhs,
	}
}

func (p *Parser) parseBlock() *BlockStmt {
	startTok := p.cur()
	if _, ok := p.expect(LBRACE); !ok {
		return nil
	}

	var stmts []Stmt
	for p.cur().Type != RBRACE && p.cur().Type != EOF {
		startPos := p.pos
		stmt := p.parseStatement()
		if stmt != nil {
			stmts = append(stmts, stmt)
		} else {
			p.synchronize()
			if p.cur().Type == RBRACE {
				break
			}
			if p.pos == startPos {
				p.advance()
			}
		}
	}

	if _, ok := p.expect(RBRACE); !ok {
		return nil
	}

	return &BlockStmt{BasePos: posFromToken(startTok), Stmts: stmts}
}

func (p *Parser) parseExpr() Expr {
	return p.parseOrExpr()
}

func (p *Parser) parseOrExpr() Expr {
	left := p.parseAndExpr()
	if left == nil {
		return nil
	}
	for p.cur().Type == OR {
		op := p.cur().Literal
		p.advance()
		right := p.parseAndExpr()
		if right == nil {
			return nil
		}
		left = &BinaryExpr{
			BasePos: BasePos{TokLine: left.Line(), TokCol: left.Column()},
			Left:    left,
			Op:      op,
			Right:   right,
		}
	}
	return left
}

func (p *Parser) parseAndExpr() Expr {
	left := p.parseEquality()
	if left == nil {
		return nil
	}
	for p.cur().Type == AND {
		op := p.cur().Literal
		p.advance()
		right := p.parseEquality()
		if right == nil {
			return nil
		}
		left = &BinaryExpr{
			BasePos: BasePos{TokLine: left.Line(), TokCol: left.Column()},
			Left:    left,
			Op:      op,
			Right:   right,
		}
	}
	return left
}

func (p *Parser) parseEquality() Expr {
	left := p.parseComparison()
	if left == nil {
		return nil
	}
	for p.cur().Type == EQ || p.cur().Type == NOT_EQ {
		op := p.cur().Literal
		p.advance()
		right := p.parseComparison()
		if right == nil {
			return nil
		}
		left = &BinaryExpr{
			BasePos: BasePos{TokLine: left.Line(), TokCol: left.Column()},
			Left:    left,
			Op:      op,
			Right:   right,
		}
	}
	return left
}

func (p *Parser) parseComparison() Expr {
	left := p.parseAdditive()
	if left == nil {
		return nil
	}
	for p.cur().Type == LT || p.cur().Type == GT || p.cur().Type == LTE || p.cur().Type == GTE {
		op := p.cur().Literal
		p.advance()
		right := p.parseAdditive()
		if right == nil {
			return nil
		}
		left = &BinaryExpr{
			BasePos: BasePos{TokLine: left.Line(), TokCol: left.Column()},
			Left:    left,
			Op:      op,
			Right:   right,
		}
	}
	return left
}

func (p *Parser) parseAdditive() Expr {
	left := p.parseMultiplicative()
	if left == nil {
		return nil
	}
	for p.cur().Type == PLUS || p.cur().Type == MINUS {
		op := p.cur().Literal
		p.advance()
		right := p.parseMultiplicative()
		if right == nil {
			return nil
		}
		left = &BinaryExpr{
			BasePos: BasePos{TokLine: left.Line(), TokCol: left.Column()},
			Left:    left,
			Op:      op,
			Right:   right,
		}
	}
	return left
}

func (p *Parser) parseMultiplicative() Expr {
	left := p.parseUnary()
	if left == nil {
		return nil
	}
	for p.cur().Type == STAR || p.cur().Type == SLASH {
		op := p.cur().Literal
		p.advance()
		right := p.parseUnary()
		if right == nil {
			return nil
		}
		left = &BinaryExpr{
			BasePos: BasePos{TokLine: left.Line(), TokCol: left.Column()},
			Left:    left,
			Op:      op,
			Right:   right,
		}
	}
	return left
}

func (p *Parser) parseUnary() Expr {
	if p.cur().Type == BANG || p.cur().Type == MINUS || p.cur().Type == INCREMENT || p.cur().Type == DECREMENT {
		opTok := p.cur()
		p.advance()
		right := p.parseUnary()
		if right == nil {
			return nil
		}
		return &UnaryExpr{
			BasePos: posFromToken(opTok),
			Op:      opTok.Literal,
			Right:   right,
		}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() Expr {
	expr := p.parsePrimary()
	if expr == nil {
		return nil
	}

	for {
		switch p.cur().Type {
		case LPAREN:
			p.advance()
			var args []Expr
			if p.cur().Type != RPAREN {
				prev := p.allowStructLit
				p.allowStructLit = true
				for {
					arg := p.parseExpr()
					if arg == nil {
						p.allowStructLit = prev
						return nil
					}
					args = append(args, arg)
					if p.cur().Type == COMMA {
						p.advance()
						continue
					}
					break
				}
				p.allowStructLit = prev
			}
			if _, ok := p.expect(RPAREN); !ok {
				return nil
			}
			expr = &CallExpr{
				BasePos: BasePos{TokLine: expr.Line(), TokCol: expr.Column()},
				Callee:  expr,
				Args:    args,
			}

		case LBRACKET:
			p.advance()
			prev := p.allowStructLit
			p.allowStructLit = true
			idx := p.parseExpr()
			p.allowStructLit = prev
			if idx == nil {
				return nil
			}
			if _, ok := p.expect(RBRACKET); !ok {
				return nil
			}
			expr = &IndexExpr{
				BasePos: BasePos{TokLine: expr.Line(), TokCol: expr.Column()},
				Target:  expr,
				Index:   idx,
			}

		case DOT:
			p.advance()
			fieldTok, ok := p.expect(IDENT)
			if !ok {
				return nil
			}
			expr = &FieldExpr{
				BasePos: BasePos{TokLine: expr.Line(), TokCol: expr.Column()},
				Target:  expr,
				Field:   fieldTok.Literal,
			}

		case INCREMENT, DECREMENT:
			op := p.cur().Literal
			p.advance()
			expr = &UnaryExpr{
				BasePos: BasePos{TokLine: expr.Line(), TokCol: expr.Column()},
				Op:      op,
				Right:   expr,
			}

		default:
			return expr
		}
	}
}

func (p *Parser) parsePrimary() Expr {
	tok := p.cur()
	switch tok.Type {
	case INT:
		p.advance()
		return &IntLit{BasePos: posFromToken(tok), Value: tok.Literal}

	case FLOAT:
		p.advance()
		return &FloatLit{BasePos: posFromToken(tok), Value: tok.Literal}

	case STRING:
		p.advance()
		return &StringLit{BasePos: posFromToken(tok), Value: tok.Literal}

	case BOOL:
		p.advance()
		val, err := strconv.ParseBool(tok.Literal)
		if err != nil {
			p.addError(tok, fmt.Sprintf("invalid boolean literal: %q", tok.Literal))
			return nil
		}
		return &BoolLit{BasePos: posFromToken(tok), Value: val}

	case LBRACKET:
		return p.parseListLit()

	case IDENT:
		p.advance()
		if p.allowStructLit && p.cur().Type == LBRACE {
			return p.parseStructLit(tok)
		}
		return &IdentExpr{BasePos: posFromToken(tok), Name: tok.Literal}

	case FN:
		return p.parseFuncLit()

	case LPAREN:
		p.advance()
		prev := p.allowStructLit
		p.allowStructLit = true
		inner := p.parseExpr()
		p.allowStructLit = prev
		if inner == nil {
			return nil
		}
		if _, ok := p.expect(RPAREN); !ok {
			return nil
		}
		return inner

	default:
		p.addError(tok, fmt.Sprintf("expected expression, got %s (%q)", tok.Type, tok.Literal))
		return nil
	}
}

func (p *Parser) parseListLit() *ListLit {
	tok := p.cur()
	p.advance()
	if _, ok := p.expect(RBRACKET); !ok {
		p.addError(tok, "expected ']' in list literal")
		return nil
	}

	elemType, ok := p.parseType()
	if !ok {
		return nil
	}

	if _, ok := p.expect(LBRACE); !ok {
		return nil
	}

	if p.cur().Type == RBRACE {
		p.advance()
		return &ListLit{
			BasePos:      posFromToken(tok),
			ElemTypeName: elemType,
			Elements:     nil,
		}
	}

	prev := p.allowStructLit
	p.allowStructLit = true
	first := p.parseExpr()
	if first == nil {
		p.allowStructLit = prev
		return nil
	}

	if p.cur().Type == SEMICOLON {
		p.advance()
		countExpr := p.parseExpr()
		p.allowStructLit = prev
		if countExpr == nil {
			return nil
		}
		if _, ok := p.expect(RBRACE); !ok {
			return nil
		}
		return &ListLit{
			BasePos:      posFromToken(tok),
			ElemTypeName: elemType,
			RepeatVal:    first,
			RepeatCount:  countExpr,
		}
	}

	elems := []Expr{first}
	for p.cur().Type == COMMA {
		p.advance()
		if p.cur().Type == RBRACE {
			break
		}
		nextEl := p.parseExpr()
		if nextEl == nil {
			p.allowStructLit = prev
			return nil
		}
		elems = append(elems, nextEl)
	}
	p.allowStructLit = prev

	if _, ok := p.expect(RBRACE); !ok {
		return nil
	}

	return &ListLit{
		BasePos:      posFromToken(tok),
		ElemTypeName: elemType,
		Elements:     elems,
	}
}

func (p *Parser) parseStructLit(typeTok Token) *StructLit {
	if _, ok := p.expect(LBRACE); !ok {
		return nil
	}

	var fields []*StructFieldInit
	if p.cur().Type != RBRACE {
		prev := p.allowStructLit
		p.allowStructLit = true
		for {
			nameTok, ok := p.expect(IDENT)
			if !ok {
				p.allowStructLit = prev
				return nil
			}
			if _, ok := p.expect(COLON); !ok {
				p.allowStructLit = prev
				return nil
			}
			val := p.parseExpr()
			if val == nil {
				p.allowStructLit = prev
				return nil
			}
			fields = append(fields, &StructFieldInit{
				BasePos: posFromToken(nameTok),
				Name:    nameTok.Literal,
				Value:   val,
			})
			if p.cur().Type == COMMA {
				p.advance()
				continue
			}
			break
		}
		p.allowStructLit = prev
	}

	if _, ok := p.expect(RBRACE); !ok {
		return nil
	}

	return &StructLit{
		BasePos:  posFromToken(typeTok),
		TypeName: typeTok.Literal,
		Fields:   fields,
	}
}

func (p *Parser) parseFuncLit() *FuncLit {
	startTok := p.cur()
	p.advance()
	params, ok := p.parseParamGroup()
	if !ok {
		return nil
	}

	var retType string
	if p.cur().Type == ARROW {
		p.advance()
		retType, ok = p.parseReturnType()
		if !ok {
			return nil
		}
	}

	body := p.parseBlock()
	if body == nil {
		return nil
	}

	return &FuncLit{
		BasePos:    posFromToken(startTok),
		Params:     params,
		ReturnType: retType,
		Body:       body,
	}
}

func isNilExpr(e Expr) bool {
	if e == nil {
		return true
	}
	v := reflect.ValueOf(e)
	return v.Kind() == reflect.Ptr && v.IsNil()
}
