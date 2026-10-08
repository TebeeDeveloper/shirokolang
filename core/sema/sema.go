package sema

import (
	"fmt"
	"reflect"
	"strings"
)

// ---------- errors ----------

type SemaError struct {
	Msg  string
	Line int
	Col  int
}

func (e *SemaError) Error() string {
	if e.Line == 0 && e.Col == 0 {
		return e.Msg
	}
	return fmt.Sprintf("line %d, col %d: %s", e.Line, e.Col, e.Msg)
}

// nodePos returns the (line, col) that the parser appended to a node.
// ok is false when the node was not built with parser.withPos.
func nodePos(n any) (int, int, bool) {
	node, ok := n.([]any)
	if !ok || len(node) < 3 {
		return 0, 0, false
	}
	line, ok1 := node[len(node)-2].(int)
	col, ok2 := node[len(node)-1].(int)
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return line, col, true
}

// ---------- types ----------

// Type is a Python-tuple-like []any with kind at index 0.
type Type = []any

var (
	TAny     = Type{"any"}
	TInt     = Type{"int"}
	TFloat   = Type{"float"}
	TString  = Type{"string"}
	TByte    = Type{"byte"}
	TBool    = Type{"bool"}
	TVoid    = Type{"void"}
	TUnknown = Type{"unknown"}
)

func isKind(t Type, k string) bool { return len(t) > 0 && t[0] == k }

func typeStr(t Type) string {
	switch t[0].(string) {
		case "int_const":
			return fmt.Sprintf("int literal %d", t[1].(int))
		case "any", "int", "float", "string", "bool", "byte", "void", "unknown":
			return t[0].(string)
		case "list":
			return "[]" + typeStr(t[1].(Type))
		case "set":
			return "{}" + typeStr(t[1].(Type))
		case "struct", "interface":
			return t[1].(string)
		case "package":
			return "package " + t[1].(string)
		case "fn":
			params := t[1].([]Type)
			var ps []string
			for _, p := range params {
				ps = append(ps, typeStr(p))
			}
			return fmt.Sprintf("fn(%s) -> %s", strings.Join(ps, ", "), typeStr(t[2].(Type)))
	}
	return fmt.Sprintf("%v", t)
}

func fitsIn(t Type, v int) bool {
	if reflect.DeepEqual(t, TInt) {
		return true
	}
	if reflect.DeepEqual(t, TByte) {
		return 0 <= v && v <= 255
	}
	if reflect.DeepEqual(t, TFloat) {
		return true
	}
	return false
}

func typesEqual(a, b Type) bool {
	if reflect.DeepEqual(a, b) {
		return true
	}
	if isKind(a, "any") || isKind(b, "any") {
		return true
	}
	if isKind(a, "unknown") || isKind(b, "unknown") {
		return true
	}
	if isKind(a, "int_const") && fitsIn(b, a[1].(int)) {
		return true
	}
	if isKind(b, "int_const") && fitsIn(a, b[1].(int)) {
		return true
	}
	if isKind(a, "list") && isKind(b, "list") {
		return typesEqual(a[1].(Type), b[1].(Type))
	}
	if isKind(a, "set") && isKind(b, "set") {
		return typesEqual(a[1].(Type), b[1].(Type))
	}
	if isKind(a, "fn") && isKind(b, "fn") {
		ap := a[1].([]Type)
		bp := b[1].([]Type)
		if len(ap) != len(bp) {
			return false
		}
		for i := range ap {
			if !typesEqual(ap[i], bp[i]) {
				return false
			}
		}
		return typesEqual(a[2].(Type), b[2].(Type))
	}
	return false
}

func isNumeric(t Type) bool {
	if reflect.DeepEqual(t, TInt) || reflect.DeepEqual(t, TFloat) {
		return true
	}
	return isKind(t, "int_const")
}

func isIntegerLike(t Type) bool {
	if reflect.DeepEqual(t, TInt) || reflect.DeepEqual(t, TByte) {
		return true
	}
	return isKind(t, "int_const")
}

func defaultType(t Type) Type {
	if isKind(t, "int_const") {
		return TInt
	}
	return t
}

var conversions = map[string]Type{
	"string": TString,
	"int":    TInt,
	"byte":   TByte,
	"float":  TFloat,
}

// ---------- symbols / scopes ----------

type Symbol struct {
	Name  string
	Kind  string
	Type  Type
	Node  any
	Const bool
}

type Scope struct {
	Symbols map[string]*Symbol
	Parent  *Scope
}

func NewScope(parent *Scope) *Scope {
	return &Scope{Symbols: map[string]*Symbol{}, Parent: parent}
}

func (s *Scope) Declare(sym *Symbol, errors *[]*SemaError) {
	if _, ok := s.Symbols[sym.Name]; ok {
		*errors = append(*errors, &SemaError{Msg: fmt.Sprintf("%q redeclared in this scope", sym.Name)})
		return
	}
	s.Symbols[sym.Name] = sym
}

func (s *Scope) Lookup(name string) *Symbol {
	for sc := s; sc != nil; sc = sc.Parent {
		if sym, ok := sc.Symbols[name]; ok {
			return sym
		}
	}
	return nil
}

type Param struct {
	Name string
	Type Type
}

type FnSig struct {
	Name     string
	Params   []Param
	Ret      Type
	Variadic bool
	Recv     string
}

