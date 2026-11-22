package main

import (
	"fmt"
	"strconv"
)

type LocalSlot struct {
	Name       string
	Mem        uint64
	Depth      int
	Captured   bool
	UpvalueIdx uint64
}

type CompilerScope struct {
	Parent *CompilerScope
	Depth  int
	Locals map[string]*LocalSlot
}

func NewCompilerScope(parent *CompilerScope) *CompilerScope {
	depth := 0
	if parent != nil {
		depth = parent.Depth + 1
	}
	return &CompilerScope{
		Parent: parent,
		Depth:  depth,
		Locals: make(map[string]*LocalSlot),
	}
}

type FuncContext struct {
	Fn           *CompiledFunction
	Parent       *FuncContext
	Scope        *CompilerScope
	NextMem      uint64
	Upvalues     []UpvalueDesc
	UpvalueIndex map[string]uint64
	LastInstPos  int
}

func NewFuncContext(name string, arity uint64, retType Type, parent *FuncContext) *FuncContext {
	fn := &CompiledFunction{
		Name:       name,
		Arity:      arity,
		ReturnType: retType,
	}
	return &FuncContext{
		Fn:           fn,
		Parent:       parent,
		Scope:        NewCompilerScope(nil),
		NextMem:      0,
		UpvalueIndex: make(map[string]uint64),
		LastInstPos:  -1,
	}
}

type LoopContext struct {
	BreakJumps    []int
	ContinueJumps []int
}

type Compiler struct {
	analyzer  *Analyzer
	fnStack   []*FuncContext
	errors    []string
	loopStack []*LoopContext
}

func NewCompiler(analyzer *Analyzer) *Compiler {
	return &Compiler{
		analyzer: analyzer,
	}
}

func (c *Compiler) Errors() []string {
	return c.errors
}

func (c *Compiler) addError(format string, args ...any) {
	c.errors = append(c.errors, fmt.Sprintf(format, args...))
}

func (c *Compiler) curFunc() *FuncContext {
	if len(c.fnStack) == 0 {
		return nil
	}
	return c.fnStack[len(c.fnStack)-1]
}

func (c *Compiler) curLoop() *LoopContext {
	if len(c.loopStack) == 0 {
		return nil
	}
	return c.loopStack[len(c.loopStack)-1]
}

func (c *Compiler) pushLoop() *LoopContext {
	loop := &LoopContext{}
	c.loopStack = append(c.loopStack, loop)
	return loop
}

func (c *Compiler) popLoop() {
	if len(c.loopStack) > 0 {
		c.loopStack = c.loopStack[:len(c.loopStack)-1]
	}
}

func (c *Compiler) emit(words ...uint64) int {
	fc := c.curFunc()
	pos := EmitWords(fc.Fn, words...)
	fc.LastInstPos = pos
	return pos
}

func (c *Compiler) patchJumpsTo(slots []int, target int) {
	fc := c.curFunc()
	for _, slot := range slots {
		PatchRel(fc.Fn, slot, target)
	}
}

func (c *Compiler) pushFunc(name string, arity uint64, retType Type) *FuncContext {
	var parent *FuncContext
	if len(c.fnStack) > 0 {
		parent = c.fnStack[len(c.fnStack)-1]
	}
	fc := NewFuncContext(name, arity, retType, parent)
	c.fnStack = append(c.fnStack, fc)
	return fc
}

func (c *Compiler) popFunc() *FuncContext {
	fc := c.curFunc()
	if fc == nil {
		return nil
	}
	fc.Fn.MemCount = fc.NextMem
	fc.Fn.UpvalueCount = uint64(len(fc.Upvalues))
	fc.Fn.Upvalues = fc.Upvalues
	c.fnStack = c.fnStack[:len(c.fnStack)-1]
	return fc
}

func (c *Compiler) allocMem() uint64 {
	fc := c.curFunc()
	slot := fc.NextMem
	fc.NextMem++
	return slot
}

