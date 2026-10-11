package interpreter

import (
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

type Interpreter struct {
	Globals     []Value
	GlobalNames map[string]int

	Schemas map[string]*StructSchema
	Methods map[string]map[string]Value

	pendingFns map[fnKey]*Function

	Out   io.Writer
	Trace bool

	mem   []Value
	stack []Frame
}

type Frame struct {
	Fn   *CompiledFn
	Args []Value
}

type exitSignal struct{ code int }

func New(out io.Writer) *Interpreter {
	ip := &Interpreter{
		Globals:     []Value{},
		GlobalNames: map[string]int{},
		Schemas:     map[string]*StructSchema{},
		Methods:     map[string]map[string]Value{},
		pendingFns:  map[fnKey]*Function{},
		Out:         out,
		mem:         make([]Value, 0, 1<<16),
	}
	ip.installBuiltins()
	return ip
}

func (ip *Interpreter) globalSlot(name string) int {
	if gi, ok := ip.GlobalNames[name]; ok {
		return gi
	}
	gi := len(ip.Globals)
	ip.GlobalNames[name] = gi
	ip.Globals = append(ip.Globals, NilVal())
	return gi
}

// ---------- program entry ----------

// Run executes a program. `main` is optional: scripts with only
// top-level declarations and statements run to completion without
// one. The REPL uses Exec instead of Run to persist state across
// inputs.
func (ip *Interpreter) Run(prog []any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(exitSignal); ok {
				if e.code != 0 {
					err = fmt.Errorf("exit status %d", e.code)
				}
				return
			}
			if e, ok := r.(*Err); ok {
				if st := ip.StackTrace(); st != "" {
					err = fmt.Errorf("%s\ncall stack:\n%s", e.Msg, st)
				} else {
					err = fmt.Errorf("%s", e.Msg)
				}
				return
			}
			panic(r)
		}
	}()

	decls := prog[3].([]any)
	for _, d := range decls {
		ip.declare(d)
	}
	for _, d := range decls {
		n := d.([]any)
		if n[0].(string) == "Fn" {
			ip.compileFn(n)
		}
	}

	chunkFn := ip.compileChunk(decls)
	if len(chunkFn.Code) > 2 {
		ip.run(chunkFn, 0)
	}

	// main is optional.
	if gi, ok := ip.GlobalNames["main"]; ok {
		mv := ip.Globals[gi]
		if mv.Kind == KindFunction {
			ip.run(mv.Obj.(*Function).Compiled, 0)
		}
	}
	return nil
}

// Exec runs the declarations in prog against the interpreter's live
// global state. Unlike Run, it does not reset globals, does not look
// for `main`, and returns the value the chunk produced (NilVal for
// pure declaration chunks).
//
// This is the REPL's workhorse: each input is lowered to a single
// declaration (or one synthetic function), Exec'd here, and any
// globals or functions it defines persist for the next input.
func (ip *Interpreter) Exec(prog []any) (result Value, err error) {
	result = NilVal()

	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(exitSignal); ok {
				return
			}
			if e, ok := r.(*Err); ok {
				err = fmt.Errorf("%s", e.Msg)
				return
			}
			panic(r)
		}
	}()

	decls, _ := prog[3].([]any)
	for _, d := range decls {
		ip.declare(d)
	}
	for _, d := range decls {
		n, ok := d.([]any)
		if !ok || len(n) == 0 {
			continue
		}
		if n[0].(string) == "Fn" {
			ip.compileFn(n)
		}
	}

	chunkFn := ip.compileChunk(decls)
	if len(chunkFn.Code) > 2 {
		result = ip.run(chunkFn, 0)
	}
	return result, nil
}

// CallGlobal invokes a named global function with args and returns
// its value. Used by the REPL to run the synthetic function it wraps
// each inline input in.
func (ip *Interpreter) CallGlobal(name string, args ...Value) (Value, error) {
	gi, ok := ip.GlobalNames[name]
	if !ok {
		return NilVal(), fmt.Errorf("undefined: %s", name)
	}
	callee := ip.Globals[gi]
	if callee.Kind != KindFunction {
		return NilVal(), fmt.Errorf("%s is not a function", name)
	}
	cf := callee.Obj.(*Function).Compiled

	nargs := len(args)
	if nargs > cf.NumArgs {
		nargs = cf.NumArgs
	}
	top := cf.NumRegs
	if top > len(ip.mem) {
		if top > cap(ip.mem) {
			newCap := cap(ip.mem) * 2
			if newCap < top {
				newCap = top
			}
			newMem := make([]Value, top, newCap)
			copy(newMem, ip.mem)
			ip.mem = newMem
		} else {
			ip.mem = ip.mem[:top]
		}
	}
	for i := 0; i < nargs; i++ {
		ip.mem[i] = args[i]
	}
	return ip.run(cf, 0), nil
}

// BindPackageMember makes `local` a top-level alias for `pkg.member`.
// Used by the REPL to expose print/println without an explicit
// import. No-op if the package or member doesn't exist.
func (ip *Interpreter) BindPackageMember(local, pkg, member string) {
	pgi, ok := ip.GlobalNames[pkg]
	if !ok {
		return
	}
	pv := ip.Globals[pgi]
	if pv.Kind != KindPackage {
		return
	}
	p, ok := pv.Obj.(*pkgValue)
	if !ok {
		return
	}
	v, ok := p.Members[member]
	if !ok {
		return
	}
	gi := ip.globalSlot(local)
	ip.Globals[gi] = v
}

func (ip *Interpreter) declare(d any) {
	n := d.([]any)
	switch n[0].(string) {
	case "Fn":
		name := n[1].(string)
		params := n[2].([]any)
		var recv string
		if len(n) > 5 && n[5] != nil {
			recv, _ = n[5].(string)
		}
		fn := &Function{
			Name:   name,
			Params: buildParams(params),
			RecvTy: recv,
		}
		ip.pendingFns[fnKey{recv, name}] = fn
		if recv != "" {
			m := ip.Methods[recv]
			if m == nil {
				m = map[string]Value{}
				ip.Methods[recv] = m
			}
			m[name] = FnVal(fn)
		} else {
			gi := ip.globalSlot(name)
			ip.Globals[gi] = FnVal(fn)
		}
	case "GlobalLet":
		for _, name := range n[1].([]string) {
			ip.globalSlot(name)
		}
	case "Struct":
		name := n[1].(string)
		fields := n[2].([]any)
		schema := &StructSchema{Name: name, Fields: map[string]string{}}
		for _, f := range fields {
			ff := f.([]any)
			schema.Fields[ff[1].(string)] = typeNodeName(ff[2])
		}
		ip.Schemas[name] = schema
	case "Interface":
	}
}

