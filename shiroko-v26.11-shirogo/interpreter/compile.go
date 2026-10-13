package interpreter

import "fmt"

// ---------- opcodes ----------

type Op uint8

const (
	OpNop Op = iota

	OpLoadConst
	OpTrue
	OpFalse
	OpNil
	OpMove
	OpLoadGlobal
	OpStoreGlobal

	OpAdd
	OpSub
	OpMul
	OpDiv
	OpMod
	OpEq
	OpNeq
	OpLt
	OpLe
	OpGt
	OpGe
	OpNeg
	OpNot
	OpTruthy
	OpInc
	OpDec

	// Fused register + constant forms. Layout: A=dst, B=reg, C=constidx.
	OpAddConst
	OpSubConst
	OpMulConst
	OpDivConst
	OpModConst
	OpEqConst
	OpNeqConst
	OpLtConst
	OpLeConst
	OpGtConst
	OpGeConst

	OpJump
	OpJumpIfFalse
	OpJumpIfTrue
	OpJumpIfNotErr

	OpCall
	OpReturn

	OpList
	OpSet
	OpMap
	OpStructLit
	OpDestructure

	OpField
	OpIndex
	OpSlice

	OpRefLocal
	OpRefGlobal
	OpRefField
	OpRefIndex
	OpRefNew
	OpDeref

	OpStoreRef
	OpStoreField
	OpStoreIndex

	OpMake
	OpClosure

	// Superinstructions.
	//
	// Comparison + conditional jump.  Layout:
	//   A = lhs reg, B = rhs reg, C = jump target, D = dst reg (bool result)
	OpLtJumpIfFalse
	OpLeJumpIfFalse
	OpGtJumpIfFalse
	OpGeJumpIfFalse

	// Comparison-const + conditional jump.  Layout:
	//   A = lhs reg, B = constidx, C = jump target, D = dst reg
	OpLtConstJumpIfFalse
	OpLeConstJumpIfFalse
	OpGtConstJumpIfFalse
	OpGeConstJumpIfFalse

	// Global call fused with LoadGlobal.  Layout:
	//   A = global slot, B = argc, C = callee register
	OpCallGlobal
)

type Instr struct {
	Op         Op
	A, B, C, D int
}

type CompiledFn struct {
	Name     string
	NumArgs  int
	NumRegs  int
	Variadic bool
	Code     []Instr
	Consts   []Value
	Fields   []string
	Structs  []string
	Closures []*CompiledFn
}

// ---------- register allocator ----------

type regScope struct {
	savedReg int
	namesEnd int
	names    map[string]int
}

type loopCtx struct {
	breaks    []int
	continues []int
}

type fnCompiler struct {
	ip      *Interpreter
	fn      *CompiledFn
	nextReg int
	scopes  []regScope
	loops   []loopCtx
}

func (c *fnCompiler) pushLoop() {
	c.loops = append(c.loops, loopCtx{})
}

func (c *fnCompiler) popLoop(continueTarget int) {
	lc := c.loops[len(c.loops)-1]
	c.loops = c.loops[:len(c.loops)-1]
	end := len(c.fn.Code)
	for _, j := range lc.continues {
		c.fn.Code[j].A = continueTarget
	}
	for _, j := range lc.breaks {
		c.fn.Code[j].A = end
	}
}

func (c *fnCompiler) pushScope() {
	c.scopes = append(c.scopes, regScope{
		savedReg: c.nextReg,
		namesEnd: c.nextReg,
		names:    map[string]int{},
	})
}

func (c *fnCompiler) popScope() {
	s := c.scopes[len(c.scopes)-1]
	c.scopes = c.scopes[:len(c.scopes)-1]
	c.nextReg = s.savedReg
}

func (c *fnCompiler) allocReg() int {
	r := c.nextReg
	c.nextReg++
	if c.nextReg > c.fn.NumRegs {
		c.fn.NumRegs = c.nextReg
	}
	return r
}

func (c *fnCompiler) define(name string) int {
	r := c.allocReg()
	s := &c.scopes[len(c.scopes)-1]
	s.names[name] = r
	s.namesEnd = c.nextReg
	return r
}

func (c *fnCompiler) resolveLocal(name string) (int, bool) {
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if r, ok := c.scopes[i].names[name]; ok {
			return r, true
		}
	}
	return 0, false
}

func (c *fnCompiler) freeTemps() {
	s := &c.scopes[len(c.scopes)-1]
	c.nextReg = s.namesEnd
}

func (c *fnCompiler) emit(op Op, a, b, cc, d int) int {
	c.fn.Code = append(c.fn.Code, Instr{op, a, b, cc, d})
	return len(c.fn.Code) - 1
}

func (c *fnCompiler) patchJump(idx int) { c.fn.Code[idx].A = len(c.fn.Code) }
func (c *fnCompiler) patchCond(idx int) { c.fn.Code[idx].B = len(c.fn.Code) }

func (c *fnCompiler) addConst(v Value) int {
	c.fn.Consts = append(c.fn.Consts, v)
	return len(c.fn.Consts) - 1
}