func (c *Compiler) enterScope() {
	fc := c.curFunc()
	fc.Scope = NewCompilerScope(fc.Scope)
}

func (c *Compiler) exitScope() {
	fc := c.curFunc()
	if fc.Scope.Parent != nil {
		fc.Scope = fc.Scope.Parent
	}
}

func (c *Compiler) declareLocal(name string) *LocalSlot {
	fc := c.curFunc()
	slot := &LocalSlot{
		Name:  name,
		Mem:   c.allocMem(),
		Depth: fc.Scope.Depth,
	}
	fc.Scope.Locals[name] = slot
	return slot
}

func (c *Compiler) resolveInFunc(fc *FuncContext, name string, isOuterCapture bool) (uint64, bool, bool) {
	for sc := fc.Scope; sc != nil; sc = sc.Parent {
		if slot, ok := sc.Locals[name]; ok {
			if isOuterCapture {
				slot.Captured = true
			}
			return slot.Mem, true, false
		}
	}

	if upIdx, ok := fc.UpvalueIndex[name]; ok {
		return upIdx, false, true
	}

	if fc.Parent == nil {
		return 0, false, false
	}

	if mem, isLocal, isUp := c.resolveInFunc(fc.Parent, name, true); isLocal {
		upIdx := uint64(len(fc.Upvalues))
		fc.Upvalues = append(fc.Upvalues, UpvalueDesc{
			Name:    name,
			IsLocal: true,
			Index:   mem,
		})
		fc.UpvalueIndex[name] = upIdx
		return upIdx, false, true
	} else if isUp {
		upIdx := uint64(len(fc.Upvalues))
		fc.Upvalues = append(fc.Upvalues, UpvalueDesc{
			Name:    name,
			IsLocal: false,
			Index:   mem,
		})
		fc.UpvalueIndex[name] = upIdx
		return upIdx, false, true
	}

	return 0, false, false
}

func (c *Compiler) Compile(prog *Program) *BytecodeProgram {
	mainCtx := c.pushFunc("main", 0, TypeUnit)
	if prog != nil {
		for _, stmt := range prog.Items {
			c.compileStmt(stmt)
		}
	}
	c.emit(uint64(OP_HALT))
	c.popFunc()

	return &BytecodeProgram{
		Main: mainCtx.Fn,
	}
}

func (c *Compiler) exprType(e Expr) Type {
	if c.analyzer != nil && e != nil {
		return c.analyzer.ExprType(e)
	}
	return nil
}

func (c *Compiler) emitUnit(dst uint64) {
	fc := c.curFunc()
	imm := AddConst(fc.Fn, nil)
	c.emit(uint64(OP_CONST), dst, imm)
}

func (c *Compiler) emitBool(dst uint64, val bool) {
	fc := c.curFunc()
	imm := AddConst(fc.Fn, val)
	c.emit(uint64(OP_CONST), dst, imm)
}

func (c *Compiler) hasTrailingRet(fc *FuncContext) bool {
	return fc.LastInstPos >= 0 &&
		fc.LastInstPos < len(fc.Fn.Code) &&
		OpCode(fc.Fn.Code[fc.LastInstPos]) == OP_RET
}

func (c *Compiler) compileStmt(stmt Stmt) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *UseStmt:
		c.compileUseStmt(s)
	case *StructDecl:
	case *FuncDecl:
		c.compileFuncDecl(s)
	case *LetStmt:
		c.compileLetStmt(s)
	case *AssignStmt:
		c.compileAssignStmt(s)
	case *ReturnStmt:
		c.compileReturnStmt(s)
	case *IfStmt:
		c.compileIfStmt(s)
	case *ForStmt:
		c.compileForStmt(s)
	case *ForEachStmt:
		c.compileForEachStmt(s)
	case *ContinueStmt:
		c.compileContinueStmt()
	case *BreakStmt:
		c.compileBreakStmt()
	case *BlockStmt:
		c.enterScope()
		for _, inner := range s.Stmts {
			c.compileStmt(inner)
		}
		c.exitScope()
	case *ExprStmt:
		if s.Expr != nil {
			_ = c.compileExpr(s.Expr)
		}
	default:
		c.addError("unsupported statement in compiler: %T", stmt)
	}
}