func (f *FnSig) Type() Type {
	ps := make([]Type, len(f.Params))
	for i, p := range f.Params {
		ps[i] = p.Type
	}
	return Type{"fn", ps, f.Ret, f.Variadic}
}

type StructInfo struct {
	Name    string
	Fields  map[string]Type
	Methods map[string]*FnSig
}

type InterfaceInfo struct {
	Name    string
	Methods map[string]*FnSig
}

// ---------- builtins ----------

var BuiltinPackages = map[string]map[string]Type{
	"fmt": {
		"println": Type{"fn", []Type{TAny}, TVoid, true},
		"printf":  Type{"fn", []Type{TAny}, TVoid, true},
		"print":   Type{"fn", []Type{TAny}, TVoid, true},
	},
}

var BuiltinFuncs = map[string]Type{
	"len":    Type{"fn", []Type{TAny}, TInt, false},
	"append": Type{"fn", []Type{Type{"list", TAny}, TAny}, Type{"list", TAny}, false},
}

// ---------- Sema ----------

type Sema struct {
	Errors     []*SemaError
	Structs    map[string]*StructInfo
	Interfaces map[string]*InterfaceInfo
	Funcs      map[string]*FnSig
	Methods    map[string]*FnSig // key: recv + "." + name
	Globals    *Scope
	CurrentRet Type
}

func New() *Sema {
	s := &Sema{
		Structs:    map[string]*StructInfo{},
		Interfaces: map[string]*InterfaceInfo{},
		Funcs:      map[string]*FnSig{},
		Methods:    map[string]*FnSig{},
		Globals:    NewScope(nil),
	}
	s.installBuiltins()
	return s
}

func (s *Sema) installBuiltins() {
	for pkg := range BuiltinPackages {
		s.Globals.Symbols[pkg] = &Symbol{Name: pkg, Kind: "package", Type: Type{"package", pkg}}
	}
	for name, sig := range BuiltinFuncs {
		s.Globals.Symbols[name] = &Symbol{Name: name, Kind: "func", Type: sig}
	}
}

func (s *Sema) err(msg string) {
	s.Errors = append(s.Errors, &SemaError{Msg: msg})
}

// errAt reports an error positioned at node n.
func (s *Sema) errAt(n any, msg string) {
	e := &SemaError{Msg: msg}
	if line, col, ok := nodePos(n); ok {
		e.Line = line
		e.Col = col
	}
	s.Errors = append(s.Errors, e)
}

func (s *Sema) Analyze(prog []any) []*SemaError {
	decls := prog[3].([]any)
	s.collectNames(decls)
	s.resolveTypes(decls)
	s.collectSignatures(decls)
	s.checkBodies(decls)
	return s.Errors
}

// ---------- pass 1a ----------

func (s *Sema) collectNames(decls []any) {
	for _, d := range decls {
		dd := d.([]any)
		switch dd[0].(string) {
			case "StructDecl":
				name := dd[1].(string)
				if _, ok := s.Structs[name]; ok {
					s.err(fmt.Sprintf("type %q already declared", name))
					continue
				}
				if _, ok := s.Interfaces[name]; ok {
					s.err(fmt.Sprintf("type %q already declared", name))
					continue
				}
				s.Structs[name] = &StructInfo{Name: name, Fields: map[string]Type{}, Methods: map[string]*FnSig{}}
			case "InterfaceDecl":
				name := dd[1].(string)
				if _, ok := s.Structs[name]; ok {
					s.err(fmt.Sprintf("type %q already declared", name))
					continue
				}
				if _, ok := s.Interfaces[name]; ok {
					s.err(fmt.Sprintf("type %q already declared", name))
					continue
				}
				s.Interfaces[name] = &InterfaceInfo{Name: name, Methods: map[string]*FnSig{}}
		}
	}
}

// ---------- pass 1b ----------

func (s *Sema) resolveTypes(decls []any) {
	for _, d := range decls {
		dd := d.([]any)
		switch dd[0].(string) {
			case "StructDecl":
				s.resolveStruct(dd)
			case "InterfaceDecl":
				s.resolveInterface(dd)
		}
	}
}

func (s *Sema) resolveTypeNode(node any) Type {
	n := node.([]any)
	switch n[0].(string) {
		case "ListType":
			return Type{"list", s.resolveTypeNode(n[1])}
		case "SetType":
			return Type{"set", s.resolveTypeNode(n[1])}
	}
	name := n[1].(string)
	switch name {
		case "int":
			return TInt
		case "float":
			return TFloat
		case "string":
			return TString
		case "byte":
			return TByte
		case "bool":
			return TBool
		case "any":
			return TAny
	}
	if _, ok := s.Structs[name]; ok {
		return Type{"struct", name}
	}
	if _, ok := s.Interfaces[name]; ok {
		return Type{"interface", name}
	}
	s.err(fmt.Sprintf("unknown type %q", name))
	return TUnknown
}

func (s *Sema) resolveStruct(d []any) {
	name := d[1].(string)
	fields := d[2].([]any)
	info := s.Structs[name]
	for _, f := range fields {
		ff := f.([]any)
		fname := ff[1].(string)
		fty := ff[2]
		if _, ok := info.Fields[fname]; ok {
			s.err(fmt.Sprintf("field %q redeclared in %s", fname, name))
			continue
		}
		info.Fields[fname] = s.resolveTypeNode(fty)
	}
}