func (c *fnCompiler) emitLoadConst(r int, v Value) {
	switch v.Kind {
	case KindNil:
		c.emit(OpNil, r, 0, 0, 0)
	case KindBool:
		if v.Bool {
			c.emit(OpTrue, r, 0, 0, 0)
		} else {
			c.emit(OpFalse, r, 0, 0, 0)
		}
	default:
		c.emit(OpLoadConst, r, c.addConst(v), 0, 0)
	}
}

func (c *fnCompiler) addField(name string) int {
	for i, f := range c.fn.Fields {
		if f == name {
			return i
		}
	}
	c.fn.Fields = append(c.fn.Fields, name)
	return len(c.fn.Fields) - 1
}

func (c *fnCompiler) addStruct(name string) int {
	for i, s := range c.fn.Structs {
		if s == name {
			return i
		}
	}
	c.fn.Structs = append(c.fn.Structs, name)
	return len(c.fn.Structs) - 1
}

// ---------- constant folding ----------

func foldConst(n []any) (Value, bool) {
	switch n[0].(string) {
	case "HInt", "HByte":
		return IntVal(n[1].(int)), true
	case "HFloat":
		return FloatVal(n[1].(float64)), true
	case "HStr":
		return StrVal(n[1].(string)), true
	case "HBool":
		return BoolVal(n[1].(bool)), true
	case "HNil":
		return NilVal(), true
	case "HUn":
		op := n[1].(string)
		inner, ok := foldConst(n[2].([]any))
		if !ok {
			return NilVal(), false
		}
		switch op {
		case "-":
			switch inner.Kind {
			case KindInt:
				return IntVal(-inner.Int), true
			case KindFloat:
				return FloatVal(-inner.Float), true
			}
		case "!":
			return BoolVal(!inner.Truthy()), true
		}
		return NilVal(), false
	case "HBin":
		op := n[1].(string)
		l, lok := foldConst(n[2].([]any))
		r, rok := foldConst(n[3].([]any))
		if !lok || !rok {
			return NilVal(), false
		}
		return foldBin(op, l, r)
	}
	return NilVal(), false
}

func foldBin(op string, a, b Value) (Value, bool) {
	if a.Kind == KindInt && b.Kind == KindInt {
		switch op {
		case "+":
			return IntVal(a.Int + b.Int), true
		case "-":
			return IntVal(a.Int - b.Int), true
		case "*":
			return IntVal(a.Int * b.Int), true
		case "/":
			if b.Int == 0 {
				return NilVal(), false
			}
			return IntVal(a.Int / b.Int), true
		case "%":
			if b.Int == 0 {
				return NilVal(), false
			}
			return IntVal(a.Int % b.Int), true
		case "==":
			return BoolVal(a.Int == b.Int), true
		case "!=":
			return BoolVal(a.Int != b.Int), true
		case "<":
			return BoolVal(a.Int < b.Int), true
		case "<=":
			return BoolVal(a.Int <= b.Int), true
		case ">":
			return BoolVal(a.Int > b.Int), true
		case ">=":
			return BoolVal(a.Int >= b.Int), true
		}
	}
	if a.Kind == KindFloat && b.Kind == KindFloat {
		switch op {
		case "+":
			return FloatVal(a.Float + b.Float), true
		case "-":
			return FloatVal(a.Float - b.Float), true
		case "*":
			return FloatVal(a.Float * b.Float), true
		case "/":
			return FloatVal(a.Float / b.Float), true
		case "==":
			return BoolVal(a.Float == b.Float), true
		case "!=":
			return BoolVal(a.Float != b.Float), true
		case "<":
			return BoolVal(a.Float < b.Float), true
		case "<=":
			return BoolVal(a.Float <= b.Float), true
		case ">":
			return BoolVal(a.Float > b.Float), true
		case ">=":
			return BoolVal(a.Float >= b.Float), true
		}
	}
	if a.Kind == KindString && b.Kind == KindString {
		switch op {
		case "+":
			return StrVal(a.Str + b.Str), true
		case "==":
			return BoolVal(a.Str == b.Str), true
		case "!=":
			return BoolVal(a.Str != b.Str), true
		case "<":
			return BoolVal(a.Str < b.Str), true
		case "<=":
			return BoolVal(a.Str <= b.Str), true
		case ">":
			return BoolVal(a.Str > b.Str), true
		case ">=":
			return BoolVal(a.Str >= b.Str), true
		}
	}
	if a.Kind == KindBool && b.Kind == KindBool {
		switch op {
		case "==":
			return BoolVal(a.Bool == b.Bool), true
		case "!=":
			return BoolVal(a.Bool != b.Bool), true
		case "&&":
			return BoolVal(a.Bool && b.Bool), true
		case "||":
			return BoolVal(a.Bool || b.Bool), true
		}
	}
	return NilVal(), false
}

// ---------- peephole ----------

