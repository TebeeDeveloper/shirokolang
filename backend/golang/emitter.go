package golang

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var goTypeMap = map[string]string{
	"int":    "int",
	"byte":   "byte",
	"float":  "float64",
	"string": "string",
	"bool":   "bool",
	"any":    "any",
	"void":   "",
}

var binopMap = map[string]string{
	"+": "+", "-": "-", "*": "*", "/": "/", "%": "%",
	"==": "==", "!=": "!=", "<": "<", ">": ">", "<=": "<=", ">=": ">=",
	"&&": "&&", "||": "||",
}

var unopMap = map[string]string{"-": "-", "!": "!"}

var convMap = map[string]string{
	"string": "string",
	"int":    "int",
	"byte":   "byte",
	"float":  "float64",
}

var reserved = map[string]bool{
	"type": true, "range": true, "func": true, "map": true, "chan": true,
	"go": true, "defer": true, "select": true, "switch": true, "case": true,
	"default": true, "fallthrough": true, "package": true, "import": true,
	"var": true, "const": true, "return": true, "break": true, "continue": true,
	"goto": true, "if": true, "else": true, "for": true,
}

type GoEmitter struct {
	prog         []any
	buf          []string
	indent       int
	structFields map[string]map[string]any
	methods      map[string]map[string]bool
	freeFns      map[string]bool
}

func New(prog []any) *GoEmitter {
	return &GoEmitter{
		prog:         prog,
		structFields: map[string]map[string]any{},
		methods:      map[string]map[string]bool{},
		freeFns:      map[string]bool{},
	}
}

// ---------- buffer helpers ----------

func (g *GoEmitter) w(line string) {
	g.buf = append(g.buf, strings.Repeat("    ", g.indent)+line)
}

func (g *GoEmitter) push() { g.indent++ }
func (g *GoEmitter) pop()  { g.indent-- }

func (g *GoEmitter) result() string {
	return strings.Join(g.buf, "\n") + "\n"
}

// ---------- entry point ----------

func (g *GoEmitter) Emit() string {
	pkg := g.prog[1].(string)
	decls := g.prog[3].([]any)

	for _, d := range decls {
		dd := d.([]any)
		switch dd[0].(string) {
			case "Struct":
				name := dd[1].(string)
				fields := dd[2].([]any)
				fmap := map[string]any{}
				for _, f := range fields {
					ff := f.([]any)
					fmap[ff[1].(string)] = ff[2]
				}
				g.structFields[name] = fmap
			case "Fn":
				name := dd[1].(string)
				recv := dd[5]
				if recv == nil {
					g.freeFns[name] = true
				} else {
					recvName := recv.(string)
					if g.methods[recvName] == nil {
						g.methods[recvName] = map[string]bool{}
					}
					g.methods[recvName][name] = true
				}
		}
	}

	g.w(fmt.Sprintf("package %s", pkg))
	g.w("")

	if g.usesFmt() {
		g.w("import (")
		g.push()
		g.w("\"fmt\"")
		g.pop()
		g.w(")")
		g.w("")
	}

	// g.emitSetPrelude()

	for _, d := range decls {
		g.emitDecl(d)
		g.w("")
	}

	src := g.result()
	return g.gofmt(src)
}

// usesFmt reports whether the IR contains any call to fmt.X(...).
func (g *GoEmitter) usesFmt() bool {
	for _, d := range g.prog[3].([]any) {
		dd := d.([]any)
		if dd[0].(string) == "Fn" {
			if blockUsesFmt(dd[4]) {
				return true
			}
		}
	}
	return false
}

func blockUsesFmt(block any) bool {
	for _, s := range block.([]any)[1].([]any) {
		if stmtUsesFmt(s) {
			return true
		}
	}
	return false
}

