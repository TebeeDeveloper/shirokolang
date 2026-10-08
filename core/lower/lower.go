package lower

import "fmt"

type LowerError struct {
	Msg string
}

func (e *LowerError) Error() string { return e.Msg }

type Lowerer struct {
	Errors     []*LowerError
	tmpCounter int
}

func New() *Lowerer { return &Lowerer{} }

func (l *Lowerer) err(msg string) {
	l.Errors = append(l.Errors, &LowerError{msg})
}

func (l *Lowerer) newTmp() string {
	l.tmpCounter++
	return fmt.Sprintf("__t%d", l.tmpCounter)
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
				l.lowerBlock(n[4]),
				nil,
			}
		case "MethodDecl":
			recv := n[1]
			selfParam := []any{"Param", "self", []any{"NamedType", recv}, false}
			params := append([]any{selfParam}, n[3].([]any)...)
			return []any{"Fn", n[2], params, n[4], l.lowerBlock(n[5]), recv}
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
			if isComp(n[3]) {
				return l.lowerComp(n[3].([]any), n[1].(string), true)
			}
			if isRange(n[3]) {
				return l.lowerRange(n[3].([]any), n[1].(string), true)
			}
			return []any{[]any{"HLet", n[1], n[2], l.lowerExpr(n[3])}}

		case "AssignStmt":
			if isComp(n[2]) {
				if tgtN, ok := n[1].([]any); ok && tgtN[0].(string) == "IdentExpr" {
					return l.lowerComp(n[2].([]any), tgtN[1].(string), false)
				}
				l.err("comprehension assignment target must be a plain identifier")
				return nil
			}
			if isRange(n[2]) {
				if tgtN, ok := n[1].([]any); ok && tgtN[0].(string) == "IdentExpr" {
					return l.lowerRange(n[2].([]any), tgtN[1].(string), false)
				}
				l.err("range assignment target must be a plain identifier")
				return nil
			}
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
			if en, ok := n[1].([]any); ok && en[0].(string) == "MatchExpr" {
				return []any{l.lowerMatch(en)}
			}
			return []any{[]any{"HExprStmt", l.lowerExpr(n[1])}}

		case "ReturnStmt":
			return []any{[]any{"HReturn", l.lowerExpr(n[1])}}

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
	}
	l.err(fmt.Sprintf("unknown statement %q", n[0].(string)))
	return nil
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

	inner := l.lowerBlock(body).([]any)[1].([]any)
	postIR := l.lowerForPost(post)

	loopStmts := append([]any{}, inner...)
	loopStmts = append(loopStmts, postIR)
	loopBody := []any{"HBlock", loopStmts}

	return []any{
		[]any{"HLet", varName, nil, l.lowerExpr(init)},
		[]any{"HWhile", l.lowerExpr(cond), loopBody},
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
			return []any{"HExprStmt", l.lowerExpr(n[1])}
	}
	l.err(fmt.Sprintf("unsupported for-post %q", n[0].(string)))
	return []any{"HExprStmt", []any{"HInt", 0}}
}