func (c *Compiler) compileContinueStmt() {
	fc := c.curFunc()
	loop := c.curLoop()
	if loop == nil {
		c.addError("continue statement outside loop")
		return
	}
	c.emit(uint64(OP_JMP), 0)
	loop.ContinueJumps = append(loop.ContinueJumps, len(fc.Fn.Code)-1)
}

func (c *Compiler) compileBreakStmt() {
	fc := c.curFunc()
	loop := c.curLoop()
	if loop == nil {
		c.addError("break statement outside loop")
		return
	}
	c.emit(uint64(OP_JMP), 0)
	loop.BreakJumps = append(loop.BreakJumps, len(fc.Fn.Code)-1)
}

func (c *Compiler) compileUseStmt(stmt *UseStmt) {
	fc := c.curFunc()
	dst := c.allocMem()
	nameImm := AddConst(fc.Fn, stmt.Module)
	c.emit(uint64(OP_USE), dst, nameImm)
	slot := c.declareLocal(stmt.Module)
	if slot.Mem != dst {
		c.emit(uint64(OP_MOV), slot.Mem, dst)
	}
}

func (c *Compiler) compileFuncDecl(decl *FuncDecl) {
	fc := c.curFunc()
	var retType Type = TypeUnit
	if decl.ReturnType != "" && c.analyzer != nil {
		if rt, ok := c.analyzer.resolveTypeName(decl.ReturnType); ok && rt != nil {
			retType = rt
		}
	}

	slot := c.declareLocal(decl.Name)

	savedLoopStack := c.loopStack
	c.loopStack = nil

	innerCtx := c.pushFunc(decl.Name, uint64(len(decl.Params)), retType)
	for _, p := range decl.Params {
		_ = c.declareLocal(p.Name)
	}

	if decl.Body != nil {
		for _, s := range decl.Body.Stmts {
			c.compileStmt(s)
		}
	}

	if !c.hasTrailingRet(innerCtx) {
		c.emit(uint64(OP_RET), 0)
	}

	compiledInner := c.popFunc().Fn
	c.loopStack = savedLoopStack

	fnImm := AddConst(fc.Fn, compiledInner)
	c.emit(uint64(OP_CLOSURE), slot.Mem, fnImm)
}

func (c *Compiler) compileCallToDsts(call *CallExpr, dsts []uint64) {
	fnMem := c.compileExpr(call.Callee)
	var argMems []uint64
	for _, arg := range call.Args {
		argMems = append(argMems, c.compileExpr(arg))
	}

	words := []uint64{uint64(OP_CALL), fnMem, uint64(len(dsts)), uint64(len(argMems))}
	words = append(words, dsts...)
	words = append(words, argMems...)
	c.emit(words...)
}

func (c *Compiler) compileLetStmt(stmt *LetStmt) {
	names := stmt.Names
	if len(names) == 0 && stmt.Name != "" {
		names = []string{stmt.Name}
	}

	if len(names) > 1 {
		var dsts []uint64
		for range names {
			dsts = append(dsts, c.allocMem())
		}
		if call, ok := stmt.Init.(*CallExpr); ok {
			c.compileCallToDsts(call, dsts)
		} else {
			initMem := c.compileExpr(stmt.Init)
			if len(dsts) > 0 && dsts[0] != initMem {
				c.emit(uint64(OP_MOV), dsts[0], initMem)
			}
		}
		for i, name := range names {
			slot := c.declareLocal(name)
			if slot.Mem != dsts[i] {
				c.emit(uint64(OP_MOV), slot.Mem, dsts[i])
			}
		}
		return
	}

	if len(names) == 0 {
		return
	}

	name := names[0]
	var initMem uint64
	if stmt.Init != nil {
		initMem = c.compileExpr(stmt.Init)
	} else {
		initMem = c.allocMem()
		c.emitUnit(initMem)
	}

	slot := c.declareLocal(name)
	if slot.Mem != initMem {
		c.emit(uint64(OP_MOV), slot.Mem, initMem)
	}
}

