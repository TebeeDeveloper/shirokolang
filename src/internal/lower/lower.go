package lower

import "fmt"

type Type = []any

type TypeLookup interface {
	TypeOf(node any) Type
}

type LowerError struct {
	Msg string
}

func (e *LowerError) Error() string { return e.Msg }

type Lowerer struct {
	Errors     []*LowerError
	tmpCounter int
	types      TypeLookup
}

func New(types TypeLookup) *Lowerer {
	return &Lowerer{types: types}
}

func (l *Lowerer) err(msg string) {
	l.Errors = append(l.Errors, &LowerError{msg})
}

func (l *Lowerer) newTmp() string {
	l.tmpCounter++
	return fmt.Sprintf("__t%d", l.tmpCounter)
}

func (l *Lowerer) typed(astNode any, hir []any) []any {
	if l.types == nil {
		return hir
	}
	ty := l.types.TypeOf(astNode)
	if ty == nil {
		return hir
	}
	return append(hir, ty)
}

// ---------- program ----------

func (l *Lowerer) LowerProgram(prog []any) ([]any, []*LowerError) {
	pkg := prog[1]
	imports := prog[2]
	decls := prog[3].([]any)

	var out []any
	for _, d := range decls {
		f := l.lowerDecl(d)
		if f != nil {
			out = append(out, f)
		}
	}
	return []any{"Program", pkg, imports, out}, l.Errors
}

func (l *Lowerer) lowerDecl(d any) any {
	n := d.([]any)
	switch n[0].(string) {
	case "StructDecl":
		return []any{"Struct", n[1], append([]any{}, n[2].([]any)...)}
	case "InterfaceDecl":
		return []any{"Interface", n[1], append([]any{}, n[2].([]any)...)}
	case "FnDecl":
		return []any{
			"Fn",
			n[1],
			append([]any{}, n[2].([]any)...),
			n[3],
			l.lowerBlock(n[5]),
			nil,
			n[4],
		}
	case "MethodDecl":
		recv := n[1]
		selfParam := []any{"Param", "self", []any{"NamedType", recv}, false}
		params := append([]any{selfParam}, n[3].([]any)...)
		return []any{"Fn", n[2], params, n[4], l.lowerBlock(n[6]), recv, n[5]}
	case "TopLet":
		return []any{"GlobalLet", n[1], n[2], l.lowerExpr(n[3])}
	case "TopConst":
		return []any{"GlobalLet", []string{n[1].(string)}, nil, l.lowerExpr(n[2])}
	case "TopStmt":
		stmts := n[1].([]any)
		var out []any
		for _, s := range stmts {
			out = append(out, l.lowerStmt(s)...)
		}
		return []any{"TopStmt", out}
	}
	l.err(fmt.Sprintf("unknown decl %q", n[0].(string)))
	return nil
}

// ---------- blocks & statements ----------

func (l *Lowerer) lowerBlock(block any) any {
	var stmts []any
	for _, s := range block.([]any)[1].([]any) {
		stmts = append(stmts, l.lowerStmt(s)...)
	}
	return []any{"HBlock", stmts}
}

func (l *Lowerer) lowerStmt(s any) []any {
	n := s.([]any)
	switch n[0].(string) {
	case "LetStmt":
		names := n[1].([]string)
		tyNode := n[2]
		expr := n[3]
		els := n[4]

		if els != nil {
			if len(names) != 1 {
				l.err("let-else requires exactly one name")
				return nil
			}
			return []any{[]any{"HErrLet",
				names[0],
				tyNode,
				l.lowerExpr(expr),
				l.lowerBlock(els),
			}}
		}

		if len(names) == 1 {
			return []any{[]any{"HLet", names[0], tyNode, l.lowerExpr(expr)}}
		}
		return []any{[]any{"HMultiLet", names, l.lowerExpr(expr)}}

	case "AssignStmt":
		return []any{[]any{"HAssign", l.lowerExpr(n[1]), l.lowerExpr(n[2])}}

	case "ConstStmt":
		return []any{[]any{"HLet", n[1], nil, l.lowerExpr(n[2])}}

	case "IncDecStmt":
		name := n[1]
		op := n[2].(string)
		bop := "+"
		if op == "--" {
			bop = "-"
		}
		return []any{[]any{"HAssign",
			[]any{"HIdent", name},
			[]any{"HBin", bop, []any{"HIdent", name}, []any{"HInt", 1}}}}

	case "ExprStmt":
		if en, ok := n[1].([]any); ok {
			if en[0].(string) == "MatchExpr" {
				return []any{l.lowerMatch(en)}
			}
			if en[0].(string) == "PostfixExpr" {
				return []any{l.lowerIncDec(n[1])}
			}
		}
		return []any{[]any{"HExprStmt", l.lowerExpr(n[1])}}

	case "ReturnStmt":
		exprs := n[1].([]any)
		var lowered []any
		for _, e := range exprs {
			lowered = append(lowered, l.lowerExpr(e))
		}
		return []any{[]any{"HReturn", lowered}}

	case "IfStmt":
		return []any{l.lowerIf(n)}

	case "ForCStmt":
		return l.lowerForC(n)
	case "ForRangeStmt":
		return l.lowerForRange(n)
	case "ForCondStmt":
		return l.lowerForCond(n)
	case "ForIterStmt":
		return l.lowerForIter(n)
	case "BreakStmt":
		return []any{[]any{"HBreak"}}
	case "ContinueStmt":
		return []any{[]any{"HContinue"}}
	}
	l.err(fmt.Sprintf("unknown statement %q", n[0].(string)))
	return nil
}