func buildParams(ps []any) []Param {
	out := make([]Param, len(ps))
	for i, p := range ps {
		pp := p.([]any)
		variadic := false
		if len(pp) > 3 {
			if v, ok := pp[3].(bool); ok {
				variadic = v
			}
		}
		out[i] = Param{Name: pp[1].(string), Variadic: variadic}
	}
	return out
}

func (ip *Interpreter) StackTrace() string {
	if len(ip.stack) <= 1 {
		return ""
	}
	var b strings.Builder
	for i := len(ip.stack) - 1; i >= 0; i-- {
		f := ip.stack[i]
		name := f.Fn.Name
		if name == "" {
			name = "<fn>"
		}
		argStrs := make([]string, len(f.Args))
		for j, a := range f.Args {
			argStrs[j] = Display(a)
		}
		fmt.Fprintf(&b, "  at %s(%s)\n", name, strings.Join(argStrs, ", "))
	}
	return b.String()
}

// ---------- the VM ----------

func (ip *Interpreter) run(fn *CompiledFn, base int) Value {
	top := base + fn.NumRegs

	if top > len(ip.mem) {
		if top > cap(ip.mem) {
			newCap := cap(ip.mem) * 2
			if newCap < top {
				newCap = top
			}
			newMem := make([]Value, top, newCap)
			copy(newMem, ip.mem)
			ip.mem = newMem
		} else {
			ip.mem = ip.mem[:top]
		}
	}

	// regs[i] == ip.mem[base+i] for the rest of this frame.
	// Must be refreshed after any call into ip.run (mem may reallocate).
	regs := ip.mem[base:top:top]

	if ip.Trace {
		ip.stack = append(ip.stack, Frame{Fn: fn})
		defer func() {
			ip.stack = ip.stack[:len(ip.stack)-1]
			fmt.Fprintf(ip.Out, "<- %s\n", traceName(fn))
		}()
		fmt.Fprintf(ip.Out, "-> %s\n", traceName(fn))
	}

	code := fn.Code
	consts := fn.Consts
	fields := fn.Fields
	structs := fn.Structs
	pc := 0

	for {
		in := code[pc]
		pc++

		switch in.Op {
		case OpNop:

		case OpLoadConst:
			regs[in.A] = consts[in.B]
		case OpTrue:
			regs[in.A] = BoolVal(true)
		case OpFalse:
			regs[in.A] = BoolVal(false)
		case OpNil:
			regs[in.A] = NilVal()
		case OpMove:
			regs[in.A] = regs[in.B]
		case OpLoadGlobal:
			regs[in.A] = ip.Globals[in.B]
		case OpStoreGlobal:
			ip.Globals[in.A] = regs[in.B]

		case OpAdd:
			lhs := regs[in.B]
			rhs := regs[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = Value{Kind: KindInt, Int: lhs.Int + rhs.Int}
			} else {
				regs[in.A] = arith(OpAdd, lhs, rhs)
			}
		case OpSub:
			lhs := regs[in.B]
			rhs := regs[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = Value{Kind: KindInt, Int: lhs.Int - rhs.Int}
			} else {
				regs[in.A] = arith(OpSub, lhs, rhs)
			}
		case OpMul:
			lhs := regs[in.B]
			rhs := regs[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = Value{Kind: KindInt, Int: lhs.Int * rhs.Int}
			} else {
				regs[in.A] = arith(OpMul, lhs, rhs)
			}
		case OpDiv:
			lhs := regs[in.B]
			rhs := regs[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				if rhs.Int == 0 {
					panic(&Err{Msg: "division by zero"})
				}
				regs[in.A] = Value{Kind: KindInt, Int: lhs.Int / rhs.Int}
			} else {
				regs[in.A] = arith(OpDiv, lhs, rhs)
			}
		case OpMod:
			lhs := regs[in.B]
			rhs := regs[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				if rhs.Int == 0 {
					panic(&Err{Msg: "modulo by zero"})
				}
				regs[in.A] = Value{Kind: KindInt, Int: lhs.Int % rhs.Int}
			} else {
				regs[in.A] = arith(OpMod, lhs, rhs)
			}
		case OpEq:
			lhs := regs[in.B]
			rhs := regs[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = BoolVal(lhs.Int == rhs.Int)
			} else {
				regs[in.A] = arith(OpEq, lhs, rhs)
			}
		case OpNeq:
			lhs := regs[in.B]
			rhs := regs[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = BoolVal(lhs.Int != rhs.Int)
			} else {
				regs[in.A] = arith(OpNeq, lhs, rhs)
			}
		case OpLt:
			lhs := regs[in.B]
			rhs := regs[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = BoolVal(lhs.Int < rhs.Int)
			} else {
				regs[in.A] = arith(OpLt, lhs, rhs)
			}
		case OpLe:
			lhs := regs[in.B]
			rhs := regs[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = BoolVal(lhs.Int <= rhs.Int)
			} else {
				regs[in.A] = arith(OpLe, lhs, rhs)
			}
		case OpGt:
			lhs := regs[in.B]
			rhs := regs[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = BoolVal(lhs.Int > rhs.Int)
			} else {
				regs[in.A] = arith(OpGt, lhs, rhs)
			}
		case OpGe:
			lhs := regs[in.B]
			rhs := regs[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = BoolVal(lhs.Int >= rhs.Int)
			} else {
				regs[in.A] = arith(OpGe, lhs, rhs)
			}

		case OpAddConst:
			lhs := regs[in.B]
			rhs := consts[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = Value{Kind: KindInt, Int: lhs.Int + rhs.Int}
			} else {
				regs[in.A] = arith(OpAdd, lhs, rhs)
			}
		case OpSubConst:
			lhs := regs[in.B]
			rhs := consts[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = Value{Kind: KindInt, Int: lhs.Int - rhs.Int}
			} else {
				regs[in.A] = arith(OpSub, lhs, rhs)
			}
		case OpMulConst:
			lhs := regs[in.B]
			rhs := consts[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = Value{Kind: KindInt, Int: lhs.Int * rhs.Int}
			} else {
				regs[in.A] = arith(OpMul, lhs, rhs)
			}
		case OpDivConst:
			lhs := regs[in.B]
			rhs := consts[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				if rhs.Int == 0 {
					panic(&Err{Msg: "division by zero"})
				}
				regs[in.A] = Value{Kind: KindInt, Int: lhs.Int / rhs.Int}
			} else {
				regs[in.A] = arith(OpDiv, lhs, rhs)
			}
		case OpModConst:
			lhs := regs[in.B]
			rhs := consts[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				if rhs.Int == 0 {
					panic(&Err{Msg: "modulo by zero"})
				}
				regs[in.A] = Value{Kind: KindInt, Int: lhs.Int % rhs.Int}
			} else {
				regs[in.A] = arith(OpMod, lhs, rhs)
			}
		case OpEqConst:
			lhs := regs[in.B]
			rhs := consts[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = BoolVal(lhs.Int == rhs.Int)
			} else {
				regs[in.A] = arith(OpEq, lhs, rhs)
			}
		case OpNeqConst:
			lhs := regs[in.B]
			rhs := consts[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = BoolVal(lhs.Int != rhs.Int)
			} else {
				regs[in.A] = arith(OpNeq, lhs, rhs)
			}
		case OpLtConst:
			lhs := regs[in.B]
			rhs := consts[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = BoolVal(lhs.Int < rhs.Int)
			} else {
				regs[in.A] = arith(OpLt, lhs, rhs)
			}
		case OpLeConst:
			lhs := regs[in.B]
			rhs := consts[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = BoolVal(lhs.Int <= rhs.Int)
			} else {
				regs[in.A] = arith(OpLe, lhs, rhs)
			}
		case OpGtConst:
			lhs := regs[in.B]
			rhs := consts[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = BoolVal(lhs.Int > rhs.Int)
			} else {
				regs[in.A] = arith(OpGt, lhs, rhs)
			}
		case OpGeConst:
			lhs := regs[in.B]
			rhs := consts[in.C]
			if lhs.Kind == KindInt && rhs.Kind == KindInt {
				regs[in.A] = BoolVal(lhs.Int >= rhs.Int)
			} else {
				regs[in.A] = arith(OpGe, lhs, rhs)
			}

		// ----- comparison + jump superinstructions (reg, reg) -----
		case OpLtJumpIfFalse, OpLeJumpIfFalse, OpGtJumpIfFalse, OpGeJumpIfFalse:
			l := regs[in.A]
			r := regs[in.B]
			var cond bool
			if l.Kind == KindInt && r.Kind == KindInt {
				switch in.Op {
				case OpLtJumpIfFalse:
					cond = l.Int < r.Int
				case OpLeJumpIfFalse:
					cond = l.Int <= r.Int
				case OpGtJumpIfFalse:
					cond = l.Int > r.Int
				case OpGeJumpIfFalse:
					cond = l.Int >= r.Int
				}
			} else {
				var op Op
				switch in.Op {
				case OpLtJumpIfFalse:
					op = OpLt
				case OpLeJumpIfFalse:
					op = OpLe
				case OpGtJumpIfFalse:
					op = OpGt
				case OpGeJumpIfFalse:
					op = OpGe
				}
				cond = arith(op, l, r).Bool
			}
			regs[in.D] = BoolVal(cond)
			if !cond {
				pc = in.C
			}

		// ----- comparison + jump superinstructions (reg, const) -----
		case OpLtConstJumpIfFalse, OpLeConstJumpIfFalse,
			OpGtConstJumpIfFalse, OpGeConstJumpIfFalse:
			l := regs[in.A]
			r := consts[in.B]
			var cond bool
			if l.Kind == KindInt && r.Kind == KindInt {
				switch in.Op {
				case OpLtConstJumpIfFalse:
					cond = l.Int < r.Int
				case OpLeConstJumpIfFalse:
					cond = l.Int <= r.Int
				case OpGtConstJumpIfFalse:
					cond = l.Int > r.Int
				case OpGeConstJumpIfFalse:
					cond = l.Int >= r.Int
				}
			} else {
				var op Op
				switch in.Op {
				case OpLtConstJumpIfFalse:
					op = OpLt
				case OpLeConstJumpIfFalse:
					op = OpLe
				case OpGtConstJumpIfFalse:
					op = OpGt
				case OpGeConstJumpIfFalse:
					op = OpGe
				}
				cond = arith(op, l, r).Bool
			}
			regs[in.D] = BoolVal(cond)
			if !cond {
				pc = in.C
			}

		case OpNeg:
			v := regs[in.B]
			switch v.Kind {
			case KindInt:
				regs[in.A] = IntVal(-v.Int)
			case KindFloat:
				regs[in.A] = FloatVal(-v.Float)
			default:
				panic(&Err{Msg: "unary - on " + kindName(v.Kind)})
			}
		case OpNot:
			regs[in.A] = BoolVal(!regs[in.B].Truthy())
		case OpTruthy:
			regs[in.A] = BoolVal(regs[in.B].Truthy())
		case OpInc:
			v := &regs[in.A]
			if v.Kind != KindInt {
				panic(&Err{Msg: "++ on " + kindName(v.Kind)})
			}
			v.Int++
		case OpDec:
			v := &regs[in.A]
			if v.Kind != KindInt {
				panic(&Err{Msg: "-- on " + kindName(v.Kind)})
			}
			v.Int--

		case OpJump:
			pc = in.A
		case OpJumpIfFalse:
			if !regs[in.A].Truthy() {
				pc = in.B
			}
		case OpJumpIfTrue:
			if regs[in.A].Truthy() {
				pc = in.B
			}
		case OpJumpIfNotErr:
			if regs[in.A].Kind != KindErr {
				pc = in.B
			}

		case OpCall:
			cr := in.A
			argc := in.B
			callee := regs[cr]
			argBaseAbs := base + cr + 1

			switch callee.Kind {
			case KindFunction:
				cf := callee.Obj.(*Function).Compiled
				r := ip.run(cf, argBaseAbs)
				regs = ip.mem[base:top:top]
				regs[cr] = r
			case KindBuiltin:
				args := regs[cr+1 : cr+1+argc : cr+1+argc]
				r := callee.Obj.(*Builtin).Fn(ip, args)
				regs[cr] = r
			case KindBoundMethod:
				bm := callee.Obj.(*BoundMethod)
				regs[cr] = bm.Recv
				r := ip.run(bm.Fn.Compiled, base+cr)
				regs = ip.mem[base:top:top]
				regs[cr] = r
			default:
				panic(&Err{Msg: fmt.Sprintf(
					"cannot call %s (fn=%s pc=%d inA=%d base=%d)",
					kindName(callee.Kind), traceName(fn), pc-1, in.A, base,
				)})
			}

		case OpCallGlobal:
			gidx := in.A
			argc := in.B
			cr := in.C
			callee := ip.Globals[gidx]
			argBaseAbs := base + cr + 1

			switch callee.Kind {
			case KindFunction:
				cf := callee.Obj.(*Function).Compiled
				r := ip.run(cf, argBaseAbs)
				regs = ip.mem[base:top:top]
				regs[cr] = r
			case KindBuiltin:
				args := regs[cr+1 : cr+1+argc : cr+1+argc]
				r := callee.Obj.(*Builtin).Fn(ip, args)
				regs[cr] = r
			case KindBoundMethod:
				bm := callee.Obj.(*BoundMethod)
				regs[cr] = bm.Recv
				r := ip.run(bm.Fn.Compiled, base+cr)
				regs = ip.mem[base:top:top]
				regs[cr] = r
			default:
				panic(&Err{Msg: fmt.Sprintf(
					"cannot call global[%d] = %s (fn=%s pc=%d base=%d)",
					gidx, kindName(callee.Kind), traceName(fn), pc-1, base,
				)})
			}

		case OpReturn:
			return regs[in.A]

		case OpList:
			n := in.C
			items := make([]Value, n)
			copy(items, regs[in.B:in.B+n])
			regs[in.A] = ListVal(&List{Items: items})

		case OpSet:
			n := in.C
			elemTy := "any"
			if in.D < len(consts) {
				elemTy = consts[in.D].Str
			}
			set := &Set{ElemTy: elemTy}
			for i := 0; i < n; i++ {
				set.Add(regs[in.B+i])
			}
			regs[in.A] = SetVal(set)

		case OpMap:
			n := in.C
			mp := &Map{Data: map[Value]Value{}}
			for i := 0; i < n; i++ {
				k := regs[in.B+2*i]
				v := regs[in.B+2*i+1]
				mp.Data[k] = v
			}
			regs[in.A] = MapVal(mp)

		case OpStructLit:
			name := structs[in.D]
			schema := ip.Schemas[name]
			s := &Struct{TypeName: name, Fields: map[string]Value{}}
			if schema != nil {
				for fname, ftype := range schema.Fields {
					s.Fields[fname] = ip.zeroValueOf(ftype)
				}
			}
			for i := 0; i < in.C; i++ {
				val := regs[in.B+2*i]
				fname := regs[in.B+2*i+1].Str
				s.Fields[fname] = val
			}
			regs[in.A] = StructVal(s)

		case OpDestructure:
			src := regs[in.B]
			if src.Kind != KindList {
				panic(&Err{Msg: "cannot destructure " + kindName(src.Kind)})
			}
			items := src.Obj.(*List).Items
			if len(items) != in.C {
				panic(&Err{Msg: fmt.Sprintf("expected %d values, got %d", in.C, len(items))})
			}
			for i := 0; i < in.C; i++ {
				regs[in.A+i] = items[i]
			}

		case OpField:
			obj := regs[in.B].deref()
			regs[in.A] = ip.selectField(obj, fields[in.C])

		case OpIndex:
			obj := regs[in.B].deref()
			key := regs[in.C]
			regs[in.A] = ip.index(obj, key)

		case OpSlice:
			obj := regs[in.B].deref()
			haveLo := in.C >= 0
			haveHi := in.D >= 0
			var lo, hi Value
			if haveLo {
				lo = regs[in.C]
			}
			if haveHi {
				hi = regs[in.D]
			}
			regs[in.A] = ip.slice(obj, lo, haveLo, hi, haveHi)

		case OpRefLocal:
			regs[in.A] = RefVal(&localRef{ip: ip, idx: base + in.B})
		case OpRefGlobal:
			regs[in.A] = RefVal(&globalRef{ip: ip, idx: in.B})
		case OpRefField:
			obj := regs[in.B].deref()
			if obj.Kind != KindStruct {
				panic(&Err{Msg: "cannot take address of field of " + kindName(obj.Kind)})
			}
			regs[in.A] = RefVal(&FieldRef{Owner: obj.Obj.(*Struct), Name: fields[in.C]})
		case OpRefIndex:
			obj := regs[in.B].deref()
			key := regs[in.C]
			switch obj.Kind {
			case KindMap:
				regs[in.A] = RefVal(&mapRef{m: obj.Obj.(*Map), key: key})
			case KindList:
				if key.Kind != KindInt {
					panic(&Err{Msg: "list index must be int"})
				}
				l := obj.Obj.(*List)
				if key.Int < 0 || key.Int >= len(l.Items) {
					panic(&Err{Msg: "index out of range"})
				}
				regs[in.A] = RefVal(&listRef{l: l, i: key.Int})
			default:
				panic(&Err{Msg: "cannot take address of index of " + kindName(obj.Kind)})
			}
		case OpRefNew:
			regs[in.A] = RefVal(&Slot{Value: regs[in.B]})
		case OpDeref:
			v := regs[in.B]
			if v.Kind != KindRef {
				panic(&Err{Msg: "cannot dereference " + kindName(v.Kind)})
			}
			regs[in.A] = v.Obj.(Ref).Get()

		case OpStoreRef:
			ref := regs[in.A]
			if ref.Kind != KindRef {
				panic(&Err{Msg: "cannot store through " + kindName(ref.Kind)})
			}
			ref.Obj.(Ref).Set(regs[in.B])
		case OpStoreField:
			obj := regs[in.A].deref()
			if obj.Kind != KindStruct {
				panic(&Err{Msg: "cannot store into field of " + kindName(obj.Kind)})
			}
			obj.Obj.(*Struct).Fields[fields[in.C]] = regs[in.B]
		case OpStoreIndex:
			obj := regs[in.A].deref()
			key := regs[in.B]
			val := regs[in.C]
			switch obj.Kind {
			case KindMap:
				obj.Obj.(*Map).Data[key] = val
			case KindList:
				if key.Kind != KindInt {
					panic(&Err{Msg: "list index must be int"})
				}
				l := obj.Obj.(*List)
				if key.Int < 0 || key.Int >= len(l.Items) {
					panic(&Err{Msg: "index out of range"})
				}
				l.Items[key.Int] = val
			default:
				panic(&Err{Msg: "cannot store through index of " + kindName(obj.Kind)})
			}

		case OpMake:
			typeName := consts[in.B].Str
			if in.C == 1 {
				length := regs[in.D].Int
				if strings.HasPrefix(typeName, "[]") {
					elemTy := typeName[2:]
					items := make([]Value, length)
					for i := 0; i < length; i++ {
						items[i] = ip.zeroValueOf(elemTy)
					}
					regs[in.A] = ListVal(&List{Items: items})
					continue
				}
			}
			v := ip.zeroValueOf(typeName)
			if v.Kind == KindStruct {
				v = RefVal(&Slot{Value: v})
			}
			regs[in.A] = v

		case OpClosure:
			cf := fn.Closures[in.B]
			regs[in.A] = FnVal(&Function{Compiled: cf})

		default:
			panic(&Err{Msg: fmt.Sprintf("vm: bad opcode %d", in.Op)})
		}
	}
}