func (c *Compiler) compileAssignStmt(stmt *AssignStmt) {
	valMem := c.compileExpr(stmt.Value)
	c.compileStoreTarget(stmt.Target, valMem)
}

func (c *Compiler) compileStoreTarget(target Expr, valMem uint64) {
	fc := c.curFunc()
	switch t := target.(type) {
	case *IdentExpr:
		idx, isLocal, isUp := c.resolveInFunc(fc, t.Name, false)
		if isLocal {
			if idx != valMem {
				c.emit(uint64(OP_MOV), idx, valMem)
			}
			return
		}
		if isUp {
			c.emit(uint64(OP_SET_UPVALUE), idx, valMem)
			return
		}
		c.addError("undefined variable in assignment: %s", t.Name)

	case *FieldExpr:
		objMem := c.compileExpr(t.Target)
		fieldImm := AddConst(fc.Fn, t.Field)
		c.emit(uint64(OP_SET_FIELD), objMem, fieldImm, valMem)

	case *IndexExpr:
		listMem := c.compileExpr(t.Target)
		idxMem := c.compileExpr(t.Index)
		c.emit(uint64(OP_INDEX_SET), listMem, idxMem, valMem)

	default:
		c.addError("unsupported assignment target: %T", target)
	}
}

func (c *Compiler) compileReturnStmt(stmt *ReturnStmt) {
	retExprs := stmt.Values
	if len(retExprs) == 0 && stmt.Value != nil {
		retExprs = []Expr{stmt.Value}
	}

	if len(retExprs) == 0 {
		c.emit(uint64(OP_RET), 0)
		return
	}

	var retMems []uint64
	for _, val := range retExprs {
		retMems = append(retMems, c.compileExpr(val))
	}
	words := []uint64{uint64(OP_RET), uint64(len(retMems))}
	words = append(words, retMems...)
	c.emit(words...)
}

func (c *Compiler) compileIfStmt(stmt *IfStmt) {
	fc := c.curFunc()
	var endJumps []int

	for _, br := range stmt.Branches {
		condMem := c.compileExpr(br.Cond)
		c.emit(uint64(OP_JMP_IF_FALSE), condMem, 0)
		jmpFalseSlot := len(fc.Fn.Code) - 1

		if br.Body != nil {
			c.enterScope()
			for _, inner := range br.Body.Stmts {
				c.compileStmt(inner)
			}
			c.exitScope()
		}

		c.emit(uint64(OP_JMP), 0)
		endJumps = append(endJumps, len(fc.Fn.Code)-1)

		PatchRel(fc.Fn, jmpFalseSlot, len(fc.Fn.Code))
	}

	if stmt.ElseBody != nil {
		c.enterScope()
		for _, inner := range stmt.ElseBody.Stmts {
			c.compileStmt(inner)
		}
		c.exitScope()
	}

	c.patchJumpsTo(endJumps, len(fc.Fn.Code))
}