func (l *Lowerer) lowerIncDec(expr any) any {
	pe := expr.([]any)
	op := pe[1].(string)
	bop := "+"
	if op == "--" {
		bop = "-"
	}
	lhs := l.lowerExpr(pe[2])
	return []any{"HAssign", lhs,
		[]any{"HBin", bop, lhs, []any{"HInt", 1}}}
}

func (l *Lowerer) lowerIf(s []any) any {
	cond := s[1]
	thenBlock := s[2]
	els := s[3]

	thenIR := l.lowerBlock(thenBlock)
	var elseIR any
	if els != nil {
		if els.([]any)[0].(string) == "IfStmt" {
			elseIR = []any{"HBlock", []any{l.lowerIf(els.([]any))}}
		} else {
			elseIR = l.lowerBlock(els)
		}
	}
	return []any{"HIf", l.lowerExpr(cond), thenIR, elseIR}
}

func (l *Lowerer) lowerForC(s []any) []any {
	varName := s[1]
	init := s[2]
	cond := s[3]
	post := s[4]
	body := s[5]

	return []any{
		[]any{"HFor",
			varName,
			l.lowerExpr(init),
			l.lowerExpr(cond),
			l.lowerForPost(post),
			l.lowerBlock(body),
		},
	}
}

func (l *Lowerer) lowerForPost(post any) any {
	n := post.([]any)
	switch n[0].(string) {
	case "IncDecStmt":
		name := n[1]
		op := n[2].(string)
		bop := "+"
		if op == "--" {
			bop = "-"
		}
		return []any{"HAssign",
			[]any{"HIdent", name},
			[]any{"HBin", bop, []any{"HIdent", name}, []any{"HInt", 1}}}
	case "AssignStmt":
		return []any{"HAssign", l.lowerExpr(n[1]), l.lowerExpr(n[2])}
	case "ExprStmt":
		if en, ok := n[1].([]any); ok && en[0].(string) == "PostfixExpr" {
			return l.lowerIncDec(n[1])
		}
		return []any{"HExprStmt", l.lowerExpr(n[1])}
	}
	l.err(fmt.Sprintf("unsupported for-post %q", n[0].(string)))
	return []any{"HExprStmt", []any{"HInt", 0}}
}

func (l *Lowerer) lowerForRange(s []any) []any {
	count := s[1]
	body := s[2]

	return []any{
		[]any{"HForRange", l.lowerExpr(count), l.lowerBlock(body)},
	}
}

func (l *Lowerer) lowerForCond(s []any) []any {
	return []any{[]any{"HWhile", l.lowerExpr(s[1]), l.lowerBlock(s[2])}}
}

func (l *Lowerer) lowerForIter(s []any) []any {
	val := s[1]
	idx := s[2]
	src := s[3]
	body := s[4]

	srcN := src.([]any)
	if srcN[0].(string) == "SeqExpr" && srcN[1].(string) == "int" {
		return l.lowerForIntPlus(val, idx, body)
	}

	i := l.newTmp()
	bodyStmts := l.lowerBlock(body).([]any)[1].([]any)

	var prelude []any
	if idx != nil {
		prelude = append(prelude, []any{"HLet", idx, nil, []any{"HIdent", i}})
	}
	prelude = append(prelude, []any{"HLet", val, nil,
		[]any{"HIndex", l.lowerExpr(src), []any{"HIdent", i}}})

	increment := []any{
		"HAssign",
		[]any{"HIdent", i},
		[]any{"HBin", "+", []any{"HIdent", i}, []any{"HInt", 1}},
	}

	loopStmts := append([]any{}, prelude...)
	loopStmts = append(loopStmts, bodyStmts...)
	loopStmts = append(loopStmts, increment)
	loopBody := []any{"HBlock", loopStmts}

	return []any{
		[]any{"HLet", i, nil, []any{"HInt", 0}},
		[]any{"HWhile",
			[]any{"HBin", "<", []any{"HIdent", i},
			[]any{"HCall", []any{"HIdent", "len"}, []any{l.lowerExpr(src)}}},
			loopBody},
	}
}