func traceName(fn *CompiledFn) string {
	if fn.Name == "" {
		return "<fn>"
	}
	return fn.Name
}

type globalRef struct {
	ip  *Interpreter
	idx int
}

func (r *globalRef) Get() Value  { return r.ip.Globals[r.idx] }
func (r *globalRef) Set(v Value) { r.ip.Globals[r.idx] = v }

// ---------- arithmetic ----------

func arith(op Op, a, b Value) Value {
	if a.Kind == KindInt && b.Kind == KindInt {
		switch op {
		case OpAdd:
			return IntVal(a.Int + b.Int)
		case OpSub:
			return IntVal(a.Int - b.Int)
		case OpMul:
			return IntVal(a.Int * b.Int)
		case OpDiv:
			if b.Int == 0 {
				panic(&Err{Msg: "division by zero"})
			}
			return IntVal(a.Int / b.Int)
		case OpMod:
			if b.Int == 0 {
				panic(&Err{Msg: "modulo by zero"})
			}
			return IntVal(a.Int % b.Int)
		case OpEq:
			return BoolVal(a.Int == b.Int)
		case OpNeq:
			return BoolVal(a.Int != b.Int)
		case OpLt:
			return BoolVal(a.Int < b.Int)
		case OpLe:
			return BoolVal(a.Int <= b.Int)
		case OpGt:
			return BoolVal(a.Int > b.Int)
		case OpGe:
			return BoolVal(a.Int >= b.Int)
		}
	}
	if a.Kind == KindFloat && b.Kind == KindFloat {
		switch op {
		case OpAdd:
			return FloatVal(a.Float + b.Float)
		case OpSub:
			return FloatVal(a.Float - b.Float)
		case OpMul:
			return FloatVal(a.Float * b.Float)
		case OpDiv:
			return FloatVal(a.Float / b.Float)
		case OpEq:
			return BoolVal(a.Float == b.Float)
		case OpNeq:
			return BoolVal(a.Float != b.Float)
		case OpLt:
			return BoolVal(a.Float < b.Float)
		case OpLe:
			return BoolVal(a.Float <= b.Float)
		case OpGt:
			return BoolVal(a.Float > b.Float)
		case OpGe:
			return BoolVal(a.Float >= b.Float)
		}
	}
	if a.Kind == KindInt && b.Kind == KindFloat {
		return arith(op, FloatVal(float64(a.Int)), b)
	}
	if a.Kind == KindFloat && b.Kind == KindInt {
		return arith(op, a, FloatVal(float64(b.Int)))
	}
	if a.Kind == KindString && b.Kind == KindString {
		switch op {
		case OpAdd:
			return StrVal(a.Str + b.Str)
		case OpEq:
			return BoolVal(a.Str == b.Str)
		case OpNeq:
			return BoolVal(a.Str != b.Str)
		case OpLt:
			return BoolVal(a.Str < b.Str)
		case OpLe:
			return BoolVal(a.Str <= b.Str)
		case OpGt:
			return BoolVal(a.Str > b.Str)
		case OpGe:
			return BoolVal(a.Str >= b.Str)
		}
	}
	if a.Kind == KindBool && b.Kind == KindBool {
		switch op {
		case OpEq:
			return BoolVal(a.Bool == b.Bool)
		case OpNeq:
			return BoolVal(a.Bool != b.Bool)
		}
	}
	if op == OpEq {
		return BoolVal(looseEq(a, b))
	}
	if op == OpNeq {
		return BoolVal(!looseEq(a, b))
	}
	panic(&Err{Msg: fmt.Sprintf("operator %s not defined for %s and %s",
		opName(op), kindName(a.Kind), kindName(b.Kind))})
}

