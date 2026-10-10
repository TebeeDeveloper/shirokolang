package sema

import (
	"fmt"
	"reflect"
	"strings"

	"shiroko/core/color"
	"shiroko/core/source"
)

// ---------- errors ----------

type SemaError struct {
	Code string
	Msg  string
	Line int
	Col  int
	Src  *source.Source
}

func (e *SemaError) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s %s %s",
		    color.Yellow(fmt.Sprintf("line %d:%d", e.Line, e.Col)),
		    color.Dim("=>"),
		    color.Magenta("sema error"),
	)

	gutter := color.Dim("    |")

	if e.Src != nil {
		for _, it := range e.Src.Window(e.Line, 4, 4) {
			if it.Line == e.Line {
				fmt.Fprintf(&b, "\n%s %s", gutter, it.Text)

				col := e.Col
				if col < 1 {
					col = 1
				}
				pad := strings.Repeat(" ", col-1)
				carets := "^"
				if n := len(it.Text) - col; n > 0 {
					carets += strings.Repeat("~", n)
				}
				fmt.Fprintf(&b, "\n%s %s%s", gutter, pad, color.Red(carets))
			} else {
				fmt.Fprintf(&b, "\n%s %s", gutter, color.Dim(it.Text))
			}
		}
	}

	fmt.Fprintf(&b, "\n%s %s",
		    color.Dim("=>"),
		    color.Yellow("reason:")+" "+e.Msg,
	)
	return b.String()
}

func (e *SemaError) Error() string { return e.String() }

// nodePos returns the (line, col) that the parser appended to a node.
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

type Type = []any

var (
	TAny     = Type{"any"}
	TInt     = Type{"int"}
	TFloat   = Type{"float"}
	TString  = Type{"string"}
	TByte    = Type{"byte"}
	TBool    = Type{"bool"}
	TNil     = Type{"nil"}
	TVoid    = Type{"void"}
	TError   = Type{"error"}
	TUnknown = Type{"unknown"}
)

func isKind(t Type, k string) bool { return len(t) > 0 && t[0] == k }