// writesReg reports whether instruction `in` writes to register r (absolute
// within its function's register file).
func writesReg(in Instr, r int) bool {
	switch in.Op {
	case OpLoadConst, OpTrue, OpFalse, OpNil, OpMove, OpLoadGlobal,
		OpAdd, OpSub, OpMul, OpDiv, OpMod,
		OpEq, OpNeq, OpLt, OpLe, OpGt, OpGe,
		OpNeg, OpNot, OpTruthy, OpInc, OpDec,
		OpAddConst, OpSubConst, OpMulConst, OpDivConst, OpModConst,
		OpEqConst, OpNeqConst, OpLtConst, OpLeConst, OpGtConst, OpGeConst,
		OpList, OpSet, OpMap, OpStructLit,
		OpField, OpIndex, OpSlice,
		OpRefLocal, OpRefGlobal, OpRefField, OpRefIndex, OpRefNew, OpDeref,
		OpMake, OpClosure:
		return in.A == r
	case OpDestructure:
		return r >= in.A && r < in.A+in.C
	case OpCall:
		return in.A == r
	case OpCallGlobal:
		return in.C == r
	case OpLtJumpIfFalse, OpLeJumpIfFalse, OpGtJumpIfFalse, OpGeJumpIfFalse,
		OpLtConstJumpIfFalse, OpLeConstJumpIfFalse,
		OpGtConstJumpIfFalse, OpGeConstJumpIfFalse:
		return in.D == r
	}
	return false
}

func constFusible(op Op) bool {
	switch op {
	case OpAdd, OpSub, OpMul, OpDiv, OpMod,
		OpEq, OpNeq, OpLt, OpLe, OpGt, OpGe:
		return true
	}
	return false
}

func commutative(op Op) bool {
	switch op {
	case OpAdd, OpMul, OpEq, OpNeq:
		return true
	}
	return false
}

func constOpcode(op Op) Op {
	switch op {
	case OpAdd:
		return OpAddConst
	case OpSub:
		return OpSubConst
	case OpMul:
		return OpMulConst
	case OpDiv:
		return OpDivConst
	case OpMod:
		return OpModConst
	case OpEq:
		return OpEqConst
	case OpNeq:
		return OpNeqConst
	case OpLt:
		return OpLtConst
	case OpLe:
		return OpLeConst
	case OpGt:
		return OpGtConst
	case OpGe:
		return OpGeConst
	}
	panic("not fusible")
}

func (c *fnCompiler) peephole() {
	code := c.fn.Code
	n := len(code)
	if n == 0 {
		return
	}

	// Pass 1: find jump targets.
	isJumpTarget := make([]bool, n+1)
	for _, in := range code {
		switch in.Op {
		case OpJump:
			if in.A >= 0 && in.A <= n {
				isJumpTarget[in.A] = true
			}
		case OpJumpIfFalse, OpJumpIfTrue, OpJumpIfNotErr:
			if in.B >= 0 && in.B <= n {
				isJumpTarget[in.B] = true
			}
		}
	}

	oldToNew := make([]int, n+1)
	out := make([]Instr, 0, n)

	for i := 0; i < n; i++ {
		oldToNew[i] = len(out)
		in := code[i]

		// --- Rule A: LoadConst r,1 ; Add/Sub dst,dst,r  →  Inc/Dec ---
		if in.Op == OpLoadConst && i+1 < n {
			next := code[i+1]
			one := c.fn.Consts[in.B]
			if one.Kind == KindInt && one.Int == 1 && next.C == in.A {
				if next.Op == OpAdd && next.A == next.B {
					out = append(out, Instr{OpInc, next.A, 0, 0, 0})
					oldToNew[i+1] = oldToNew[i]
					i++
					continue
				}
				if next.Op == OpSub && next.A == next.B {
					out = append(out, Instr{OpDec, next.A, 0, 0, 0})
					oldToNew[i+1] = oldToNew[i]
					i++
					continue
				}
			}
		}

		// --- Rule B: LoadConst + binop (constant on the right) ---
		// If the binop is a comparison followed by JumpIfFalse, fuse all
		// three into one superinstruction.
		if in.Op == OpLoadConst && i+1 < n {
			next := code[i+1]
			if next.C == in.A && constFusible(next.Op) {
				fusedOp := constOpcode(next.Op)
				if i+2 < n && code[i+2].Op == OpJumpIfFalse && code[i+2].A == next.A {
					var jumpFused Op
					switch fusedOp {
					case OpLtConst:
						jumpFused = OpLtConstJumpIfFalse
					case OpLeConst:
						jumpFused = OpLeConstJumpIfFalse
					case OpGtConst:
						jumpFused = OpGtConstJumpIfFalse
					case OpGeConst:
						jumpFused = OpGeConstJumpIfFalse
					}
					if jumpFused != OpNop {
						out = append(out, Instr{jumpFused, next.B, in.B, code[i+2].B, next.A})
						oldToNew[i+1] = oldToNew[i]
						oldToNew[i+2] = oldToNew[i]
						i += 2
						continue
					}
				}
				out = append(out, Instr{fusedOp, next.A, next.B, in.B, 0})
				oldToNew[i+1] = oldToNew[i]
				i++
				continue
			}
			if next.B == in.A && constFusible(next.Op) && commutative(next.Op) {
				out = append(out, Instr{constOpcode(next.Op), next.A, next.C, in.B, 0})
				oldToNew[i+1] = oldToNew[i]
				i++
				continue
			}
		}

		// --- Rule C: comparison + JumpIfFalse (two registers) ---
		if i+1 < n && code[i+1].Op == OpJumpIfFalse && code[i+1].A == in.A {
			var fused Op
			switch in.Op {
			case OpLt:
				fused = OpLtJumpIfFalse
			case OpLe:
				fused = OpLeJumpIfFalse
			case OpGt:
				fused = OpGtJumpIfFalse
			case OpGe:
				fused = OpGeJumpIfFalse
			}
			if fused != OpNop {
				out = append(out, Instr{fused, in.B, in.C, code[i+1].B, in.A})
				oldToNew[i+1] = oldToNew[i]
				i++
				continue
			}
		}

		// --- Rule D: LoadGlobal dst, gidx ; ... ; Call dst, argc ---
		if in.Op == OpLoadGlobal && i+1 < n && !isJumpTarget[i+1] {
			dst := in.A
			j := i + 1
			found := false
			for j < n {
				if isJumpTarget[j] {
					break
				}
				nx := code[j]
				if nx.Op == OpCall && nx.A == dst {
					found = true
					break
				}
				if writesReg(nx, dst) {
					break
				}
				j++
			}
			if found {
				for k := i + 1; k < j; k++ {
					oldToNew[k] = len(out)
					out = append(out, code[k])
				}
				cgPos := len(out)
				oldToNew[j] = cgPos
				out = append(out, Instr{OpCallGlobal, in.B, code[j].B, dst, 0})
				i = j
				continue
			}
		}

		out = append(out, in)
	}
	oldToNew[n] = len(out)

	// Rewrite jump targets.
	for i := range out {
		ins := &out[i]
		switch ins.Op {
		case OpJump:
			if ins.A >= 0 && ins.A <= n {
				ins.A = oldToNew[ins.A]
			}
		case OpJumpIfFalse, OpJumpIfTrue, OpJumpIfNotErr:
			if ins.B >= 0 && ins.B <= n {
				ins.B = oldToNew[ins.B]
			}
		case OpLtJumpIfFalse, OpLeJumpIfFalse, OpGtJumpIfFalse, OpGeJumpIfFalse,
			OpLtConstJumpIfFalse, OpLeConstJumpIfFalse,
			OpGtConstJumpIfFalse, OpGeConstJumpIfFalse:
			if ins.C >= 0 && ins.C <= n {
				ins.C = oldToNew[ins.C]
			}
		}
	}

	c.fn.Code = out
}