func stmtUsesFmt(s any) bool {
	n := s.([]any)
	switch n[0].(string) {
		case "HLet":
			return exprUsesFmt(n[3])
		case "HAssign":
			return exprUsesFmt(n[1]) || exprUsesFmt(n[2])
		case "HExprStmt":
			return exprUsesFmt(n[1])
		case "HReturn":
			return exprUsesFmt(n[1])
		case "HIf":
			if exprUsesFmt(n[1]) {
				return true
			}
			if blockUsesFmt(n[2]) {
				return true
			}
			if n[3] != nil && blockUsesFmt(n[3]) {
				return true
			}
			return false
		case "HWhile":
			if exprUsesFmt(n[1]) {
				return true
			}
			return blockUsesFmt(n[2])
		case "HSwitch":
			cases := n[1].([]any)
			defaultBody := n[2]
			for _, c := range cases {
				cs := c.([]any)
				if exprUsesFmt(cs[1]) {
					return true
				}
				if blockUsesFmt(cs[2]) {
					return true
				}
			}
			if defaultBody != nil && blockUsesFmt(defaultBody) {
				return true
			}
			return false
	}
	return false
}

func exprUsesFmt(e any) bool {
	if e == nil {
		return false
	}
	n := e.([]any)
	switch n[0].(string) {
		case "HInt", "HFloat", "HByte", "HStr", "HBool", "HIdent":
			return false
		case "HUn":
			return exprUsesFmt(n[2])
		case "HBin":
			return exprUsesFmt(n[2]) || exprUsesFmt(n[3])
		case "HCall":
			if cn, ok := n[1].([]any); ok && cn[0].(string) == "HSel" {
				if bn, ok := cn[1].([]any); ok &&
					bn[0].(string) == "HIdent" && bn[1].(string) == "fmt" {
						return true
					}
			}
			if exprUsesFmt(n[1]) {
				return true
			}
			for _, a := range n[2].([]any) {
				if exprUsesFmt(a) {
					return true
				}
			}
			return false
		case "HSel":
			return exprUsesFmt(n[1])
		case "HIndex":
			return exprUsesFmt(n[1]) || exprUsesFmt(n[2])
		case "HSlice":
			return exprUsesFmt(n[1]) || exprUsesFmt(n[2]) || exprUsesFmt(n[3])
		case "HStructLit":
			for _, f := range n[2].([]any) {
				ff := f.([]any)
				if exprUsesFmt(ff[2]) {
					return true
				}
			}
			return false
		case "HListLit", "HSetLit":
			for _, x := range n[2].([]any) {
				if exprUsesFmt(x) {
					return true
				}
			}
			return false
		case "HSpread":
			return exprUsesFmt(n[1])
	}
	return false
}

func (g *GoEmitter) gofmt(src string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gofmt")
	cmd.Stdin = strings.NewReader(src)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		return out.String()
	}
	return src
}

func (g *GoEmitter) emitSetPrelude() {
	g.w("type __Set[K comparable] map[K]struct{}")
	g.w("")
	g.w("func __setAdd[K comparable](s __Set[K], k K) __Set[K] {")
	g.push()
	g.w("s[k] = struct{}{}")
	g.w("return s")
	g.pop()
	g.w("}")
	g.w("")
	g.w("func __setHas[K comparable](s __Set[K], k K) bool {")
	g.push()
	g.w("_, ok := s[k]")
	g.w("return ok")
	g.pop()
	g.w("}")
	g.w("")
}

// ---------- types ----------

func (g *GoEmitter) goType(t any) string {
	if t == nil {
		return ""
	}
	tt := t.([]any)
	switch tt[0].(string) {
		case "NamedType":
			name := tt[1].(string)
			if mapped, ok := goTypeMap[name]; ok {
				return mapped
			}
			return name
		case "ListType":
			return "[]" + g.goType(tt[1])
		case "SetType":
			return "__Set[" + g.goType(tt[1]) + "]"
	}
	return "any"
}

// ---------- declarations ----------

func (g *GoEmitter) emitDecl(d any) {
	dd := d.([]any)
	switch dd[0].(string) {
		case "Struct":
			g.emitStruct(dd)
		case "Fn":
			g.emitFn(dd)
		case "Interface":
			g.emitInterface(dd)
	}
}