func (s *Sema) resolveInterface(d []any) {
	name := d[1].(string)
	methods := d[2].([]any)
	info := s.Interfaces[name]
	for _, m := range methods {
		sig := m.([]any)
		mname := sig[1].(string)
		params := sig[2].([]any)
		retNode := sig[3]
		if _, ok := info.Methods[mname]; ok {
			s.err(fmt.Sprintf("method %q redeclared in interface %s", mname, name))
			continue
		}
		var ps []Param
		variadic := false
		for _, p := range params {
			pp := p.([]any)
			ps = append(ps, Param{Name: pp[1].(string), Type: s.resolveTypeNode(pp[2])})
			if v, ok := pp[3].(bool); ok && v {
				variadic = true
			}
		}
		rt := TVoid
		if retNode != nil {
			rt = s.resolveTypeNode(retNode)
		}
		info.Methods[mname] = &FnSig{Name: mname, Params: ps, Ret: rt, Variadic: variadic, Recv: name}
	}
}

// ---------- pass 1c ----------

func (s *Sema) collectSignatures(decls []any) {
	for _, d := range decls {
		dd := d.([]any)
		switch dd[0].(string) {
			case "FnDecl":
				name := dd[1].(string)
				params := dd[2].([]any)
				retNode := dd[3]
				if _, ok := s.Funcs[name]; ok {
					s.err(fmt.Sprintf("function %q already declared", name))
					continue
				}
				var ps []Param
				variadic := false
				for _, p := range params {
					pp := p.([]any)
					ps = append(ps, Param{Name: pp[1].(string), Type: s.resolveTypeNode(pp[2])})
					if v, ok := pp[3].(bool); ok && v {
						variadic = true
					}
				}
				rt := TVoid
				if retNode != nil {
					rt = s.resolveTypeNode(retNode)
				}
				sig := &FnSig{Name: name, Params: ps, Ret: rt, Variadic: variadic}
				s.Funcs[name] = sig
				s.Globals.Declare(&Symbol{Name: name, Kind: "func", Type: sig.Type()}, &s.Errors)
			case "MethodDecl":
				recv := dd[1].(string)
				name := dd[2].(string)
				params := dd[3].([]any)
				retNode := dd[4]
				if _, ok := s.Structs[recv]; !ok {
					if _, ok := s.Interfaces[recv]; !ok {
						s.err(fmt.Sprintf("receiver type %q not declared", recv))
						continue
					}
				}
				var ps []Param
				variadic := false
				for _, p := range params {
					pp := p.([]any)
					ps = append(ps, Param{Name: pp[1].(string), Type: s.resolveTypeNode(pp[2])})
					if v, ok := pp[3].(bool); ok && v {
						variadic = true
					}
				}
				rt := TVoid
				if retNode != nil {
					rt = s.resolveTypeNode(retNode)
				}
				sig := &FnSig{Name: name, Params: ps, Ret: rt, Variadic: variadic, Recv: recv}
				key := recv + "." + name
				if _, ok := s.Methods[key]; ok {
					s.err(fmt.Sprintf("method %s.%s already declared", recv, name))
					continue
				}
				s.Methods[key] = sig
				if info, ok := s.Structs[recv]; ok {
					info.Methods[name] = sig
				} else {
					s.Interfaces[recv].Methods[name] = sig
				}
		}
	}
}

// ---------- pass 2 ----------

func (s *Sema) checkBodies(decls []any) {
	for _, d := range decls {
		dd := d.([]any)
		switch dd[0].(string) {
			case "FnDecl":
				name := dd[1].(string)
				params := dd[2].([]any)
				body := dd[4]
				sig, ok := s.Funcs[name]
				if !ok {
					continue
				}
				s.CurrentRet = sig.Ret
				scope := NewScope(s.Globals)
				for _, p := range params {
					pp := p.([]any)
					scope.Declare(&Symbol{Name: pp[1].(string), Kind: "var", Type: s.resolveTypeNode(pp[2])}, &s.Errors)
				}
				s.checkBlock(body, scope)
				s.CurrentRet = nil
			case "MethodDecl":
				recv := dd[1].(string)
				name := dd[2].(string)
				params := dd[3].([]any)
				body := dd[5]
				sig, ok := s.Methods[recv+"."+name]
				if !ok {
					continue
				}
				s.CurrentRet = sig.Ret
				scope := NewScope(s.Globals)
				var recvType Type
				if _, ok := s.Structs[recv]; ok {
					recvType = Type{"struct", recv}
				} else {
					recvType = Type{"interface", recv}
				}
				scope.Declare(&Symbol{Name: "self", Kind: "var", Type: recvType}, &s.Errors)
				for _, p := range params {
					pp := p.([]any)
					scope.Declare(&Symbol{Name: pp[1].(string), Kind: "var", Type: s.resolveTypeNode(pp[2])}, &s.Errors)
				}
				s.checkBlock(body, scope)
				s.CurrentRet = nil
		}
	}
}

func (s *Sema) checkBlock(block any, scope *Scope) {
	stmts := block.([]any)[1].([]any)
	for _, st := range stmts {
		s.checkStmt(st, scope)
	}
}