func opName(op Op) string {
	switch op {
	case OpAdd:
		return "+"
	case OpSub:
		return "-"
	case OpMul:
		return "*"
	case OpDiv:
		return "/"
	case OpMod:
		return "%"
	case OpEq:
		return "=="
	case OpNeq:
		return "!="
	case OpLt:
		return "<"
	case OpLe:
		return "<="
	case OpGt:
		return ">"
	case OpGe:
		return ">="
	}
	return "?"
}

// ---------- builtins ----------

func (ip *Interpreter) define(name string, fn func(*Interpreter, []Value) Value) {
	gi := ip.globalSlot(name)
	ip.Globals[gi] = BuiltinVal(&Builtin{Name: name, Fn: fn})
}

func (ip *Interpreter) definePkg(name string, members map[string]Value) {
	gi := ip.globalSlot(name)
	ip.Globals[gi] = PkgVal(&pkgValue{Name: name, Members: members})
}

func (ip *Interpreter) installBuiltins() {
	ip.define("int", func(ip *Interpreter, args []Value) Value {
	v := args[0].deref()
	switch v.Kind {
	case KindInt:
		return v
	case KindByte:
		return IntVal(v.Int)
	case KindFloat:
		return IntVal(int(v.Float))
	case KindBool:
		if v.Bool {
		return IntVal(1)
		}
		return IntVal(0)
	case KindString:
		n, err := strconv.Atoi(strings.TrimSpace(v.Str))
		if err != nil {
		panic(&Err{Msg: "int(): invalid string " + strconv.Quote(v.Str)})
		}
		return IntVal(n)
	}
	panic(&Err{Msg: "int(): cannot convert " + kindName(v.Kind)})
	})

	ip.define("float", func(ip *Interpreter, args []Value) Value {
	v := args[0].deref()
	switch v.Kind {
	case KindFloat:
		return v
	case KindInt:
		return FloatVal(float64(v.Int))
	case KindByte:
		return FloatVal(float64(v.Int))
	case KindBool:
		if v.Bool {
		return FloatVal(1)
		}
		return FloatVal(0)
	case KindString:
		f, err := strconv.ParseFloat(strings.TrimSpace(v.Str), 64)
		if err != nil {
		panic(&Err{Msg: "float(): invalid string " + strconv.Quote(v.Str)})
		}
		return FloatVal(f)
	}
	panic(&Err{Msg: "float(): cannot convert " + kindName(v.Kind)})
	})

	ip.define("byte", func(ip *Interpreter, args []Value) Value {
	v := args[0].deref()
	switch v.Kind {
	case KindByte, KindInt:
		if v.Int < 0 || v.Int > 255 {
		panic(&Err{Msg: fmt.Sprintf("byte(): %d out of range", v.Int)})
		}
		return IntVal(v.Int)
	case KindString:
		if len(v.Str) != 1 {
		panic(&Err{Msg: "byte(): string must be length 1"})
		}
		return IntVal(int(v.Str[0]))
	case KindBool:
		if v.Bool {
		return IntVal(1)
		}
		return IntVal(0)
	}
	panic(&Err{Msg: "byte(): cannot convert " + kindName(v.Kind)})
	})

	ip.define("string", func(ip *Interpreter, args []Value) Value {
	return StrVal(Display(args[0].deref()))
	})
	ip.define("len", func(ip *Interpreter, args []Value) Value {
		switch args[0].Kind {
		case KindString:
			return IntVal(len(args[0].Str))
		case KindList:
			return IntVal(len(args[0].Obj.(*List).Items))
		case KindSet:
			return IntVal(len(args[0].Obj.(*Set).Items))
		case KindMap:
			return IntVal(len(args[0].Obj.(*Map).Data))
		}
		panic(&Err{Msg: "len: unsupported " + kindName(args[0].Kind)})
	})
	ip.define("append", func(ip *Interpreter, args []Value) Value {
		if args[0].Kind != KindList {
			panic(&Err{Msg: "append: first arg must be a list"})
		}
		src := args[0].Obj.(*List).Items
		out := make([]Value, len(src)+1)
		copy(out, src)
		out[len(src)] = args[1]
		return ListVal(&List{Items: out})
	})
	ip.define("deepEqual", func(ip *Interpreter, args []Value) Value {
		return BoolVal(DeepEqual(args[0], args[1]))
	})
	ip.define("methodsOf", func(ip *Interpreter, args []Value) Value {
		return ip.methodsOf(args[0])
	})
	ip.define("fieldsOf", func(ip *Interpreter, args []Value) Value {
		return ip.fieldsOf(args[0])
	})
	ip.define("__setAdd", func(ip *Interpreter, args []Value) Value {
		if args[0].Kind != KindSet {
			panic(&Err{Msg: "__setAdd: first arg must be a set"})
		}
		args[0].Obj.(*Set).Add(args[1])
		return NilVal()
	})
	ip.define("__range", func(ip *Interpreter, args []Value) Value {
		if args[0].Kind != KindInt || args[1].Kind != KindInt {
			panic(&Err{Msg: "__range: args must be ints"})
		}
		lo, hi := args[0].Int, args[1].Int
		if hi < lo {
			return ListVal(&List{Items: []Value{}})
		}
		out := make([]Value, 0, hi-lo+1)
		for i := lo; i <= hi; i++ {
			out = append(out, IntVal(i))
		}
		return ListVal(&List{Items: out})
	})

	ip.definePkg("fmt", map[string]Value{
		"println":  BuiltinVal(&Builtin{Name: "println", Fn: builtinPrintln}),
		"print":    BuiltinVal(&Builtin{Name: "print", Fn: builtinPrint}),
		"printf":   BuiltinVal(&Builtin{Name: "printf", Fn: builtinPrintf}),
		"sprintf":  BuiltinVal(&Builtin{Name: "sprintf", Fn: builtinSprintf}),
		"sprintln": BuiltinVal(&Builtin{Name: "sprintln", Fn: builtinSprintln}),
		"errorf":   BuiltinVal(&Builtin{Name: "errorf", Fn: builtinErrorf}),
	})
	ip.definePkg("strings", map[string]Value{
		"contains":   str1(strings.Contains),
		"has_prefix": str1(strings.HasPrefix),
		"has_suffix": str1(strings.HasSuffix),
		"split":      str2(strings.Split),
		"join":       BuiltinVal(&Builtin{Name: "join", Fn: builtinJoin}),
		"trim":       str0(strings.TrimSpace),
		"to_upper":   str0(strings.ToUpper),
		"to_lower":   str0(strings.ToLower),
		"index":      BuiltinVal(&Builtin{Name: "index", Fn: strIndex}),
		"repeat":     BuiltinVal(&Builtin{Name: "repeat", Fn: strRepeat}),
	})
	ip.definePkg("strconv", map[string]Value{
		"atoi": BuiltinVal(&Builtin{Name: "atoi", Fn: builtinAtoi}),
		"itoa": BuiltinVal(&Builtin{Name: "itoa", Fn: builtinItoa}),
	})
	ip.definePkg("os", map[string]Value{
		"exit":   BuiltinVal(&Builtin{Name: "exit", Fn: builtinExit}),
		"args":   BuiltinVal(&Builtin{Name: "args", Fn: builtinArgs}),
		"getenv": str0(os.Getenv),
	})
	ip.definePkg("math", map[string]Value{
		"sqrt":  float1(math.Sqrt),
		"abs":   float1(math.Abs),
		"pow":   float2(math.Pow),
		"floor": float1(math.Floor),
		"ceil":  float1(math.Ceil),
	})
	ip.definePkg("sort", map[string]Value{
		"ints":    BuiltinVal(&Builtin{Name: "ints", Fn: builtinSortInts}),
		"strings": BuiltinVal(&Builtin{Name: "strings", Fn: builtinSortStrings}),
	})
}

