package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type Type interface {
	String() string
	Equals(other Type) bool
}

type PrimitiveType string

const (
	TypeInt    PrimitiveType = "int"
	TypeFloat  PrimitiveType = "float"
	TypeString PrimitiveType = "string"
	TypeBool   PrimitiveType = "bool"
	TypeUnit   PrimitiveType = "unit"
	TypeAny    PrimitiveType = "any"
)

func (t PrimitiveType) String() string { return string(t) }
func (t PrimitiveType) Equals(other Type) bool {
	o, ok := other.(PrimitiveType)
	return ok && t == o
}

type TupleType struct {
	Elems []Type
}

func (t *TupleType) String() string {
	if t == nil || len(t.Elems) == 0 {
		return "unit"
	}
	var parts []string
	for _, el := range t.Elems {
		if el == nil {
			parts = append(parts, "unknown")
		} else {
			parts = append(parts, el.String())
		}
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func (t *TupleType) Equals(other Type) bool {
	o, ok := other.(*TupleType)
	if !ok || len(t.Elems) != len(o.Elems) {
		return false
	}
	for i := range t.Elems {
		if t.Elems[i] == nil || o.Elems[i] == nil {
			if t.Elems[i] != o.Elems[i] {
				return false
			}
			continue
		}
		if !t.Elems[i].Equals(o.Elems[i]) {
			return false
		}
	}
	return true
}

type StructType struct {
	Name   string
	Fields map[string]Type
	Order  []string
}

func (s *StructType) String() string { return s.Name }
func (s *StructType) Equals(other Type) bool {
	o, ok := other.(*StructType)
	return ok && s == o
}

type ListType struct {
	Elem Type
}

func (l *ListType) String() string {
	if l == nil || l.Elem == nil {
		return "[]unknown"
	}
	return "[]" + l.Elem.String()
}

func (l *ListType) Equals(other Type) bool {
	o, ok := other.(*ListType)
	if !ok {
		return false
	}
	if l.Elem == nil || o.Elem == nil {
		return l.Elem == o.Elem
	}
	return l.Elem.Equals(o.Elem)
}

type FuncType struct {
	Params       []Type
	ReturnType   Type
	Variadic     bool
	VariadicElem Type
}

func (f *FuncType) String() string {
	var ps []string
	for _, p := range f.Params {
		ps = append(ps, p.String())
	}
	if f.Variadic {
		if f.VariadicElem != nil {
			ps = append(ps, "..."+f.VariadicElem.String())
		} else {
			ps = append(ps, "...any")
		}
	}
	return fmt.Sprintf("fn(%s) -> %s", strings.Join(ps, ", "), f.ReturnType.String())
}

func (f *FuncType) Equals(other Type) bool {
	o, ok := other.(*FuncType)
	if !ok || f.Variadic != o.Variadic || len(f.Params) != len(o.Params) {
		return false
	}
	for i := range f.Params {
		if !f.Params[i].Equals(o.Params[i]) {
			return false
		}
	}
	if f.Variadic {
		if f.VariadicElem == nil || o.VariadicElem == nil {
			if f.VariadicElem != o.VariadicElem {
				return false
			}
		} else if !f.VariadicElem.Equals(o.VariadicElem) {
			return false
		}
	}
	return f.ReturnType.Equals(o.ReturnType)
}

func isValueType(t Type) bool {
	if t == nil {
		return false
	}
	if p, ok := t.(PrimitiveType); ok && p == TypeUnit {
		return false
	}
	return true
}

func isAssignableStrict(expected Type, actual Type) bool {
	if expected == nil || actual == nil {
		return false
	}
	if !isValueType(actual) {
		return false
	}
	if p, ok := expected.(PrimitiveType); ok && p == TypeAny {
		return true
	}
	return expected.Equals(actual)
}

func blockAlwaysReturns(block *BlockStmt) bool {
	if block == nil {
		return false
	}
	for _, s := range block.Stmts {
		if stmtAlwaysReturns(s) {
			return true
		}
	}
	return false
}

func stmtAlwaysReturns(stmt Stmt) bool {
	if stmt == nil {
		return false
	}
	switch s := stmt.(type) {
	case *ReturnStmt:
		return true
	case *BlockStmt:
		return blockAlwaysReturns(s)
	case *IfStmt:
		if len(s.Branches) == 0 || s.ElseBody == nil {
			return false
		}
		for _, br := range s.Branches {
			if !blockAlwaysReturns(br.Body) {
				return false
			}
		}
		return blockAlwaysReturns(s.ElseBody)
	default:
		return false
	}
}

type SymbolKind int

const (
	SymVar SymbolKind = iota
	SymParam
	SymFunc
	SymModule
)

type Symbol struct {
	Name          string
	Kind          SymbolKind
	Type          Type
	Depth         int
	CapturedByRef bool
	IsConst       bool
}

type Scope struct {
	Parent  *Scope
	Depth   int
	Symbols map[string]*Symbol
	Structs map[string]*StructType
}

func NewScope(parent *Scope) *Scope {
	depth := 0
	if parent != nil {
		depth = parent.Depth + 1
	}
	return &Scope{
		Parent:  parent,
		Depth:   depth,
		Symbols: make(map[string]*Symbol),
		Structs: make(map[string]*StructType),
	}
}

func (s *Scope) DeclareSymbol(name string, kind SymbolKind, typ Type) (*Symbol, bool) {
	if _, exists := s.Symbols[name]; exists {
		return nil, false
	}
	sym := &Symbol{
		Name:  name,
		Kind:  kind,
		Type:  typ,
		Depth: s.Depth,
	}
	s.Symbols[name] = sym
	return sym, true
}

func (s *Scope) LookupSymbol(name string) (*Symbol, *Scope) {
	for cur := s; cur != nil; cur = cur.Parent {
		if sym, ok := cur.Symbols[name]; ok {
			return sym, cur
		}
	}
	return nil, nil
}

func (s *Scope) DeclareStruct(name string, st *StructType) bool {
	if _, exists := s.Structs[name]; exists {
		return false
	}
	s.Structs[name] = st
	return true
}

func (s *Scope) LookupStruct(name string) *StructType {
	for cur := s; cur != nil; cur = cur.Parent {
		if st, ok := cur.Structs[name]; ok {
			return st
		}
	}
	return nil
}

type ClosureCapture struct {
	Symbol    *Symbol
	OuterType Type
	Mutated   bool
}

type ClosureInfo struct {
	Captures map[string]*ClosureCapture
}

func newClosureInfo() *ClosureInfo {
	return &ClosureInfo{
		Captures: make(map[string]*ClosureCapture),
	}
}

type Analyzer struct {
	globalScope  *Scope
	currentScope *Scope
	registry     *ModuleRegistry
	errors       []string
	sourceLines  []string
	nodeStack    []Node
	exprTypes    map[Expr]Type
	callArgTypes map[*CallExpr][]Type
	closures     map[*FuncLit]*ClosureInfo
	returnStack  []Type
	closureStack []*ClosureInfo
	loopDepth    int
}

func NewAnalyzer() *Analyzer {
	return NewAnalyzerWithRegistry(DefaultModuleRegistry())
}

func NewAnalyzerWithRegistry(registry *ModuleRegistry) *Analyzer {
	if registry == nil {
		registry = NewModuleRegistry()
	}
	g := NewScope(nil)
	return &Analyzer{
		globalScope:  g,
		currentScope: g,
		registry:     registry,
		exprTypes:    make(map[Expr]Type),
		callArgTypes: make(map[*CallExpr][]Type),
		closures:     make(map[*FuncLit]*ClosureInfo),
	}
}

func (a *Analyzer) SetSource(src string) {
	if src == "" {
		a.sourceLines = nil
		return
	}
	a.sourceLines = strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
}

func (a *Analyzer) RegisterModule(def *ModuleDef) {
	a.registry.Register(def)
}

func (a *Analyzer) Errors() []string {
	return a.errors
}

func (a *Analyzer) ExprType(e Expr) Type {
	return a.exprTypes[e]
}

func (a *Analyzer) CallArgTypes(call *CallExpr) []Type {
	return a.callArgTypes[call]
}

func (a *Analyzer) ClosureCaptures(fn *FuncLit) *ClosureInfo {
	return a.closures[fn]
}

func extractNodePos(n Node) (line int, col int) {
	if n == nil {
		return 0, 0
	}
	return n.Line(), n.Column()
}

func analyzerSpanOnLine(lineText string, col int, spanHint int) (int, int) {
	lineRunes := []rune(lineText)
	lineLen := len(lineRunes)
	if lineLen == 0 {
		return 1, 1
	}
	if col < 1 {
		col = 1
	}
	if col > lineLen {
		col = lineLen
	}
	spanLen := spanHint
	if spanLen < 1 {
		spanLen = 1
	}
	if col-1+spanLen > lineLen {
		spanLen = lineLen - col + 1
	}
	if spanLen < 1 {
		spanLen = 1
	}
	return col, spanLen
}

func analyzerPointerLine(lineText string, startCol int, spanLen int) string {
	lineRunes := []rune(lineText)
	var prefix strings.Builder
	w := 0
	for i := 0; i < startCol-1 && i < len(lineRunes); i++ {
		r := lineRunes[i]
		if r == '\t' {
			step := 4 - (w % 4)
			prefix.WriteString(strings.Repeat(" ", step))
			w += step
		} else {
			prefix.WriteByte(' ')
			w++
		}
	}

	var mark strings.Builder
	for i := 0; i < spanLen; i++ {
		idx := startCol - 1 + i
		r := rune(' ')
		if idx < len(lineRunes) {
			r = lineRunes[idx]
		}
		step := 1
		if r == '\t' {
			step = 4 - (w % 4)
		}
		if i == 0 {
			mark.WriteByte('^')
			if step > 1 {
				mark.WriteString(strings.Repeat("~", step-1))
			}
		} else {
			mark.WriteString(strings.Repeat("~", step))
		}
		w += step
	}

	return prefix.String() + mark.String()
}

func (a *Analyzer) formatErrorWithContext(line int, col int, msg string) string {
	if line > 0 && line <= len(a.sourceLines) {
		lineText := a.sourceLines[line-1]
		startCol, spanLen := analyzerSpanOnLine(lineText, col, utf8.RuneCountInString(strings.TrimSpace(lineText)))
		if spanLen > 24 {
			spanLen = 1
		}

		var b strings.Builder
		fmt.Fprintf(&b, "error[line %d:%d]: %s", line, startCol, msg)

		numWidth := len(fmt.Sprintf("%d", line))
		if numWidth < 3 {
			numWidth = 3
		}
		gutterPad := strings.Repeat(" ", numWidth)

		if line-2 >= 0 && line-2 < len(a.sourceLines) {
			prevLine := strings.ReplaceAll(a.sourceLines[line-2], "\t", "    ")
			fmt.Fprintf(&b, "\n  %*d | %s", numWidth, line-1, prevLine)
		}

		displayLine := strings.ReplaceAll(lineText, "\t", "    ")
		pointer := analyzerPointerLine(lineText, startCol, spanLen)

		fmt.Fprintf(&b, "\n  %*d | %s", numWidth, line, displayLine)
		fmt.Fprintf(&b, "\n  %s | %s", gutterPad, pointer)
		return b.String()
	}

	if line > 0 {
		return fmt.Sprintf("error[line %d:%d]: %s", line, col, msg)
	}
	return fmt.Sprintf("error: %s", msg)
}

func (a *Analyzer) currentPos() (int, int) {
	for i := len(a.nodeStack) - 1; i >= 0; i-- {
		l, c := extractNodePos(a.nodeStack[i])
		if l > 0 {
			return l, c
		}
	}
	return 0, 0
}

func (a *Analyzer) addError(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	line, col := a.currentPos()
	a.errors = append(a.errors, a.formatErrorWithContext(line, col, msg))
}

func (a *Analyzer) addErrorAt(n Node, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	line, col := extractNodePos(n)
	if line <= 0 {
		line, col = a.currentPos()
	}
	a.errors = append(a.errors, a.formatErrorWithContext(line, col, msg))
}

func (a *Analyzer) enterScope() {
	a.currentScope = NewScope(a.currentScope)
}

func (a *Analyzer) exitScope() {
	if a.currentScope.Parent != nil {
		a.currentScope = a.currentScope.Parent
	}
}

func (a *Analyzer) resolveTypeName(name string) (Type, bool) {
	trimmed := strings.TrimSpace(name)
	switch trimmed {
	case "int":
		return TypeInt, true
	case "float":
		return TypeFloat, true
	case "string":
		return TypeString, true
	case "bool":
		return TypeBool, true
	case "unit":
		return TypeUnit, true
	case "any":
		return TypeAny, true
	}

	if strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")") {
		inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
		if inner == "" {
			return TypeUnit, true
		}
		rawElems := splitTopLevelComma(inner)
		if len(rawElems) == 1 {
			return a.resolveTypeName(rawElems[0])
		}
		var elems []Type
		for _, re := range rawElems {
			elStr := strings.TrimSpace(re)
			if elStr == "" {
				a.addError("invalid tuple type: %q", name)
				return nil, false
			}
			et, ok := a.resolveTypeName(elStr)
			if !ok || et == nil {
				return nil, false
			}
			elems = append(elems, et)
		}
		return &TupleType{Elems: elems}, true
	}

	if strings.HasPrefix(trimmed, "[]") {
		elemName := strings.TrimPrefix(trimmed, "[]")
		elemTyp, ok := a.resolveTypeName(elemName)
		if !ok {
			return nil, false
		}
		return &ListType{Elem: elemTyp}, true
	}

	if strings.HasPrefix(trimmed, "fn(") {
		if ft, ok := a.parseFuncType(trimmed); ok {
			return ft, true
		}
		a.addError("invalid function type: %q", name)
		return nil, false
	}

	if st := a.currentScope.LookupStruct(trimmed); st != nil {
		return st, true
	}
	a.addError("undeclared type: %q", name)
	return nil, false
}

func (a *Analyzer) parseFuncType(spec string) (*FuncType, bool) {
	s := strings.TrimSpace(spec)
	if !strings.HasPrefix(s, "fn(") {
		return nil, false
	}

	depth := 0
	closeIdx := -1
	for i := 2; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				closeIdx = i
			}
		}
		if closeIdx != -1 {
			break
		}
	}
	if closeIdx == -1 {
		return nil, false
	}

	paramsPart := strings.TrimSpace(s[3:closeIdx])
	rest := strings.TrimSpace(s[closeIdx+1:])

	var params []Type
	var variadic bool
	var variadicElem Type

	if paramsPart != "" {
		rawParams := splitTopLevelComma(paramsPart)
		for idx, rawParam := range rawParams {
			pStr := strings.TrimSpace(rawParam)
			if pStr == "" {
				return nil, false
			}
			if strings.HasPrefix(pStr, "...") {
				if idx != len(rawParams)-1 {
					return nil, false
				}
				elemStr := strings.TrimSpace(strings.TrimPrefix(pStr, "..."))
				if elemStr == "" {
					variadic = true
					variadicElem = TypeAny
					continue
				}
				vt, ok := a.resolveTypeName(elemStr)
				if !ok || vt == nil {
					return nil, false
				}
				variadic = true
				variadicElem = vt
				continue
			}
			pt, ok := a.resolveTypeName(pStr)
			if !ok || pt == nil {
				return nil, false
			}
			params = append(params, pt)
		}
	}

	var retType Type = TypeUnit
	if rest != "" {
		if !strings.HasPrefix(rest, "->") {
			return nil, false
		}
		retStr := strings.TrimSpace(strings.TrimPrefix(rest, "->"))
		if retStr == "" {
			return nil, false
		}
		rt, ok := a.resolveTypeName(retStr)
		if !ok || rt == nil {
			return nil, false
		}
		retType = rt
	}

	return &FuncType{
		Params:       params,
		ReturnType:   retType,
		Variadic:     variadic,
		VariadicElem: variadicElem,
	}, true
}