func (l *Lowerer) lowerForIntPlus(val, idx, body any) []any {
	if idx != nil {
		l.err("`for i, x = iter int+` is not supported " +
			"(no index for an unbounded stream)")
	}
	bodyStmts := l.lowerBlock(body).([]any)[1].([]any)

	increment := []any{
		"HAssign",
		[]any{"HIdent", val},
		[]any{"HBin", "+", []any{"HIdent", val}, []any{"HInt", 1}},
	}

	loopStmts := append([]any{}, bodyStmts...)
	loopStmts = append(loopStmts, increment)
	loopBody := []any{"HBlock", loopStmts}

	return []any{
		[]any{"HLet", val, nil, []any{"HInt", 1}},
		[]any{"HWhile", []any{"HBool", true}, loopBody},
	}
}

// ---------- match ----------

func (l *Lowerer) patLitIR(v any) any {
	switch x := v.(type) {
	case bool:
		return []any{"HBool", x}
	case float64:
		return []any{"HFloat", x}
	case string:
		return []any{"HStr", x}
	case int:
		return []any{"HInt", x}
	}
	return []any{"HInt", 0}
}

func (l *Lowerer) lowerArmBody(body any) any {
	if bn, ok := body.([]any); ok && bn[0].(string) == "Block" {
		return l.lowerBlock(body)
	}
	return []any{"HBlock", []any{[]any{"HExprStmt", l.lowerExpr(body)}}}
}

func (l *Lowerer) lowerMatch(m []any) any {
	scrutinee := m[1]
	arms := m[2].([]any)

	scIR := l.lowerExpr(scrutinee)
	var cases []any
	var defaultBody any

	for _, a := range arms {
		arm := a.([]any)
		pat := arm[1].([]any)
		body := arm[2]
		switch pat[0].(string) {
		case "WildcardPattern":
			defaultBody = l.lowerArmBody(body)
			continue
		case "LitPattern":
			cases = append(cases, []any{"SwitchCase",
				[]any{"HBin", "==", scIR, l.lowerExpr(pat[1])},
				l.lowerArmBody(body)})
		case "RangePattern":
			lo := []any{"HBin", "<=", l.lowerExpr(pat[1]), scIR}
			hi := []any{"HBin", "<=", scIR, l.lowerExpr(pat[2])}
			cases = append(cases, []any{"SwitchCase",
				[]any{"HBin", "&&", lo, hi},
				l.lowerArmBody(body)})
		}
	}
	return []any{"HSwitch", cases, defaultBody}
}

// ---------- expressions ----------

func (l *Lowerer) lowerExpr(e any) any {
	if e == nil {
		return nil
	}
	h := l.lowerExprRaw(e)
	if h == nil {
		return nil
	}
	n, ok := h.([]any)
	if !ok {
		return h
	}
	return l.typed(e, n)
}