func builtinPrintln(ip *Interpreter, args []Value) Value {
	for i, a := range args {
		if i > 0 {
			fmt.Fprint(ip.Out, " ")
		}
		fmt.Fprint(ip.Out, Display(a))
	}
	fmt.Fprintln(ip.Out)
	return NilVal()
}

func builtinPrint(ip *Interpreter, args []Value) Value {
	for i, a := range args {
		if i > 0 {
			fmt.Fprint(ip.Out, " ")
		}
		fmt.Fprint(ip.Out, Display(a))
	}
	return NilVal()
}

func builtinPrintf(ip *Interpreter, args []Value) Value {
	if len(args) == 0 {
		return NilVal()
	}
	rest := make([]any, len(args)-1)
	for i := 1; i < len(args); i++ {
		rest[i-1] = valueToAny(args[i])
	}
	fmt.Fprintf(ip.Out, args[0].Str, rest...)
	return NilVal()
}

func builtinSprintf(ip *Interpreter, args []Value) Value {
	if len(args) == 0 {
		return StrVal("")
	}
	rest := make([]any, len(args)-1)
	for i := 1; i < len(args); i++ {
		rest[i-1] = valueToAny(args[i])
	}
	return StrVal(fmt.Sprintf(args[0].Str, rest...))
}