// ---------- statements ----------

func (c *fnCompiler) compileStmt(s any) {
	n := s.([]any)
	switch n[0].(string) {
	case "HBlock":
		c.pushScope()
		for _, st := range n[1].([]any) {
			c.compileStmt(st)
		}
		c.popScope()

	case "HLet":
		name := n[1].(string)
		expr := n[3]
		slot := c.define(name)
		if expr == nil {
			c.emit(OpNil, slot, 0, 0, 0)
		} else {
			c.compileInto(slot, expr)
		}
		c.freeTemps()

	case "HErrLet":
		name := n[1].(string)
		expr := n[3]
		els := n[4].([]any)
		slot := c.define(name)
		c.compileInto(slot, expr)
		c.freeTemps()

		jmpNotErr := c.emit(OpJumpIfNotErr, slot, 0, 0, 0)

		c.pushScope()
		errSlot := c.define("err")
		c.emit(OpMove, errSlot, slot, 0, 0)
		for _, st := range els[1].([]any) {
			c.compileStmt(st)
		}
		c.popScope()
		c.patchCond(jmpNotErr)

	case "HMultiLet":
		names := n[1].([]string)
		src := c.compileExpr(n[2])
		base := c.allocReg()
		for i := 1; i < len(names); i++ {
			c.allocReg()
		}
		c.emit(OpDestructure, base, src, len(names), 0)
		c.freeTemps()
		slots := make([]int, len(names))
		for i, name := range names {
			slots[i] = c.define(name)
		}
		for i, slot := range slots {
			if slot != base+i {
				c.emit(OpMove, slot, base+i, 0, 0)
			}
		}
		c.freeTemps()

	case "HAssign":
		tgt := n[1].([]any)
		if tgt[0].(string) == "HIdent" {
			name := tgt[1].(string)
			if slot, ok := c.resolveLocal(name); ok {
				c.compileInto(slot, n[2])
				c.freeTemps()
				return
			}
		}
		r := c.compileExpr(n[2])
		c.compileStore(tgt, r)
		c.freeTemps()

	case "HExprStmt":
		c.compileExpr(n[1])
		c.freeTemps()

	case "HReturn":
		exprs := n[1].([]any)
		var r int
		switch len(exprs) {
		case 0:
			r = c.allocReg()
			c.emit(OpNil, r, 0, 0, 0)
		case 1:
			r = c.compileExpr(exprs[0])
		default:
			regs := make([]int, len(exprs))
			for i, e := range exprs {
				regs[i] = c.compileExpr(e)
			}
			dst := c.allocReg()
			base := c.allocReg()
			for i := 1; i < len(regs); i++ {
				c.allocReg()
			}
			for i, rr := range regs {
				if rr != base+i {
					c.emit(OpMove, base+i, rr, 0, 0)
				}
			}
			c.emit(OpList, dst, base, len(regs), 0)
			r = dst
		}
		c.emit(OpReturn, r, 0, 0, 0)

	case "HIf":
		if v, ok := foldConst(n[1].([]any)); ok {
			if v.Kind == KindBool {
				if v.Bool {
					c.compileStmt(n[2])
				} else if n[3] != nil {
					c.compileStmt(n[3])
				}
				c.freeTemps()
				return
			}
		}
		cond := c.compileExpr(n[1])
		jmpFalse := c.emit(OpJumpIfFalse, cond, 0, 0, 0)
		c.compileStmt(n[2])
		if n[3] != nil {
			jmpEnd := c.emit(OpJump, 0, 0, 0, 0)
			c.patchCond(jmpFalse)
			c.compileStmt(n[3])
			c.patchJump(jmpEnd)
		} else {
			c.patchCond(jmpFalse)
		}
		c.freeTemps()

	case "HWhile":
		if v, ok := foldConst(n[1].([]any)); ok {
			if v.Kind == KindBool && !v.Bool {
				c.freeTemps()
				return
			}
		}
		top := len(c.fn.Code)
		cond := c.compileExpr(n[1])
		jmpFalse := c.emit(OpJumpIfFalse, cond, 0, 0, 0)
		c.pushLoop()
		c.compileStmt(n[2])
		c.emit(OpJump, top, 0, 0, 0)
		c.patchCond(jmpFalse)
		c.popLoop(top)
		c.freeTemps()

	case "HFor":
		varName := n[1].(string)
		c.pushScope()
		init := c.compileExpr(n[2])
		slot := c.define(varName)
		if init != slot {
			c.emit(OpMove, slot, init, 0, 0)
		}
		c.freeTemps()
		top := len(c.fn.Code)
		cond := c.compileExpr(n[3])
		jmpFalse := c.emit(OpJumpIfFalse, cond, 0, 0, 0)
		c.pushLoop()
		c.compileStmt(n[5])
		continuePC := len(c.fn.Code)
		c.compileStmt(n[4])
		c.emit(OpJump, top, 0, 0, 0)
		c.patchCond(jmpFalse)
		c.popLoop(continuePC)
		c.popScope()

	case "HForRange":
		countReg := c.compileExpr(n[1])
		// Do NOT freeTemps here: it would clobber countReg when n[1] is
		// a literal and countReg is a fresh temp.
		iterSlot := c.allocReg()
		c.emit(OpLoadConst, iterSlot, c.addConst(IntVal(0)), 0, 0)
		top := len(c.fn.Code)
		cond := c.allocReg()
		c.emit(OpLt, cond, iterSlot, countReg, 0)
		jmpFalse := c.emit(OpJumpIfFalse, cond, 0, 0, 0)
		c.pushLoop()
		c.compileStmt(n[2])
		continuePC := len(c.fn.Code)
		c.emit(OpInc, iterSlot, 0, 0, 0)
		c.emit(OpJump, top, 0, 0, 0)
		c.patchCond(jmpFalse)
		c.popLoop(continuePC)
		c.freeTemps()

	case "HSwitch":
		var endJumps []int
		for _, cc := range n[1].([]any) {
			cn := cc.([]any)
			cond := c.compileExpr(cn[1])
			jmpNext := c.emit(OpJumpIfFalse, cond, 0, 0, 0)
			c.compileStmt(cn[2])
			endJumps = append(endJumps, c.emit(OpJump, 0, 0, 0, 0))
			c.patchCond(jmpNext)
		}
		if n[2] != nil {
			c.compileStmt(n[2])
		}
		for _, j := range endJumps {
			c.patchJump(j)
		}
		c.freeTemps()

	case "HBreak":
		if len(c.loops) == 0 {
			panic(&Err{Msg: "break outside loop"})
		}
		j := c.emit(OpJump, 0, 0, 0, 0)
		c.loops[len(c.loops)-1].breaks = append(c.loops[len(c.loops)-1].breaks, j)

	case "HContinue":
		if len(c.loops) == 0 {
			panic(&Err{Msg: "continue outside loop"})
		}
		j := c.emit(OpJump, 0, 0, 0, 0)
		c.loops[len(c.loops)-1].continues = append(c.loops[len(c.loops)-1].continues, j)

	default:
		panic(&Err{Msg: fmt.Sprintf("compile: unknown statement %q", n[0].(string))})
	}
}