func (l *Lowerer) lowerExprRaw(e any) any {
	if e == nil {
		return nil
	}
	n := e.([]any)
	switch n[0].(string) {
	case "IntExpr":
		return []any{"HInt", n[1]}
	case "FloatExpr":
		return []any{"HFloat", n[1]}
	case "ByteExpr":
		return []any{"HByte", n[1]}
	case "StrExpr":
		return []any{"HStr", n[1]}
	case "BoolExpr":
		return []any{"HBool", n[1]}
	case "NilExpr":
		return []any{"HNil"}
	case "IdentExpr":
		return []any{"HIdent", n[1]}
	case "UnaryExpr":
		return []any{"HUn", n[1], l.lowerExpr(n[2])}
	case "BinaryExpr":
		return []any{"HBin", n[1], l.lowerExpr(n[2]), l.lowerExpr(n[3])}
	case "PostfixExpr":
		l.err("postfix ++/-- in expression position is not supported; " +
			"use a separate statement")
		return []any{"HInt", 0}
	case "CallExpr":
		callee := n[1]
		args := n[2].([]any)
		var lowered []any
		for _, a := range args {
			an := a.([]any)
			if an[0].(string) == "SpreadExpr" {
				lowered = append(lowered, []any{"HSpread", l.lowerExpr(an[1])})
			} else {
				lowered = append(lowered, l.lowerExpr(a))
			}
		}
		return []any{"HCall", l.lowerExpr(callee), lowered}
	case "SelectorExpr":
		return []any{"HSel", l.lowerExpr(n[1]), n[2]}
	case "IndexExpr":
		return []any{"HIndex", l.lowerExpr(n[1]), l.lowerExpr(n[2])}
	case "SliceExpr":
		return []any{"HSlice", l.lowerExpr(n[1]), l.lowerExpr(n[2]), l.lowerExpr(n[3])}
	case "StructLitExpr":
		name := n[1]
		finits := n[2].([]any)
		var fields []any
		for _, fi := range finits {
			f := fi.([]any)
			fields = append(fields, []any{"FieldInit", f[1], l.lowerExpr(f[2])})
		}
		return []any{"HStructLit", name, fields}
	case "ListLitExpr":
		elems := n[2].([]any)
		var out []any
		for _, x := range elems {
			out = append(out, l.lowerExpr(x))
		}
		return []any{"HListLit", n[1], out}
	case "MapLitExpr":
		entries := n[2].([]any)
		var out []any
		for _, e := range entries {
			entry := e.([]any)
			out = append(out, []any{
				"MapEntry",
				l.lowerExpr(entry[1]),
				l.lowerExpr(entry[2]),
			})
		}
		return []any{"HMapLit", n[1], out}
	case "ListCompExpr":
		return l.lowerCompIR(n)
	case "SeqExpr":
		l.err("`int+` is only valid as a `for ... iter` source")
		return []any{"HInt", 0}
	case "SpreadExpr":
		l.err("spread `...` only allowed in call arguments")
		return []any{"HInt", 0}
	case "FnLitExpr":
		return []any{"HFnLit",
			append([]any{}, n[1].([]any)...),
			n[2],
			n[3],
			l.lowerBlock(n[4])}
	case "MatchExpr":
		l.err("match is only valid as a statement")
		return []any{"HInt", 0}
	case "RangeLitExpr":
		return l.lowerRangeIR(n)
	// ★ interpolated string literal
	case "InterpStrExpr":
		return l.lowerInterpStr(n)
	}
	l.err(fmt.Sprintf("unknown expression %q", n[0].(string)))
	return []any{"HInt", 0}
}

// ★ lowerInterpStr lowers an interpolated string literal.
//
// AST:  InterpStrExpr{parts, line, col}
// HIR:  HInterpStr{parts}
//
// Each part is one of:
//
//	[]any{"HStrPart", text}
//	[]any{"HExprPart", hirExpr}
func (l *Lowerer) lowerInterpStr(n []any) any {
	parts := n[1].([]any)
	out := make([]any, 0, len(parts))
	for _, p := range parts {
		pn, ok := p.([]any)
		if !ok || len(pn) < 2 {
			continue
		}
		switch pn[0].(string) {
		case "StrPart":
			out = append(out, []any{"HStrPart", pn[1].(string)})
		case "ExprPart":
			out = append(out, []any{"HExprPart", l.lowerExpr(pn[1]), pn[2]})
		}
	}
	return []any{"HInterpStr", out}
}

func (l *Lowerer) lowerCompIR(n []any) any {
	fullTy := n[1]
	varName := n[2].(string)
	srcNode := n[3]
	filt := n[4]
	stop := n[5]
	elem := n[6]

	var srcIR any
	if srcN, ok := srcNode.([]any); ok && srcN[0].(string) == "SeqExpr" {
		srcIR = []any{"HSeq", srcN[1]}
	} else {
		srcIR = l.lowerExpr(srcNode)
	}

	var filtIR, stopIR any
	if filt != nil {
		filtIR = l.lowerExpr(filt)
	}
	if stop != nil {
		stopIR = l.lowerExpr(stop)
	}

	return []any{
		"HComp",
		fullTy,
		varName,
		srcIR,
		filtIR,
		stopIR,
		l.lowerExpr(elem),
	}
}

func (l *Lowerer) lowerRangeIR(n []any) any {
	return []any{
		"HRange",
		n[1],
		l.lowerExpr(n[2]),
		l.lowerExpr(n[3]),
	}
}

func Lower(prog []any, types TypeLookup) ([]any, []*LowerError) {
	return New(types).LowerProgram(prog)
}