func typeStr(t Type) string {
	switch t[0].(string) {
		case "int_const":
			return fmt.Sprintf("int literal %d", t[1].(int))
		case "any", "int", "float", "string", "bool", "byte", "nil", "void", "unknown", "error":
			return t[0].(string)
		case "list":
			return "[]" + typeStr(t[1].(Type))
		case "set":
			return "{}" + typeStr(t[1].(Type))
		case "tuple":
			elems := t[1].([]Type)
			var ss []string
			for _, e := range elems {
				ss = append(ss, typeStr(e))
			}
			return "(" + strings.Join(ss, ", ") + ")"
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
	if isKind(a, "nil") || isKind(b, "nil") {
		other := a
		if isKind(a, "nil") {
			other = b
		}
		return isKind(other, "nil") ||
		isKind(other, "list") ||
		isKind(other, "interface") ||
		isKind(other, "fn") ||
		isKind(other, "error")
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
	if isKind(a, "tuple") && isKind(b, "tuple") {
		ae := a[1].([]Type)
		be := b[1].([]Type)
		if len(ae) != len(be) {
			return false
		}
		for i := range ae {
			if !typesEqual(ae[i], be[i]) {
				return false
			}
		}
		return true
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
	if isKind(t, "nil") {
		return TAny
	}
	if isKind(t, "tuple") {
		elems := t[1].([]Type)
		out := make([]Type, len(elems))
		for i, e := range elems {
			out[i] = defaultType(e)
		}
		return Type{"tuple", out}
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
	Used  bool
	Param bool
}

type Scope struct {
	Symbols map[string]*Symbol
	Parent  *Scope
}

func NewScope(parent *Scope) *Scope {
	return &Scope{Symbols: map[string]*Symbol{}, Parent: parent}
}

func (s *Scope) Declare(sym *Symbol) *Symbol {
	if existing, ok := s.Symbols[sym.Name]; ok {
		return existing
	}
	s.Symbols[sym.Name] = sym
	return nil
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
	ErrType  Type // nil if not fallible
	Variadic bool
	Recv     string
}

func (f *FnSig) Fallible() bool { return f.ErrType != nil }

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
		"errorf":  Type{"fn", []Type{TAny}, TError, true},
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
	Methods    map[string]*FnSig
	Globals    *Scope
	CurrentRet Type
	CurrentErr Type // nil if enclosing function is not fallible
	Src        *source.Source
}

func New(src *source.Source) *Sema {
	s := &Sema{
		Structs:    map[string]*StructInfo{},
		Interfaces: map[string]*InterfaceInfo{},
		Funcs:      map[string]*FnSig{},
		Methods:    map[string]*FnSig{},
		Globals:    NewScope(nil),
		Src:        src,
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

func (s *Sema) errAt(n any, msg string) {
	e := &SemaError{Msg: msg, Src: s.Src}
	if line, col, ok := nodePos(n); ok {
		e.Line, e.Col = line, col
	}
	s.Errors = append(s.Errors, e)
}

func (s *Sema) errAtCode(n any, code, msg string) {
	e := &SemaError{Code: code, Msg: msg, Src: s.Src}
	if line, col, ok := nodePos(n); ok {
		e.Line, e.Col = line, col
	}
	s.Errors = append(s.Errors, e)
}

func (s *Sema) reportUnused(scope *Scope) {
	for _, sym := range scope.Symbols {
		if sym.Name == "_" {
			continue
		}
		if sym.Param {
			continue
		}
		switch sym.Kind {
			case "const", "func", "package":
				continue
		}
		if !sym.Used {
			s.errAtCode(sym.Node, "S_UNUSED",
				    fmt.Sprintf("%s declared and not used", sym.Name))
		}
	}
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
					s.errAt(d, fmt.Sprintf("type %q already declared", name))
					continue
				}
				if _, ok := s.Interfaces[name]; ok {
					s.errAt(d, fmt.Sprintf("type %q already declared", name))
					continue
				}
				s.Structs[name] = &StructInfo{Name: name, Fields: map[string]Type{}, Methods: map[string]*FnSig{}}
			case "InterfaceDecl":
				name := dd[1].(string)
				if _, ok := s.Structs[name]; ok {
					s.errAt(d, fmt.Sprintf("type %q already declared", name))
					continue
				}
				if _, ok := s.Interfaces[name]; ok {
					s.errAt(d, fmt.Sprintf("type %q already declared", name))
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
	return s.resolveTypeNodeAt(node, node)
}

func (s *Sema) resolveTypeNodeAt(node any, at any) Type {
	n := node.([]any)
	switch n[0].(string) {
		case "ListType":
			return Type{"list", s.resolveTypeNodeAt(n[1], at)}
		case "SetType":
			return Type{"set", s.resolveTypeNodeAt(n[1], at)}
		case "TupleType":
			elems := n[1].([]any)
			var ts []Type
			for _, e := range elems {
				ts = append(ts, s.resolveTypeNodeAt(e, at))
			}
			return Type{"tuple", ts}
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
		case "error":
			return TError
		case "any":
			return TAny
	}
	if _, ok := s.Structs[name]; ok {
		return Type{"struct", name}
	}
	if _, ok := s.Interfaces[name]; ok {
		return Type{"interface", name}
	}
	s.errAt(at, fmt.Sprintf("unknown type %q", name))
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
			s.errAt(f, fmt.Sprintf("field %q redeclared in %s", fname, name))
			continue
		}
		info.Fields[fname] = s.resolveTypeNodeAt(fty, f)
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
		var errNode any
		if len(sig) > 4 {
			errNode = sig[4]
		}
		if _, ok := info.Methods[mname]; ok {
			s.errAt(m, fmt.Sprintf("method %q redeclared in interface %s", mname, name))
			continue
		}
		var ps []Param
		variadic := false
		for _, p := range params {
			pp := p.([]any)
			ps = append(ps, Param{Name: pp[1].(string), Type: s.resolveTypeNodeAt(pp[2], p)})
			if v, ok := pp[3].(bool); ok && v {
				variadic = true
			}
		}
		rt := TVoid
		if retNode != nil {
			rt = s.resolveTypeNodeAt(retNode, m)
		}
		var et Type
		if errNode != nil {
			et = s.resolveTypeNodeAt(errNode, m)
		}
		info.Methods[mname] = &FnSig{Name: mname, Params: ps, Ret: rt, ErrType: et, Variadic: variadic, Recv: name}
	}
}

// ---------- pass 1c ----------

func (s *Sema) collectSignatures(decls []any) {
	for _, d := range decls {
		dd := d.([]any)
		switch dd[0].(string) {
			case "FnDecl":
				// ["FnDecl", name, params, ret, errTy, body]
				name := dd[1].(string)
				params := dd[2].([]any)
				retNode := dd[3]
				errNode := dd[4]
				if _, ok := s.Funcs[name]; ok {
					s.errAt(d, fmt.Sprintf("function %q already declared", name))
					continue
				}
				var ps []Param
				variadic := false
				for _, p := range params {
					pp := p.([]any)
					ps = append(ps, Param{Name: pp[1].(string), Type: s.resolveTypeNodeAt(pp[2], p)})
					if v, ok := pp[3].(bool); ok && v {
						variadic = true
					}
				}
				rt := TVoid
				if retNode != nil {
					rt = s.resolveTypeNodeAt(retNode, d)
				}
				var et Type
				if errNode != nil {
					et = s.resolveTypeNodeAt(errNode, d)
				}
				sig := &FnSig{Name: name, Params: ps, Ret: rt, ErrType: et, Variadic: variadic}
				s.Funcs[name] = sig
				if prev := s.Globals.Declare(&Symbol{Name: name, Kind: "func", Type: sig.Type()}); prev != nil {
					s.errAt(d, fmt.Sprintf("%q redeclared in this scope", name))
				}
			case "MethodDecl":
				// ["MethodDecl", recv, name, params, ret, errTy, body]
				recv := dd[1].(string)
				name := dd[2].(string)
				params := dd[3].([]any)
				retNode := dd[4]
				errNode := dd[5]
				if _, ok := s.Structs[recv]; !ok {
					if _, ok := s.Interfaces[recv]; !ok {
						s.errAt(d, fmt.Sprintf("receiver type %q not declared", recv))
						continue
					}
				}
				var ps []Param
				variadic := false
				for _, p := range params {
					pp := p.([]any)
					ps = append(ps, Param{Name: pp[1].(string), Type: s.resolveTypeNodeAt(pp[2], p)})
					if v, ok := pp[3].(bool); ok && v {
						variadic = true
					}
				}
				rt := TVoid
				if retNode != nil {
					rt = s.resolveTypeNodeAt(retNode, d)
				}
				var et Type
				if errNode != nil {
					et = s.resolveTypeNodeAt(errNode, d)
				}
				sig := &FnSig{Name: name, Params: ps, Ret: rt, ErrType: et, Variadic: variadic, Recv: recv}
				key := recv + "." + name
				if _, ok := s.Methods[key]; ok {
					s.errAt(d, fmt.Sprintf("method %s.%s already declared", recv, name))
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
				body := dd[5]
				sig, ok := s.Funcs[name]
				if !ok {
					continue
				}
				s.CurrentRet = sig.Ret
				s.CurrentErr = sig.ErrType
				scope := NewScope(s.Globals)
				for _, p := range params {
					pp := p.([]any)
					pname := pp[1].(string)
					if prev := scope.Declare(&Symbol{
						Name: pname, Kind: "var", Param: true, Node: p,
						Type: s.resolveTypeNodeAt(pp[2], p),
					}); prev != nil {
						s.errAt(p, fmt.Sprintf("%q redeclared in this scope", pname))
					}
				}
				s.checkBlock(body, scope)
				s.CurrentRet = nil
				s.CurrentErr = nil
			case "MethodDecl":
				recv := dd[1].(string)
				name := dd[2].(string)
				params := dd[3].([]any)
				body := dd[6]
				sig, ok := s.Methods[recv+"."+name]
				if !ok {
					continue
				}
				s.CurrentRet = sig.Ret
				s.CurrentErr = sig.ErrType
				scope := NewScope(s.Globals)
				var recvType Type
				if _, ok := s.Structs[recv]; ok {
					recvType = Type{"struct", recv}
				} else {
					recvType = Type{"interface", recv}
				}
				if prev := scope.Declare(&Symbol{
					Name: "self", Kind: "var", Param: true, Node: d,
					Type: recvType,
				}); prev != nil {
					s.errAt(d, `"self" redeclared in this scope`)
				}
				for _, p := range params {
					pp := p.([]any)
					pname := pp[1].(string)
					if prev := scope.Declare(&Symbol{
						Name: pname, Kind: "var", Param: true, Node: p,
						Type: s.resolveTypeNodeAt(pp[2], p),
					}); prev != nil {
						s.errAt(p, fmt.Sprintf("%q redeclared in this scope", pname))
					}
				}
				s.checkBlock(body, scope)
				s.CurrentRet = nil
				s.CurrentErr = nil
		}
	}
}

func (s *Sema) checkBlock(block any, scope *Scope) {
	stmts := block.([]any)[1].([]any)
	for _, st := range stmts {
		s.checkStmt(st, scope)
	}
	s.reportUnused(scope)
}

func (s *Sema) checkStmt(stmt any, scope *Scope) {
	st := stmt.([]any)
	switch st[0].(string) {
		case "LetStmt":
			names := st[1].([]string)
			tyNode := st[2]
			expr := st[3]
			els := st[4]

			// let-else: the else block runs on failure of the RHS. It
			// is checked in a scope derived from the OUTER scope (the
			// success binding is not visible), plus a special `err`
			// symbol of type `error` bound to the failure value.
			if els != nil {
				if len(names) != 1 {
					s.errAt(stmt, "let-else requires exactly one name")
				}
				if expr == nil {
					s.errAt(stmt, "let-else requires an initializer")
				} else if !s.isFallibleCall(expr, scope) {
					s.errAtCode(stmt, "S_ERR_LET_RHS",
						    "let-else RHS must be a call to a function marked `! error`")
				}
				elsScope := NewScope(scope)
				elsScope.Declare(&Symbol{
					Name:  "err",
					Kind:  "var",
					Type:  TError,
					Node:  stmt,
					Param: true, // suppress unused warning
				})
				s.checkBlock(els, elsScope)
			}

			var varTypes []Type

			if tyNode != nil {
				tn := tyNode.([]any)
				if tn[0].(string) == "TupleType" {
					for _, e := range tn[1].([]any) {
						varTypes = append(varTypes, s.resolveTypeNodeAt(e, tyNode))
					}
				} else {
					varTypes = []Type{s.resolveTypeNodeAt(tyNode, stmt)}
				}
				if expr != nil {
					et := s.infer(expr, scope)
					if len(varTypes) == 1 {
						if !typesEqual(et, varTypes[0]) {
							s.errAt(stmt, fmt.Sprintf("cannot assign %s to %s",
										  typeStr(et), typeStr(varTypes[0])))
						}
					} else if !typesEqual(et, TUnknown) {
						if !isKind(et, "tuple") {
							s.errAt(stmt, fmt.Sprintf("cannot destructure %s into %d variables",
										  typeStr(et), len(varTypes)))
						} else {
							elems := et[1].([]Type)
							if len(elems) != len(varTypes) {
								s.errAt(stmt, fmt.Sprintf("expected %d values, got %d",
											  len(varTypes), len(elems)))
							} else {
								for i := range varTypes {
									if !typesEqual(elems[i], varTypes[i]) {
										s.errAt(stmt, fmt.Sprintf("value %d: cannot assign %s to %s",
													  i+1, typeStr(elems[i]), typeStr(varTypes[i])))
									}
								}
							}
						}
					}
				}
			} else if expr != nil {
				et := s.infer(expr, scope)
				if len(names) == 1 {
					varTypes = []Type{defaultType(et)}
				} else if typesEqual(et, TUnknown) {
					for range names {
						varTypes = append(varTypes, TUnknown)
					}
				} else if !isKind(et, "tuple") {
					s.errAt(stmt, fmt.Sprintf("cannot destructure non-tuple %s into %d variables",
								  typeStr(et), len(names)))
					for range names {
						varTypes = append(varTypes, TUnknown)
					}
				} else {
					elems := et[1].([]Type)
					if len(elems) != len(names) {
						s.errAt(stmt, fmt.Sprintf("expected %d values, got %d",
									  len(names), len(elems)))
						for range names {
							varTypes = append(varTypes, TUnknown)
						}
					} else {
						for _, e := range elems {
							varTypes = append(varTypes, defaultType(e))
						}
					}
				}
			} else {
				s.errAt(stmt, fmt.Sprintf("let %s needs a type or initializer",
							  strings.Join(names, ", ")))
				for range names {
					varTypes = append(varTypes, TUnknown)
				}
			}

			if len(varTypes) != len(names) {
				varTypes = varTypes[:0]
				for range names {
					varTypes = append(varTypes, TUnknown)
				}
			}

			for i, n := range names {
				if n == "_" {
					continue
				}
				if prev := scope.Declare(&Symbol{
					Name: n, Kind: "var", Type: varTypes[i], Node: stmt,
				}); prev != nil {
					s.errAt(stmt, fmt.Sprintf("%q redeclared in this scope", n))
				}
			}

			case "ConstStmt":
				name := st[1].(string)
				expr := st[2]
				ty := defaultType(s.infer(expr, scope))
				if prev := scope.Declare(&Symbol{
					Name: name, Kind: "const", Type: ty, Const: true, Node: stmt,
				}); prev != nil {
					s.errAt(stmt, fmt.Sprintf("%q redeclared in this scope", name))
				}

			case "AssignStmt":
				tgt := st[1]
				expr := st[2]
				et := s.infer(expr, scope)
				tt := s.checkLValue(tgt, scope)
				if !typesEqual(et, tt) {
					s.errAt(stmt, fmt.Sprintf("cannot assign %s to %s", typeStr(et), typeStr(tt)))
				}

			case "ExprStmt":
				s.infer(st[1], scope)

			case "ReturnStmt":
				s.checkReturn(stmt, scope)

			case "IncDecStmt":
				name := st[1].(string)
				sym := scope.Lookup(name)
				if sym == nil {
					s.errAt(stmt, fmt.Sprintf("undefined: %s", name))
				} else if sym.Const {
					s.errAt(stmt, fmt.Sprintf("cannot modify const %q", name))
				} else if !isNumeric(sym.Type) {
					s.errAt(stmt, fmt.Sprintf("cannot increment %s", typeStr(sym.Type)))
				} else {
					sym.Used = true
				}

			case "IfStmt":
				cond := st[1]
				thenBlock := st[2]
				els := st[3]
				ct := s.infer(cond, scope)
				if !typesEqual(ct, TBool) {
					s.errAt(stmt, fmt.Sprintf("if condition must be bool, got %s", typeStr(ct)))
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
				if prev := inner.Declare(&Symbol{
					Name: varName, Kind: "var", Type: it, Node: stmt,
				}); prev != nil {
					s.errAt(stmt, fmt.Sprintf("%q redeclared in this scope", varName))
				}
				ct := s.infer(cond, inner)
				if !typesEqual(ct, TBool) {
					s.errAt(stmt, fmt.Sprintf("for condition must be bool, got %s", typeStr(ct)))
				}
				s.checkForPost(post, inner)
				s.checkBlock(body, inner)

			case "ForRangeStmt":
				count := st[1]
				body := st[2]
				ct := s.infer(count, scope)
				if !typesEqual(ct, TInt) {
					s.errAt(stmt, fmt.Sprintf("range count must be int, got %s", typeStr(ct)))
				}
				s.checkBlock(body, NewScope(scope))

			case "ForCondStmt":
				cond := st[1]
				body := st[2]
				ct := s.infer(cond, scope)
				if !typesEqual(ct, TBool) {
					s.errAt(stmt, fmt.Sprintf("for condition must be bool, got %s", typeStr(ct)))
				}
				s.checkBlock(body, NewScope(scope))

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
						s.errAt(stmt, fmt.Sprintf("cannot iterate over %s", typeStr(st_)))
						elemT = TUnknown
					}
				}
				inner := NewScope(scope)
				if idx != nil {
					iname := idx.(string)
					if prev := inner.Declare(&Symbol{
						Name: iname, Kind: "var", Type: TInt, Node: stmt,
					}); prev != nil {
						s.errAt(stmt, fmt.Sprintf("%q redeclared in this scope", iname))
					}
				}
				if prev := inner.Declare(&Symbol{
					Name: val, Kind: "var", Type: elemT, Node: stmt,
				}); prev != nil {
					s.errAt(stmt, fmt.Sprintf("%q redeclared in this scope", val))
				}
				s.checkBlock(body, inner)

			default:
				s.errAt(stmt, fmt.Sprintf("unknown statement kind %q", st[0].(string)))
	}
}

// checkReturn validates a `return` against the enclosing function.
// In a fallible function (`! error`) the user can write either:
//
//	return <success values...>              // implicit nil error
//	return <success values...>, <errExpr>   // explicit error
//
// A bare `return` in a fallible function re-raises (or returns nil err).
func (s *Sema) checkReturn(stmt any, scope *Scope) {
	st := stmt.([]any)
	exprs := st[1].([]any)

	if s.CurrentRet == nil {
		s.errAt(stmt, "return outside function")
		return
	}

	tupleRet := isKind(s.CurrentRet, "tuple")
	var wants []Type
	if tupleRet {
		wants = s.CurrentRet[1].([]Type)
	} else if typesEqual(s.CurrentRet, TVoid) {
		wants = nil
	} else {
		wants = []Type{s.CurrentRet}
	}

	fallible := s.CurrentErr != nil

	if len(exprs) == 0 {
		if fallible {
			return
		}
		if len(wants) > 0 {
			s.errAt(stmt, fmt.Sprintf("missing return value(s) (expected %d)", len(wants)))
		}
		return
	}

	valueExprs := exprs
	var errExpr any
	if fallible && len(exprs) == len(wants)+1 {
		valueExprs = exprs[:len(wants)]
		errExpr = exprs[len(wants)]
	}

	if len(valueExprs) != len(wants) {
		if fallible {
			s.errAt(stmt, fmt.Sprintf(
				"expected %d or %d return value(s), got %d",
						  len(wants), len(wants)+1, len(exprs)))
		} else {
			s.errAt(stmt, fmt.Sprintf("expected %d return value(s), got %d",
						  len(wants), len(exprs)))
		}
		for _, e := range exprs {
			s.infer(e, scope)
		}
		return
	}

	for i, e := range valueExprs {
		et := s.infer(e, scope)
		want := wants[i]

		if !tupleRet && typesEqual(want, TFloat) {
			switch {
				case typesEqual(et, TInt):
					s.errAtCode(stmt, "S_INT_TO_FLOAT",
						    "cannot return int from function returning float")
					continue
				case isKind(et, "int_const"):
					continue
				case isIntegerLike(et):
					s.errAtCode(stmt, "S_INTLIKE_TO_FLOAT",
						    fmt.Sprintf("cannot return %s from function returning float",
								typeStr(et)))
					continue
			}
		}

		if !typesEqual(et, want) {
			if tupleRet {
				s.errAt(e, fmt.Sprintf("return value %d: cannot return %s, expected %s",
						       i+1, typeStr(et), typeStr(want)))
			} else {
				s.errAt(stmt, fmt.Sprintf("cannot return %s from function returning %s",
							  typeStr(et), typeStr(s.CurrentRet)))
			}
		}
	}

	if errExpr != nil {
		et := s.infer(errExpr, scope)
		if !typesEqual(et, s.CurrentErr) {
			s.errAt(errExpr, fmt.Sprintf(
				"cannot return %s as error, expected %s",
				typeStr(et), typeStr(s.CurrentErr)))
		}
	}
}

// isFallibleCall reports whether the given expression is a call to a
// function whose signature is marked `! error`.
func (s *Sema) isFallibleCall(e any, scope *Scope) bool {
	n, ok := e.([]any)
	if !ok || len(n) == 0 {
		return false
	}
	if n[0].(string) != "CallExpr" {
		return false
	}
	callee := n[1].([]any)
	switch callee[0].(string) {
		case "IdentExpr":
			name := callee[1].(string)
			if sig, ok := s.Funcs[name]; ok {
				return sig.Fallible()
			}
			return false
		case "SelectorExpr":
			recvExpr := callee[1].([]any)
			mname := callee[2].(string)
			if recvExpr[0].(string) == "IdentExpr" {
				rname := recvExpr[1].(string)
				if info, ok := s.Structs[rname]; ok {
					if sig, ok := info.Methods[mname]; ok {
						return sig.Fallible()
					}
				}
				if info, ok := s.Interfaces[rname]; ok {
					if sig, ok := info.Methods[mname]; ok {
						return sig.Fallible()
					}
				}
			}
			rt := s.infer(recvExpr, scope)
			if isKind(rt, "struct") {
				if info, ok := s.Structs[rt[1].(string)]; ok {
					if sig, ok := info.Methods[mname]; ok {
						return sig.Fallible()
					}
				}
			}
			if isKind(rt, "interface") {
				if info, ok := s.Interfaces[rt[1].(string)]; ok {
					if sig, ok := info.Methods[mname]; ok {
						return sig.Fallible()
					}
				}
			}
	}
	return false
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
				s.errAt(tgt, fmt.Sprintf("undefined: %s", name))
				return TUnknown
			}
			if sym.Const {
				s.errAt(tgt, fmt.Sprintf("cannot assign to const %q", name))
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
			s.errAt(tgt, fmt.Sprintf("cannot assign to %q", t[2].(string)))
			return TUnknown
		case "IndexExpr":
			bt := s.infer(t[1], scope)
			if isKind(bt, "list") {
				return bt[1].(Type)
			}
			s.errAt(tgt, "cannot assign through index")
			return TUnknown
	}
	s.errAt(tgt, "invalid assignment target")
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
		case "NilExpr":
			return TNil
		case "IdentExpr":
			return s.inferIdent(n, e, scope)
		case "UnaryExpr":
			return s.inferUnary(n, e, scope)
		case "BinaryExpr":
			return s.inferBinary(n, e, scope)
		case "CallExpr":
			return s.inferCall(n, e, scope)
		case "SelectorExpr":
			return s.inferSelector(n, e, scope)
		case "IndexExpr":
			return s.inferIndex(n, e, scope)
		case "SliceExpr":
			return s.inferSlice(n, e, scope)
		case "StructLitExpr":
			return s.inferStructLit(n, e, scope)
		case "ListLitExpr":
			return s.inferListLit(n, e, scope)
		case "SetLitExpr":
			return s.inferSetLit(n, e, scope)
		case "ListCompExpr", "SetCompExpr":
			return s.inferComp(n, e, scope)
		case "PostfixExpr":
			t := s.infer(n[2], scope)
			if !isNumeric(t) {
				s.errAt(e, fmt.Sprintf("operator %q requires numeric, got %s", n[1].(string), typeStr(t)))
			}
			return t
		case "SpreadExpr":
			s.errAt(e, "spread '...' is only valid in call arguments")
			return TUnknown
		case "FnLitExpr":
			return s.inferFnLit(n, e, scope)
		case "MatchExpr":
			return s.inferMatch(n, e, scope)
		case "RangeLitExpr":
			return s.inferRangeLit(n, e, scope)
	}
	return TUnknown
}

func (s *Sema) inferIdent(n []any, at any, scope *Scope) Type {
	name := n[1].(string)
	sym := scope.Lookup(name)
	if sym == nil {
		s.errAt(at, fmt.Sprintf("undefined: %s", name))
		return TUnknown
	}
	sym.Used = true
	return sym.Type
}

func (s *Sema) inferUnary(n []any, at any, scope *Scope) Type {
	op := n[1].(string)
	t := s.infer(n[2], scope)
	switch op {
		case "-":
			if !isNumeric(t) {
				s.errAt(at, fmt.Sprintf("unary '-' expects numeric, got %s", typeStr(t)))
				return TUnknown
			}
			if isKind(t, "int_const") {
				return Type{"int_const", -t[1].(int)}
			}
			return t
		case "!":
			if !typesEqual(t, TBool) {
				s.errAt(at, fmt.Sprintf("unary '!' expects bool, got %s", typeStr(t)))
			}
			return TBool
	}
	return TUnknown
}

func (s *Sema) inferBinary(n []any, at any, scope *Scope) Type {
	op := n[1].(string)
	lt := s.infer(n[2], scope)
	rt := s.infer(n[3], scope)
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
			s.errAt(at, fmt.Sprintf("operator %q not defined for %s and %s", op, typeStr(lt), typeStr(rt)))
			return TUnknown
		case "%":
			if isIntegerLike(lt) && isIntegerLike(rt) {
				return TInt
			}
			s.errAt(at, fmt.Sprintf("operator '%%' requires ints, got %s, %s", typeStr(lt), typeStr(rt)))
			return TUnknown
		case "<", ">", "<=", ">=":
			if (isNumeric(lt) && isNumeric(rt)) ||
				(typesEqual(lt, TString) && typesEqual(rt, TString)) ||
				(isIntegerLike(lt) && isIntegerLike(rt)) {
					return TBool
				}
				s.errAt(at, fmt.Sprintf("operator %q requires matching numeric or string operands", op))
				return TBool
		case "==", "!=":
			if !(typesEqual(lt, rt) || (isIntegerLike(lt) && isIntegerLike(rt))) {
				s.errAt(at, fmt.Sprintf("cannot compare %s with %s", typeStr(lt), typeStr(rt)))
			}
			return TBool
		case "&&", "||":
			if !typesEqual(lt, TBool) {
				s.errAt(at, fmt.Sprintf("%q expects bool on left", op))
			}
			if !typesEqual(rt, TBool) {
				s.errAt(at, fmt.Sprintf("%q expects bool on right", op))
			}
			return TBool
	}
	return TUnknown
}

