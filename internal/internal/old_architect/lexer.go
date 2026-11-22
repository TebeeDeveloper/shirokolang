package main

import "unicode/utf8"

type Lexer struct {
	input        string
	position     int
	readPosition int
	ch           rune
	line         int
	column       int
}

func NewLexer(input string) *Lexer {
	l := &Lexer{
		input:  input,
		line:   1,
		column: 0,
	}
	l.readChar()
	return l
}

func Tokenize(input string) []Token {
	toks := []Token{}
	l := NewLexer(input)
	for {
		tok := l.NextToken()
		toks = append(toks, tok)
		if tok.Type == EOF {
			break
		}
	}
	return toks
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
		l.position = l.readPosition
		l.column++
		return
	}
	r, width := utf8.DecodeRuneInString(l.input[l.readPosition:])
	l.ch = r
	l.position = l.readPosition
	l.readPosition += width
	if r == '\n' {
		l.line++
		l.column = 0
	} else {
		l.column++
	}
}

func (l *Lexer) peekChar() rune {
	if l.readPosition >= len(l.input) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.readPosition:])
	return r
}

func (l *Lexer) peekAtOffset(byteOffset int) rune {
	idx := l.readPosition + byteOffset
	if idx < 0 || idx >= len(l.input) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.input[idx:])
	return r
}

func isLetter(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func isWhitespace(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func (l *Lexer) skipWhitespace() {
	for isWhitespace(l.ch) {
		l.readChar()
	}
}

func (l *Lexer) readString(line, col int) Token {
	l.readChar() // bỏ qua dấu '"' mở

	var out []rune
	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			l.readChar() // ăn '\'
			if l.ch == 0 {
				return Token{Type: ILLEGAL, Literal: string(out), Line: line, Column: col}
			}
			switch l.ch {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case '\\':
				out = append(out, '\\')
			case '"':
				out = append(out, '"')
			case '0':
				out = append(out, 0)
			default:
				out = append(out, l.ch)
			}
			l.readChar()
			continue
		}
		out = append(out, l.ch)
		l.readChar()
	}

	if l.ch != '"' {
		return Token{Type: ILLEGAL, Literal: string(out), Line: line, Column: col}
	}

	l.readChar() // bỏ qua dấu '"' đóng
	return Token{Type: STRING, Literal: string(out), Line: line, Column: col}
}