func (c *fnCompiler) compileStore(tgt []any, valueReg int) {
	switch tgt[0].(string) {
	case "HIdent":
		name := tgt[1].(string)
		if slot, ok := c.resolveLocal(name); ok {
			c.emit(OpMove, slot, valueReg, 0, 0)
			return
		}
		if gi, ok := c.ip.GlobalNames[name]; ok {
			c.emit(OpStoreGlobal, gi, valueReg, 0, 0)
			return
		}
		panic(&Err{Msg: "undefined: " + name})

	case "HSel":
		base := c.compileExpr(tgt[1])
		c.emit(OpStoreField, base, valueReg, c.addField(tgt[2].(string)), 0)

	case "HIndex":
		base := c.compileExpr(tgt[1])
		key := c.compileExpr(tgt[2])
		c.emit(OpStoreIndex, base, key, valueReg, 0)

	case "HUn":
		if tgt[1].(string) == "*" {
			ref := c.compileExpr(tgt[2])
			c.emit(OpStoreRef, ref, valueReg, 0, 0)
			return
		}
		panic(&Err{Msg: "invalid store target"})

	default:
		panic(&Err{Msg: "invalid store target " + tgt[0].(string)})
	}
}

// ---------- expressions ----------

func (c *fnCompiler) compileInto(dest int, e any) {
	if e == nil {
		c.emit(OpNil, dest, 0, 0, 0)
		return
	}
	n := e.([]any)

	if v, ok := foldConst(n); ok {
		c.emitLoadConst(dest, v)
		return
	}

	switch n[0].(string) {
	case "HIdent":
		name := n[1].(string)
		if slot, ok := c.resolveLocal(name); ok {
			if slot != dest {
				c.emit(OpMove, dest, slot, 0, 0)
			}
			return
		}
		if gi, ok := c.ip.GlobalNames[name]; ok {
			c.emit(OpLoadGlobal, dest, gi, 0, 0)
			return
		}
		panic(&Err{Msg: "undefined: " + name})

	case "HUn":
		op := n[1].(string)
		if op == "&" {
			ref := c.compileRef(n[2].([]any))
			if ref != dest {
				c.emit(OpMove, dest, ref, 0, 0)
			}
			return
		}
		inner := c.compileExpr(n[2])
		switch op {
		case "-":
			c.emit(OpNeg, dest, inner, 0, 0)
		case "!":
			c.emit(OpNot, dest, inner, 0, 0)
		case "*":
			c.emit(OpDeref, dest, inner, 0, 0)
		default:
			panic(&Err{Msg: "unknown unary op " + op})
		}
		return

	case "HBin":
		op := n[1].(string)
		if op == "&&" || op == "||" {
			break
		}
		lr := c.compileExpr(n[2])
		rr := c.compileExpr(n[3])
		c.emit(binOp(op), dest, lr, rr, 0)
		return

	case "HCall":
		calleeReg := c.compileCall(n)
		if calleeReg != dest {
			c.emit(OpMove, dest, calleeReg, 0, 0)
		}
		return
	}

	r := c.compileExpr(e)
	if r != dest {
		c.emit(OpMove, dest, r, 0, 0)
	}
}