func (s *Sema) inferCall(n []any, at any, scope *Scope) Type {
	callee := n[1]
	args := n[2].([]any)

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
		s.errAt(at, fmt.Sprintf("cannot call %s", typeStr(calleeT)))
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
				s.errAt(a, fmt.Sprintf("spread requires list, got %s", typeStr(t)))
				argTypes = append(argTypes, TUnknown)
			} else {
				argTypes = append(argTypes, t[1].(Type))
			}
		} else {
			argTypes = append(argTypes, s.infer(a, scope))
		}
	}

	nargs := len(argTypes)
	if variadic {
		if nargs < len(params)-1 {
			s.errAt(at, fmt.Sprintf("expected at least %d args, got %d", len(params)-1, nargs))
		}
	} else if nargs != len(params) {
		s.errAt(at, fmt.Sprintf("expected %d args, got %d", len(params), nargs))
	}

	for i, at_ := range argTypes {
		var pt Type
		if variadic && i >= len(params)-1 {
			pt = params[len(params)-1]
		} else if i < len(params) {
			pt = params[i]
		} else {
			break
		}
		if !typesEqual(at_, pt) {
			s.errAt(at, fmt.Sprintf("arg %d: expected %s, got %s", i+1, typeStr(pt), typeStr(at_)))
		}
	}
	return ret
}