func (c *Compiler) compileForStmt(stmt *ForStmt) {
	fc := c.curFunc()
	c.enterScope()
	if stmt.Init != nil {
		c.compileStmt(stmt.Init)
	}
	loop := c.pushLoop()
	condPos := len(fc.Fn.Code)
	jmpOutSlot := -1
	if stmt.Cond != nil {
		condMem := c.compileExpr(stmt.Cond)
		c.emit(uint64(OP_JMP_IF_FALSE), condMem, 0)
		jmpOutSlot = len(fc.Fn.Code) - 1
	}
	if stmt.Body != nil {
		c.enterScope()
		for _, inner := range stmt.Body.Stmts {
			c.compileStmt(inner)
		}
		c.exitScope()
	}
	continueTarget := len(fc.Fn.Code)
	c.patchJumpsTo(loop.ContinueJumps, continueTarget)
	if stmt.Post != nil {
		c.compileStmt(stmt.Post)
	}
	c.emit(uint64(OP_JMP), 0)
	PatchRel(fc.Fn, len(fc.Fn.Code)-1, condPos)
	endPos := len(fc.Fn.Code)
	if jmpOutSlot >= 0 {
		PatchRel(fc.Fn, jmpOutSlot, endPos)
	}
	c.patchJumpsTo(loop.BreakJumps, endPos)
	c.popLoop()
	c.exitScope()
}

func (c *Compiler) compileForEachStmt(stmt *ForEachStmt) {
	fc := c.curFunc()
	c.enterScope()
	listMem := c.compileExpr(stmt.Iterable)

	idxMem := c.allocMem()
	zeroImm := AddConst(fc.Fn, int64(0))
	c.emit(uint64(OP_CONST), idxMem, zeroImm)

	oneMem := c.allocMem()
	oneImm := AddConst(fc.Fn, int64(1))
	c.emit(uint64(OP_CONST), oneMem, oneImm)

	lenMem := c.allocMem()
	elemSlot := c.declareLocal(stmt.VarName)

	loop := c.pushLoop()
	condPos := len(fc.Fn.Code)
	c.emit(uint64(OP_LIST_LEN), lenMem, listMem)

	c.emit(uint64(OP_CMP), idxMem, lenMem, JCC_GTE, 0)
	jmpOutSlot := len(fc.Fn.Code) - 1

	c.emit(uint64(OP_INDEX_GET), elemSlot.Mem, listMem, idxMem)

	if stmt.Body != nil {
		c.enterScope()
		for _, inner := range stmt.Body.Stmts {
			c.compileStmt(inner)
		}
		c.exitScope()
	}

	continueTarget := len(fc.Fn.Code)
	c.patchJumpsTo(loop.ContinueJumps, continueTarget)
	c.emit(uint64(OP_ADD), idxMem, idxMem, oneMem)
	c.emit(uint64(OP_JMP), 0)
	PatchRel(fc.Fn, len(fc.Fn.Code)-1, condPos)

	endPos := len(fc.Fn.Code)
	PatchRel(fc.Fn, jmpOutSlot, endPos)
	c.patchJumpsTo(loop.BreakJumps, endPos)
	c.popLoop()
	c.exitScope()
}