func (s *Sema) checkStmt(stmt any, scope *Scope) {
	st := stmt.([]any)
	switch st[0].(string) {
		case "LetStmt":
			name := st[1].(string)
			tyNode := st[2]
			expr := st[3]
			var ty Type
			if tyNode != nil {
				ty = s.resolveTypeNode(tyNode)
				if expr != nil {
					et := s.infer(expr, scope)
					if !typesEqual(et, ty) {
						s.err(fmt.Sprintf("cannot assign %s to %s", typeStr(et), typeStr(ty)))
					}
				}
			} else if expr != nil {
				ty = defaultType(s.infer(expr, scope))
			} else {
				s.err(fmt.Sprintf("let %s needs a type or initializer", name))
				ty = TUnknown
			}
			scope.Declare(&Symbol{Name: name, Kind: "var", Type: ty}, &s.Errors)

		case "ConstStmt":
			name := st[1].(string)
			expr := st[2]
			ty := defaultType(s.infer(expr, scope))
			scope.Declare(&Symbol{Name: name, Kind: "const", Type: ty, Const: true}, &s.Errors)

		case "AssignStmt":
			tgt := st[1]
			expr := st[2]
			et := s.infer(expr, scope)
			tt := s.checkLValue(tgt, scope)
			if !typesEqual(et, tt) {
				s.err(fmt.Sprintf("cannot assign %s to %s", typeStr(et), typeStr(tt)))
			}

		case "ExprStmt":
			s.infer(st[1], scope)

		case "ReturnStmt":
			expr := st[1]
			if s.CurrentRet == nil {
				s.err("return outside function")
			} else if expr != nil {
				et := s.infer(expr, scope)
				if !typesEqual(et, s.CurrentRet) {
					s.err(fmt.Sprintf("cannot return %s from function returning %s", typeStr(et), typeStr(s.CurrentRet)))
				}
			} else if !typesEqual(s.CurrentRet, TVoid) {
				s.err(fmt.Sprintf("missing return value (expected %s)", typeStr(s.CurrentRet)))
			}

		case "IncDecStmt":
			name := st[1].(string)
			sym := scope.Lookup(name)
			if sym == nil {
				s.err(fmt.Sprintf("undefined: %s", name))
			} else if sym.Const {
				s.err(fmt.Sprintf("cannot modify const %q", name))
			} else if !isNumeric(sym.Type) {
				s.err(fmt.Sprintf("cannot increment %s", typeStr(sym.Type)))
			}

		case "IfStmt":
			cond := st[1]
			thenBlock := st[2]
			els := st[3]
			ct := s.infer(cond, scope)
			if !typesEqual(ct, TBool) {
				s.err(fmt.Sprintf("if condition must be bool, got %s", typeStr(ct)))
			}
			s.checkBlock(thenBlock, NewScope(scope))
			if els != nil {
				if els.([]any)[0].(string) == "IfStmt" {
					s.checkStmt(els, scope)
				} else {
					s.checkBlock(els, NewScope(scope))
				}
			}

		case "ForCStmt":
			varName := st[1].(string)
			init := st[2]
			cond := st[3]
			post := st[4]
			body := st[5]
			inner := NewScope(scope)
			it := defaultType(s.infer(init, scope))
			inner.Declare(&Symbol{Name: varName, Kind: "var", Type: it}, &s.Errors)
			ct := s.infer(cond, inner)
			if !typesEqual(ct, TBool) {
				s.err(fmt.Sprintf("for condition must be bool, got %s", typeStr(ct)))
			}
			s.checkForPost(post, inner)
			s.checkBlock(body, inner)

		case "ForRangeStmt":
			count := st[1]
			body := st[2]
			ct := s.infer(count, scope)
			if !typesEqual(ct, TInt) {
				s.err(fmt.Sprintf("range count must be int, got %s", typeStr(ct)))
			}
			s.checkBlock(body, scope)

		case "ForCondStmt":
			cond := st[1]
			body := st[2]
			ct := s.infer(cond, scope)
			if !typesEqual(ct, TBool) {
				s.err(fmt.Sprintf("for condition must be bool, got %s", typeStr(ct)))
			}
			s.checkBlock(body, scope)

		case "ForIterStmt":
			val := st[1].(string)
			idx := st[2]
			src := st[3]
			body := st[4]
			var elemT Type
			srcN := src.([]any)
			if srcN[0].(string) == "SeqExpr" {
				elemT = TInt
			} else {
				st_ := s.infer(src, scope)
				if typesEqual(st_, TUnknown) {
					elemT = TUnknown
				} else if isKind(st_, "list") || isKind(st_, "set") {
					elemT = st_[1].(Type)
				} else {
					s.err(fmt.Sprintf("cannot iterate over %s", typeStr(st_)))
					elemT = TUnknown
				}
			}
			inner := NewScope(scope)
			if idx != nil {
				inner.Declare(&Symbol{Name: idx.(string), Kind: "var", Type: TInt}, &s.Errors)
			}
			inner.Declare(&Symbol{Name: val, Kind: "var", Type: elemT}, &s.Errors)
			s.checkBlock(body, inner)

		default:
			s.err(fmt.Sprintf("unknown statement kind %q", st[0].(string)))
	}
}

func (s *Sema) checkForPost(post any, scope *Scope) {
	k := post.([]any)[0].(string)
	switch k {
		case "IncDecStmt", "AssignStmt", "ExprStmt":
			s.checkStmt(post, scope)
		default:
			s.infer(post, scope)
	}
}

// ---------- l-values ----------