func (s *Sema) inferSelector(n []any, at any, scope *Scope) Type {
	name := n[2].(string)
	bt := s.infer(n[1], scope)
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
			s.errAt(at, fmt.Sprintf("package %q has no member %q", pkg, name))
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
			s.errAt(at, fmt.Sprintf("%s has no field or method %q", sname, name))
			return TUnknown
		case isKind(bt, "interface"):
			sname := bt[1].(string)
			info := s.Interfaces[sname]
			if info != nil {
				if sig, ok := info.Methods[name]; ok {
					return sig.Type()
				}
			}
			s.errAt(at, fmt.Sprintf("interface %s has no method %q", sname, name))
			return TUnknown
	}
	s.errAt(at, fmt.Sprintf("cannot select %q on %s", name, typeStr(bt)))
	return TUnknown
}

func (s *Sema) inferIndex(n []any, at any, scope *Scope) Type {
	bt := s.infer(n[1], scope)
	it := s.infer(n[2], scope)
	if !typesEqual(it, TUnknown) && !typesEqual(it, TInt) {
		s.errAt(at, fmt.Sprintf("index must be int, got %s", typeStr(it)))
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
		s.errAt(at, "cannot index a set (sets are unordered)")
		return bt[1].(Type)
	}
	s.errAt(at, fmt.Sprintf("cannot index %s", typeStr(bt)))
	return TUnknown
}