func (g *GoEmitter) emitStruct(d []any) {
	name := d[1].(string)
	fields := d[2].([]any)
	g.w(fmt.Sprintf("type %s struct {", name))
	g.push()
	for _, f := range fields {
		ff := f.([]any)
		g.w(fmt.Sprintf("%s %s", ff[1].(string), g.goType(ff[2])))
	}
	g.pop()
	g.w("}")
}

func (g *GoEmitter) emitInterface(d []any) {
	name := d[1].(string)
	methods := d[2].([]any)
	g.w(fmt.Sprintf("type %s interface {", name))
	g.push()
	for _, m := range methods {
		sig := m.([]any)
		mname := sig[1].(string)
		params := sig[2].([]any)
		ret := sig[3]

		var parts []string
		for _, p := range params {
			pp := p.([]any)
			pname := pp[1].(string)
			pty := pp[2]
			variadic := pp[3].(bool)
			t := g.goType(pty)
			if variadic {
				t = "..." + t
			}
			parts = append(parts, fmt.Sprintf("%s %s", pname, t))
		}
		retGo := ""
		if ret != nil {
			retGo = g.goType(ret)
		}
		line := fmt.Sprintf("%s(%s) %s", mname, strings.Join(parts, ", "), retGo)
		g.w(strings.TrimRight(line, " "))
	}
	g.pop()
	g.w("}")
}

func (g *GoEmitter) emitFn(d []any) {
	name := d[1].(string)
	params := d[2].([]any)
	ret := d[3]
	body := d[4]
	recv := d[5]

	var parts []string
	for _, p := range params {
		pp := p.([]any)
		pname := pp[1].(string)
		pty := pp[2]
		variadic := pp[3].(bool)
		if recv != nil && pname == "self" {
			continue
		}
		t := g.goType(pty)
		if variadic {
			t = "..." + t
		}
		parts = append(parts, fmt.Sprintf("%s %s", pname, t))
	}

	retGo := ""
	if ret != nil {
		retGo = g.goType(ret)
	}

	recvPrefix := ""
	if recv != nil {
		recvPrefix = fmt.Sprintf("(self *%s) ", recv.(string))
	}

	head := fmt.Sprintf("func %s%s(%s)", recvPrefix, name, strings.Join(parts, ", "))
	if retGo != "" {
		head += " " + retGo
	}
	g.w(head + " {")
	g.push()
	g.emitBlockStmts(body)
	g.pop()
	g.w("}")
}

// ---------- statements ----------

func (g *GoEmitter) emitBlockStmts(block any) {
	stmts := block.([]any)[1].([]any)
	for i, s := range stmts {
		g.emitStmt(s, stmts, i)
	}
}

func (g *GoEmitter) emitStmt(s any, siblings []any, idx int) {
	n := s.([]any)
	switch n[0].(string) {
		case "HLet":
			name := n[1].(string)
			ty := n[2]
			expr := n[3]
			if ty != nil {
				g.w(fmt.Sprintf("var %s %s", name, g.goType(ty)))
				if expr != nil {
					g.w(fmt.Sprintf("%s = %s", name, g.emitExpr(expr)))
				}
			} else {
				g.w(fmt.Sprintf("%s := %s", name, g.emitExpr(expr)))
			}
			if !strings.HasPrefix(name, "__") && !g.usedAfter(name, siblings, idx) {
				g.w(fmt.Sprintf("_ = %s", name))
			}

		case "HAssign":
			tgt := n[1]
			expr := n[2]
			g.w(fmt.Sprintf("%s = %s", g.emitExpr(tgt), g.emitExpr(expr)))

		case "HExprStmt":
			g.w(g.emitExpr(n[1]))

		case "HReturn":
			expr := n[1]
			if expr == nil {
				g.w("return")
			} else {
				g.w(fmt.Sprintf("return %s", g.emitExpr(expr)))
			}

		case "HIf":
			g.emitIf(n)

		case "HWhile":
			cond := n[1]
			body := n[2]
			g.w(fmt.Sprintf("for %s {", g.emitExpr(cond)))
			g.push()
			g.emitBlockStmts(body)
			g.pop()
			g.w("}")

		case "HSwitch":
			cases := n[1].([]any)
			defaultBody := n[2]
			g.w("switch {")
			g.push()
			for _, c := range cases {
				cs := c.([]any)
				cond := cs[1]
				body := cs[2]
				g.w(fmt.Sprintf("case %s:", g.emitExpr(cond)))
				g.push()
				g.emitBlockStmts(body)
				g.pop()
			}
			if defaultBody != nil {
				g.w("default:")
				g.push()
				g.emitBlockStmts(defaultBody)
				g.pop()
			}
			g.pop()
			g.w("}")

		case "HBreak":
			g.w("break")

		case "HContinue":
			g.w("continue")

		default:
			panic(fmt.Sprintf("emit_stmt: %s", n[0].(string)))
	}
}