func (s *Sema) checkLValue(tgt any, scope *Scope) Type {
	t := tgt.([]any)
	switch t[0].(string) {
		case "IdentExpr":
			name := t[1].(string)
			sym := scope.Lookup(name)
			if sym == nil {
				s.err(fmt.Sprintf("undefined: %s", name))
				return TUnknown
			}
			if sym.Const {
				s.err(fmt.Sprintf("cannot assign to const %q", name))
				return TUnknown
			}
			return sym.Type
		case "SelectorExpr":
			bt := s.infer(t[1], scope)
			if isKind(bt, "struct") {
				sname := bt[1].(string)
				info := s.Structs[sname]
				fname := t[2].(string)
				if info != nil {
					if ft, ok := info.Fields[fname]; ok {
						return ft
					}
				}
			}
			s.err(fmt.Sprintf("cannot assign to %q", t[2].(string)))
			return TUnknown
		case "IndexExpr":
			bt := s.infer(t[1], scope)
			if isKind(bt, "list") {
				return bt[1].(Type)
			}
			s.err("cannot assign through index")
			return TUnknown
	}
	s.err("invalid assignment target")
	return TUnknown
}

// ---------- expressions ----------

func (s *Sema) infer(e any, scope *Scope) Type {
	n := e.([]any)
	switch n[0].(string) {
		case "IntExpr":
			return Type{"int_const", n[1].(int)}
		case "FloatExpr":
			return TFloat
		case "ByteExpr":
			return TByte
		case "StrExpr":
			return TString
		case "BoolExpr":
			return TBool
		case "IdentExpr":
			return s.inferIdent(n, scope)
		case "UnaryExpr":
			return s.inferUnary(n, scope)
		case "BinaryExpr":
			return s.inferBinary(n, scope)
		case "CallExpr":
			return s.inferCall(n, scope)
		case "SelectorExpr":
			return s.inferSelector(n, scope)
		case "IndexExpr":
			return s.inferIndex(n, scope)
		case "SliceExpr":
			return s.inferSlice(n, scope)
		case "StructLitExpr":
			return s.inferStructLit(n, scope)
		case "ListLitExpr":
			return s.inferListLit(n, scope)
		case "SetLitExpr":
			return s.inferSetLit(n, scope)
		case "ListCompExpr", "SetCompExpr":
			return s.inferComp(n, scope)
		case "PostfixExpr":
			t := s.infer(n[2], scope)
			if !isNumeric(t) {
				s.err(fmt.Sprintf("operator %q requires numeric, got %s", n[1].(string), typeStr(t)))
			}
			return t
		case "SpreadExpr":
			s.err("spread '...' is only valid in call arguments")
			return TUnknown
		case "FnLitExpr":
			return s.inferFnLit(n, scope)
		case "MatchExpr":
			return s.inferMatch(n, scope)
		case "RangeLitExpr":
			return s.inferRangeLit(n, scope)
	}
	return TUnknown
}

func (s *Sema) inferIdent(e []any, scope *Scope) Type {
	name := e[1].(string)
	sym := scope.Lookup(name)
	if sym == nil {
		s.err(fmt.Sprintf("undefined: %s", name))
		return TUnknown
	}
	return sym.Type
}

func (s *Sema) inferUnary(e []any, scope *Scope) Type {
	op := e[1].(string)
	t := s.infer(e[2], scope)
	switch op {
		case "-":
			if !isNumeric(t) {
				s.err(fmt.Sprintf("unary '-' expects numeric, got %s", typeStr(t)))
				return TUnknown
			}
			if isKind(t, "int_const") {
				return Type{"int_const", -t[1].(int)}
			}
			return t
		case "!":
			if !typesEqual(t, TBool) {
				s.err(fmt.Sprintf("unary '!' expects bool, got %s", typeStr(t)))
			}
			return TBool
	}
	return TUnknown
}

func (s *Sema) inferBinary(e []any, scope *Scope) Type {
	op := e[1].(string)
	lt := s.infer(e[2], scope)
	rt := s.infer(e[3], scope)
	if typesEqual(lt, TUnknown) || typesEqual(rt, TUnknown) {
		return TUnknown
	}

	switch op {
		case "+", "-", "*", "/":
			if op == "+" && typesEqual(lt, TString) && typesEqual(rt, TString) {
				return TString
			}
			if isNumeric(lt) && isNumeric(rt) {
				if typesEqual(lt, TFloat) || typesEqual(rt, TFloat) {
					return TFloat
				}
				return TInt
			}
			s.err(fmt.Sprintf("operator %q not defined for %s and %s", op, typeStr(lt), typeStr(rt)))
			return TUnknown
		case "%":
			if isIntegerLike(lt) && isIntegerLike(rt) {
				return TInt
			}
			s.err(fmt.Sprintf("operator '%%' requires ints, got %s, %s", typeStr(lt), typeStr(rt)))
			return TUnknown
		case "<", ">", "<=", ">=":
			if (isNumeric(lt) && isNumeric(rt)) ||
				(typesEqual(lt, TString) && typesEqual(rt, TString)) ||
				(isIntegerLike(lt) && isIntegerLike(rt)) {
					return TBool
				}
				s.err(fmt.Sprintf("operator %q requires matching numeric or string operands", op))
				return TBool
		case "==", "!=":
			if !(typesEqual(lt, rt) || (isIntegerLike(lt) && isIntegerLike(rt))) {
				s.err(fmt.Sprintf("cannot compare %s with %s", typeStr(lt), typeStr(rt)))
			}
			return TBool
		case "&&", "||":
			if !typesEqual(lt, TBool) {
				s.err(fmt.Sprintf("%q expects bool on left", op))
			}
			if !typesEqual(rt, TBool) {
				s.err(fmt.Sprintf("%q expects bool on right", op))
			}
			return TBool
	}
	return TUnknown
}