func (s *Sema) inferSlice(n []any, at any, scope *Scope) Type {
	bt := s.infer(n[1], scope)
	if n[2] != nil {
		lt := s.infer(n[2], scope)
		if !typesEqual(lt, TUnknown) && !typesEqual(lt, TInt) {
			s.errAt(at, fmt.Sprintf("slice bounds must be int, got %s", typeStr(lt)))
		}
	}
	if n[3] != nil {
		ht := s.infer(n[3], scope)
		if !typesEqual(ht, TUnknown) && !typesEqual(ht, TInt) {
			s.errAt(at, fmt.Sprintf("slice bounds must be int, got %s", typeStr(ht)))
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
	s.errAt(at, fmt.Sprintf("cannot slice %s", typeStr(bt)))
	return TUnknown
}

func (s *Sema) inferStructLit(n []any, at any, scope *Scope) Type {
	name := n[1].(string)
	finits := n[2].([]any)
	info := s.Structs[name]
	if info == nil {
		s.errAt(at, fmt.Sprintf("unknown struct %q", name))
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
			s.errAt(f, fmt.Sprintf("duplicate field %q in %s literal", fname, name))
		}
		seen[fname] = true
		ft, ok := info.Fields[fname]
		if !ok {
			s.errAt(f, fmt.Sprintf("%s has no field %q", name, fname))
			s.infer(fval, scope)
			continue
		}
		vt := s.infer(fval, scope)
		if !typesEqual(vt, ft) {
			s.errAt(f, fmt.Sprintf("field %q: expected %s, got %s", fname, typeStr(ft), typeStr(vt)))
		}
	}
	for fname := range info.Fields {
		if !seen[fname] {
			s.errAt(at, fmt.Sprintf("missing field %q in %s literal", fname, name))
		}
	}
	return Type{"struct", name}
}

func (s *Sema) inferListLit(n []any, at any, scope *Scope) Type {
	fullTy := s.resolveTypeNodeAt(n[1], at)
	elemT := fullTy[1].(Type)
	for _, x := range n[2].([]any) {
		xt := s.infer(x, scope)
		if !typesEqual(xt, elemT) {
			s.errAt(x, fmt.Sprintf("list element: expected %s, got %s", typeStr(elemT), typeStr(xt)))
		}
	}
	return fullTy
}

func (s *Sema) inferSetLit(n []any, at any, scope *Scope) Type {
	fullTy := s.resolveTypeNodeAt(n[1], at)
	elemT := fullTy[1].(Type)
	for _, x := range n[2].([]any) {
		xt := s.infer(x, scope)
		if !typesEqual(xt, elemT) {
			s.errAt(x, fmt.Sprintf("set element: expected %s, got %s", typeStr(elemT), typeStr(xt)))
		}
	}
	return fullTy
}

func (s *Sema) inferComp(n []any, at any, scope *Scope) Type {
	fullTy := s.resolveTypeNodeAt(n[1], at)
	elemT := fullTy[1].(Type)
	varName := n[2].(string)
	srcNode := n[3]
	filt := n[4]
	stop := n[5]
	elem := n[6]

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
		s.errAt(at, fmt.Sprintf("cannot iterate over %s", typeStr(srcT)))
		iterT = TUnknown
	}

	inner := NewScope(scope)
	if prev := inner.Declare(&Symbol{
		Name: varName, Kind: "var", Type: iterT, Node: at,
	}); prev != nil {
		s.errAt(at, fmt.Sprintf("%q redeclared in this scope", varName))
	}

	et := s.infer(elem, inner)
	if !typesEqual(iterT, TUnknown) && !typesEqual(et, elemT) {
		s.errAt(elem, fmt.Sprintf("comprehension element: expected %s, got %s",
					  typeStr(elemT), typeStr(et)))
	}

	if filt != nil {
		ft := s.infer(filt, inner)
		if !typesEqual(ft, TBool) {
			s.errAt(filt, fmt.Sprintf("comprehension filter must be bool, got %s", typeStr(ft)))
		}
	}
	if stop != nil {
		st := s.infer(stop, inner)
		if !typesEqual(st, TBool) {
			s.errAt(stop, fmt.Sprintf("comprehension stop must be bool, got %s", typeStr(st)))
		}
	}

	s.reportUnused(inner)
	return fullTy
}