func (c *Compiler) compileExpr(expr Expr) uint64 {
	fc := c.curFunc()
	if expr == nil {
		dst := c.allocMem()
		c.emitUnit(dst)
		return dst
	}
	switch e := expr.(type) {
	case *IntLit:
		val, err := strconv.ParseInt(e.Value, 10, 64)
		if err != nil {
			c.addError("invalid integer literal %s: %v", e.Value, err)
		}
		dst := c.allocMem()
		imm := AddConst(fc.Fn, val)
		c.emit(uint64(OP_CONST), dst, imm)
		return dst

	case *FloatLit:
		val, err := strconv.ParseFloat(e.Value, 64)
		if err != nil {
			c.addError("invalid float literal %s: %v", e.Value, err)
		}
		dst := c.allocMem()
		imm := AddConst(fc.Fn, val)
		c.emit(uint64(OP_CONST), dst, imm)
		return dst

	case *StringLit:
		dst := c.allocMem()
		imm := AddConst(fc.Fn, e.Value)
		c.emit(uint64(OP_CONST), dst, imm)
		return dst

	case *BoolLit:
		dst := c.allocMem()
		c.emitBool(dst, e.Value)
		return dst

	case *IdentExpr:
		idx, isLocal, isUp := c.resolveInFunc(fc, e.Name, false)
		if isLocal {
			return idx
		}
		if isUp {
			dst := c.allocMem()
			c.emit(uint64(OP_GET_UPVALUE), dst, idx)
			return dst
		}
		c.addError("undefined variable: %s", e.Name)
		dst := c.allocMem()
		c.emitUnit(dst)
		return dst

	case *UnaryExpr:
		rightMem := c.compileExpr(e.Right)
		dst := c.allocMem()
		switch e.Op {
		case "-":
			c.emit(uint64(OP_NEG), dst, rightMem)
		case "!":
			c.emit(uint64(OP_NOT), dst, rightMem)
		case "++":
			c.emit(uint64(OP_INC), dst, rightMem)
			c.compileStoreTarget(e.Right, dst)
		case "--":
			c.emit(uint64(OP_DEC), dst, rightMem)
			c.compileStoreTarget(e.Right, dst)
		default:
			c.addError("unsupported unary operator: %s", e.Op)
		}
		return dst

	case *BinaryExpr:
		return c.compileBinaryExpr(e)

	case *CallExpr:
		if field, ok := e.Callee.(*FieldExpr); ok {
			if field.Field == "len" && len(e.Args) == 0 {
				objMem := c.compileExpr(field.Target)
				dst := c.allocMem()
				c.emit(uint64(OP_LIST_LEN), dst, objMem)
				return dst
			}
		}
		dst := c.allocMem()
		c.compileCallToDsts(e, []uint64{dst})
		return dst

	case *FieldExpr:
		objMem := c.compileExpr(e.Target)
		dst := c.allocMem()
		fieldImm := AddConst(fc.Fn, e.Field)
		c.emit(uint64(OP_GET_FIELD), dst, objMem, fieldImm)
		return dst

	case *IndexExpr:
		objMem := c.compileExpr(e.Target)
		idxMem := c.compileExpr(e.Index)
		dst := c.allocMem()
		c.emit(uint64(OP_INDEX_GET), dst, objMem, idxMem)
		return dst

	case *ListLit:
		elemTypeImm := AddConst(fc.Fn, e.ElemTypeName)
		dst := c.allocMem()
		if e.RepeatVal != nil || e.RepeatCount != nil {
			valMem := c.compileExpr(e.RepeatVal)
			cntMem := c.compileExpr(e.RepeatCount)
			c.emit(uint64(OP_NEW_LIST), dst, elemTypeImm, 1, valMem, cntMem)
			return dst
		}
		elemMems := make([]uint64, 0, len(e.Elements))
		for _, el := range e.Elements {
			elemMems = append(elemMems, c.compileExpr(el))
		}
		words := []uint64{uint64(OP_NEW_LIST), dst, elemTypeImm, 0, uint64(len(elemMems))}
		words = append(words, elemMems...)
		c.emit(words...)
		return dst

	case *StructLit:
		dst := c.allocMem()
		typeImm := AddConst(fc.Fn, e.TypeName)
		words := []uint64{uint64(OP_NEW_STRUCT), dst, typeImm, uint64(len(e.Fields))}
		for _, f := range e.Fields {
			fImm := AddConst(fc.Fn, f.Name)
			valMem := c.compileExpr(f.Value)
			words = append(words, fImm, valMem)
		}
		c.emit(words...)
		return dst

	case *FuncLit:
		return c.compileFuncLit(e)

	default:
		c.addError("unsupported expression in compiler: %T", expr)
		dst := c.allocMem()
		c.emitUnit(dst)
		return dst
	}
}