func (g *GoEmitter) emitIf(s []any) {
	cond := s[1]
	thenB := s[2]
	elseB := s[3]

		g.w(fmt.Sprintf("if %s {", g.emitExpr(cond)))
		g.push()
		g.emitBlockStmts(thenB)
		g.pop()

		for elseB != nil {
			eb := elseB.([]any)
			stmts := eb[1].([]any)
			if len(stmts) == 1 && stmts[0].([]any)[0].(string) == "HIf" {
				inner := stmts[0].([]any)
				iCond := inner[1]
				iThen := inner[2]
				iElse := inner[3]
				g.w(fmt.Sprintf("} else if %s {", g.emitExpr(iCond)))
				g.push()
				g.emitBlockStmts(iThen)
				g.pop()
				elseB = iElse
					continue
			}
			g.w("} else {")
			g.push()
			g.emitBlockStmts(elseB)
			g.pop()
			break
		}
		g.w("}")
}

// ---------- "is this local used later in the block?" ----------

func (g *GoEmitter) usedAfter(name string, stmts []any, idx int) bool {
	for j := idx + 1; j < len(stmts); j++ {
		if g.stmtMentions(stmts[j], name) {
			return true
		}
	}
	return false
}

func (g *GoEmitter) stmtMentions(s any, name string) bool {
	n := s.([]any)
	switch n[0].(string) {
		case "HLet":
			return n[3] != nil && g.exprMentions(n[3], name)
		case "HAssign":
			return g.exprMentions(n[1], name) || g.exprMentions(n[2], name)
		case "HExprStmt":
			return g.exprMentions(n[1], name)
		case "HReturn":
			return n[1] != nil && g.exprMentions(n[1], name)
		case "HIf":
			c := n[1]
			t := n[2]
			e := n[3]
			if g.exprMentions(c, name) {
				return true
			}
			for _, x := range t.([]any)[1].([]any) {
				if g.stmtMentions(x, name) {
					return true
				}
			}
			if e != nil {
				for _, x := range e.([]any)[1].([]any) {
					if g.stmtMentions(x, name) {
						return true
					}
				}
			}
			return false
		case "HWhile":
			c := n[1]
			b := n[2]
			if g.exprMentions(c, name) {
				return true
			}
			for _, x := range b.([]any)[1].([]any) {
				if g.stmtMentions(x, name) {
					return true
				}
			}
			return false

		case "HSwitch":
			cases := n[1].([]any)
			defaultBody := n[2]
			for _, c := range cases {
				cs := c.([]any)
				if g.exprMentions(cs[1], name) {
					return true
				}
				for _, x := range cs[2].([]any)[1].([]any) {
					if g.stmtMentions(x, name) {
						return true
					}
				}
			}
			if defaultBody != nil {
				for _, x := range defaultBody.([]any)[1].([]any) {
					if g.stmtMentions(x, name) {
						return true
					}
				}
			}
			return false
	}
	return false
}

