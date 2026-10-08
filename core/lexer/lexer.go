package lexer

import (
	"fmt"
	"strconv"
	"strings"
)

var Keywords = map[string]bool{
	"package": true, "import": true, "interface": true, "struct": true,
	"fn": true, "let": true, "const": true, "for": true, "range": true,
	"iter": true, "if": true, "else": true, "return": true,
	"match": true,
}

var Symbols = []string{
	"...", "=>", "==", "!=", "<=", ">=", "&&", "||", "->", "++", "--", ":=", "..",
	"+", "-", "*", "/", "%", "<", ">", "=", "!",
	"(", ")", "{", "}", "[", "]",
	",", ";", ":", ".", "@", "|",
}

type Token struct {
	Kind  string
	Value any
	Line  int
	Col   int
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isAlnum(c byte) bool { return isAlpha(c) || isDigit(c) }

func Lex(src string) ([]Token, error) {
	var toks []Token
	i := 0
	line, col := 1, 1
	n := len(src)

	for i < n {
		c := src[i]

		if c == '\n' {
			toks = append(toks, Token{"NEWLINE", "\n", line, col})
			i++
			line++
			col = 1
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' {
			i++
			col++
			continue
		}

		// line comment
		if c == '/' && i+1 < n && src[i+1] == '/' {
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		}

		// block comment
		if c == '/' && i+1 < n && src[i+1] == '*' {
			sl := line
			i += 2
			col += 2
			for i+1 < n && !(src[i] == '*' && src[i+1] == '/') {
				if src[i] == '\n' {
					line++
					col = 1
				}
				i++
				col++
			}
			if i+1 >= n {
				return nil, fmt.Errorf("unterminated block comment at line %d", sl)
			}
			i += 2
			col += 2
			continue
		}

		// string literal
		if c == '"' {
			sl, sc := line, col
			i++
			col++
			var buf []byte
			for i < n && src[i] != '"' {
				if src[i] == '\\' && i+1 < n {
					nxt := src[i+1]
					var repl byte
					switch nxt {
						case 'n':
							repl = '\n'
						case 't':
							repl = '\t'
						case 'r':
							repl = '\r'
						case '"':
							repl = '"'
						case '\\':
							repl = '\\'
						default:
							repl = nxt
					}
					buf = append(buf, repl)
					i += 2
					col += 2
				} else {
					if src[i] == '\n' {
						return nil, fmt.Errorf("newline in string at line %d, col %d", sl, sc)
					}
					buf = append(buf, src[i])
					i++
					col++
				}
			}
			if i >= n {
				return nil, fmt.Errorf("unterminated string at line %d, col %d", sl, sc)
			}
			i++
			col++
			toks = append(toks, Token{"STRING", string(buf), sl, sc})
			continue
		}

		// char literal -> BYTE
		if c == '\'' {
			sl, sc := line, col
			i++
			col++
			if i >= n {
				return nil, fmt.Errorf("unterminated char literal at line %d, col %d", sl, sc)
			}
			var val int
			if src[i] == '\\' {
				if i+1 >= n {
					return nil, fmt.Errorf("unterminated escape at line %d, col %d", sl, sc)
				}
				nxt := src[i+1]
				ok := true
				switch nxt {
					case 'n':
						val = 10
					case 't':
						val = 9
					case 'r':
						val = 13
					case '0':
						val = 0
					case '\\':
						val = 92
					case '\'':
						val = 39
					case '"':
						val = 34
					default:
						ok = false
				}
				if !ok {
					return nil, fmt.Errorf("unknown escape '\\%c' at line %d, col %d", nxt, sl, sc)
				}
				i += 2
				col += 2
			} else {
				if src[i] == '\n' {
					return nil, fmt.Errorf("newline in char literal at line %d, col %d", sl, sc)
				}
				val = int(src[i])
				i++
				col++
			}
			if val > 255 {
				return nil, fmt.Errorf("char literal out of byte range at line %d, col %d", sl, sc)
			}
			if i >= n || src[i] != '\'' {
				return nil, fmt.Errorf("expected closing ' at line %d, col %d", sl, sc)
			}
			i++
			col++
			toks = append(toks, Token{"BYTE", val, sl, sc})
			continue
		}

		// number
		if isDigit(c) {
			sl, sc := line, col
			start := i
			for i < n && isDigit(src[i]) {
				i++
				col++
			}
			kind := "INT"
			if i < n && src[i] == '.' && i+1 < n && isDigit(src[i+1]) {
				kind = "FLOAT"
				i++
				col++
				for i < n && isDigit(src[i]) {
					i++
					col++
				}
			}
			text := src[start:i]
			if kind == "FLOAT" {
				f, err := strconv.ParseFloat(text, 64)
				if err != nil {
					return nil, err
				}
				toks = append(toks, Token{"FLOAT", f, sl, sc})
			} else {
				v, err := strconv.Atoi(text)
				if err != nil {
					return nil, err
				}
				toks = append(toks, Token{"INT", v, sl, sc})
			}
			continue
		}

		// identifier or keyword
		if isAlpha(c) || c == '_' {
			sl, sc := line, col
			start := i
			for i < n && (isAlnum(src[i]) || src[i] == '_') {
				i++
				col++
			}
			word := src[start:i]
			kind := "IDENT"
			if Keywords[word] {
				kind = strings.ToUpper(word)
			}
			toks = append(toks, Token{kind, word, sl, sc})
			continue
		}

		// symbols
		matched := false
		for _, s := range Symbols {
			if strings.HasPrefix(src[i:], s) {
				toks = append(toks, Token{s, s, line, col})
				i += len(s)
				col += len(s)
				matched = true
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("unexpected char %q at line %d, col %d", c, line, col)
		}
	}

	toks = append(toks, Token{"EOF", nil, line, col})
	return toks, nil
}