func (c *fnCompiler) compileExpr(e any) int {
	if e == nil {
		r := c.allocReg()
		c.emit(OpNil, r, 0, 0, 0)
		return r
	}
	n := e.([]any)

	if v, ok := foldConst(n); ok {
		r := c.allocReg()
		c.emitLoadConst(r, v)
		return r
	}

	switch n[0].(string) {
	case "HIdent":
		name := n[1].(string)
		if slot, ok := c.resolveLocal(name); ok {
			return slot
		}
		if gi, ok := c.ip.GlobalNames[name]; ok {
			r := c.allocReg()
			c.emit(OpLoadGlobal, r, gi, 0, 0)
			return r
		}
		panic(&Err{Msg: "undefined: " + name})

	case "HUn":
		op := n[1].(string)
		if op == "&" {
			return c.compileRef(n[2].([]any))
		}
		inner := c.compileExpr(n[2])
		r := c.allocReg()
		switch op {
		case "-":
			c.emit(OpNeg, r, inner, 0, 0)
		case "!":
			c.emit(OpNot, r, inner, 0, 0)
		case "*":
			c.emit(OpDeref, r, inner, 0, 0)
		default:
			panic(&Err{Msg: "unknown unary op " + op})
		}
		return r

	case "HBin":
		op := n[1].(string)
		if op == "&&" || op == "||" {
			return c.compileLogical(op, n[2], n[3])
		}
		lr := c.compileExpr(n[2])
		rr := c.compileExpr(n[3])
		dr := c.allocReg()
		c.emit(binOp(op), dr, lr, rr, 0)
		return dr

	case "HCall":
		return c.compileCall(n)

	case "HSel":
		base := c.compileExpr(n[1])
		r := c.allocReg()
		c.emit(OpField, r, base, c.addField(n[2].(string)), 0)
		return r

	case "HIndex":
		base := c.compileExpr(n[1])
		key := c.compileExpr(n[2])
		r := c.allocReg()
		c.emit(OpIndex, r, base, key, 0)
		return r

	case "HSlice":
		base := c.compileExpr(n[1])
		loReg := -1
		if n[2] != nil {
			loReg = c.compileExpr(n[2])
		}
		hiReg := -1
		if n[3] != nil {
			hiReg = c.compileExpr(n[3])
		}
		r := c.allocReg()
		c.emit(OpSlice, r, base, loReg, hiReg)
		return r

	case "HStructLit":
		name := n[1].(string)
		fields := n[2].([]any)
		fieldRegs := make([]int, len(fields))
		for i, fi := range fields {
			f := fi.([]any)
			fieldRegs[i] = c.compileExpr(f[2])
		}
		base := c.allocReg()
		for i := 1; i < 2*len(fields); i++ {
			c.allocReg()
		}
		for i, fi := range fields {
			f := fi.([]any)
			if fieldRegs[i] != base+2*i {
				c.emit(OpMove, base+2*i, fieldRegs[i], 0, 0)
			}
			c.emit(OpLoadConst, base+2*i+1, c.addConst(StrVal(f[1].(string))), 0, 0)
		}
		r := c.allocReg()
		c.emit(OpStructLit, r, base, len(fields), c.addStruct(name))
		return r

	case "HListLit":
		elems := n[2].([]any)
		regs := make([]int, len(elems))
		for i, e := range elems {
			regs[i] = c.compileExpr(e)
		}
		base := c.allocReg()
		for i := 1; i < len(elems); i++ {
			c.allocReg()
		}
		for i, r := range regs {
			if r != base+i {
				c.emit(OpMove, base+i, r, 0, 0)
			}
		}
		r := c.allocReg()
		c.emit(OpList, r, base, len(elems), 0)
		return r

	case "HSetLit":
		elemTy := "any"
		if tyNode, ok := n[1].([]any); ok && len(tyNode) >= 2 {
			if et, ok := tyNode[1].([]any); ok && len(et) >= 2 {
				elemTy, _ = et[1].(string)
			}
		}
		elems := n[2].([]any)
		regs := make([]int, len(elems))
		for i, e := range elems {
			regs[i] = c.compileExpr(e)
		}
		base := c.allocReg()
		for i := 1; i < len(elems); i++ {
			c.allocReg()
		}
		for i, r := range regs {
			if r != base+i {
				c.emit(OpMove, base+i, r, 0, 0)
			}
		}
		r := c.allocReg()
		c.emit(OpSet, r, base, len(elems), c.addConst(StrVal(elemTy)))
		return r

	case "HMapLit":
		entries := n[2].([]any)
		var regs []int
		for _, e := range entries {
			entry := e.([]any)
			regs = append(regs, c.compileExpr(entry[1]))
			regs = append(regs, c.compileExpr(entry[2]))
		}
		base := c.allocReg()
		for i := 1; i < len(regs); i++ {
			c.allocReg()
		}
		for i, r := range regs {
			if r != base+i {
				c.emit(OpMove, base+i, r, 0, 0)
			}
		}
		r := c.allocReg()
		c.emit(OpMap, r, base, len(entries), 0)
		return r

	case "HFnLit":
		compiled := c.compileClosure(n[1].([]any), n[4])
		idx := len(c.fn.Closures)
		c.fn.Closures = append(c.fn.Closures, compiled)
		r := c.allocReg()
		c.emit(OpClosure, r, idx, 0, 0)
		return r

	case "HSpread":
		panic(&Err{Msg: "spread outside call args"})
	}
	panic(&Err{Msg: fmt.Sprintf("compile: unknown expr %q", n[0].(string))})
}

