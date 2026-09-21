package tokenize

type Tokenizer struct {
	Code string
	Size int
	Pos int
	Line int
	Col int
}

func Init(code string) Tokenizer {
	return Tokenizer{
		Code: code,
		Size: len(code),
		Pos: 0,
		Line: 1,
		Col: 1,
	}
}

func (t *Tokenizer) NextToken() Token {
	if t.Pos >= t.Size {
		return Token{
			Type: "EOF",
			Value: "",
			Message: "",
		}
	}

	var skipspace bool = true
	for t.Pos < t.Size && skipspace {
		switch t.Code[t.Pos] {
			case ' ':
				t.Pos++
				t.Col++
			case '\t':
				t.Pos++
				t.Col = t.Col + ((t.Col - 1) & 3)
			case '\n':
				t.Pos++
				t.Line++
				t.Col = 1
				return Token{
					Type: "NewLine",
					Value: "NewLine",
				}
			default:
				skipspace = false
		}
	}

	if t.Pos >= t.Size {
		return Token{
			Type: "EOF",
			Value: "",
			Message: "",
		}
	}

	var ch byte = t.Code[t.Pos]
	if ch == '"' {
		t.Pos++
		t.Col++
		
		var start int = t.Pos
		
		for t.Pos < t.Size {
			if t.Code[t.Pos] == '"' && t.Code[t.Pos - 1] != '\\' {
				break
			}
			if t.Code[t.Pos] == '\n' {
				t.Pos++
				t.Line++
				t.Col = 0
				continue
			}
			t.Pos++
			t.Col++
		}
		var s string = t.Code[start : t.Pos]
		if t.Pos < t.Size {
			t.Pos++
		} else {
			return Token{
				Type: "ERR",
				Message: "String was not closed by \"\"\"",
			}
		}
		return Token{
			Type: "String",
			Value: s,
			Message: "",
		}
	}
	if t.Pos + 1 < t.Size {
		var s string = t.Code[t.Pos : t.Pos + 2]
		switch s {
		case "==", "!=", "<=", ">=":
			t.Pos += 2
			t.Col += 2
			return Token{
				Type: "OP",
				Value: s,
			}
		}
	}

	if t.Pos < t.Size {
		var c byte = t.Code[t.Pos]
		switch c {
		case '!', '+', '-', '*', '/', '{', '}', '[', ']', '(', ')', '=', '|', ';', '@', ',', '<', '>':
			t.Pos++
			t.Col++
			return Token{
				Type: "Char",
				Value: string(c),
			}
		case '\\':
			t.Pos++
			t.Col++
			return Token{
				Type: "ListOp",
				Value: string(c),
			}
		}
	}

	if isAlnum(ch) {
		if isNum(ch) {
			var start int = t.Pos
			for isNum(t.Code[t.Pos]) {
				t.Pos++
				t.Col++
			}
			if t.Pos - start > 7 {
				return Token{
					Type: "ERR",
					Value: "",
					Message: "Number overflow",
				}
			}
			return Token{
				Type: "Number",
				Value: t.Code[start : t.Pos],
			}
		}
		if isAlpha(ch) {
			var start int = t.Pos
			for isAlpha(t.Code[t.Pos]) {
				t.Pos++
				t.Col++
			}
			var s string = t.Code[start : t.Pos]
			
			switch s {
			case "var", "list", "contract", "for", "if", "else", "fn", "V", "E":
				return Token{
					Type: "KW",
					Value: s,
				}
			case "int", "double", "string", "ListInt", "ListDouble":
				return Token{
					Type: "Type",
					Value: s,
				}
			case "c", "u", "n":
				return Token{
					Type: "ListOp",
					Value: s,
				}
			default:
				return Token{
					Type: "ID",
					Value: s,
				}
			}
		}
	}
	t.Pos++
	t.Col++
	return Token{
		Type: "ERR",
		Value: string(ch),
		Message: "Unknown Charactor",
	}
}