func builtinSprintln(ip *Interpreter, args []Value) Value {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = Display(a)
	}
	return StrVal(strings.Join(parts, " ") + "\n")
}

func builtinErrorf(ip *Interpreter, args []Value) Value {
	if len(args) == 0 {
		return ErrVal(&Err{Msg: ""})
	}
	rest := make([]any, len(args)-1)
	for i := 1; i < len(args); i++ {
		rest[i-1] = valueToAny(args[i])
	}
	return ErrVal(&Err{Msg: fmt.Sprintf(args[0].Str, rest...)})
}

func valueToAny(v Value) any {
	switch v.Kind {
	case KindNil:
		return nil
	case KindBool:
		return v.Bool
	case KindInt:
		return v.Int
	case KindFloat:
		return v.Float
	case KindString:
		return v.Str
	}
	return Display(v)
}

func str0(fn func(string) string) Value {
	return BuiltinVal(&Builtin{Fn: func(_ *Interpreter, args []Value) Value {
		return StrVal(fn(args[0].Str))
	}})
}

func str1(fn func(string, string) bool) Value {
	return BuiltinVal(&Builtin{Fn: func(_ *Interpreter, args []Value) Value {
		return BoolVal(fn(args[0].Str, args[1].Str))
	}})
}

func str2(fn func(string, string) []string) Value {
	return BuiltinVal(&Builtin{Fn: func(_ *Interpreter, args []Value) Value {
		out := fn(args[0].Str, args[1].Str)
		items := make([]Value, len(out))
		for i, s := range out {
			items[i] = StrVal(s)
		}
		return ListVal(&List{Items: items})
	}})
}