func (s *Sema) inferCall(e []any, scope *Scope) Type {
	callee := e[1]
	args := e[2].([]any)

	if cn, ok := callee.([]any); ok && cn[0].(string) == "IdentExpr" {
		if conv, ok := conversions[cn[1].(string)]; ok {
			for _, a := range args {
				an := a.([]any)
				if an[0].(string) == "SpreadExpr" {
					s.infer(an[1], scope)
				} else {
					s.infer(a, scope)
				}
			}
			return conv
		}
	}

	inferAll := func() {
		for _, a := range args {
			an := a.([]any)
			if an[0].(string) == "SpreadExpr" {
				s.infer(an[1], scope)
			} else {
				s.infer(a, scope)
			}
		}
	}

	calleeT := s.infer(callee, scope)
	if typesEqual(calleeT, TUnknown) {
		inferAll()
		return TUnknown
	}
	if !isKind(calleeT, "fn") {
		s.err(fmt.Sprintf("cannot call %s", typeStr(calleeT)))
		inferAll()
		return TUnknown
	}

	params := calleeT[1].([]Type)
	ret := calleeT[2].(Type)
	variadic := calleeT[3].(bool)

	var argTypes []Type
	for _, a := range args {
		an := a.([]any)
		if an[0].(string) == "SpreadExpr" {
			t := s.infer(an[1], scope)
			if typesEqual(t, TUnknown) {
				argTypes = append(argTypes, TUnknown)
			} else if !isKind(t, "list") {
				s.err(fmt.Sprintf("spread requires list, got %s", typeStr(t)))
				argTypes = append(argTypes, TUnknown)
			} else {
				argTypes = append(argTypes, t[1].(Type))
			}
		} else {
			argTypes = append(argTypes, s.infer(a, scope))
		}
	}

	n := len(argTypes)
	if variadic {
		if n < len(params)-1 {
			s.err(fmt.Sprintf("expected at least %d args, got %d", len(params)-1, n))
		}
	} else if n != len(params) {
		s.err(fmt.Sprintf("expected %d args, got %d", len(params), n))
	}

	for i, at := range argTypes {
		var pt Type
		if variadic && i >= len(params)-1 {
			pt = params[len(params)-1]
		} else if i < len(params) {
			pt = params[i]
		} else {
			break
		}
		if !typesEqual(at, pt) {
			s.err(fmt.Sprintf("arg %d: expected %s, got %s", i+1, typeStr(pt), typeStr(at)))
		}
	}
	return ret
}

func (s *Sema) inferSelector(e []any, scope *Scope) Type {
	name := e[2].(string)
	bt := s.infer(e[1], scope)
	if typesEqual(bt, TUnknown) {
		return TUnknown
	}
	switch {
		case isKind(bt, "package"):
			pkg := bt[1].(string)
			members := BuiltinPackages[pkg]
			if t, ok := members[name]; ok {
				return t
			}
			s.err(fmt.Sprintf("package %q has no member %q", pkg, name))
			return TUnknown
		case isKind(bt, "struct"):
			sname := bt[1].(string)
			info := s.Structs[sname]
			if info == nil {
				return TUnknown
			}
			if t, ok := info.Fields[name]; ok {
				return t
			}
			if sig, ok := info.Methods[name]; ok {
				return sig.Type()
			}
			s.err(fmt.Sprintf("%s has no field or method %q", sname, name))
			return TUnknown
		case isKind(bt, "interface"):
			sname := bt[1].(string)
			info := s.Interfaces[sname]
			if info != nil {
				if sig, ok := info.Methods[name]; ok {
					return sig.Type()
				}
			}
			s.err(fmt.Sprintf("interface %s has no method %q", sname, name))
			return TUnknown
	}
	s.err(fmt.Sprintf("cannot select %q on %s", name, typeStr(bt)))
	return TUnknown
}

func (s *Sema) inferIndex(e []any, scope *Scope) Type {
	bt := s.infer(e[1], scope)
	it := s.infer(e[2], scope)
	if !typesEqual(it, TUnknown) && !typesEqual(it, TInt) {
		s.err(fmt.Sprintf("index must be int, got %s", typeStr(it)))
	}
	if typesEqual(bt, TUnknown) {
		return TUnknown
	}
	if isKind(bt, "list") {
		return bt[1].(Type)
	}
	if typesEqual(bt, TString) {
		return TByte
	}
	if isKind(bt, "set") {
		s.err("cannot index a set (sets are unordered)")
		return bt[1].(Type)
	}
	s.err(fmt.Sprintf("cannot index %s", typeStr(bt)))
	return TUnknown
}

func (s *Sema) inferSlice(e []any, scope *Scope) Type {
	bt := s.infer(e[1], scope)
	if e[2] != nil {
		lt := s.infer(e[2], scope)
		if !typesEqual(lt, TUnknown) && !typesEqual(lt, TInt) {
			s.err(fmt.Sprintf("slice bounds must be int, got %s", typeStr(lt)))
		}
	}
	if e[3] != nil {
		ht := s.infer(e[3], scope)
		if !typesEqual(ht, TUnknown) && !typesEqual(ht, TInt) {
			s.err(fmt.Sprintf("slice bounds must be int, got %s", typeStr(ht)))
		}
	}
	if typesEqual(bt, TUnknown) {
		return TUnknown
	}
	if typesEqual(bt, TString) {
		return TString
	}
	if isKind(bt, "list") {
		return bt
	}
	s.err(fmt.Sprintf("cannot slice %s", typeStr(bt)))
	return TUnknown
}

