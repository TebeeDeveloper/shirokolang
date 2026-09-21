package tokenize

type Token struct {
	Type string
	Value string
	Message string
}

func isAlnum(ch byte) bool {
	return isAlpha(ch) || isNum(ch)
}

func isAlpha(ch byte) bool {
	return ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || (ch == '_')
}

func isNum(ch byte) bool {
	return ('0' <= ch && ch <= '9') || ch == '.'
}