func splitTopLevelComma(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func (a *Analyzer) useSymbol(name string, isWrite bool) (*Symbol, Type) {
	sym, ownerScope := a.currentScope.LookupSymbol(name)
	if sym == nil {
		a.addError("undeclared variable or function: %q", name)
		return nil, nil
	}

	inClosureCapture := len(a.closureStack) > 0 && ownerScope.Depth < a.currentScope.Depth
	if isWrite && sym.IsConst && inClosureCapture {
		a.addError("cannot modify constant %q inside closure", name)
	}

	if inClosureCapture {
		sym.CapturedByRef = true
		for i := len(a.closureStack) - 1; i >= 0; i-- {
			ci := a.closureStack[i]
			capEntry, exists := ci.Captures[sym.Name]
			if !exists {
				capEntry = &ClosureCapture{
					Symbol:    sym,
					OuterType: sym.Type,
					Mutated:   isWrite,
				}
				ci.Captures[sym.Name] = capEntry
			} else if isWrite {
				capEntry.Mutated = true
			}
			if ownerScope.Depth == sym.Depth {
				break
			}
		}
	}

	return sym, sym.Type
}

func (a *Analyzer) Analyze(prog *Program) {
	if prog == nil {
		return
	}
	for _, item := range prog.Items {
		a.analyzeStmt(item)
	}
}

func (a *Analyzer) analyzeStmt(stmt Stmt) {
	if stmt == nil {
		return
	}
	a.nodeStack = append(a.nodeStack, stmt)
	defer func() { a.nodeStack = a.nodeStack[:len(a.nodeStack)-1] }()

	switch s := stmt.(type) {
	case *UseStmt:
		a.analyzeUseStmt(s)
	case *StructDecl:
		a.analyzeStructDecl(s)
	case *FuncDecl:
		a.analyzeFuncDecl(s)
	case *LetStmt:
		a.analyzeLetStmt(s)
	case *AssignStmt:
		a.analyzeAssignStmt(s)
	case *ReturnStmt:
		a.analyzeReturnStmt(s)
	case *IfStmt:
		a.analyzeIfStmt(s)
	case *ForStmt:
		a.analyzeForStmt(s)
	case *ForEachStmt:
		a.analyzeForEachStmt(s)
	case *BlockStmt:
		a.analyzeBlock(s)
	case *ExprStmt:
		a.analyzeExpr(s.Expr, false)
	case *ContinueStmt:
		if a.loopDepth == 0 {
			a.addErrorAt(s, "continue statement outside loop")
		}
	case *BreakStmt:
		if a.loopDepth == 0 {
			a.addErrorAt(s, "break statement outside loop")
		}
	default:
		a.addErrorAt(stmt, "unsupported statement: %T", stmt)
	}
}

func (a *Analyzer) analyzeUseStmt(stmt *UseStmt) {
	def, ok := a.registry.Get(stmt.Module)
	if !ok {
		a.addErrorAt(stmt, "module %q does not exist", stmt.Module)
		return
	}
	if _, declared := a.currentScope.DeclareSymbol(stmt.Module, SymModule, def.StaticType()); !declared {
		a.addErrorAt(stmt, "name %q is already declared in the same scope", stmt.Module)
	}
}

func (a *Analyzer) analyzeStructDecl(decl *StructDecl) {
	fields := make(map[string]Type, len(decl.Fields))
	order := make([]string, 0, len(decl.Fields))

	for _, f := range decl.Fields {
		if _, dup := fields[f.Name]; dup {
			a.addErrorAt(f, "duplicate field %q in struct %q", f.Name, decl.Name)
			continue
		}
		ft, ok := a.resolveTypeName(f.Type)
		if !ok {
			continue
		}
		if !isValueType(ft) {
			a.addErrorAt(f, "field %q of struct %q cannot have type %s", f.Name, decl.Name, ft.String())
			continue
		}
		fields[f.Name] = ft
		order = append(order, f.Name)
	}

	st := &StructType{
		Name:   decl.Name,
		Fields: fields,
		Order:  order,
	}
	if !a.currentScope.DeclareStruct(decl.Name, st) {
		a.addErrorAt(decl, "struct %q is already declared", decl.Name)
	}
}

func (a *Analyzer) buildFuncSignature(params []*Param, retTypeStr string) (paramTypes []Type, symParamTypes []Type, retType Type, variadic bool, variadicElem Type) {
	for i, p := range params {
		pt, ok := a.resolveTypeName(p.Type)
		if !ok {
			pt = TypeInt
		} else if !isValueType(pt) {
			a.addErrorAt(p, "parameter %q cannot have type %s", p.Name, pt.String())
		}

		if p.Variadic {
			if i != len(params)-1 {
				a.addErrorAt(p, "variadic parameter %q must be the last parameter", p.Name)
			}
			variadic = true
			if pt == nil {
				variadicElem = TypeAny
			} else {
				variadicElem = pt
			}
			symParamTypes = append(symParamTypes, &ListType{Elem: variadicElem})
			continue
		}

		paramTypes = append(paramTypes, pt)
		symParamTypes = append(symParamTypes, pt)
	}

	retType = TypeUnit
	if retTypeStr != "" {
		rt, ok := a.resolveTypeName(retTypeStr)
		if !ok {
			rt = TypeInt
		}
		retType = rt
	}
	return paramTypes, symParamTypes, retType, variadic, variadicElem
}

func (a *Analyzer) checkMissingReturn(body *BlockStmt, retType Type, fnLabel string, fallback Node) {
	if retType != nil && !retType.Equals(TypeUnit) && !blockAlwaysReturns(body) {
		target := fallback
		if target == nil || target.Line() <= 0 {
			target = body
		}
		a.addErrorAt(target, "%s expects return value of type %s on all paths", fnLabel, retType.String())
	}
}

func (a *Analyzer) analyzeFuncDecl(decl *FuncDecl) {
	paramTypes, symParamTypes, retType, variadic, variadicElem := a.buildFuncSignature(decl.Params, decl.ReturnType)

	fnType := &FuncType{
		Params:       paramTypes,
		ReturnType:   retType,
		Variadic:     variadic,
		VariadicElem: variadicElem,
	}

	if _, ok := a.currentScope.DeclareSymbol(decl.Name, SymFunc, fnType); !ok {
		a.addErrorAt(decl, "function %q is already declared in the same scope", decl.Name)
	}

	a.enterScope()
	seenParams := make(map[string]bool, len(decl.Params))
	for i, p := range decl.Params {
		if seenParams[p.Name] {
			a.addErrorAt(p, "duplicate parameter %q in function %q", p.Name, decl.Name)
			continue
		}
		seenParams[p.Name] = true
		a.currentScope.DeclareSymbol(p.Name, SymParam, symParamTypes[i])
	}

	savedLoopDepth := a.loopDepth
	a.loopDepth = 0

	a.returnStack = append(a.returnStack, retType)
	a.analyzeBlockBody(decl.Body)
	a.returnStack = a.returnStack[:len(a.returnStack)-1]

	a.loopDepth = savedLoopDepth

	a.checkMissingReturn(decl.Body, retType, fmt.Sprintf("function %q", decl.Name), decl)
	a.exitScope()
}

func (a *Analyzer) analyzeLetStmt(stmt *LetStmt) {
	initType := a.analyzeExpr(stmt.Init, false)

	names := stmt.Names
	if len(names) == 0 && stmt.Name != "" {
		names = []string{stmt.Name}
	}

	if len(names) > 1 {
		var elemTypes []Type
		if tup, ok := initType.(*TupleType); ok {
			elemTypes = tup.Elems
			if len(elemTypes) != len(names) {
				a.addErrorAt(stmt, "cannot bind %d values to %d variables", len(elemTypes), len(names))
			}
		} else if initType != nil {
			a.addErrorAt(stmt, "cannot destructure non-tuple type %s into %d variables", initType.String(), len(names))
		}

		for i, n := range names {
			var inferred Type
			if i < len(elemTypes) {
				inferred = elemTypes[i]
			}
			var declTyp Type
			if i < len(stmt.Types) && stmt.Types[i] != "" {
				if dt, ok := a.resolveTypeName(stmt.Types[i]); ok {
					declTyp = dt
				}
			}
			finalTyp := inferred
			if declTyp != nil {
				finalTyp = declTyp
				if inferred != nil && !isAssignableStrict(declTyp, inferred) {
					a.addErrorAt(stmt, "cannot assign value of type %s to variable %q of type %s",
						inferred.String(), n, declTyp.String())
				}
			}
			if finalTyp == nil {
				finalTyp = TypeInt
			}
			sym, ok := a.currentScope.DeclareSymbol(n, SymVar, finalTyp)
			if !ok {
				a.addErrorAt(stmt, "variable %q is already declared in the same scope", n)
			} else {
				sym.IsConst = stmt.IsConst
			}
		}
		return
	}

	singleName := stmt.Name
	if singleName == "" && len(names) == 1 {
		singleName = names[0]
	}

	if initType != nil && !isValueType(initType) {
		a.addErrorAt(stmt, "cannot initialize variable %q with expression of type %s", singleName, initType.String())
	}

	var finalType Type
	declTypeStr := stmt.Type
	if declTypeStr == "" && len(stmt.Types) == 1 {
		declTypeStr = stmt.Types[0]
	}
	if declTypeStr != "" {
		declaredType, ok := a.resolveTypeName(declTypeStr)
		if !ok {
			finalType = initType
		} else {
			finalType = declaredType
			if initType != nil && !isAssignableStrict(declaredType, initType) {
				a.addErrorAt(stmt, "cannot assign value of type %s to variable %q of type %s",
					initType.String(), singleName, declaredType.String())
			}
		}
	} else {
		finalType = initType
	}

	if finalType == nil {
		finalType = TypeInt
	}

	sym, ok := a.currentScope.DeclareSymbol(singleName, SymVar, finalType)
	if !ok {
		a.addErrorAt(stmt, "variable %q is already declared in the same scope", singleName)
	} else {
		sym.IsConst = stmt.IsConst
	}
}

func (a *Analyzer) analyzeAssignStmt(stmt *AssignStmt) {
	var targetType Type

	switch t := stmt.Target.(type) {
	case *IdentExpr:
		sym, typ := a.useSymbol(t.Name, true)
		if sym != nil {
			switch sym.Kind {
			case SymFunc:
				a.addErrorAt(stmt, "cannot reassign function %q", t.Name)
			case SymModule:
				a.addErrorAt(stmt, "cannot reassign module %q", t.Name)
			default:
				if sym.IsConst {
					a.addErrorAt(stmt, "cannot reassign constant %q", t.Name)
				}
			}
		}
		targetType = typ
		if typ != nil {
			a.exprTypes[t] = typ
		}
	case *FieldExpr:
		targetType = a.analyzeFieldExpr(t, true)
	case *IndexExpr:
		targetType = a.analyzeIndexExpr(t)
	default:
		a.addErrorAt(stmt, "assignment target must be a variable, field, or list index, got %T", stmt.Target)
		a.analyzeExpr(stmt.Target, false)
	}

	valType := a.analyzeExpr(stmt.Value, false)
	if targetType != nil && valType != nil {
		if !isValueType(valType) {
			a.addErrorAt(stmt.Value, "cannot assign value of type %s", valType.String())
		} else if !isAssignableStrict(targetType, valType) {
			a.addErrorAt(stmt.Value, "type mismatch in assignment: expected %s, got %s",
				targetType.String(), valType.String())
		}
	}
}

func (a *Analyzer) analyzeReturnStmt(stmt *ReturnStmt) {
	if len(a.returnStack) == 0 {
		a.addErrorAt(stmt, "return statement outside function")
		for _, v := range stmt.Values {
			a.analyzeExpr(v, false)
		}
		if len(stmt.Values) == 0 && stmt.Value != nil {
			a.analyzeExpr(stmt.Value, false)
		}
		return
	}

	expectedRet := a.returnStack[len(a.returnStack)-1]
	retExprs := stmt.Values
	if len(retExprs) == 0 && stmt.Value != nil {
		retExprs = []Expr{stmt.Value}
	}

	if len(retExprs) == 0 {
		if !expectedRet.Equals(TypeUnit) {
			a.addErrorAt(stmt, "function expects return value of type %s, got empty return", expectedRet.String())
		}
		return
	}

	if len(retExprs) > 1 {
		var elemTypes []Type
		hasNil := false
		for _, re := range retExprs {
			vt := a.analyzeExpr(re, false)
			if vt == nil {
				hasNil = true
				elemTypes = append(elemTypes, nil)
				continue
			}
			if !isValueType(vt) {
				a.addErrorAt(re, "cannot return expression of type %s", vt.String())
			}
			elemTypes = append(elemTypes, vt)
		}
		if expectedRet.Equals(TypeUnit) {
			a.addErrorAt(stmt, "function has unit return type but returns multiple values")
			return
		}
		tupRet, ok := expectedRet.(*TupleType)
		if !ok {
			gotTuple := &TupleType{Elems: elemTypes}
			a.addErrorAt(stmt, "return type mismatch: expected %s, got %s", expectedRet.String(), gotTuple.String())
			return
		}
		if len(tupRet.Elems) != len(elemTypes) {
			a.addErrorAt(stmt, "return value count mismatch: expected %d, got %d", len(tupRet.Elems), len(elemTypes))
			return
		}
		if !hasNil {
			for i := range tupRet.Elems {
				if elemTypes[i] != nil && !isAssignableStrict(tupRet.Elems[i], elemTypes[i]) {
					a.addErrorAt(retExprs[i], "return value %d type mismatch: expected %s, got %s",
						i+1, tupRet.Elems[i].String(), elemTypes[i].String())
				}
			}
		}
		return
	}

	valType := a.analyzeExpr(retExprs[0], false)
	if valType == nil {
		return
	}
	if !isValueType(valType) {
		a.addErrorAt(retExprs[0], "cannot return expression of type %s", valType.String())
		return
	}
	if expectedRet.Equals(TypeUnit) {
		a.addErrorAt(stmt, "function has unit return type but returns value of type %s", valType.String())
		return
	}
	if !isAssignableStrict(expectedRet, valType) {
		a.addErrorAt(retExprs[0], "return type mismatch: expected %s, got %s",
			expectedRet.String(), valType.String())
	}
}

func (a *Analyzer) analyzeIfStmt(stmt *IfStmt) {
	for _, br := range stmt.Branches {
		condType := a.analyzeExpr(br.Cond, false)
		if condType != nil && !condType.Equals(TypeBool) {
			a.addErrorAt(br.Cond, "if condition must have type bool, got %s", condType.String())
		}
		a.analyzeBlock(br.Body)
	}
	if stmt.ElseBody != nil {
		a.analyzeBlock(stmt.ElseBody)
	}
}

func (a *Analyzer) analyzeForStmt(stmt *ForStmt) {
	a.enterScope()
	if stmt.Init != nil {
		a.analyzeStmt(stmt.Init)
	}
	if stmt.Cond != nil {
		condType := a.analyzeExpr(stmt.Cond, false)
		if condType != nil && !condType.Equals(TypeBool) {
			a.addErrorAt(stmt.Cond, "for condition must have type bool, got %s", condType.String())
		}
	}
	a.loopDepth++
	if stmt.Body != nil {
		a.analyzeBlockBody(stmt.Body)
	}
	a.loopDepth--
	if stmt.Post != nil {
		a.analyzeStmt(stmt.Post)
	}
	a.exitScope()
}

func (a *Analyzer) analyzeForEachStmt(stmt *ForEachStmt) {
	iterTyp := a.analyzeExpr(stmt.Iterable, false)
	var elemTyp Type = TypeInt
	if iterTyp != nil {
		listTyp, ok := iterTyp.(*ListType)
		if !ok {
			a.addErrorAt(stmt.Iterable, "for-each iterable must be a list, got %s", iterTyp.String())
		} else if listTyp.Elem != nil {
			elemTyp = listTyp.Elem
		}
	}

	a.enterScope()
	if _, ok := a.currentScope.DeclareSymbol(stmt.VarName, SymVar, elemTyp); !ok {
		a.addErrorAt(stmt, "variable %q is already declared in the same scope", stmt.VarName)
	}
	a.loopDepth++
	a.analyzeBlockBody(stmt.Body)
	a.loopDepth--
	a.exitScope()
}

func (a *Analyzer) analyzeBlock(block *BlockStmt) {
	if block == nil {
		return
	}
	a.enterScope()
	a.analyzeBlockBody(block)
	a.exitScope()
}

func (a *Analyzer) analyzeBlockBody(block *BlockStmt) {
	if block == nil {
		return
	}
	for _, s := range block.Stmts {
		a.analyzeStmt(s)
	}
}

func (a *Analyzer) analyzeExpr(expr Expr, isWriteRoot bool) Type {
	if expr == nil {
		return nil
	}
	a.nodeStack = append(a.nodeStack, expr)
	defer func() { a.nodeStack = a.nodeStack[:len(a.nodeStack)-1] }()

	var typ Type
	switch e := expr.(type) {
	case *IntLit:
		typ = TypeInt
	case *FloatLit:
		typ = TypeFloat
	case *StringLit:
		typ = TypeString
	case *BoolLit:
		typ = TypeBool
	case *IdentExpr:
		_, typ = a.useSymbol(e.Name, isWriteRoot)
	case *StructLit:
		typ = a.analyzeStructLit(e)
	case *ListLit:
		typ = a.analyzeListLit(e)
	case *IndexExpr:
		typ = a.analyzeIndexExpr(e)
	case *FuncLit:
		typ = a.analyzeFuncLit(e)
	case *UnaryExpr:
		typ = a.analyzeUnaryExpr(e)
	case *BinaryExpr:
		typ = a.analyzeBinaryExpr(e)
	case *CallExpr:
		typ = a.analyzeCallExpr(e)
	case *FieldExpr:
		typ = a.analyzeFieldExpr(e, isWriteRoot)
	default:
		a.addErrorAt(expr, "unsupported expression: %T", expr)
	}

	if typ != nil {
		a.exprTypes[expr] = typ
	}
	return typ
}

func (a *Analyzer) analyzeListLit(lit *ListLit) Type {
	elemTyp, ok := a.resolveTypeName(lit.ElemTypeName)
	if !ok {
		return nil
	}

	if lit.RepeatVal != nil || lit.RepeatCount != nil {
		valTyp := a.analyzeExpr(lit.RepeatVal, false)
		cntTyp := a.analyzeExpr(lit.RepeatCount, false)
		if valTyp != nil && !isAssignableStrict(elemTyp, valTyp) {
			a.addErrorAt(lit.RepeatVal, "list repeat value type mismatch: expected %s, got %s", elemTyp.String(), valTyp.String())
		}
		if cntTyp != nil && !cntTyp.Equals(TypeInt) {
			a.addErrorAt(lit.RepeatCount, "list repeat count must be int, got %s", cntTyp.String())
		}
		return &ListType{Elem: elemTyp}
	}

	for _, el := range lit.Elements {
		elTyp := a.analyzeExpr(el, false)
		if elTyp != nil && !isAssignableStrict(elemTyp, elTyp) {
			a.addErrorAt(el, "list element type mismatch: expected %s, got %s", elemTyp.String(), elTyp.String())
		}
	}

	return &ListType{Elem: elemTyp}
}

func (a *Analyzer) analyzeIndexExpr(expr *IndexExpr) Type {
	targetTyp := a.analyzeExpr(expr.Target, false)
	idxTyp := a.analyzeExpr(expr.Index, false)
	if targetTyp == nil {
		return nil
	}

	listTyp, ok := targetTyp.(*ListType)
	if !ok {
		a.addErrorAt(expr.Target, "cannot index non-list type %s", targetTyp.String())
		return nil
	}

	if idxTyp != nil && !idxTyp.Equals(TypeInt) {
		a.addErrorAt(expr.Index, "list index must be int, got %s", idxTyp.String())
	}

	return listTyp.Elem
}

func (a *Analyzer) analyzeStructLit(lit *StructLit) Type {
	st := a.currentScope.LookupStruct(lit.TypeName)
	if st == nil {
		a.addErrorAt(lit, "undeclared struct %q", lit.TypeName)
		for _, f := range lit.Fields {
			a.analyzeExpr(f.Value, false)
		}
		return nil
	}

	seen := make(map[string]bool, len(lit.Fields))
	for _, f := range lit.Fields {
		valType := a.analyzeExpr(f.Value, false)
		if seen[f.Name] {
			a.addErrorAt(f, "duplicate field %q in struct literal %q", f.Name, lit.TypeName)
			continue
		}
		seen[f.Name] = true

		expectedFieldType, ok := st.Fields[f.Name]
		if !ok {
			a.addErrorAt(f, "struct %q has no field %q", lit.TypeName, f.Name)
			continue
		}
		if valType != nil && !isAssignableStrict(expectedFieldType, valType) {
			a.addErrorAt(f.Value, "field %q of %q expects type %s, got %s",
				f.Name, lit.TypeName, expectedFieldType.String(), valType.String())
		}
	}

	for _, fieldName := range st.Order {
		if !seen[fieldName] {
			a.addErrorAt(lit, "missing initializer for field %q in struct %q", fieldName, lit.TypeName)
		}
	}

	return st
}

func (a *Analyzer) analyzeFuncLit(lit *FuncLit) Type {
	paramTypes, symParamTypes, retType, variadic, variadicElem := a.buildFuncSignature(lit.Params, lit.ReturnType)

	fnType := &FuncType{
		Params:       paramTypes,
		ReturnType:   retType,
		Variadic:     variadic,
		VariadicElem: variadicElem,
	}

	ci := newClosureInfo()
	a.closures[lit] = ci

	a.enterScope()
	seenParams := make(map[string]bool, len(lit.Params))
	for i, p := range lit.Params {
		if seenParams[p.Name] {
			a.addErrorAt(p, "duplicate parameter %q in function literal", p.Name)
			continue
		}
		seenParams[p.Name] = true
		a.currentScope.DeclareSymbol(p.Name, SymParam, symParamTypes[i])
	}

	savedLoopDepth := a.loopDepth
	a.loopDepth = 0

	a.closureStack = append(a.closureStack, ci)
	a.returnStack = append(a.returnStack, retType)
	a.analyzeBlockBody(lit.Body)
	a.returnStack = a.returnStack[:len(a.returnStack)-1]
	a.closureStack = a.closureStack[:len(a.closureStack)-1]

	a.loopDepth = savedLoopDepth

	a.checkMissingReturn(lit.Body, retType, "closure", lit)
	a.exitScope()
	return fnType
}

func (a *Analyzer) analyzeUnaryExpr(e *UnaryExpr) Type {
	rightType := a.analyzeExpr(e.Right, false)
	if rightType == nil {
		return nil
	}

	switch e.Op {
	case "!":
		if !rightType.Equals(TypeBool) {
			a.addErrorAt(e, "operator '!' requires bool operand, got %s", rightType.String())
			return nil
		}
		return TypeBool
	case "-":
		if rightType.Equals(TypeInt) || rightType.Equals(TypeFloat) {
			return rightType
		}
		a.addErrorAt(e, "operator '-' requires int or float operand, got %s", rightType.String())
		return nil

	case "++", "--":
		if !rightType.Equals(TypeInt) && !rightType.Equals(TypeFloat) {
			a.addErrorAt(e, "operator %q requires int or float operand, got %s", e.Op, rightType.String())
			return nil
		}
		if id, ok := e.Right.(*IdentExpr); ok {
			if sym, _ := a.currentScope.LookupSymbol(id.Name); sym != nil && sym.IsConst {
				a.addErrorAt(e, "cannot mutate constant %q with %q", id.Name, e.Op)
			}
		}
		if e.Right != nil {
			a.analyzeExpr(e.Right, true)
		}
		return rightType

	default:
		a.addErrorAt(e, "invalid unary operator: %q", e.Op)
		return nil
	}
}

func (a *Analyzer) analyzeBinaryExpr(e *BinaryExpr) Type {
	leftType := a.analyzeExpr(e.Left, false)
	rightType := a.analyzeExpr(e.Right, false)
	if leftType == nil || rightType == nil {
		return nil
	}

	switch e.Op {
	case "||", "&&":
		if !leftType.Equals(TypeBool) || !rightType.Equals(TypeBool) {
			a.addErrorAt(e, "operator %q requires bool operands, got %s and %s",
				e.Op, leftType.String(), rightType.String())
			return nil
		}
		return TypeBool

	case "==", "!=":
		if !leftType.Equals(rightType) {
			a.addErrorAt(e, "cannot compare %q between different types %s and %s",
				e.Op, leftType.String(), rightType.String())
			return nil
		}
		if _, isFunc := leftType.(*FuncType); isFunc {
			a.addErrorAt(e, "cannot use operator %q on function type %s", e.Op, leftType.String())
			return nil
		}
		return TypeBool

	case "<", ">", "<=", ">=":
		if !leftType.Equals(rightType) {
			a.addErrorAt(e, "cannot compare %q between %s and %s",
				e.Op, leftType.String(), rightType.String())
			return nil
		}
		if leftType.Equals(TypeInt) || leftType.Equals(TypeFloat) {
			return TypeBool
		}
		a.addErrorAt(e, "operator %q only applies to int or float, got %s", e.Op, leftType.String())
		return nil

	case "+", "-", "*", "/":
		if !leftType.Equals(rightType) {
			a.addErrorAt(e, "type mismatch for operator %q: %s and %s",
				e.Op, leftType.String(), rightType.String())
			return nil
		}
		if e.Op == "+" && leftType.Equals(TypeString) {
			return TypeString
		}
		if leftType.Equals(TypeInt) || leftType.Equals(TypeFloat) {
			return leftType
		}
		a.addErrorAt(e, "operator %q does not support type %s", e.Op, leftType.String())
		return nil

	default:
		a.addErrorAt(e, "invalid binary operator: %q", e.Op)
		return nil
	}
}

func (a *Analyzer) analyzeCallExpr(e *CallExpr) Type {
	calleeType := a.analyzeExpr(e.Callee, false)
	argTypes := make([]Type, 0, len(e.Args))
	for _, arg := range e.Args {
		argTypes = append(argTypes, a.analyzeExpr(arg, false))
	}
	a.callArgTypes[e] = argTypes

	if calleeType == nil {
		return nil
	}
	fnType, ok := calleeType.(*FuncType)
	if !ok {
		a.addErrorAt(e.Callee, "expression of type %s is not callable", calleeType.String())
		return nil
	}

	if fnType.Variadic {
		if len(argTypes) < len(fnType.Params) {
			a.addErrorAt(e, "argument count mismatch: expected at least %d, got %d",
				len(fnType.Params), len(argTypes))
		}
	} else if len(argTypes) != len(fnType.Params) {
		a.addErrorAt(e, "argument count mismatch: expected %d, got %d", len(fnType.Params), len(argTypes))
	}

	limit := len(argTypes)
	if len(fnType.Params) < limit {
		limit = len(fnType.Params)
	}
	for i := 0; i < limit; i++ {
		if argTypes[i] != nil && !isAssignableStrict(fnType.Params[i], argTypes[i]) {
			a.addErrorAt(e.Args[i], "argument %d expects type %s, got %s",
				i+1, fnType.Params[i].String(), argTypes[i].String())
		}
	}

	if fnType.Variadic {
		varElem := fnType.VariadicElem
		if varElem == nil {
			varElem = TypeAny
		}
		for i := len(fnType.Params); i < len(argTypes); i++ {
			if argTypes[i] == nil {
				continue
			}
			if !isValueType(argTypes[i]) {
				a.addErrorAt(e.Args[i], "argument %d cannot have type %s", i+1, argTypes[i].String())
				continue
			}
			if !isAssignableStrict(varElem, argTypes[i]) {
				a.addErrorAt(e.Args[i], "argument %d expects type %s, got %s",
					i+1, varElem.String(), argTypes[i].String())
			}
		}
	}

	return fnType.ReturnType
}

func (a *Analyzer) analyzeFieldExpr(e *FieldExpr, isWrite bool) Type {
	targetType := a.analyzeExpr(e.Target, false)
	if targetType == nil {
		return nil
	}

	switch t := targetType.(type) {
	case *ModuleType:
		if isWrite {
			a.addErrorAt(e, "cannot assign to member %q of module %q", e.Field, t.Name)
			return nil
		}
		expType, exists := t.Exports[e.Field]
		if !exists {
			a.addErrorAt(e, "module %q has no export %q", t.Name, e.Field)
			return nil
		}
		return expType

	case *StructType:
		fieldType, exists := t.Fields[e.Field]
		if !exists {
			a.addErrorAt(e, "struct %q has no field %q", t.Name, e.Field)
			return nil
		}
		_ = isWrite
		return fieldType

	default:
		a.addErrorAt(e, "cannot access field %q on type %s", e.Field, targetType.String())
		return nil
	}
}
