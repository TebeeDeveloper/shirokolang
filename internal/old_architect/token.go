package main

import "fmt"

type TokenType int

const (
	ILLEGAL TokenType = iota
	EOF

	// Literals & Identifiers
	IDENT  // ident
	INT    // int_lit
	FLOAT  // float_lit
	STRING // string_lit
	BOOL   // bool_lit ("true" | "false")

	// Keywords
	STRUCT   // "struct"
	FN       // "fn"
	LET      // "let"
	CONST    // "const"
	RETURN   // "return"
	IF       // "if"
	ELSE     // "else"
	FOR      // "for"
	USE      // "use"
	CONTINUE // "continue"
	BREAK    // "break"

	// Operators & Delimiters
	ASSIGN    // "="
	EQ        // "=="
	NOT_EQ    // "!="
	BANG      // "!"
	LT        // "<"
	GT        // ">"
	LTE       // "<="
	GTE       // ">="
	AND       // "&&"
	OR        // "||"
	PLUS      // "+"
	INCREMENT // "++"
	MINUS     // "-"
	DECREMENT // "--"
	STAR      // "*"
	SLASH     // "/"
	ARROW     // "->"
	COLON     // ":"
	SEMICOLON // ";"
	COMMA     // ","
	DOT       // "."
	LPAREN    // "("
	RPAREN    // ")"
	LBRACE    // "{"
	RBRACE    // "}"
	LBRACKET  // "["
	RBRACKET  // "]"
	ELLIPSIS  // "..."
)

var tokenNames = map[TokenType]string{
	ILLEGAL:   "ILLEGAL",
	EOF:       "EOF",
	IDENT:     "IDENT",
	INT:       "INT",
	FLOAT:     "FLOAT",
	STRING:    "STRING",
	BOOL:      "BOOL",
	STRUCT:    "STRUCT",
	FN:        "FN",
	LET:       "LET",
	CONST:     "CONST",
	RETURN:    "RETURN",
	IF:        "IF",
	ELSE:      "ELSE",
	FOR:       "FOR",
	USE:       "USE",
	CONTINUE:  "CONTINUE",
	BREAK:     "BREAK",
	ASSIGN:    "=",
	EQ:        "==",
	NOT_EQ:    "!=",
	BANG:      "!",
	LT:        "<",
	GT:        ">",
	LTE:       "<=",
	GTE:       ">=",
	AND:       "&&",
	OR:        "||",
	PLUS:      "+",
	INCREMENT: "++",
	MINUS:     "-",
	DECREMENT: "--",
	STAR:      "*",
	SLASH:     "/",
	ARROW:     "->",
	COLON:     ":",
	SEMICOLON: ";",
	COMMA:     ",",
	DOT:       ".",
	LPAREN:    "(",
	RPAREN:    ")",
	LBRACE:    "{",
	RBRACE:    "}",
	LBRACKET:  "[",
	RBRACKET:  "]",
	ELLIPSIS:   "...",
}

func (t TokenType) String() string {
	if name, ok := tokenNames[t]; ok {
		return name
	}
	return fmt.Sprintf("TokenType(%d)", int(t))
}

var keywords = map[string]TokenType{
	"true":     BOOL,
	"false":    BOOL,
	"struct":   STRUCT,
	"fn":       FN,
	"let":      LET,
	"const":    CONST,
	"return":   RETURN,
	"if":       IF,
	"else":     ELSE,
	"for":      FOR,
	"use":      USE,
	"continue": CONTINUE,
	"break":    BREAK,
}

func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return IDENT
}

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

func (t Token) String() string {
	return fmt.Sprintf("%s(%q) at %d:%d", t.Type, t.Literal, t.Line, t.Column)
}