func (s *Sema) inferRangeLit(n []any, at any, scope *Scope) Type {
	fullTy := s.resolveTypeNodeAt(n[1], at)
	elemT := fullTy[1].(Type)
	lt := s.infer(n[2], scope)
	ht := s.infer(n[3], scope)
	if !typesEqual(lt, elemT) {
		s.errAt(n[2], fmt.Sprintf("range start: expected %s, got %s", typeStr(elemT), typeStr(lt)))
	}
	if !typesEqual(ht, elemT) {
		s.errAt(n[3], fmt.Sprintf("range end: expected %s, got %s", typeStr(elemT), typeStr(ht)))
	}
	return fullTy
}

func (s *Sema) inferFnLit(n []any, at any, scope *Scope) Type {
	// ["FnLitExpr", params, ret, errTy, body]
	params := n[1].([]any)
	retNode := n[2]
	errNode := n[3]
	body := n[4]

	var ps []Type
	variadic := false
	for _, p := range params {
		pp := p.([]any)
		ps = append(ps, s.resolveTypeNodeAt(pp[2], p))
		if v, ok := pp[3].(bool); ok && v {
			variadic = true
		}
	}
	rt := TVoid
	if retNode != nil {
		rt = s.resolveTypeNodeAt(retNode, at)
	}
	var et Type
	if errNode != nil {
		et = s.resolveTypeNodeAt(errNode, at)
	}

	inner := NewScope(scope)
	for _, p := range params {
		pp := p.([]any)
		pty := s.resolveTypeNodeAt(pp[2], p)
		if v, ok := pp[3].(bool); ok && v {
			pty = Type{"list", pty}
		}
		pname := pp[1].(string)
		if prev := inner.Declare(&Symbol{
			Name: pname, Kind: "var", Param: true, Node: p, Type: pty,
		}); prev != nil {
			s.errAt(p, fmt.Sprintf("%q redeclared in this scope", pname))
		}
	}

	savedRet := s.CurrentRet
	savedErr := s.CurrentErr
	s.CurrentRet = rt
	s.CurrentErr = et
	s.checkBlock(body, inner)
	s.CurrentRet = savedRet
	s.CurrentErr = savedErr

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
	if af, aok := toFloat(a); aok {
		if bf, bok := toFloat(b); bok {
			switch {
				case af < bf:
					return -1, true
				case af > bf:
					return 1, true
				default:
					return 0, true
			}
		}
	}
	switch av := a.(type) {
		case string:
			if bv, ok := b.(string); ok {
				switch {
					case av < bv:
						return -1, true
					case av > bv:
						return 1, true
					default:
						return 0, true
				}
			}
					case bool:
						if bv, ok := b.(bool); ok {
							switch {
								case !av && bv:
									return -1, true
								case av && !bv:
									return 1, true
								default:
									return 0, true
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
	lo, hi      any
	isRange     bool
	hasWildcard bool
}

func (s *Sema) inferMatch(n []any, at any, scope *Scope) Type {
	scrutinee := n[1]
	arms := n[2].([]any)

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
		s.errAt(at, "match must have a wildcard `_` arm")
	}
	return TVoid
}

func (s *Sema) checkOverlap(seen []patternInterval, pat any, lo, hi any, isRange bool) []patternInterval {
	for _, prev := range seen {
		if prev.hasWildcard {
			s.errAt(pat, "pattern is unreachable: a wildcard `_` was already matched")
			return seen
		}
		if c, ok := s.compareLit(hi, prev.lo); ok && c < 0 {
			continue
		}
		if c, ok := s.compareLit(lo, prev.hi); ok && c > 0 {
			continue
		}
		if prev.isRange && isRange {
			s.errAt(pat, fmt.Sprintf("overlapping range patterns: %v..%v and %v..%v",
						 prev.lo, prev.hi, lo, hi))
		} else if isRange {
			s.errAt(pat, fmt.Sprintf("range %v..%v overlaps literal %v", lo, hi, prev.lo))
		} else if prev.isRange {
			s.errAt(pat, fmt.Sprintf("literal %v overlaps range %v..%v", lo, prev.lo, prev.hi))
		} else {
			s.errAt(pat, fmt.Sprintf("duplicate literal pattern %v", lo))
		}
		return seen
	}
	return append(seen, patternInterval{lo: lo, hi: hi, isRange: isRange})
}