func strIndex(_ *Interpreter, args []Value) Value {
	return IntVal(strings.Index(args[0].Str, args[1].Str))
}
func strRepeat(_ *Interpreter, args []Value) Value {
	return StrVal(strings.Repeat(args[0].Str, args[1].Int))
}
func builtinJoin(_ *Interpreter, args []Value) Value {
	if args[0].Kind != KindList {
		panic(&Err{Msg: "join: first arg must be a list"})
	}
	items := args[0].Obj.(*List).Items
	parts := make([]string, len(items))
	for i, v := range items {
		parts[i] = v.Str
	}
	return StrVal(strings.Join(parts, args[1].Str))
}
func builtinAtoi(_ *Interpreter, args []Value) Value {
	n, err := strconv.Atoi(args[0].Str)
	if err != nil {
		return ListVal(&List{Items: []Value{IntVal(0), ErrVal(&Err{Msg: err.Error()})}})
	}
	return ListVal(&List{Items: []Value{IntVal(n), NilVal()}})
}
func builtinItoa(_ *Interpreter, args []Value) Value {
	return StrVal(strconv.Itoa(args[0].Int))
}
func builtinExit(_ *Interpreter, args []Value) Value {
	panic(exitSignal{code: args[0].Int})
}
func builtinArgs(_ *Interpreter, _ []Value) Value {
	out := make([]Value, len(os.Args))
	for i, s := range os.Args {
		out[i] = StrVal(s)
	}
	return ListVal(&List{Items: out})
}
func float1(fn func(float64) float64) Value {
	return BuiltinVal(&Builtin{Fn: func(_ *Interpreter, args []Value) Value {
		return FloatVal(fn(toFloat(args[0])))
	}})
}
func float2(fn func(float64, float64) float64) Value {
	return BuiltinVal(&Builtin{Fn: func(_ *Interpreter, args []Value) Value {
		return FloatVal(fn(toFloat(args[0]), toFloat(args[1])))
	}})
}
func toFloat(v Value) float64 {
	switch v.Kind {
	case KindInt:
		return float64(v.Int)
	case KindFloat:
		return v.Float
	}
	return 0
}
func builtinSortInts(_ *Interpreter, args []Value) Value {
	if args[0].Kind != KindList {
		panic(&Err{Msg: "sort.ints: not a list"})
	}
	items := args[0].Obj.(*List).Items
	ns := make([]int, len(items))
	for i, v := range items {
		ns[i] = v.Int
	}
	sort.Ints(ns)
	for i, n := range ns {
		items[i] = IntVal(n)
	}
	return NilVal()
}
func builtinSortStrings(_ *Interpreter, args []Value) Value {
	if args[0].Kind != KindList {
		panic(&Err{Msg: "sort.strings: not a list"})
	}
	items := args[0].Obj.(*List).Items
	ss := make([]string, len(items))
	for i, v := range items {
		ss[i] = v.Str
	}
	sort.Strings(ss)
	for i, s := range ss {
		items[i] = StrVal(s)
	}
	return NilVal()
}

// ---------- field / index / slice ----------

func (ip *Interpreter) selectField(base Value, name string) Value {
	switch base.Kind {
	case KindStruct:
		s := base.Obj.(*Struct)
		if v, ok := s.Fields[name]; ok {
			return v
		}
		if m, ok := ip.Methods[s.TypeName]; ok {
			if fn, ok := m[name]; ok {
				return BoundVal(&BoundMethod{Recv: base, Fn: fn.Obj.(*Function)})
			}
		}
		panic(&Err{Msg: fmt.Sprintf("%s has no field or method %q", s.TypeName, name)})
	case KindMap:
		m := base.Obj.(*Map)
		if v, ok := m.Data[StrVal(name)]; ok {
			return v
		}
		panic(&Err{Msg: fmt.Sprintf("map has no key or method %q", name)})
	case KindPackage:
		p := base.Obj.(*pkgValue)
		if v, ok := p.Members[name]; ok {
			return v
		}
		panic(&Err{Msg: fmt.Sprintf("package %q has no member %q", p.Name, name)})
	}
	panic(&Err{Msg: fmt.Sprintf("cannot select %q on %s", name, kindName(base.Kind))})
}

func (ip *Interpreter) index(base, idx Value) Value {
	switch base.Kind {
	case KindList:
		if idx.Kind != KindInt {
			panic(&Err{Msg: "list index must be int"})
		}
		items := base.Obj.(*List).Items
		if idx.Int < 0 || idx.Int >= len(items) {
			panic(&Err{Msg: fmt.Sprintf("index %d out of range", idx.Int)})
		}
		return items[idx.Int]
	case KindSet:
		if idx.Kind != KindInt {
			panic(&Err{Msg: "set index must be int"})
		}
		items := base.Obj.(*Set).Items
		if idx.Int < 0 || idx.Int >= len(items) {
			panic(&Err{Msg: fmt.Sprintf("index %d out of range", idx.Int)})
		}
		return items[idx.Int]
	case KindString:
		if idx.Kind != KindInt {
			panic(&Err{Msg: "string index must be int"})
		}
		s := base.Str
		if idx.Int < 0 || idx.Int >= len(s) {
			panic(&Err{Msg: fmt.Sprintf("index %d out of range", idx.Int)})
		}
		return IntVal(int(s[idx.Int]))
	case KindMap:
		return base.Obj.(*Map).Data[idx]
	}
	panic(&Err{Msg: "cannot index " + kindName(base.Kind)})
}

func (ip *Interpreter) slice(base, lo Value, haveLo bool, hi Value, haveHi bool) Value {
	loI, hiI := 0, -1
	switch base.Kind {
	case KindList:
		items := base.Obj.(*List).Items
		if haveLo {
			loI = lo.Int
		}
		if haveHi {
			hiI = hi.Int
		} else {
			hiI = len(items)
		}
		if loI < 0 || hiI > len(items) || loI > hiI {
			panic(&Err{Msg: "slice out of range"})
		}
		out := make([]Value, hiI-loI)
		copy(out, items[loI:hiI])
		return ListVal(&List{Items: out})
	case KindString:
		s := base.Str
		if haveLo {
			loI = lo.Int
		}
		if haveHi {
			hiI = hi.Int
		} else {
			hiI = len(s)
		}
		if loI < 0 || hiI > len(s) || loI > hiI {
			panic(&Err{Msg: "slice out of range"})
		}
		return StrVal(s[loI:hiI])
	}
	panic(&Err{Msg: "cannot slice " + kindName(base.Kind)})
}