func (l *Lowerer) lowerForRange(s []any) []any {
	count := s[1]
	body := s[2]

	i := l.newTmp()
	bodyStmts := l.lowerBlock(body).([]any)[1].([]any)

	increment := []any{
		"HAssign",
		[]any{"HIdent", i},
		[]any{"HBin", "+", []any{"HIdent", i}, []any{"HInt", 1}},
	}

	loopStmts := append([]any{}, bodyStmts...)
	loopStmts = append(loopStmts, increment)
	loopBody := []any{"HBlock", loopStmts}

	return []any{
		[]any{"HLet", i, nil, []any{"HInt", 0}},
		[]any{"HWhile",
			[]any{"HBin", "<", []any{"HIdent", i}, l.lowerExpr(count)},
			loopBody},
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

// ---------- comprehensions ----------

func isComp(e any) bool {
	if e == nil {
		return false
	}
	n, ok := e.([]any)
	if !ok {
		return false
	}
	k := n[0].(string)
	return k == "ListCompExpr" || k == "SetCompExpr"
}

func (l *Lowerer) compAppendStmt(target string, elem any, isSet bool) any {
	targetIR := []any{"HIdent", target}
	elemIR := l.lowerExpr(elem)
	if isSet {
		return []any{"HExprStmt",
			[]any{"HCall", []any{"HIdent", "__setAdd"},
			[]any{targetIR, elemIR}}}
	}
	return []any{"HAssign", targetIR,
		[]any{"HCall", []any{"HIdent", "append"},
		[]any{targetIR, elemIR}}}
}

func (l *Lowerer) lowerComp(comp []any, target string, declare bool) []any {
	kind := comp[0].(string)
	fullTy := comp[1]
	varName := comp[2].(string)
	src := comp[3]
	filt := comp[4]
	stop := comp[5]
	elem := comp[6]

	isSet := kind == "SetCompExpr"

	var empty any
	if isSet {
		empty = []any{"HSetLit", fullTy, []any{}}
	} else {
		empty = []any{"HListLit", fullTy, []any{}}
	}

	var stmts []any
	if declare {
		stmts = append(stmts, []any{"HLet", target, nil, empty})
	} else {
		stmts = append(stmts, []any{"HAssign", []any{"HIdent", target}, empty})
	}

	srcN := src.([]any)
	if srcN[0].(string) == "SeqExpr" && srcN[1].(string) == "int" {
		if stop == nil {
			l.err("`int+` comprehension needs a stop condition (`; cond`)")
			return nil
		}
		stmts = append(stmts, []any{"HLet", varName, nil, []any{"HInt", 1}})

		inner := l.compAppendStmt(target, elem, isSet)
		if filt != nil {
			inner = []any{"HIf", l.lowerExpr(filt),
				[]any{"HBlock", []any{inner}}, nil}
		}
		body := []any{
			[]any{"HIf", l.lowerExpr(stop),
				[]any{"HBlock", []any{[]any{"HBreak"}}}, nil},
				inner,
				[]any{"HAssign", []any{"HIdent", varName},
				[]any{"HBin", "+", []any{"HIdent", varName}, []any{"HInt", 1}}},
		}
		stmts = append(stmts, []any{"HWhile", []any{"HBool", true}, []any{"HBlock", body}})
		return stmts
	}

	i := l.newTmp()
	stmts = append(stmts, []any{"HLet", i, nil, []any{"HInt", 0}})

	body := []any{
		[]any{"HLet", varName, nil,
			[]any{"HIndex", l.lowerExpr(src), []any{"HIdent", i}}},
	}

	inner := l.compAppendStmt(target, elem, isSet)
	if stop != nil {
		inner = []any{"HIf", l.lowerExpr(stop),
			[]any{"HBlock", []any{inner}}, nil}
	}
	if filt != nil {
		inner = []any{"HIf", l.lowerExpr(filt),
			[]any{"HBlock", []any{inner}}, nil}
	}
	body = append(body, inner)

	body = append(body, []any{"HAssign", []any{"HIdent", i},
		      []any{"HBin", "+", []any{"HIdent", i}, []any{"HInt", 1}}})

	cond := []any{"HBin", "<", []any{"HIdent", i},
	[]any{"HCall", []any{"HIdent", "len"}, []any{l.lowerExpr(src)}}}
	stmts = append(stmts, []any{"HWhile", cond, []any{"HBlock", body}})
	return stmts
}

// lowerComp lowers `[]T{ v @ src | f ; g }` into a statement list.
// declare=true emits `:=` for the initial empty literal, false emits `=`.

func isRange(e any) bool {
	if e == nil {
		return false
	}
	n, ok := e.([]any)
	if !ok {
		return false
	}
	return n[0].(string) == "RangeLitExpr"
}

func (l *Lowerer) lowerRange(rng []any, target string, declare bool) []any {
	fullTy := rng[1]
	lo := rng[2]
	hi := rng[3]

	isSet := fullTy.([]any)[0].(string) == "SetType"

	var empty any
	if isSet {
		empty = []any{"HSetLit", fullTy, []any{}}
	} else {
		empty = []any{"HListLit", fullTy, []any{}}
	}

	targetIR := []any{"HIdent", target}
	loIR := l.lowerExpr(lo)
	hiIR := l.lowerExpr(hi)
	i := l.newTmp()

	var stmts []any
	if declare {
		stmts = append(stmts, []any{"HLet", target, nil, empty})
	} else {
		stmts = append(stmts, []any{"HAssign", targetIR, empty})
	}

	stmts = append(stmts, []any{"HLet", i, nil, loIR})

	var appendStmt any
	if isSet {
		appendStmt = []any{"HExprStmt",
			[]any{"HCall", []any{"HIdent", "__setAdd"},
			[]any{targetIR, []any{"HIdent", i}}}}
	} else {
		appendStmt = []any{"HAssign", targetIR,
			[]any{"HCall", []any{"HIdent", "append"},
			[]any{targetIR, []any{"HIdent", i}}}}
	}

	body := []any{
		appendStmt,
		[]any{"HAssign", []any{"HIdent", i},
		[]any{"HBin", "+", []any{"HIdent", i}, []any{"HInt", 1}}},
	}
	cond := []any{"HBin", "<=", []any{"HIdent", i}, hiIR}
	stmts = append(stmts, []any{"HWhile", cond, []any{"HBlock", body}})
	return stmts
}

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
					[]any{"HBin", "==", scIR, l.patLitIR(pat[1])},
					       l.lowerArmBody(body)})
			case "RangePattern":
				lo := []any{"HBin", "<=", l.patLitIR(pat[1]), scIR}
				hi := []any{"HBin", "<=", scIR, l.patLitIR(pat[2])}
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
		case "SetLitExpr":
			elems := n[2].([]any)
			var out []any
			for _, x := range elems {
				out = append(out, l.lowerExpr(x))
			}
			return []any{"HSetLit", n[1], out}
		case "ListCompExpr", "SetCompExpr":
			l.err("comprehensions must appear on the RHS of a " +
			"let/assign (expression-position comprehensions " +
			"not supported yet)")
			return []any{"HInt", 0}
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
				l.lowerBlock(n[3])}
		case "MatchExpr":
			l.err("match is only valid as a statement")
			return []any{"HInt", 0}

		case "RangeLitExpr":
			l.err("range literal is only valid as the RHS of a let/assign")
			return []any{"HInt", 0}
	}
	l.err(fmt.Sprintf("unknown expression %q", n[0].(string)))
	return []any{"HInt", 0}
}

func Lower(prog []any) ([]any, []*LowerError) {
	return New().LowerProgram(prog)
}