func (c *fnCompiler) compileCall(n []any) int {
	args := n[2].([]any)
	for _, a := range args {
		if a.([]any)[0].(string) == "HSpread" {
			panic(&Err{Msg: "spread not yet supported in register VM"})
		}
	}

	calleeReg := c.allocReg()
	argStart := calleeReg + 1
	for i := 0; i < len(args); i++ {
		c.allocReg()
	}

	c.compileInto(calleeReg, n[1])
	for i, a := range args {
		c.compileInto(argStart+i, a)
	}

	c.emit(OpCall, calleeReg, len(args), 0, 0)
	return calleeReg
}

func (c *fnCompiler) compileLogical(op string, l, r any) int {
	lr := c.compileExpr(l)
	dr := c.allocReg()
	if op == "&&" {
		c.emit(OpFalse, dr, 0, 0, 0)
		jmp := c.emit(OpJumpIfFalse, lr, 0, 0, 0)
		rr := c.compileExpr(r)
		c.emit(OpTruthy, dr, rr, 0, 0)
		c.patchCond(jmp)
	} else {
		c.emit(OpTrue, dr, 0, 0, 0)
		jmp := c.emit(OpJumpIfTrue, lr, 0, 0, 0)
		rr := c.compileExpr(r)
		c.emit(OpTruthy, dr, rr, 0, 0)
		c.patchCond(jmp)
	}
	return dr
}

func (c *fnCompiler) compileRef(tgt []any) int {
	switch tgt[0].(string) {
	case "HIdent":
		name := tgt[1].(string)
		if slot, ok := c.resolveLocal(name); ok {
			r := c.allocReg()
			c.emit(OpRefLocal, r, slot, 0, 0)
			return r
		}
		if gi, ok := c.ip.GlobalNames[name]; ok {
			r := c.allocReg()
			c.emit(OpRefGlobal, r, gi, 0, 0)
			return r
		}
		panic(&Err{Msg: "undefined: " + name})
	case "HSel":
		base := c.compileExpr(tgt[1])
		r := c.allocReg()
		c.emit(OpRefField, r, base, c.addField(tgt[2].(string)), 0)
		return r
	case "HIndex":
		base := c.compileExpr(tgt[1])
		key := c.compileExpr(tgt[2])
		r := c.allocReg()
		c.emit(OpRefIndex, r, base, key, 0)
		return r
	case "HStructLit":
		inner := c.compileExpr(tgt)
		r := c.allocReg()
		c.emit(OpRefNew, r, inner, 0, 0)
		return r
	default:
		panic(&Err{Msg: "cannot take address of " + tgt[0].(string)})
	}
}