func (s *Sema) inferStructLit(e []any, scope *Scope) Type {
	name := e[1].(string)
	finits := e[2].([]any)
	info := s.Structs[name]
	if info == nil {
		s.err(fmt.Sprintf("unknown struct %q", name))
		for _, fi := range finits {
			f := fi.([]any)
			s.infer(f[2], scope)
		}
		return TUnknown
	}
	seen := map[string]bool{}
	for _, fi := range finits {
		f := fi.([]any)
		fname := f[1].(string)
		fval := f[2]
		if seen[fname] {
			s.err(fmt.Sprintf("duplicate field %q in %s literal", fname, name))
		}
		seen[fname] = true
		ft, ok := info.Fields[fname]
		if !ok {
			s.err(fmt.Sprintf("%s has no field %q", name, fname))
			s.infer(fval, scope)
			continue
		}
		vt := s.infer(fval, scope)
		if !typesEqual(vt, ft) {
			s.err(fmt.Sprintf("field %q: expected %s, got %s", fname, typeStr(ft), typeStr(vt)))
		}
	}
	for fname := range info.Fields {
		if !seen[fname] {
			s.err(fmt.Sprintf("missing field %q in %s literal", fname, name))
		}
	}
	return Type{"struct", name}
}

func (s *Sema) inferListLit(e []any, scope *Scope) Type {
	fullTy := s.resolveTypeNode(e[1])
	elemT := fullTy[1].(Type)
	for _, x := range e[2].([]any) {
		xt := s.infer(x, scope)
		if !typesEqual(xt, elemT) {
			s.err(fmt.Sprintf("list element: expected %s, got %s", typeStr(elemT), typeStr(xt)))
		}
	}
	return fullTy
}

func (s *Sema) inferSetLit(e []any, scope *Scope) Type {
	fullTy := s.resolveTypeNode(e[1])
	elemT := fullTy[1].(Type)
	for _, x := range e[2].([]any) {
		xt := s.infer(x, scope)
		if !typesEqual(xt, elemT) {
			s.err(fmt.Sprintf("set element: expected %s, got %s", typeStr(elemT), typeStr(xt)))
		}
	}
	return fullTy
}

func (s *Sema) inferComp(e []any, scope *Scope) Type {
	fullTy := s.resolveTypeNode(e[1])
	elemT := fullTy[1].(Type)
	varName := e[2].(string)
	srcNode := e[3]
	filt := e[4]
	stop := e[5]
	elem := e[6]

	srcN := srcNode.([]any)
	var srcT Type
	if srcN[0].(string) == "SeqExpr" {
		srcT = TInt
	} else {
		srcT = s.infer(srcNode, scope)
	}

	var iterT Type
	if typesEqual(srcT, TUnknown) {
		iterT = TUnknown
	} else if isKind(srcT, "list") || isKind(srcT, "set") {
		iterT = srcT[1].(Type)
	} else if typesEqual(srcT, TInt) && srcN[0].(string) == "SeqExpr" {
		iterT = TInt
	} else {
		s.err(fmt.Sprintf("cannot iterate over %s", typeStr(srcT)))
		iterT = TUnknown
	}

	inner := NewScope(scope)
	inner.Declare(&Symbol{Name: varName, Kind: "var", Type: iterT}, &s.Errors)

	et := s.infer(elem, inner)
	if !typesEqual(iterT, TUnknown) && !typesEqual(et, elemT) {
		s.err(fmt.Sprintf("comprehension element: expected %s, got %s",
				  typeStr(elemT), typeStr(et)))
	}

	if filt != nil {
		ft := s.infer(filt, inner)
		if !typesEqual(ft, TBool) {
			s.err(fmt.Sprintf("comprehension filter must be bool, got %s",
					  typeStr(ft)))
		}
	}
	if stop != nil {
		st := s.infer(stop, inner)
		if !typesEqual(st, TBool) {
			s.err(fmt.Sprintf("comprehension stop must be bool, got %s",
					  typeStr(st)))
		}
	}
	return fullTy
}

func (s *Sema) inferRangeLit(e []any, scope *Scope) Type {
	fullTy := s.resolveTypeNode(e[1])
	elemT := fullTy[1].(Type)
	lt := s.infer(e[2], scope)
	ht := s.infer(e[3], scope)
	if !typesEqual(lt, elemT) {
		s.err(fmt.Sprintf("range start: expected %s, got %s",
				  typeStr(elemT), typeStr(lt)))
	}
	if !typesEqual(ht, elemT) {
		s.err(fmt.Sprintf("range end: expected %s, got %s",
				  typeStr(elemT), typeStr(ht)))
	}
	return fullTy
}

func (s *Sema) inferFnLit(e []any, scope *Scope) Type {
	params := e[1].([]any)
	retNode := e[2]
	body := e[3]

	var ps []Type
	variadic := false
	for _, p := range params {
		pp := p.([]any)
		ps = append(ps, s.resolveTypeNode(pp[2]))
		if v, ok := pp[3].(bool); ok && v {
			variadic = true
		}
	}
	rt := TVoid
	if retNode != nil {
		rt = s.resolveTypeNode(retNode)
	}

	inner := NewScope(scope)
	for _, p := range params {
		pp := p.([]any)
		pty := s.resolveTypeNode(pp[2])
		if v, ok := pp[3].(bool); ok && v {
			pty = Type{"list", pty}
		}
		inner.Declare(&Symbol{Name: pp[1].(string), Kind: "var",
			Type: pty}, &s.Errors)
	}

	saved := s.CurrentRet
	s.CurrentRet = rt
	s.checkBlock(body, inner)
	s.CurrentRet = saved

	return Type{"fn", ps, rt, variadic}
}