func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	line, col := l.line, l.column

	if l.ch == 0 {
		return Token{Type: EOF, Literal: "", Line: line, Column: col}
	}

	if l.ch == '"' {
		return l.readString(line, col)
	}

	if isLetter(l.ch) {
		start := l.position
		for isLetter(l.ch) || isDigit(l.ch) {
			l.readChar()
		}
		lit := l.input[start:l.position]
		return Token{Type: LookupIdent(lit), Literal: lit, Line: line, Column: col}
	}

	if isDigit(l.ch) {
		return l.readNumber(line, col)
	}

	switch l.ch {
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Type: EQ, Literal: "==", Line: line, Column: col}
		}
		l.readChar()
		return Token{Type: ASSIGN, Literal: "=", Line: line, Column: col}
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Type: NOT_EQ, Literal: "!=", Line: line, Column: col}
		}
		l.readChar()
		return Token{Type: BANG, Literal: "!", Line: line, Column: col}
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Type: LTE, Literal: "<=", Line: line, Column: col}
		}
		l.readChar()
		return Token{Type: LT, Literal: "<", Line: line, Column: col}
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{Type: GTE, Literal: ">=", Line: line, Column: col}
		}
		l.readChar()
		return Token{Type: GT, Literal: ">", Line: line, Column: col}
	case '&':
		if l.peekChar() == '&' {
			l.readChar()
			l.readChar()
			return Token{Type: AND, Literal: "&&", Line: line, Column: col}
		}
		ch := string(l.ch)
		l.readChar()
		return Token{Type: ILLEGAL, Literal: ch, Line: line, Column: col}
	case '|':
		if l.peekChar() == '|' {
			l.readChar()
			l.readChar()
			return Token{Type: OR, Literal: "||", Line: line, Column: col}
		}
		ch := string(l.ch)
		l.readChar()
		return Token{Type: ILLEGAL, Literal: ch, Line: line, Column: col}
	case '-':
		if l.peekChar() == '-' {
			l.readChar()
			l.readChar()
			return Token{Type: DECREMENT, Literal: "--", Line: line, Column: col}
		}
		if l.peekChar() == '>' {
			l.readChar()
			l.readChar()
			return Token{Type: ARROW, Literal: "->", Line: line, Column: col}
		}
		l.readChar()
		return Token{Type: MINUS, Literal: "-", Line: line, Column: col}
	case '+':
		if l.peekChar() == '+' {
			l.readChar()
			l.readChar()
			return Token{Type: INCREMENT, Literal: "++", Line: line, Column: col}
		}
		l.readChar()
		return Token{Type: PLUS, Literal: "+", Line: line, Column: col}
	case '*':
		l.readChar()
		return Token{Type: STAR, Literal: "*", Line: line, Column: col}
	case '/':
		l.readChar()
		return Token{Type: SLASH, Literal: "/", Line: line, Column: col}
	case ':':
		l.readChar()
		return Token{Type: COLON, Literal: ":", Line: line, Column: col}
	case ';':
		l.readChar()
		return Token{Type: SEMICOLON, Literal: ";", Line: line, Column: col}
	case ',':
		l.readChar()
		return Token{Type: COMMA, Literal: ",", Line: line, Column: col}
	case '.':
		if l.peekChar() == '.' && l.peekAtOffset(1) == '.' {
			l.readChar()
			l.readChar()
			l.readChar()
			return Token{Type: ELLIPSIS, Literal: "...", Line: line, Column: col}
		}
		l.readChar()
		return Token{Type: DOT, Literal: ".", Line: line, Column: col}
	case '(':
		l.readChar()
		return Token{Type: LPAREN, Literal: "(", Line: line, Column: col}
	case ')':
		l.readChar()
		return Token{Type: RPAREN, Literal: ")", Line: line, Column: col}
	case '{':
		l.readChar()
		return Token{Type: LBRACE, Literal: "{", Line: line, Column: col}
	case '}':
		l.readChar()
		return Token{Type: RBRACE, Literal: "}", Line: line, Column: col}
	case '[':
		l.readChar()
		return Token{Type: LBRACKET, Literal: "[", Line: line, Column: col}
	case ']':
		l.readChar()
		return Token{Type: RBRACKET, Literal: "]", Line: line, Column: col}

	default:
		ch := string(l.ch)
		l.readChar()
		return Token{Type: ILLEGAL, Literal: ch, Line: line, Column: col}
	}
}

func (l *Lexer) readNumber(line, col int) Token {
	start := l.position
	tokType := INT

	l.consumeIntDigits()

	if l.ch == '.' && isDigit(l.peekChar()) {
		tokType = FLOAT
		l.readChar()
		for isDigit(l.ch) {
			l.readChar()
		}
	}

	if l.ch == 'e' || l.ch == 'E' {
		next := l.peekChar()
		offset := 0
		if next == '+' || next == '-' {
			offset = 1
		}
		if isDigit(l.peekAtOffset(offset)) {
			tokType = FLOAT
			l.readChar()
			if l.ch == '+' || l.ch == '-' {
				l.readChar()
			}
			for isDigit(l.ch) {
				l.readChar()
			}
		}
	}

	return Token{
		Type:    tokType,
		Literal: l.input[start:l.position],
		Line:    line,
		Column:  col,
	}
}

func (l *Lexer) consumeIntDigits() {
	for isDigit(l.ch) {
		l.readChar()
		if l.ch == '_' && isDigit(l.peekChar()) {
			l.readChar()
		}
	}
}
