// shiroko/core/token/token.go
package token

type Kind int

const (
	EOF Kind = iota
	NEWLINE
	IDENT
	INT
	FLOAT
	STRING
	BYTE
	// keywords
	PACKAGE
	IMPORT
	INTERFACE
	STRUCT
	FN
	LET
	CONST
	FOR
	RANGE
	ITER
	IF
	ELSE
	RETURN
	// symbols
	ELLIPSIS
	EQEQ
	NEQ
	LEQ
	GEQ
	ANDAND
	OROR
	ARROW
	PLUSPLUS
	MINUSMINUS
	COLONEQ
	DOTDOT
	PLUS
	MINUS
	STAR
	SLASH
	PERCENT
	LT
	GT
	EQ
	BANG
	LPAREN
	RPAREN
	LBRACE
	RBRACE
	LBRACKET
	RBRACKET
	COMMA
	SEMI
	COLON
	DOT
	AT
	PIPE
)

type Token struct {
	Kind Kind
	Text string
	Int  int
	Float float64
	Line int
	Col  int
}

var Keywords = map[string]Kind{
	"package":   PACKAGE,
	"import":    IMPORT,
	"interface": INTERFACE,
	"struct":    STRUCT,
	"fn":        FN,
	"let":       LET,
	"const":     CONST,
	"for":       FOR,
	"range":     RANGE,
	"iter":      ITER,
	"if":        IF,
	"else":      ELSE,
	"return":    RETURN,
}

type Symbol struct {
	Text string
	Kind Kind
}

// longest-first
var Symbols = []Symbol{
	{"...", ELLIPSIS},
	{"==", EQEQ},
	{"!=", NEQ},
	{"<=", LEQ},
	{">=", GEQ},
	{"&&", ANDAND},
	{"||", OROR},
	{"->", ARROW},
	{"++", PLUSPLUS},
	{"--", MINUSMINUS},
	{":=", COLONEQ},
	{"..", DOTDOT},
	{"+", PLUS},
	{"-", MINUS},
	{"*", STAR},
	{"/", SLASH},
	{"%", PERCENT},
	{"<", LT},
	{">", GT},
	{"=", EQ},
	{"!", BANG},
	{"(", LPAREN},
	{")", RPAREN},
	{"{", LBRACE},
	{"}", RBRACE},
	{"[", LBRACKET},
	{"]", RBRACKET},
	{",", COMMA},
	{";", SEMI},
	{":", COLON},
	{".", DOT},
	{"@", AT},
	{"|", PIPE},
}

func (k Kind) String() string {
	names := map[Kind]string{
		EOF: "EOF", NEWLINE: "NEWLINE", IDENT: "IDENT", INT: "INT",
		FLOAT: "FLOAT", STRING: "STRING", BYTE: "BYTE",
		PACKAGE: "PACKAGE", IMPORT: "IMPORT", INTERFACE: "INTERFACE",
		STRUCT: "STRUCT", FN: "FN", LET: "LET", CONST: "CONST",
		FOR: "FOR", RANGE: "RANGE", ITER: "ITER", IF: "IF",
		ELSE: "ELSE", RETURN: "RETURN",
		ELLIPSIS: "...", EQEQ: "==", NEQ: "!=", LEQ: "<=", GEQ: ">=",
		ANDAND: "&&", OROR: "||", ARROW: "->", PLUSPLUS: "++",
		MINUSMINUS: "--", COLONEQ: ":=", DOTDOT: "..",
		PLUS: "+", MINUS: "-", STAR: "*", SLASH: "/", PERCENT: "%",
		LT: "<", GT: ">", EQ: "=", BANG: "!",
		LPAREN: "(", RPAREN: ")", LBRACE: "{", RBRACE: "}",
		LBRACKET: "[", RBRACKET: "]", COMMA: ",", SEMI: ";",
		COLON: ":", DOT: ".", AT: "@", PIPE: "|",
	}
	if s, ok := names[k]; ok {
		return s
	}
	return "UNKNOWN"
}