func (s *Sema) litType(v any) Type {
	switch x := v.(type) {
		case bool:
			return TBool
		case float64:
			return TFloat
		case string:
			return TString
		case int:
			return Type{"int_const", x}
	}
	return TUnknown
}

func (s *Sema) compareLit(a, b any) (int, bool) {
	// Numeric: allow int <-> float mixing.
	if af, aok := toFloat(a); aok {
		if bf, bok := toFloat(b); bok {
			switch {
				case af < bf: return -1, true
				case af > bf: return 1, true
				default:      return 0, true
			}
		}
	}
	switch av := a.(type) {
	case string:
		if bv, ok := b.(string); ok {
			switch {
				case av < bv: return -1, true
				case av > bv: return 1, true
				default:      return 0, true
			}
		}
	case bool:
		if bv, ok := b.(bool); ok {
			switch {
				case !av && bv: return -1, true
				case av && !bv: return 1, true
				default:        return 0, true
			}
		}
	}
	return 0, false
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
		case int:
			return float64(x), true
		case float64:
			return x, true
	}
	return 0, false
}

type patternInterval struct {
	lo, hi      any  // literal values (comparable via s.compareLit)
	isRange     bool // false if it was a LitPattern
	hasWildcard bool // a wildcard absorbs everything after it
}

func (s *Sema) inferMatch(e []any, scope *Scope) Type {
	scrutinee := e[1]
	arms := e[2].([]any)

	st := s.infer(scrutinee, scope)
	seenWildcard := false
	var seen []patternInterval

	for _, a := range arms {
		arm := a.([]any)
		pat := arm[1].([]any)
		body := arm[2]

		switch pat[0].(string) {
			case "WildcardPattern":
				if seenWildcard {
					s.errAt(pat, "duplicate wildcard pattern in match")
				}
				seenWildcard = true
				// wildcard absorbs everything; subsequent patterns are unreachable
				seen = append(seen, patternInterval{hasWildcard: true})

			case "LitPattern":
				lt := s.litType(pat[1])
				if !typesEqual(lt, st) {
					s.errAt(pat, fmt.Sprintf(
						"match pattern type %s does not fit scrutinee type %s",
			      typeStr(lt), typeStr(st)))
				}
				seen = s.checkOverlap(seen, pat, pat[1], pat[1], false)

			case "RangePattern":
				lt := s.litType(pat[1])
				ht := s.litType(pat[2])
				if !typesEqual(lt, st) || !typesEqual(ht, st) {
					s.errAt(pat, fmt.Sprintf(
						"match range pattern types (%s..%s) do not fit scrutinee type %s",
								 typeStr(lt), typeStr(ht), typeStr(st)))
				}
				if cmp, ok := s.compareLit(pat[1], pat[2]); ok && cmp > 0 {
					s.errAt(pat, fmt.Sprintf(
						"match range pattern lower bound %v exceeds upper bound %v",
			      pat[1], pat[2]))
				}
				seen = s.checkOverlap(seen, pat, pat[1], pat[2], true)
		}

		if bn, ok := body.([]any); ok && bn[0].(string) == "Block" {
			s.checkBlock(body, NewScope(scope))
		} else {
			s.infer(body, scope)
		}
	}

	if !seenWildcard {
		s.errAt(e, "match must have a wildcard `_` arm")
	}
	return TVoid
}

// checkOverlap reports an error if [lo, hi] overlaps any previously seen
// pattern, and returns seen with [lo, hi] appended. The error is positioned
// at the pattern node `pat`.
//
// All intervals are treated as closed: both endpoints are included.
func (s *Sema) checkOverlap(seen []patternInterval, pat any, lo, hi any, isRange bool) []patternInterval {
	for _, prev := range seen {
		if prev.hasWildcard {
			s.errAt(pat, "pattern is unreachable: a wildcard `_` was already matched")
			return seen
		}

		// Closed intervals [lo,hi] and [prev.lo,prev.hi] are disjoint iff
		//     hi < prev.lo   OR   lo > prev.hi
		if c, ok := s.compareLit(hi, prev.lo); ok && c < 0 {
			continue
		}
		if c, ok := s.compareLit(lo, prev.hi); ok && c > 0 {
			continue
		}

		// Overlap (or endpoints not comparable → be conservative).
		if prev.isRange && isRange {
			s.errAt(pat, fmt.Sprintf("overlapping range patterns: %v..%v and %v..%v",
						 prev.lo, prev.hi, lo, hi))
		} else if isRange {
			// new pattern is a range, previous was a literal
			s.errAt(pat, fmt.Sprintf("range %v..%v overlaps literal %v",
						 lo, hi, prev.lo))
		} else if prev.isRange {
			// new pattern is a literal, previous was a range
			s.errAt(pat, fmt.Sprintf("literal %v overlaps range %v..%v",
						 lo, prev.lo, prev.hi))
		} else {
			// both are literals
			s.errAt(pat, fmt.Sprintf("duplicate literal pattern %v", lo))
		}
		return seen
	}
	return append(seen, patternInterval{lo: lo, hi: hi, isRange: isRange})
}
