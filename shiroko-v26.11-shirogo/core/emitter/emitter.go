package emitter

import (
	"fmt"
	"strconv"
	"strings"

	"shiroko/core/color"
	"shiroko/core/source"
)

// ---------- errors ----------

type CodegenError struct {
	Code string
	Msg  string
	Line int
	Col  int
	Src  *source.Source
}

func (e *CodegenError) String() string {
	return fmt.Sprintf("%s %s %s",
			   color.Magenta("golang error"),
			   color.Dim("=>"),
			   e.Msg,
	)
}

func (e *CodegenError) Error() string { return e.String() }

// ---------- Writer ----------

type Writer struct {
	sb     strings.Builder
	indent int
}

func NewWriter() *Writer { return &Writer{} }

func (w *Writer) Line(parts ...string) {
	w.tabs()
	for _, p := range parts {
		w.sb.WriteString(p)
	}
	w.sb.WriteByte('\n')
}

func (w *Writer) LineNoIndent(parts ...string) {
	for _, p := range parts {
		w.sb.WriteString(p)
	}
	w.sb.WriteByte('\n')
}

func (w *Writer) Raw(s string) { w.sb.WriteString(s) }

func (w *Writer) Indented(fn func()) {
	w.indent++
	fn()
	w.indent--
}

func (w *Writer) Block(header string, body func()) {
	w.Line(header)
	w.Indented(body)
	w.Line("}")
}

func (w *Writer) tabs() {
	for i := 0; i < w.indent; i++ {
		w.sb.WriteByte('\t')
	}
}

func (w *Writer) String() string { return w.sb.String() }

// ---------- stdlib table ----------

type stdlibEntry struct {
	path    string
	members map[string]string
}

var stdlib = map[string]stdlibEntry{
	"fmt": {
		path: "fmt",
		members: map[string]string{
			"println":  "Println",
			"printf":   "Printf",
			"print":    "Print",
			"sprintf":  "Sprintf",
			"sprintln": "Sprintln",
			"errorf":   "Errorf",
		},
	},
	"strings": {
		path: "strings",
		members: map[string]string{
			"contains":   "Contains",
			"has_prefix": "HasPrefix",
			"has_suffix": "HasSuffix",
			"split":      "Split",
			"join":       "Join",
			"trim":       "TrimSpace",
			"to_upper":   "ToUpper",
			"to_lower":   "ToLower",
			"replace":    "Replace",
			"repeat":     "Repeat",
			"index":      "Index",
		},
	},
	"strconv": {
		path: "strconv",
		members: map[string]string{
			"atoi":         "Atoi",
			"itoa":         "Itoa",
			"parse_int":    "ParseInt",
			"parse_float":  "ParseFloat",
			"format_int":   "FormatInt",
			"format_float": "FormatFloat",
			"quote":        "Quote",
		},
	},
	"os": {
		path: "os",
		members: map[string]string{
			"exit":   "Exit",
			"args":   "Args",
			"getenv": "Getenv",
			"setenv": "Setenv",
		},
	},
	"math": {
		path: "math",
		members: map[string]string{
			"sqrt":  "Sqrt",
			"abs":   "Abs",
			"pow":   "Pow",
			"floor": "Floor",
			"ceil":  "Ceil",
		},
	},
	"sort": {
		path: "sort",
		members: map[string]string{
			"ints":    "Ints",
			"strings": "Strings",
			"slice":   "Slice",
		},
	},
}

// ---------- emitter ----------

type GoEmitter struct {
	Errors []*CodegenError
	w      *Writer
	src    *source.Source

	needsRange bool

	pkgs map[string]stdlibEntry

	fnRetTy    any
	fnErrTy    any
	fnRetSlots int
	errVar     string
	errRename  string
	errCounter int
}

func New(src *source.Source) *GoEmitter {
	return &GoEmitter{
		w:    NewWriter(),
		src:  src,
		pkgs: map[string]stdlibEntry{},
	}
}

func Generate(prog []any, src *source.Source) (string, []*CodegenError) {
	return New(src).Generate(prog)
}