func (g *GoEmitter) exprMentions(e any, name string) bool {
	if e == nil {
		return false
	}
	n := e.([]any)
	switch n[0].(string) {
		case "HIdent":
			return n[1].(string) == name
		case "HInt", "HFloat", "HByte", "HStr", "HBool":
			return false
		case "HUn":
			return g.exprMentions(n[2], name)
		case "HBin":
			return g.exprMentions(n[2], name) || g.exprMentions(n[3], name)
		case "HCall":
			if g.exprMentions(n[1], name) {
				return true
			}
			for _, a := range n[2].([]any) {
				if g.exprMentions(a, name) {
					return true
				}
			}
			return false
		case "HSel":
			return g.exprMentions(n[1], name)
		case "HIndex":
			return g.exprMentions(n[1], name) || g.exprMentions(n[2], name)
		case "HSlice":
			return g.exprMentions(n[1], name) ||
			g.exprMentions(n[2], name) ||
			g.exprMentions(n[3], name)
		case "HStructLit":
			for _, f := range n[2].([]any) {
				ff := f.([]any)
				if g.exprMentions(ff[2], name) {
					return true
				}
			}
			return false
		case "HListLit", "HSetLit":
			for _, x := range n[2].([]any) {
				if g.exprMentions(x, name) {
					return true
				}
			}
			return false
		case "HSpread":
			return g.exprMentions(n[1], name)
	}
	return false
}

// ---------- expressions ----------

func (g *GoEmitter) emitExpr(e any) string {
	n := e.([]any)
	switch n[0].(string) {
		case "HInt":
			return fmt.Sprintf("%v", n[1])
		case "HFloat":
			return formatFloat(n[1].(float64))
		case "HByte":
			return fmt.Sprintf("byte(%v)", n[1])
		case "HStr":
			return "\"" + escapeStr(n[1].(string)) + "\""
		case "HBool":
			if n[1].(bool) {
				return "true"
			}
			return "false"
		case "HIdent":
			return mangleIdent(n[1].(string))

		case "HUn":
			op := n[1].(string)
			return unopMap[op] + g.emitExpr(n[2])

		case "HBin":
			op := n[1].(string)
			return fmt.Sprintf("%s %s %s",
					   g.emitExpr(n[2]), binopMap[op], g.emitExpr(n[3]))

		case "HCall":
			return g.emitCall(n[1], n[2].([]any))

		case "HSel":
			return fmt.Sprintf("%s.%s", g.emitExpr(n[1]), n[2].(string))

		case "HIndex":
			return fmt.Sprintf("%s[%s]", g.emitExpr(n[1]), g.emitExpr(n[2]))

		case "HSlice":
			loS, hiS := "", ""
			if n[2] != nil {
				loS = g.emitExpr(n[2])
			}
			if n[3] != nil {
				hiS = g.emitExpr(n[3])
			}
			return fmt.Sprintf("%s[%s:%s]", g.emitExpr(n[1]), loS, hiS)

		case "HStructLit":
			name := n[1].(string)
			var parts []string
			for _, f := range n[2].([]any) {
				ff := f.([]any)
				parts = append(parts, fmt.Sprintf("%s: %s",
								  ff[1].(string), g.emitExpr(ff[2])))
			}
			return fmt.Sprintf("%s{%s}", name, strings.Join(parts, ", "))

		case "HListLit":
			var parts []string
			for _, x := range n[2].([]any) {
				parts = append(parts, g.emitExpr(x))
			}
			return fmt.Sprintf("%s{%s}", g.goType(n[1]), strings.Join(parts, ", "))

		case "HSetLit":
			elems := n[2].([]any)
			if len(elems) == 0 {
				return fmt.Sprintf("make(%s)", g.goType(n[1]))
			}
			var parts []string
			for _, x := range elems {
				parts = append(parts, fmt.Sprintf("%s: struct{}{}", g.emitExpr(x)))
			}
			return fmt.Sprintf("%s{%s}", g.goType(n[1]), strings.Join(parts, ", "))

		case "HSpread":
			panic("HSpread outside a call")

		case "HFnLit":
			return g.emitFnLit(n)
	}
	panic(fmt.Sprintf("emit_expr: %s", n[0].(string)))
}

