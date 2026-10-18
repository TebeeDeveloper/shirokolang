package lexer

import (
	"fmt"
	"strconv"
	"strings"
)

var Keywords = map[string]bool{
	"package": true, "import": true, "interface": true, "struct": true,
	"func": true, "let": true, "const": true, "for": true, "range": true,
	"iter": true, "if": true, "else": true, "return": true,
	"match": true, "map": true, "break": true, "continue": true,
}

var Symbols = []string{
	"...", "=>", "==", "!=", "<=", ">=", "&&", "||", "->", "++", "--", ":=", "..",
	"+", "-", "*", "/", "%", "<", ">", "=", "!",
	"(", ")", "{", "}", "[", "]",
	",", ";", ":", ".", "@", "|",
	"&",
}

type Token struct {
	Kind  string
	Value any
	Line  int
	Col   int
}

// ★ InterpSeg is one piece of an interpolated string literal (ISTRING).
// A segment is either literal text or a hole — an expression source
// that the parser re-lexes and re-parses on its own.
type InterpSeg struct {
	Kind string // "text" | "expr"
	Text string // decoded text (for "text")
	Src  string // raw source of the expression (for "expr")
	Fmt  string // format spec after ':' — empty when absent
	Line int
	Col  int
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
			if c == '\t' {
				i++
				col += 4 - ((col - 1) & 4)
				continue
			}
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

		// ★ interpolated string literal  #"..."
		if c == '#' {
			if i+1 < n && src[i+1] == '"' {
				tok, ni, nl, nc, err := lexInterpString(src, i, line, col)
				if err != nil {
					return nil, err
				}
				toks = append(toks, tok)
				i, line, col = ni, nl, nc
				continue
			}
			return nil, fmt.Errorf(
				"unexpected char %q at line %d, col %d", c, line, col)
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

// ★ lexInterpString scans  #"..."  starting at src[at] == '#'.
//
// Returns the ISTRING token and the new position after the closing
// quote. The token's Value is a []InterpSeg.
//
// Rules:
//   - Same escape table as a normal string, plus \{ and \} for a
//     literal brace.
//   - { starts a hole; the hole ends at the matching } at depth 0.
//     String literals inside the hole are skipped so a " in an
//     expression does not end the outer string.
//   - Nested #"..." inside a hole is rejected.
//   - Unescaped } outside a hole is an error.
func lexInterpString(src string, at, line, col int) (Token, int, int, int, error) {
	sl, sc := line, col
	i := at + 2 // skip #"
	col += 2

	var segs []InterpSeg
	var text []byte
	textStartLine, textStartCol := line, col

	flushText := func() {
		if len(text) == 0 {
			return
		}
		segs = append(segs, InterpSeg{
			Kind: "text",
			Text: string(text),
			Line: textStartLine,
			Col:  textStartCol,
		})
		text = text[:0]
	}

	for i < len(src) {
		c := src[i]

		if c == '\n' {
			return Token{}, 0, 0, 0, fmt.Errorf(
				"newline in interpolated string at line %d, col %d", sl, sc)
		}

		// closing quote
		if c == '"' {
			flushText()
			i++
			col++
			return Token{"ISTRING", segs, sl, sc}, i, line, col, nil
		}

		// escape
		if c == '\\' {
			if i+1 >= len(src) {
				return Token{}, 0, 0, 0, fmt.Errorf(
					"unterminated escape at line %d, col %d", line, col)
			}
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
			case '{':
				repl = '{'
			case '}':
				repl = '}'
			default:
				repl = nxt
			}
			if len(text) == 0 {
				textStartLine, textStartCol = line, col
			}
			text = append(text, repl)
			i += 2
			col += 2
			continue
		}

		// bare brace error
		if c == '}' {
			return Token{}, 0, 0, 0, fmt.Errorf(
				"unmatched '}' in interpolated string at line %d, col %d "+
					"(use \\} for a literal brace)", line, col)
		}

		// hole
		if c == '{' {
			flushText()
			holeStartLine, holeStartCol := line, col
			srcStart := i + 1
			i++
			col++

			depth := 1
			for i < len(src) {
				h := src[i]

				if h == '\n' {
					line++
					col = 1
					i++
					continue
				}

				// nested interpolated string is not allowed
				if h == '#' && i+1 < len(src) && src[i+1] == '"' {
					return Token{}, 0, 0, 0, fmt.Errorf(
						"nested #\"...\" inside interpolated-string hole "+
							"at line %d, col %d (hoist it to a let)",
						line, col)
				}

				// skip ordinary strings inside the hole
				if h == '"' {
					i++
					col++
					for i < len(src) && src[i] != '"' {
						if src[i] == '\\' && i+1 < len(src) {
							i += 2
							col += 2
							continue
						}
						if src[i] == '\n' {
							return Token{}, 0, 0, 0, fmt.Errorf(
								"newline in string inside interpolated-string "+
									"hole at line %d, col %d", line, col)
						}
						i++
						col++
					}
					if i >= len(src) {
						return Token{}, 0, 0, 0, fmt.Errorf(
							"unterminated string inside interpolated-string "+
								"hole at line %d, col %d", line, col)
					}
					i++
					col++
					continue
				}

				// skip comments inside the hole so braces in them
				// don't throw off the depth counter
				if h == '/' && i+1 < len(src) {
					if src[i+1] == '/' {
						i += 2
						col += 2
						for i < len(src) && src[i] != '\n' {
							i++
							col++
						}
						continue
					}
					if src[i+1] == '*' {
						i += 2
						col += 2
						for i+1 < len(src) &&
							!(src[i] == '*' && src[i+1] == '/') {
							if src[i] == '\n' {
								line++
								col = 1
							} else {
								col++
							}
							i++
						}
						if i+1 >= len(src) {
							return Token{}, 0, 0, 0, fmt.Errorf(
								"unterminated block comment inside "+
									"interpolated-string hole at line %d, col %d",
								line, col)
						}
						i += 2
						col += 2
						continue
					}
				}

				if h == '{' {
					depth++
				} else if h == '}' {
					depth--
					if depth == 0 {
						raw := src[srcStart:i]
						exprSrc, fmtSrc := splitHoleSrc(raw)
						segs = append(segs, InterpSeg{
							Kind: "expr",
							Src:  exprSrc,
							Fmt:  fmtSrc,
							Line: holeStartLine,
							Col:  holeStartCol,
						})
						i++
						col++
						break
					}
				}
				i++
				col++
			}

			if depth != 0 {
				return Token{}, 0, 0, 0, fmt.Errorf(
					"unterminated hole in interpolated string at line %d, col %d",
					holeStartLine, holeStartCol)
			}
			// remember where the next text segment starts
			textStartLine, textStartCol = line, col
			continue
		}

		// ordinary character
		if len(text) == 0 {
			textStartLine, textStartCol = line, col
		}
		text = append(text, c)
		i++
		col++
	}

	return Token{}, 0, 0, 0, fmt.Errorf(
		"unterminated interpolated string at line %d, col %d", sl, sc)
}

// splitHoleSrc splits a hole's raw source at the first top-level ':'.
// "top-level" means not inside (), [], {}, or a "..." string.
// Returns (exprSrc, fmtSrc). fmtSrc is "" if no top-level ':' exists.
func splitHoleSrc(s string) (string, string) {
	depth := 0
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '"' {
			i++
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' && i+1 < len(s) {
					i += 2
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
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ':':
			if depth == 0 {
				return s[:i], strings.TrimSpace(s[i+1:])
			}
		}
		i++
	}
	return s, ""
}