func (e *GoEmitter) err(format string, args ...any) {
	e.Errors = append(e.Errors, &CodegenError{Msg: fmt.Sprintf(format, args...)})
}

func lastSeg(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// ---------- constant folding ----------

func constInt(node any) (int, bool) {
	n, ok := node.([]any)
	if !ok || len(n) == 0 {
		return 0, false
	}
	switch n[0].(string) {
		case "HInt":
			v, ok := n[1].(int)
			return v, ok
		case "HByte":
			v, ok := n[1].(int)
			return v, ok
		case "HUn":
			if n[1].(string) == "-" {
				if v, ok := constInt(n[2]); ok {
					return -v, true
				}
			}
	}
	return 0, false
}

func rangeLit(lo, hi int) string {
	if hi < lo {
		return "[]int{}"
	}
	var parts []string
	for i := lo; i <= hi; i++ {
		parts = append(parts, strconv.Itoa(i))
	}
	return "[]int{" + strings.Join(parts, ", ") + "}"
}

// ---------- types ----------

var goBuiltins = map[string]string{
	"int":    "int",
	"float":  "float64",
	"string": "string",
	"byte":   "byte",
	"bool":   "bool",
	"any":    "any",
	"error":  "error",
}

func (e *GoEmitter) goType(node any) string {
	if node == nil {
		return ""
	}
	n, ok := node.([]any)
	if !ok || len(n) == 0 {
		return "any"
	}
	switch n[0].(string) {
		case "NamedType":
			name := n[1].(string)
			if g, ok := goBuiltins[name]; ok {
				return g
			}
			return name
		case "ListType":
			return "[]" + e.goType(n[1])
		case "SetType":
			return "[]" + e.goType(n[1])
		case "MapType":
			return "map[" + e.goType(n[1]) + "]" + e.goType(n[2])
		case "PtrType":
			return "*" + e.goType(n[1])
		case "TupleType":
			var parts []string
			for _, el := range n[1].([]any) {
				parts = append(parts, e.goType(el))
			}
			if len(parts) == 1 {
				return parts[0]
			}
			return "(" + strings.Join(parts, ", ") + ")"
	}
	e.err("unknown type node %q", n[0])
	return "any"
}

func returnSlotCount(retNode any) int {
	if retNode == nil {
		return 0
	}
	n, ok := retNode.([]any)
	if !ok || len(n) == 0 {
		return 1
	}
	if n[0].(string) == "TupleType" {
		return len(n[1].([]any))
	}
	return 1
}

func (e *GoEmitter) zeroValue(retNode any) string {
	if retNode == nil {
		return ""
	}
	n, ok := retNode.([]any)
	if !ok || len(n) == 0 {
		return "nil"
	}
	switch n[0].(string) {
		case "NamedType":
			switch n[1].(string) {
				case "int", "byte":
					return "0"
				case "float":
					return "0"
				case "string":
					return `""`
				case "bool":
					return "false"
				case "any":
					return "nil"
			}
			return "nil"
		case "ListType", "SetType", "MapType", "PtrType":
			return "nil"
	}
	return "nil"
}

func (e *GoEmitter) zeroValues() []string {
	if e.fnRetTy == nil {
		return nil
	}
	n, ok := e.fnRetTy.([]any)
	if !ok || len(n) == 0 {
		return []string{"nil"}
	}
	if n[0].(string) == "TupleType" {
		var out []string
		for _, el := range n[1].([]any) {
			out = append(out, e.zeroValue(el))
		}
		return out
	}
	return []string{e.zeroValue(e.fnRetTy)}
}

// ---------- program ----------

func (e *GoEmitter) Generate(prog []any) (string, []*CodegenError) {
	pkg := "main"
	if len(prog) > 1 {
		if p, ok := prog[1].(string); ok && p != "" && p != "<error>" {
			pkg = p
		}
	}
	var imports []string
	if len(prog) > 2 {
		if im, ok := prog[2].([]string); ok {
			imports = im
		}
	}
	var decls []any
	if len(prog) > 3 {
		if d, ok := prog[3].([]any); ok {
			decls = d
		}
	}

	goImports := e.registerImports(imports)

	bodyW := NewWriter()
	saved := e.w
	e.w = bodyW
	for _, d := range decls {
		e.emitDecl(d)
	}
	e.w = saved

	e.w.Line("package ", pkg)
	e.w.Line("")
	e.writeImports(goImports)

	if e.needsRange {
		e.emitRangeHelper()
	}

	e.w.Raw(bodyW.String())
	return e.w.String(), e.Errors
}

func (e *GoEmitter) registerImports(imports []string) []string {
	seen := map[string]bool{}
	var clean []string

	for _, im := range imports {
		if im == "" {
			continue
		}
		key := strings.TrimPrefix(im, "std/")
		entry, ok := stdlib[key]
		if !ok {
			if strings.HasPrefix(im, "std/") {
				e.err("unknown stdlib package %q", im)
			}
			continue
		}
		if !seen[entry.path] {
			seen[entry.path] = true
			clean = append(clean, entry.path)
		}
		e.pkgs[lastSeg(key)] = entry
	}
	return clean
}

func (e *GoEmitter) writeImports(clean []string) {
	if len(clean) == 0 {
		return
	}
	if len(clean) == 1 {
		e.w.Line(`import "`, clean[0], `"`)
	} else {
		e.w.Line("import (")
		e.w.Indented(func() {
			for _, im := range clean {
				e.w.Line(`"`, im, `"`)
			}
		})
		e.w.Line(")")
	}
	e.w.Line("")
}

func (e *GoEmitter) emitRangeHelper() {
	e.w.Block("func __range(lo, hi int) []int {", func() {
	e.w.Block("if hi < lo {", func() {
	e.w.Line("return []int{}")
	})
	e.w.Line("out := make([]int, 0, hi-lo+1)")
	e.w.Block("for i := lo; i <= hi; i++ {", func() {
	e.w.Line("out = append(out, i)")
	})
	e.w.Line("return out")
	})
	e.w.Line("")
}

// ---------- declarations ----------

func (e *GoEmitter) emitDecl(d any) {
	n, ok := d.([]any)
	if !ok || len(n) == 0 {
		return
	}
	switch n[0].(string) {
		case "Struct":
			e.emitStruct(n)
		case "Interface":
			e.emitInterface(n)
		case "Fn":
			e.emitFn(n)
		case "GlobalLet":
			e.emitGlobalLet(n)
		default:
			e.err("unknown decl %q", n[0])
	}
}

func (e *GoEmitter) emitStruct(n []any) {
	name := n[1].(string)
	fields := n[2].([]any)
	e.w.Block("type "+name+" struct {", func() {
	for _, f := range fields {
		ff := f.([]any)
		e.w.Line(ff[1].(string), " ", e.goType(ff[2]))
	}
	})
	e.w.Line("")
}

func (e *GoEmitter) emitInterface(n []any) {
	name := n[1].(string)
	methods := n[2].([]any)
	e.w.Block("type "+name+" interface {", func() {
	for _, m := range methods {
		mm := m.([]any)
		mname := mm[1].(string)
		params := mm[2].([]any)
		retNode := mm[3]
		var errNode any
		if len(mm) > 4 {
			errNode = mm[4]
		}
		var ps []string
		for _, p := range params {
			pp := p.([]any)
			ps = append(ps, pp[1].(string)+" "+e.goType(pp[2]))
		}
		sig := mname + "(" + strings.Join(ps, ", ") + ")"
		switch {
			case retNode != nil && errNode != nil:
				sig += " (" + e.goType(retNode) + ", error)"
			case retNode != nil:
				sig += " " + e.goType(retNode)
			case errNode != nil:
				sig += " error"
		}
		e.w.Line(sig)
	}
	})
	e.w.Line("")
}

func (e *GoEmitter) emitGlobalLet(n []any) {
	names := n[1].([]string)
	tyNode := n[2]
	expr := n[3]

	if len(names) > 1 {
		if expr == nil {
			e.err("GlobalLet with multiple names needs an initializer")
			return
		}
		e.w.Line(strings.Join(names, ", "), " = ", e.emitExpr(expr))
		return
	}
	name := names[0]
	switch {
		case expr == nil && tyNode != nil:
			e.w.Line("var ", name, " ", e.goType(tyNode))
		case expr != nil && tyNode == nil:
			e.w.Line("var ", name, " = ", e.emitExpr(expr))
		case expr != nil && tyNode != nil:
			e.w.Line("var ", name, " ", e.goType(tyNode), " = ", e.emitExpr(expr))
		default:
			e.err("GlobalLet %q: no type and no initializer", name)
	}
	e.w.Line("")
}

func (e *GoEmitter) emitFn(n []any) {
	name := n[1].(string)
	params := n[2].([]any)
	retNode := n[3]
	body := n[4]
	recv := n[5]
	var errNode any
	if len(n) > 6 {
		errNode = n[6]
	}

	var header strings.Builder
	header.WriteString("func ")
	if recv != nil {
		recvName, _ := recv.(string)
		header.WriteString("(self *")
		header.WriteString(recvName)
		header.WriteString(") ")
		if len(params) > 0 {
			params = params[1:]
		}
	}
	header.WriteString(name)
	header.WriteString("(")
	var ps []string
	for _, p := range params {
		pp := p.([]any)
		pname := pp[1].(string)
		variadic := false
		if v, ok := pp[3].(bool); ok {
			variadic = v
		}
		pty := e.goType(pp[2])
		if variadic {
			pty = "..." + pty
		}
		ps = append(ps, pname+" "+pty)
	}
	header.WriteString(strings.Join(ps, ", "))
	header.WriteString(")")

	switch {
		case retNode != nil && errNode != nil:
			header.WriteString(" (")
			header.WriteString(e.goType(retNode))
			header.WriteString(", error)")
		case retNode != nil:
			header.WriteString(" ")
			header.WriteString(e.goType(retNode))
		case errNode != nil:
			header.WriteString(" error")
	}
	header.WriteString(" {")

	savedRet := e.fnRetTy
	savedErr := e.fnErrTy
	savedErrVar := e.errVar
	savedRename := e.errRename
	savedSlots := e.fnRetSlots
	e.fnRetTy = retNode
	e.fnErrTy = errNode
	e.errVar = ""
	e.errRename = ""
	e.fnRetSlots = returnSlotCount(retNode)

	e.w.Block(header.String(), func() {
		e.emitBlockBody(body)
	})
	e.w.Line("")

	e.fnRetTy = savedRet
	e.fnErrTy = savedErr
	e.errVar = savedErrVar
	e.errRename = savedRename
	e.fnRetSlots = savedSlots
}

// ---------- statements ----------

func (e *GoEmitter) emitBlockBody(block any) {
	if block == nil {
		return
	}
	n, ok := block.([]any)
	if !ok || n[0].(string) != "HBlock" {
		e.err("expected HBlock, got %v", block)
		return
	}
	for _, s := range n[1].([]any) {
		e.emitStmt(s)
	}
}

func (e *GoEmitter) inlineStmt(s any) string {
	scratch := NewWriter()
	saved := e.w
	e.w = scratch
	e.emitStmt(s)
	e.w = saved
	return strings.TrimRight(scratch.String(), "\n")
}

func (e *GoEmitter) emitStmt(s any) {
	n, ok := s.([]any)
	if !ok || len(n) == 0 {
		return
	}
	switch n[0].(string) {
		case "HBlock":
			e.w.Block("{", func() {
			e.emitBlockBody(s)
			})

		case "HLet":
			e.emitLet(n)

		case "HErrLet":
			e.emitErrLet(n)

		case "HMultiLet":
			names := n[1].([]string)
			expr := n[2]
			var lhs []string
			allBlank := true
			for _, nm := range names {
				lhs = append(lhs, nm)
				if nm != "_" {
					allBlank = false
				}
			}
			if allBlank {
				e.w.Line("_ = ", e.emitExpr(expr))
				return
			}
			e.w.Line(strings.Join(lhs, ", "), " := ", e.emitExpr(expr))

		case "HAssign":
			e.w.Line(e.emitExpr(n[1]), " = ", e.emitExpr(n[2]))

		case "HExprStmt":
			e.w.Line(e.emitExpr(n[1]))

		case "HReturn":
			e.emitReturn(n)

		case "HIf":
			e.emitIfChain(n, true)

		case "HWhile":
			e.w.Block("for "+e.emitExpr(n[1])+" {", func() {
			e.emitBlockBody(n[2])
			})

		case "HFor":
			e.emitFor(n)

		case "HSwitch":
			e.emitSwitch(n)

		case "HBreak":
			e.w.Line("break")
		case "HContinue":
			e.w.Line("continue")

		default:
			e.err("unknown statement %q", n[0])
	}
}

func (e *GoEmitter) emitLet(n []any) {
	name := n[1].(string)
	tyNode := n[2]
	expr := n[3]

	if name == "_" {
		if expr != nil {
			e.w.Line("_ = ", e.emitExpr(expr))
		}
		return
	}
	switch {
		case expr == nil && tyNode != nil:
			e.w.Line("var ", name, " ", e.goType(tyNode))
		case expr != nil && tyNode == nil:
			if isHNil(expr) {
				e.w.Line("var ", name, " any = nil")
			} else {
				e.w.Line(name, " := ", e.emitExpr(expr))
			}
		case expr != nil && tyNode != nil:
			e.w.Line("var ", name, " ", e.goType(tyNode), " = ", e.emitExpr(expr))
		default:
			e.err("HLet %q: no type and no initializer", name)
	}
}

func (e *GoEmitter) emitErrLet(n []any) {
	name := n[1].(string)
	expr := n[3]
	els := n[4].([]any)

	e.errCounter++
	errVar := fmt.Sprintf("__err%d", e.errCounter)

	e.w.Line(name, ", ", errVar, " := ", e.emitExpr(expr))
	e.w.Block("if "+errVar+" != nil {", func() {
	savedEV := e.errVar
	savedRename := e.errRename
	e.errVar = errVar
	e.errRename = errVar
	e.emitBlockBody(els)
	e.errVar = savedEV
	e.errRename = savedRename
	})
}

func (e *GoEmitter) emitReturn(n []any) {
	exprs := n[1].([]any)

	if len(exprs) == 0 {
		switch {
			case e.fnErrTy != nil && e.errVar != "":
				zs := e.zeroValues()
				parts := append(append([]string{}, zs...), e.errVar)
				e.w.Line("return ", strings.Join(parts, ", "))
			case e.fnErrTy != nil:
				zs := e.zeroValues()
				if len(zs) == 0 {
					e.w.Line("return nil")
				} else {
					parts := append(append([]string{}, zs...), "nil")
					e.w.Line("return ", strings.Join(parts, ", "))
				}
			default:
				e.w.Line("return")
		}
		return
	}

	var parts []string
	for _, ex := range exprs {
		parts = append(parts, e.emitExpr(ex))
	}

	if e.fnErrTy != nil {
		if len(exprs) == e.fnRetSlots+1 {
			e.w.Line("return ", strings.Join(parts, ", "))
			return
		}
		parts = append(parts, "nil")
	}
	e.w.Line("return ", strings.Join(parts, ", "))
}

func (e *GoEmitter) emitIfChain(n []any, firstLine bool) {
	cond := e.emitExpr(n[1])
	thenBlock := n[2].([]any)
	els := n[3]

	header := "if " + cond + " {"
	if firstLine {
		e.w.Line(header)
	} else {
		e.w.LineNoIndent(header)
	}
	e.w.Indented(func() { e.emitBlockBody(thenBlock) })

	if els == nil {
		e.w.Line("}")
		return
	}

	if elsN, ok := els.([]any); ok && elsN[0].(string) == "HBlock" {
		inner := elsN[1].([]any)
		if len(inner) == 1 {
			first := inner[0].([]any)
			if first[0].(string) == "HIf" {
				e.w.Raw(strings.Repeat("\t", e.w.indent))
				e.w.Raw("} else ")
				e.emitIfChain(first, false)
				return
			}
		}
	}

	e.w.Line("} else {")
	e.w.Indented(func() { e.emitBlockBody(els) })
	e.w.Line("}")
}

func (e *GoEmitter) emitFor(n []any) {
	varName := n[1].(string)
	init := e.emitExpr(n[2])
	cond := e.emitExpr(n[3])
	post := e.inlineStmt(n[4])
	body := n[5].([]any)

	e.w.Line("for ", varName, " := ", init, "; ", cond, "; ", post, " {")
	e.w.Indented(func() { e.emitBlockBody(body) })
	e.w.Line("}")
}

func (e *GoEmitter) emitSwitch(n []any) {
	cases := n[1].([]any)
		defaultBody := n[2]
			e.w.Line("switch {")
			e.w.Indented(func() {
				for _, ca := range cases {
					cn := ca.([]any)
					e.w.Line("case ", e.emitExpr(cn[1]), ":")
					e.w.Indented(func() { e.emitBlockBody(cn[2]) })
				}
				if defaultBody != nil {
					e.w.Line("default:")
					e.w.Indented(func() { e.emitBlockBody(defaultBody) })
				}
			})
			e.w.Line("}")
}

// ---------- expressions ----------

func (e *GoEmitter) emitExpr(expr any) string {
	if expr == nil {
		return ""
	}
	n, ok := expr.([]any)
	if !ok {
		e.err("expected expression node, got %T", expr)
		return "nil"
	}
	switch n[0].(string) {
		case "HInt":
			return strconv.Itoa(n[1].(int))

		case "HFloat":
			return formatFloat(n[1].(float64))

		case "HByte":
			return fmt.Sprintf("byte(%d)", n[1])

		case "HStr":
			return strconv.Quote(n[1].(string))

		case "HBool":
			if n[1].(bool) {
				return "true"
			}
			return "false"

		case "HNil":
			return "nil"

		case "HIdent":
			name := n[1].(string)
			if name == "err" && e.errRename != "" {
				return e.errRename
			}
			if name == "__range" {
				e.needsRange = true
			}
			return name

		case "HUn":
			return n[1].(string) + e.emitExpr(n[2])

		case "HBin":
			return "(" + e.emitExpr(n[2]) + " " + n[1].(string) + " " + e.emitExpr(n[3]) + ")"

		case "HCall":
			return e.emitCall(n)

		case "HSel":
			return e.emitSel(n)

		case "HIndex":
			return e.emitExpr(n[1]) + "[" + e.emitExpr(n[2]) + "]"

		case "HSlice":
			var lo, hi string
			if n[2] != nil {
				lo = e.emitExpr(n[2])
			}
			if n[3] != nil {
				hi = e.emitExpr(n[3])
			}
			return e.emitExpr(n[1]) + "[" + lo + ":" + hi + "]"

		case "HStructLit":
			name := n[1].(string)
			var parts []string
			for _, f := range n[2].([]any) {
				ff := f.([]any)
				parts = append(parts, ff[1].(string)+": "+e.emitExpr(ff[2]))
			}
			return name + "{" + strings.Join(parts, ", ") + "}"

		case "HListLit":
			tyNode := n[1]
			var parts []string
			for _, el := range n[2].([]any) {
				parts = append(parts, e.emitExpr(el))
			}
			return e.goType(tyNode) + "{" + strings.Join(parts, ", ") + "}"

		case "HSetLit":
			tyNode := n[1]
			var parts []string
			for _, el := range n[2].([]any) {
				parts = append(parts, e.emitExpr(el))
			}
			return e.goType(tyNode) + "{" + strings.Join(parts, ", ") + "}"

		case "HMapLit":
			tyNode := n[1]
			var parts []string
			for _, me := range n[2].([]any) {
				entry := me.([]any)
				parts = append(parts,
					       e.emitExpr(entry[1])+": "+e.emitExpr(entry[2]))
			}
			return e.goType(tyNode) + "{" + strings.Join(parts, ", ") + "}"

		case "HFnLit":
			return e.emitFnLit(n)

		case "HSpread":
			e.err("spread `...` used outside call arguments")
			return e.emitExpr(n[1]) + "..."
	}

	e.err("unknown expression %q", n[0])
	return "nil"
}

func (e *GoEmitter) emitCall(n []any) string {
	if id, ok := n[1].([]any); ok && len(id) == 2 {
		if id[0].(string) == "HIdent" && id[1].(string) == "__range" {
			args := n[2].([]any)
			if len(args) == 2 {
				lo, ok1 := constInt(args[0])
				hi, ok2 := constInt(args[1])
				if ok1 && ok2 {
					return rangeLit(lo, hi)
				}
			}
			e.needsRange = true
		}
	}

	callee := e.emitExpr(n[1])
	var parts []string
	for _, a := range n[2].([]any) {
		an := a.([]any)
		if an[0].(string) == "HSpread" {
			parts = append(parts, e.emitExpr(an[1])+"...")
		} else {
			parts = append(parts, e.emitExpr(a))
		}
	}
	return callee + "(" + strings.Join(parts, ", ") + ")"
}

func (e *GoEmitter) emitSel(n []any) string {
	base := e.emitExpr(n[1])
	name := n[2].(string)
	if entry, ok := e.pkgs[base]; ok {
		if renamed, ok := entry.members[name]; ok {
			name = renamed
		}
	}
	return base + "." + name
}

func (e *GoEmitter) emitFnLit(n []any) string {
	params := n[1].([]any)
	retNode := n[2]
	var errNode any
	if len(n) > 3 {
		errNode = n[3]
	}
	var body any
	if len(n) > 4 {
		body = n[4]
	}

	var sb strings.Builder
	sb.WriteString("func(")
	var ps []string
	for _, p := range params {
		pp := p.([]any)
		variadic := false
		if v, ok := pp[3].(bool); ok {
			variadic = v
		}
		pty := e.goType(pp[2])
		if variadic {
			pty = "..." + pty
		}
		ps = append(ps, pp[1].(string)+" "+pty)
	}
	sb.WriteString(strings.Join(ps, ", "))
	sb.WriteString(")")

	switch {
		case retNode != nil && errNode != nil:
			sb.WriteString(" (")
			sb.WriteString(e.goType(retNode))
			sb.WriteString(", error)")
		case retNode != nil:
			sb.WriteString(" ")
			sb.WriteString(e.goType(retNode))
		case errNode != nil:
			sb.WriteString(" error")
	}
	sb.WriteString(" {\n")

	bodyW := NewWriter()
	bodyW.indent = e.w.indent + 1
	saved := e.w
	savedRet := e.fnRetTy
	savedErr := e.fnErrTy
	savedErrVar := e.errVar
	savedRename := e.errRename
	savedSlots := e.fnRetSlots
	e.w = bodyW
	e.fnRetTy = retNode
	e.fnErrTy = errNode
	e.errVar = ""
	e.errRename = ""
	e.fnRetSlots = returnSlotCount(retNode)
	e.emitBlockBody(body)
	e.fnRetTy = savedRet
	e.fnErrTy = savedErr
	e.errVar = savedErrVar
	e.errRename = savedRename
	e.fnRetSlots = savedSlots
	e.w = saved

	sb.WriteString(bodyW.String())
	sb.WriteString(strings.Repeat("\t", e.w.indent))
	sb.WriteString("}")
	return sb.String()
}

func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eEnN") {
		s += ".0"
	}
	return s
}

func isHNil(e any) bool {
	n, ok := e.([]any)
	return ok && len(n) > 0 && n[0].(string) == "HNil"
}