func (g *GoEmitter) emitCall(fn any, args []any) string {
	fnN := fn.([]any)

	// conversions: string(x), int(x), byte(x), float(x)
	if fnN[0].(string) == "HIdent" {
		name := fnN[1].(string)
		if conv, ok := convMap[name]; ok {
			return fmt.Sprintf("%s(%s)", conv, g.emitExpr(args[0]))
		}
		switch name {
			case "len":
				return fmt.Sprintf("len(%s)", g.emitExpr(args[0]))
			case "append":
				return fmt.Sprintf("append(%s, %s)",
						   g.emitExpr(args[0]), g.emitExpr(args[1]))
		}
	}

	// fmt.println / fmt.print / fmt.printf
	if fnN[0].(string) == "HSel" {
		base := fnN[1].([]any)
		if base[0].(string) == "HIdent" && base[1].(string) == "fmt" {
			method := fnN[2].(string)
			goName := method
			switch method {
				case "println":
					goName = "Println"
				case "printf":
					goName = "Printf"
				case "print":
					goName = "Print"
			}
			var parts []string
			for _, a := range args {
				an := a.([]any)
				if an[0].(string) == "HSpread" {
					parts = append(parts, g.emitExpr(an[1])+"...")
				} else {
					parts = append(parts, g.emitExpr(a))
				}
			}
			return fmt.Sprintf("fmt.%s(%s)", goName, strings.Join(parts, ", "))
		}
	}

	callee := g.emitExpr(fn)
	var parts []string
	for _, a := range args {
		an := a.([]any)
		if an[0].(string) == "HSpread" {
			parts = append(parts, g.emitExpr(an[1])+"...")
		} else {
			parts = append(parts, g.emitExpr(a))
		}
	}
	return fmt.Sprintf("%s(%s)", callee, strings.Join(parts, ", "))
}

func (g *GoEmitter) emitFnLit(n []any) string {
	params := n[1].([]any)
	ret := n[2]
	body := n[3]

	var parts []string
	for _, p := range params {
		pp := p.([]any)
		pname := pp[1].(string)
		pty := pp[2]
		variadic := pp[3].(bool)
		t := g.goType(pty)
		if variadic {
			t = "..." + t
		}
		parts = append(parts, fmt.Sprintf("%s %s", pname, t))
	}

	retGo := ""
	if ret != nil {
		retGo = g.goType(ret)
	}

	head := fmt.Sprintf("func(%s)", strings.Join(parts, ", "))
	if retGo != "" {
		head += " " + retGo
	}

	// Render body into a sub-buffer.
	savedBuf := g.buf
	savedIndent := g.indent
	g.buf = nil
	g.indent++
	g.emitBlockStmts(body)
	bodyLines := g.buf
	g.buf = savedBuf
	g.indent = savedIndent

	if len(bodyLines) == 0 {
		return head + " {}"
	}
	inner := strings.Join(bodyLines, "\n")
	clos := strings.Repeat("    ", g.indent) + "}"
	return head + " {\n" + inner + "\n" + clos
}

// ---------- helpers ----------

func mangleIdent(name string) string {
	if reserved[name] {
		return name + "_"
	}
	return name
}

func escapeStr(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
			case '\\':
				b.WriteString("\\\\")
			case '"':
				b.WriteString("\\\"")
			case '\n':
				b.WriteString("\\n")
			case '\t':
				b.WriteString("\\t")
			case '\r':
				b.WriteString("\\r")
			default:
				b.WriteRune(r)
		}
	}
	return b.String()
}

func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

func EmitGo(prog []any) string {
	return New(prog).Emit()
}