func (c *fnCompiler) compileClosure(params []any, body any) *CompiledFn {
	child := &fnCompiler{
		ip: c.ip,
		fn: &CompiledFn{},
	}
	child.pushScope()
	for _, p := range params {
		pp := p.([]any)
		child.define(pp[1].(string))
		child.fn.NumArgs++
		if len(pp) > 3 {
			if v, ok := pp[3].(bool); ok && v {
				child.fn.Variadic = true
			}
		}
	}
	child.compileStmt(body)
	r := child.allocReg()
	child.emit(OpNil, r, 0, 0, 0)
	child.emit(OpReturn, r, 0, 0, 0)
	child.popScope()
	child.peephole()
	return child.fn
}

// ---------- top-level compilation ----------

type fnKey struct {
	recv, name string
}

func (ip *Interpreter) compileFn(n []any) {
	name := n[1].(string)
	params := n[2].([]any)
	body := n[4]
	var recv string
	if len(n) > 5 && n[5] != nil {
		recv, _ = n[5].(string)
	}

	c := &fnCompiler{
		ip: ip,
		fn: &CompiledFn{Name: name},
	}
	c.pushScope()
	for _, p := range params {
		pp := p.([]any)
		c.define(pp[1].(string))
		c.fn.NumArgs++
		if len(pp) > 3 {
			if v, ok := pp[3].(bool); ok && v {
				c.fn.Variadic = true
			}
		}
	}
	c.compileStmt(body)
	r := c.allocReg()
	c.emit(OpNil, r, 0, 0, 0)
	c.emit(OpReturn, r, 0, 0, 0)
	c.popScope()
	c.peephole()

	fn := ip.pendingFns[fnKey{recv, name}]
	if fn == nil {
		panic(&Err{Msg: "internal: no pending fn for " + recv + "." + name})
	}
	fn.Compiled = c.fn
}

func (ip *Interpreter) compileInit(decls []any) *CompiledFn {
	c := &fnCompiler{ip: ip, fn: &CompiledFn{Name: "<init>"}}
	c.pushScope()
	for _, d := range decls {
		n := d.([]any)
		if n[0].(string) != "GlobalLet" {
			continue
		}
		names := n[1].([]string)
		expr := n[3]
		if expr == nil {
			continue
		}
		r := c.compileExpr(expr)
		if len(names) == 1 {
			gi := ip.globalSlot(names[0])
			c.emit(OpStoreGlobal, gi, r, 0, 0)
			c.freeTemps()
			continue
		}
		base := c.allocReg()
		for i := 1; i < len(names); i++ {
			c.allocReg()
		}
		c.emit(OpDestructure, base, r, len(names), 0)
		for i := len(names) - 1; i >= 0; i-- {
			gi := ip.globalSlot(names[i])
			c.emit(OpStoreGlobal, gi, base+i, 0, 0)
		}
		c.freeTemps()
	}
	c.emit(OpNil, 0, 0, 0, 0)
	c.emit(OpReturn, 0, 0, 0, 0)
	c.popScope()
	c.peephole()
	return c.fn
}

// compileChunk lowers top-level declarations into a single CompiledFn:
// GlobalLet initializers run as OpStoreGlobal; TopStmt items run as
// plain statements. It's what makes `Run` and `Exec` able to process
// a whole program chunk in one `ip.run` call.
//
// Functions in `decls` are expected to have already been declared and
// compiled by the caller (see Run / Exec); this only handles the
// executable top-level statements.
func (ip *Interpreter) compileChunk(decls []any) *CompiledFn {
	c := &fnCompiler{ip: ip, fn: &CompiledFn{Name: "<chunk>"}}
	c.pushScope()
	for _, d := range decls {
		n, ok := d.([]any)
		if !ok || len(n) == 0 {
			continue
		}
		switch n[0].(string) {
		case "GlobalLet":
			names := n[1].([]string)
			expr := n[3]
			if expr == nil {
				continue
			}
			r := c.compileExpr(expr)
			if len(names) == 1 {
				gi := ip.globalSlot(names[0])
				c.emit(OpStoreGlobal, gi, r, 0, 0)
				c.freeTemps()
				continue
			}
			base := c.allocReg()
			for i := 1; i < len(names); i++ {
				c.allocReg()
			}
			c.emit(OpDestructure, base, r, len(names), 0)
			for i := len(names) - 1; i >= 0; i-- {
				gi := ip.globalSlot(names[i])
				c.emit(OpStoreGlobal, gi, base+i, 0, 0)
			}
			c.freeTemps()

		case "TopStmt":
			for _, st := range n[1].([]any) {
				c.compileStmt(st)
			}
		}
	}
	c.emit(OpNil, 0, 0, 0, 0)
	c.emit(OpReturn, 0, 0, 0, 0)
	c.popScope()
	c.peephole()
	return c.fn
}

func binOp(op string) Op {
	switch op {
	case "+":
		return OpAdd
	case "-":
		return OpSub
	case "*":
		return OpMul
	case "/":
		return OpDiv
	case "%":
		return OpMod
	case "==":
		return OpEq
	case "!=":
		return OpNeq
	case "<":
		return OpLt
	case "<=":
		return OpLe
	case ">":
		return OpGt
	case ">=":
		return OpGe
	}
	panic(&Err{Msg: "unknown binary op " + op})
}