func (c *Compiler) compileCompareExpr(e *BinaryExpr, jcc uint64) uint64 {
	fc := c.curFunc()
	leftMem := c.compileExpr(e.Left)
	rightMem := c.compileExpr(e.Right)
	dst := c.allocMem()
	c.emitBool(dst, true)
	c.emit(uint64(OP_CMP), leftMem, rightMem, jcc, 0)
	jmpTrueSlot := len(fc.Fn.Code) - 1
	c.emitBool(dst, false)
	PatchRel(fc.Fn, jmpTrueSlot, len(fc.Fn.Code))
	return dst
}

func (c *Compiler) compileBinaryExpr(e *BinaryExpr) uint64 {
	fc := c.curFunc()
	if e.Op == "&&" {
		leftMem := c.compileExpr(e.Left)
		dst := c.allocMem()
		c.emit(uint64(OP_MOV), dst, leftMem)
		c.emit(uint64(OP_JMP_IF_FALSE), dst, 0)
		jmpSlot := len(fc.Fn.Code) - 1
		rightMem := c.compileExpr(e.Right)
		c.emit(uint64(OP_MOV), dst, rightMem)
		PatchRel(fc.Fn, jmpSlot, len(fc.Fn.Code))
		return dst
	}
	if e.Op == "||" {
		leftMem := c.compileExpr(e.Left)
		dst := c.allocMem()
		c.emit(uint64(OP_MOV), dst, leftMem)
		c.emit(uint64(OP_JMP_IF_TRUE), dst, 0)
		jmpTrueSlot := len(fc.Fn.Code) - 1
		rightMem := c.compileExpr(e.Right)
		c.emit(uint64(OP_MOV), dst, rightMem)
		PatchRel(fc.Fn, jmpTrueSlot, len(fc.Fn.Code))
		return dst
	}

	switch e.Op {
	case "==":
		return c.compileCompareExpr(e, JCC_EQ)
	case "!=":
		return c.compileCompareExpr(e, JCC_NEQ)
	case "<":
		return c.compileCompareExpr(e, JCC_LT)
	case "<=":
		return c.compileCompareExpr(e, JCC_LTE)
	case ">":
		return c.compileCompareExpr(e, JCC_GT)
	case ">=":
		return c.compileCompareExpr(e, JCC_GTE)
	}

	leftMem := c.compileExpr(e.Left)
	rightMem := c.compileExpr(e.Right)
	dst := c.allocMem()
	switch e.Op {
	case "+":
		c.emit(uint64(OP_ADD), dst, leftMem, rightMem)
	case "-":
		c.emit(uint64(OP_SUB), dst, leftMem, rightMem)
	case "*":
		c.emit(uint64(OP_MUL), dst, leftMem, rightMem)
	case "/":
		c.emit(uint64(OP_DIV), dst, leftMem, rightMem)
	default:
		c.addError("unsupported binary operator: %s", e.Op)
	}
	return dst
}

func (c *Compiler) compileFuncLit(lit *FuncLit) uint64 {
	fc := c.curFunc()
	var retType Type = TypeUnit
	if lit.ReturnType != "" && c.analyzer != nil {
		if rt, ok := c.analyzer.resolveTypeName(lit.ReturnType); ok && rt != nil {
			retType = rt
		}
	}
	savedLoopStack := c.loopStack
	c.loopStack = nil
	innerCtx := c.pushFunc("lambda", uint64(len(lit.Params)), retType)
	for _, p := range lit.Params {
		_ = c.declareLocal(p.Name)
	}
	if lit.Body != nil {
		for _, s := range lit.Body.Stmts {
			c.compileStmt(s)
		}
	}
	if !c.hasTrailingRet(innerCtx) {
		c.emit(uint64(OP_RET), 0)
	}
	compiledInner := c.popFunc().Fn
	c.loopStack = savedLoopStack
	fnImm := AddConst(fc.Fn, compiledInner)
	dst := c.allocMem()
	c.emit(uint64(OP_CLOSURE), dst, fnImm)
	return dst
}