func (ip *Interpreter) zeroValueOf(typeName string) Value {
	typeName = strings.TrimSpace(typeName)
	if strings.HasPrefix(typeName, "*") {
		return NilVal()
	}
	if strings.HasPrefix(typeName, "[]") {
		return ListVal(&List{Items: []Value{}})
	}
	if strings.HasPrefix(typeName, "{}") {
		return SetVal(&Set{})
	}
	if strings.HasPrefix(typeName, "map[") {
		close := strings.Index(typeName, "]")
		if close < 0 {
			panic(&Err{Msg: "make: malformed map type " + typeName})
		}
		return MapVal(&Map{
			KeyTy: typeName[4:close],
			ValTy: typeName[close+1:],
			Data:  map[Value]Value{},
		})
	}
	switch typeName {
	case "int", "byte":
		return IntVal(0)
	case "float":
		return FloatVal(0)
	case "string":
		return StrVal("")
	case "bool":
		return BoolVal(false)
	case "any", "error":
		return NilVal()
	}
	if schema, ok := ip.Schemas[typeName]; ok {
		fields := make(map[string]Value, len(schema.Fields))
		for fname, ftype := range schema.Fields {
			fields[fname] = ip.zeroValueOf(ftype)
		}
		return StructVal(&Struct{TypeName: typeName, Fields: fields})
	}
	panic(&Err{Msg: "make: unknown type " + typeName})
}

// ---------- reflection ----------

func (ip *Interpreter) methodsOf(v Value) Value {
	v = v.deref()
	var typeName string
	switch v.Kind {
	case KindStruct:
		typeName = v.Obj.(*Struct).TypeName
	case KindString:
		typeName = v.Str
	default:
		panic(&Err{Msg: "methodsOf: need a struct or type name"})
	}
	m := ip.Methods[typeName]
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Value, len(names))
	for i, n := range names {
		out[i] = StrVal(n)
	}
	return ListVal(&List{Items: out})
}

func (ip *Interpreter) fieldsOf(v Value) Value {
	v = v.deref()
	if v.Kind != KindStruct {
		panic(&Err{Msg: "fieldsOf: need a struct"})
	}
	fields := v.Obj.(*Struct).Fields
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Value, len(names))
	for i, n := range names {
		out[i] = StrVal(n)
	}
	return ListVal(&List{Items: out})
}

// ---------- deep equal ----------

func DeepEqual(a, b Value) bool {
	if a.Kind != b.Kind {
		if a.Kind == KindInt && b.Kind == KindFloat {
			return float64(a.Int) == b.Float
		}
		if a.Kind == KindFloat && b.Kind == KindInt {
			return a.Float == float64(b.Int)
		}
		return false
	}
	switch a.Kind {
	case KindNil:
		return true
	case KindBool:
		return a.Bool == b.Bool
	case KindInt:
		return a.Int == b.Int
	case KindFloat:
		return a.Float == b.Float
	case KindString:
		return a.Str == b.Str
	case KindList:
		al, bl := a.Obj.(*List), b.Obj.(*List)
		if len(al.Items) != len(bl.Items) {
			return false
		}
		for i := range al.Items {
			if !DeepEqual(al.Items[i], bl.Items[i]) {
				return false
			}
		}
		return true
	case KindSet:
		as, bs := a.Obj.(*Set), b.Obj.(*Set)
		if len(as.Items) != len(bs.Items) {
			return false
		}
		for _, x := range as.Items {
			if !bs.Contains(x) {
				return false
			}
		}
		return true
	case KindMap:
		am, bm := a.Obj.(*Map), b.Obj.(*Map)
		if len(am.Data) != len(bm.Data) {
			return false
		}
		for k, v := range am.Data {
			other, ok := bm.Data[k]
			if !ok || !DeepEqual(v, other) {
				return false
			}
		}
		return true
	case KindStruct:
		as, bs := a.Obj.(*Struct), b.Obj.(*Struct)
		if as.TypeName != bs.TypeName || len(as.Fields) != len(bs.Fields) {
			return false
		}
		for k, v := range as.Fields {
			other, ok := bs.Fields[k]
			if !ok || !DeepEqual(v, other) {
				return false
			}
		}
		return true
	case KindErr:
		return a.Obj.(*Err).Msg == b.Obj.(*Err).Msg
	}
	return a.Obj == b.Obj
}

// ---------- display ----------

func Display(v Value) string {
	switch v.Kind {
	case KindNil:
		return "nil"
	case KindBool:
		if v.Bool {
			return "true"
		}
		return "false"
	case KindInt:
		return strconv.Itoa(v.Int)
	case KindFloat:
		return strconv.FormatFloat(v.Float, 'g', -1, 64)
	case KindString:
		return v.Str
	case KindRef:
		return Display(v.Obj.(Ref).Get())
	case KindErr:
		return "error: " + v.Obj.(*Err).Msg
	case KindList:
		items := v.Obj.(*List).Items
		var b strings.Builder
		b.WriteByte('[')
		for i, x := range items {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(Display(x))
		}
		b.WriteByte(']')
		return b.String()
	case KindSet:
		items := v.Obj.(*Set).Items
		var b strings.Builder
		b.WriteByte('{')
		for i, x := range items {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(Display(x))
		}
		b.WriteByte('}')
		return b.String()
	case KindMap:
		m := v.Obj.(*Map)
		parts := make([]string, 0, len(m.Data))
		for k, val := range m.Data {
			parts = append(parts, Display(k)+": "+Display(val))
		}
		sort.Strings(parts)
		return "{" + strings.Join(parts, ", ") + "}"
	case KindStruct:
		s := v.Obj.(*Struct)
		keys := make([]string, 0, len(s.Fields))
		for k := range s.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString(s.TypeName)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(Display(s.Fields[k]))
		}
		b.WriteByte('}')
		return b.String()
	case KindFunction:
		f := v.Obj.(*Function)
		if f.Name == "" {
			return "<fn>"
		}
		return "<fn " + f.Name + ">"
	case KindBoundMethod:
		return "<method " + v.Obj.(*BoundMethod).Fn.Name + ">"
	case KindBuiltin:
		return "<builtin " + v.Obj.(*Builtin).Name + ">"
	case KindPackage:
		return "<package " + v.Obj.(*pkgValue).Name + ">"
	}
	return "<unknown>"
}
