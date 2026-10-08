package main

import (
	"fmt"
)

type Token struct {
	kind  string
	value string
	line  int
	col   int
}

type Lexer struct {
	src  string
	pos  int
	line int
	col  int
	toks []Token
}

func (self *Lexer) peek() byte {
	if self.pos >= len(self.src) {
		return 0
	}
	return self.src[self.pos]
}

func (self *Lexer) advance() byte {
	c := self.peek()
	if c == 0 {
		return 0
	}
	self.pos = self.pos + 1
	self.col = self.col + 1
	return c
}

func (self *Lexer) isDigit(c byte) bool {
	return c >= byte(48) && c <= byte(57)
}

func (self *Lexer) isAlpha(c byte) bool {
	return c >= byte(97) && c <= byte(122) || c >= byte(65) && c <= byte(90) || c == byte(95)
}

func (self *Lexer) isSpace(c byte) bool {
	return c == byte(32) || c == byte(9) || c == byte(13) || c == byte(10)
}

func (self *Lexer) push(kind string, value string, line int, col int) {
	t := Token{kind: kind, value: value, line: line, col: col}
	self.toks = append(self.toks, t)
}

func (self *Lexer) scanNumber() string {
	start := self.pos
	for self.peek() != 0 && self.isDigit(self.peek()) {
		self.advance()
	}
	return self.src[start:self.pos]
}

func (self *Lexer) scanIdent() string {
	start := self.pos
	for self.peek() != 0 && self.isAlpha(self.peek()) || self.isDigit(self.peek()) {
		self.advance()
	}
	return self.src[start:self.pos]
}

func (self *Lexer) run() []Token {
	for self.pos < len(self.src) {
		c := self.peek()
		if c == byte(10) {
			self.pos = self.pos + 1
			self.line = self.line + 1
			self.col = 1
		} else if self.isSpace(c) {
			self.advance()
		} else if self.isDigit(c) {
			startLine := self.line
			startCol := self.col
			n := self.scanNumber()
			self.push("INT", n, startLine, startCol)
		} else if self.isAlpha(c) {
			startLine := self.line
			startCol := self.col
			w := self.scanIdent()
			self.push("IDENT", w, startLine, startCol)
		} else {
			startLine := self.line
			startCol := self.col
			ch := self.advance()
			self.push(string(ch), string(ch), startLine, startCol)
		}
	}
	return self.toks
}

func main() {
	src := "let x = 1 + 2"
	lx := Lexer{src: src, pos: 0, line: 1, col: 1, toks: []Token{}}
	toks := lx.run()
	__t1 := 0
	for __t1 < len(toks) {
		t := toks[__t1]
		fmt.Printf("%s: %s\n", t.kind, t.value)
		__t1 = __t1 + 1
	}
}
