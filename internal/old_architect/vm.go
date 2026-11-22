package main

import (
	"fmt"
	"math"
	"reflect"
	"strings"
)

type VMModule struct {
	Name    string
	Exports map[string]any
}

type VMStruct struct {
	header     RCHeader
	vm         *VM
	TypeName   string
	Fields     map[string]any
	FieldOrder []string
	disposed   bool
}

func NewVMStruct(vm *VM, typeName string, fields map[string]any, fieldOrder []string) *VMStruct {
	order := make([]string, 0, len(fields))
	seen := make(map[string]bool, len(fields))
	for _, k := range fieldOrder {
		if _, ok := fields[k]; ok && !seen[k] {
			order = append(order, k)
			seen[k] = true
		}
	}
	for k := range fields {
		if !seen[k] {
			order = append(order, k)
			seen[k] = true
		}
	}

	s := &VMStruct{
		header:     NewRCHeader(),
		vm:         vm,
		TypeName:   typeName,
		Fields:     fields,
		FieldOrder: order,
	}
	if vm != nil && vm.heap != nil {
		vm.heap.Register(s)
	}
	return s
}

func (s *VMStruct) AddRef() {
	if s == nil || s.disposed {
		return
	}
	s.header.IncRef()
	s.header.SetCycleColor(ColorBlack)
}

func (s *VMStruct) Release() {
	if s == nil || s.disposed {
		return
	}
	if s.header.DecRef() == 0 {
		if s.vm != nil && s.vm.heap != nil {
			s.vm.heap.Unregister(s)
		}
		s.Dispose()
	} else if s.vm != nil && s.vm.cc != nil {
		s.vm.cc.PossibleRoot(s)
	}
}

func (s *VMStruct) RefCount() int64            { return s.header.RefCount() }
func (s *VMStruct) MarkForCycle()              { s.header.MarkForCycle() }
func (s *VMStruct) ClearCycleMark()            { s.header.ClearCycleMark() }
func (s *VMStruct) IsCycleMarked() bool        { return s.header.IsCycleMarked() }
func (s *VMStruct) CycleColor() CycleColor     { return s.header.CycleColor() }
func (s *VMStruct) SetCycleColor(c CycleColor) { s.header.SetCycleColor(c) }
func (s *VMStruct) IsBuffered() bool           { return s.header.IsBuffered() }
func (s *VMStruct) SetBuffered(b bool)         { s.header.SetBuffered(b) }

func (s *VMStruct) Traverse(fn func(RCObject)) {
	if s.disposed {
		return
	}
	for _, v := range s.Fields {
		if rc, ok := v.(RCObject); ok {
			fn(rc)
		}
	}
}

func (s *VMStruct) Dispose() {
	if s.disposed {
		return
	}
	s.disposed = true
	for k, v := range s.Fields {
		if rc, ok := v.(RCObject); ok {
			rc.Release()
		}
		delete(s.Fields, k)
	}
	s.FieldOrder = nil
}

type VMList struct {
	header   RCHeader
	vm       *VM
	ElemType string
	Elements []any
	disposed bool
}

func NewVMList(vm *VM, elemType string, elems []any) *VMList {
	copied := make([]any, len(elems))
	for i, v := range elems {
		copied[i] = v
		if rc, ok := v.(RCObject); ok {
			rc.AddRef()
		}
	}
	l := &VMList{
		header:   NewRCHeader(),
		vm:       vm,
		ElemType: elemType,
		Elements: copied,
	}
	if vm != nil && vm.heap != nil {
		vm.heap.Register(l)
	}
	return l
}

func (l *VMList) AddRef() {
	if l == nil || l.disposed {
		return
	}
	l.header.IncRef()
	l.header.SetCycleColor(ColorBlack)
}

func (l *VMList) Release() {
	if l == nil || l.disposed {
		return
	}
	if l.header.DecRef() == 0 {
		if l.vm != nil && l.vm.heap != nil {
			l.vm.heap.Unregister(l)
		}
		l.Dispose()
	} else if l.vm != nil && l.vm.cc != nil {
		l.vm.cc.PossibleRoot(l)
	}
}

func (l *VMList) RefCount() int64            { return l.header.RefCount() }
func (l *VMList) MarkForCycle()              { l.header.MarkForCycle() }
func (l *VMList) ClearCycleMark()            { l.header.ClearCycleMark() }
func (l *VMList) IsCycleMarked() bool        { return l.header.IsCycleMarked() }
func (l *VMList) CycleColor() CycleColor     { return l.header.CycleColor() }
func (l *VMList) SetCycleColor(c CycleColor) { l.header.SetCycleColor(c) }
func (l *VMList) IsBuffered() bool           { return l.header.IsBuffered() }
func (l *VMList) SetBuffered(b bool)         { l.header.SetBuffered(b) }

func (l *VMList) Traverse(fn func(RCObject)) {
	if l.disposed {
		return
	}
	for _, v := range l.Elements {
		if rc, ok := v.(RCObject); ok {
			fn(rc)
		}
	}
}

func (l *VMList) Dispose() {
	if l.disposed {
		return
	}
	l.disposed = true
	for i, v := range l.Elements {
		if rc, ok := v.(RCObject); ok {
			rc.Release()
		}
		l.Elements[i] = nil
	}
	l.Elements = nil
}

type VMClosure struct {
	header   RCHeader
	vm       *VM
	Fn       *CompiledFunction
	Upvalues []*VMUpvalue
	disposed bool
}

type VMUpvalue struct {
	Closed bool
	Val    any
	Slot   *any
}

func NewVMClosure(vm *VM, fn *CompiledFunction, upvals []*VMUpvalue) *VMClosure {
	c := &VMClosure{
		header:   NewRCHeader(),
		vm:       vm,
		Fn:       fn,
		Upvalues: upvals,
	}
	if vm != nil && vm.heap != nil {
		vm.heap.Register(c)
	}
	return c
}

func (c *VMClosure) AddRef() {
	if c == nil || c.disposed {
		return
	}
	c.header.IncRef()
	c.header.SetCycleColor(ColorBlack)
}

func (c *VMClosure) Release() {
	if c == nil || c.disposed {
		return
	}
	if c.header.DecRef() == 0 {
		if c.vm != nil && c.vm.heap != nil {
			c.vm.heap.Unregister(c)
		}
		c.Dispose()
	} else if c.vm != nil && c.vm.cc != nil {
		c.vm.cc.PossibleRoot(c)
	}
}

func (c *VMClosure) RefCount() int64              { return c.header.RefCount() }
func (c *VMClosure) MarkForCycle()                { c.header.MarkForCycle() }
func (c *VMClosure) ClearCycleMark()              { c.header.ClearCycleMark() }
func (c *VMClosure) IsCycleMarked() bool          { return c.header.IsCycleMarked() }
func (c *VMClosure) CycleColor() CycleColor       { return c.header.CycleColor() }
func (c *VMClosure) SetCycleColor(col CycleColor) { c.header.SetCycleColor(col) }
func (c *VMClosure) IsBuffered() bool             { return c.header.IsBuffered() }
func (c *VMClosure) SetBuffered(b bool)           { c.header.SetBuffered(b) }

func (c *VMClosure) Traverse(fn func(RCObject)) {
	if c.disposed {
		return
	}
	for _, uv := range c.Upvalues {
		if uv == nil {
			continue
		}
		var val any
		if uv.Closed {
			val = uv.Val
		} else if uv.Slot != nil {
			val = *uv.Slot
		}
		if rc, ok := val.(RCObject); ok {
			fn(rc)
		}
	}
}

func (c *VMClosure) Dispose() {
	if c.disposed {
		return
	}
	c.disposed = true
	for i, uv := range c.Upvalues {
		if uv != nil && uv.Closed {
			if rc, ok := uv.Val.(RCObject); ok {
				rc.Release()
			}
		}
		c.Upvalues[i] = nil
	}
	c.Upvalues = nil
}

type multiReturnVals struct {
	vals []any
}

type callFrame struct {
	closure *VMClosure
	ip      int
	mem     []any
	retDst  uint64
	retDsts []uint64
	hasRet  bool
}

type VM struct {
	registry  *ModuleRegistry
	globals   map[string]any
	frames    []*callFrame
	framePool []*callFrame
	heap      *RCHeaderHeap
	cc        *CycleCollector
	stackCap  uint64
	heapCap   uint64
	heapSlots []any
}

func NewVM(stackSize uint64, heapSize uint64) *VM {
	reg := DefaultModuleRegistry()
	heap := NewRCHeaderHeap()
	cc := NewCycleCollector(heap)
	return &VM{
		registry:  reg,
		globals:   make(map[string]any),
		framePool: make([]*callFrame, 0, 64),
		heap:      heap,
		cc:        cc,
		stackCap:  stackSize,
		heapCap:   heapSize,
		heapSlots: []any{nil},
	}
}

func (vm *VM) acquireFrame(c *VMClosure, minMem int, retDst uint64, retDsts []uint64, hasRet bool) *callFrame {
	var f *callFrame
	if n := len(vm.framePool); n > 0 {
		f = vm.framePool[n-1]
		vm.framePool[n-1] = nil
		vm.framePool = vm.framePool[:n-1]
	} else {
		f = &callFrame{}
	}

	if cap(f.mem) >= minMem {
		f.mem = f.mem[:minMem]
		clear(f.mem)
	} else {
		f.mem = make([]any, minMem)
	}

	f.closure = c
	f.ip = 0
	f.retDst = retDst
	f.retDsts = retDsts
	f.hasRet = hasRet
	return f
}

func (vm *VM) releaseFrame(f *callFrame) {
	if f == nil {
		return
	}
	vm.cleanupFrame(f)
	f.closure = nil
	f.retDsts = nil
	if len(vm.framePool) < 256 {
		vm.framePool = append(vm.framePool, f)
	}
}

func valuesEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}
	switch x := a.(type) {
	case int64:
		y, ok := b.(int64)
		return ok && x == y
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case *VMList:
		y, ok := b.(*VMList)
		return ok && x == y
	case *VMStruct:
		y, ok := b.(*VMStruct)
		return ok && x == y
	case *VMClosure:
		y, ok := b.(*VMClosure)
		return ok && x == y
	case *VMModule:
		y, ok := b.(*VMModule)
		return ok && x == y
	default:
		ta := reflect.TypeOf(a)
		tb := reflect.TypeOf(b)
		if ta != tb || !ta.Comparable() {
			return false
		}
		return a == b
	}
}

func (vm *VM) unitAddr() uint64 {
	return 0
}

func (vm *VM) allocHeapSlot(val any) uint64 {
	if val == nil {
		return 0
	}
	if rc, ok := val.(RCObject); ok {
		rc.AddRef()
	}
	addr := uint64(len(vm.heapSlots))
	vm.heapSlots = append(vm.heapSlots, val)
	return addr
}

func (vm *VM) derefHeap(addr uint64) (any, bool) {
	if addr == 0 {
		return nil, true
	}
	if int(addr) < len(vm.heapSlots) {
		return vm.heapSlots[addr], true
	}
	if len(vm.frames) > 0 {
		top := vm.frames[len(vm.frames)-1]
		if int(addr) < len(top.mem) {
			return top.mem[addr], true
		}
	}
	return nil, false
}

func (vm *VM) formatStruct(s *VMStruct, visited map[any]bool) string {
	if s == nil {
		return "unit"
	}
	if s.disposed {
		return fmt.Sprintf("<disposed %s>", s.TypeName)
	}
	if visited[s] {
		return fmt.Sprintf("%s{<cycle>}", s.TypeName)
	}
	visited[s] = true
	defer delete(visited, s)

	fields := make([]string, 0, len(s.FieldOrder))
	for _, k := range s.FieldOrder {
		if fv, ok := s.Fields[k]; ok {
			fields = append(fields, fmt.Sprintf("%s: %s", k, vm.stringifyAnyVisited(fv, visited)))
		}
	}
	return fmt.Sprintf("%s{%s}", s.TypeName, strings.Join(fields, ", "))
}

func (vm *VM) stringifyTypedVal(valOrAddr uint64, typ Type) string {
	val, ok := vm.derefHeap(valOrAddr)
	if !ok {
		return fmt.Sprintf("<addr:%d>", valOrAddr)
	}
	return vm.stringifyAnyVisited(val, make(map[any]bool))
}

func (vm *VM) stringifyAny(v any) string {
	return vm.stringifyAnyVisited(v, make(map[any]bool))
}

func (vm *VM) stringifyAnyVisited(v any, visited map[any]bool) string {
	switch x := v.(type) {
	case nil:
		return "unit"
	case int64:
		return fmt.Sprintf("%d", x)
	case float64:
		return fmt.Sprintf("%g", x)
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case *VMList:
		if x.disposed {
			return "<disposed list>"
		}
		if visited[x] {
			return "[<cycle>]"
		}
		visited[x] = true
		defer delete(visited, x)

		var parts []string
		for _, el := range x.Elements {
			parts = append(parts, vm.stringifyAnyVisited(el, visited))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *VMStruct:
		return vm.formatStruct(x, visited)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func (vm *VM) setMem(frame *callFrame, idx uint64, val any) {
	if int(idx) >= len(frame.mem) {
		newLen := int(idx) + 1
		if newLen <= cap(frame.mem) {
			prevLen := len(frame.mem)
			frame.mem = frame.mem[:newLen]
			clear(frame.mem[prevLen:])
		} else {
			newMem := make([]any, newLen)
			copy(newMem, frame.mem)
			frame.mem = newMem
		}
	}
	old := frame.mem[idx]
	if valuesEqual(old, val) {
		return
	}
	if rc, ok := val.(RCObject); ok {
		rc.AddRef()
	}
	if rc, ok := old.(RCObject); ok {
		rc.Release()
	}
	frame.mem[idx] = val
}

func (vm *VM) getMem(frame *callFrame, idx uint64) any {
	if int(idx) >= len(frame.mem) {
		return nil
	}
	return frame.mem[idx]
}

func (vm *VM) setGlobal(name string, val any) {
	old := vm.globals[name]
	if valuesEqual(old, val) {
		return
	}
	if rc, ok := val.(RCObject); ok {
		rc.AddRef()
	}
	if rc, ok := old.(RCObject); ok {
		rc.Release()
	}
	vm.globals[name] = val
}

func (vm *VM) cleanupFrame(frame *callFrame) {
	for i, v := range frame.mem {
		if rc, ok := v.(RCObject); ok {
			rc.Release()
		}
		frame.mem[i] = nil
	}
}

func (vm *VM) storeReturnToCaller(callerFrame *callFrame, dst uint64, dsts []uint64, retVal any) {
	if mr, ok := retVal.(multiReturnVals); ok {
		if len(dsts) > 0 {
			for i, d := range dsts {
				if i < len(mr.vals) {
					vm.setMem(callerFrame, d, mr.vals[i])
				} else {
					vm.setMem(callerFrame, d, nil)
				}
			}
			return
		}
		if len(mr.vals) > 0 {
			vm.setMem(callerFrame, dst, mr.vals[0])
			return
		}
		vm.setMem(callerFrame, dst, nil)
		return
	}

	if len(dsts) > 0 {
		vm.setMem(callerFrame, dsts[0], retVal)
		for i := 1; i < len(dsts); i++ {
			vm.setMem(callerFrame, dsts[i], nil)
		}
		return
	}
	vm.setMem(callerFrame, dst, retVal)
}

func (vm *VM) Run(prog *BytecodeProgram) (any, error) {
	if prog == nil || prog.Main == nil {
		return nil, fmt.Errorf("empty bytecode program")
	}

	mainClosure := NewVMClosure(vm, prog.Main, nil)
	vm.frames = []*callFrame{
		vm.acquireFrame(mainClosure, int(prog.Main.MemCount), 0, nil, false),
	}

	var lastResult any

	for len(vm.frames) > 0 {
		frame := vm.frames[len(vm.frames)-1]
		fn := frame.closure.Fn
		code := fn.Code
		ip := frame.ip

		if ip >= len(code) {
			vm.releaseFrame(frame)
			vm.frames = vm.frames[:len(vm.frames)-1]
			continue
		}

		op := OpCode(code[ip])
		ip++

		switch op {
		case OP_MOV:
			dst, src := code[ip], code[ip+1]
			ip += 2
			vm.setMem(frame, dst, vm.getMem(frame, src))

		case OP_CONST:
			dst, imm := code[ip], code[ip+1]
			ip += 2
			vm.setMem(frame, dst, fn.Constants[imm])

		case OP_ADD:
			dst, lhs, rhs := code[ip], code[ip+1], code[ip+2]
			ip += 3
			res, err := vm.evalAdd(vm.getMem(frame, lhs), vm.getMem(frame, rhs))
			if err != nil {
				return nil, err
			}
			vm.setMem(frame, dst, res)

		case OP_SUB:
			dst, lhs, rhs := code[ip], code[ip+1], code[ip+2]
			ip += 3
			res, err := vm.evalSub(vm.getMem(frame, lhs), vm.getMem(frame, rhs))
			if err != nil {
				return nil, err
			}
			vm.setMem(frame, dst, res)

		case OP_MUL:
			dst, lhs, rhs := code[ip], code[ip+1], code[ip+2]
			ip += 3
			res, err := vm.evalMul(vm.getMem(frame, lhs), vm.getMem(frame, rhs))
			if err != nil {
				return nil, err
			}
			vm.setMem(frame, dst, res)

		case OP_DIV:
			dst, lhs, rhs := code[ip], code[ip+1], code[ip+2]
			ip += 3
			res, err := vm.evalDiv(vm.getMem(frame, lhs), vm.getMem(frame, rhs))
			if err != nil {
				return nil, err
			}
			vm.setMem(frame, dst, res)

		case OP_INC:
			dst, src := code[ip], code[ip+1]
			ip += 2
			vAny := vm.getMem(frame, src)
			switch v := vAny.(type) {
			case int64:
				vm.setMem(frame, dst, v+1)
			case float64:
				vm.setMem(frame, dst, v+1)
			default:
				return nil, fmt.Errorf("cannot increment value of type %T", vAny)
			}

		case OP_DEC:
			dst, src := code[ip], code[ip+1]
			ip += 2
			vAny := vm.getMem(frame, src)
			switch v := vAny.(type) {
			case int64:
				vm.setMem(frame, dst, v-1)
			case float64:
				vm.setMem(frame, dst, v-1)
			default:
				return nil, fmt.Errorf("cannot decrement value of type %T", vAny)
			}

		case OP_NEG:
			dst, src := code[ip], code[ip+1]
			ip += 2
			vAny := vm.getMem(frame, src)
			switch v := vAny.(type) {
			case int64:
				vm.setMem(frame, dst, -v)
			case float64:
				vm.setMem(frame, dst, -v)
			default:
				return nil, fmt.Errorf("cannot negate value of type %T", vAny)
			}

		case OP_NOT:
			dst, src := code[ip], code[ip+1]
			ip += 2
			vAny := vm.getMem(frame, src)
			b, ok := vAny.(bool)
			if !ok {
				return nil, fmt.Errorf("operator '!' requires bool, got %T", vAny)
			}
			vm.setMem(frame, dst, !b)

		case OP_CMP:
			lhs, rhs, jcc := code[ip], code[ip+1], code[ip+2]
			relPos := ip + 3
			rel := DecodeRel(code[relPos])
			ip += 4
			matched, err := vm.evalCmp(vm.getMem(frame, lhs), vm.getMem(frame, rhs), jcc)
			if err != nil {
				return nil, err
			}
			if matched {
				ip = int(int64(relPos) + rel)
			}

		case OP_USE:
			dst, nameImm := code[ip], code[ip+1]
			ip += 2
			modName, _ := fn.Constants[nameImm].(string)
			def, ok := vm.registry.Get(modName)
			if !ok {
				return nil, fmt.Errorf("module %q not found", modName)
			}
			modObj := &VMModule{
				Name:    modName,
				Exports: make(map[string]any),
			}
			for k, exp := range def.Exports {
				if exp.Native != nil {
					modObj.Exports[k] = exp.Native
				} else {
					modObj.Exports[k] = exp.Type
				}
			}
			vm.setMem(frame, dst, modObj)

		case OP_LOAD_GLOBAL:
			dst, nameImm := code[ip], code[ip+1]
			ip += 2
			name, _ := fn.Constants[nameImm].(string)
			val, exists := vm.globals[name]
			if !exists {
				return nil, fmt.Errorf("undefined global %q", name)
			}
			vm.setMem(frame, dst, val)

		case OP_STORE_GLOBAL:
			nameImm, src := code[ip], code[ip+1]
			ip += 2
			name, _ := fn.Constants[nameImm].(string)
			vm.setGlobal(name, vm.getMem(frame, src))

		case OP_NEW_STRUCT:
			dst, typeImm, fieldCount := code[ip], code[ip+1], code[ip+2]
			ip += 3
			typeName, _ := fn.Constants[typeImm].(string)
			fields := make(map[string]any, int(fieldCount))
			fieldOrder := make([]string, 0, int(fieldCount))
			for i := uint64(0); i < fieldCount; i++ {
				fNameImm := code[ip]
				fValMem := code[ip+1]
				ip += 2
				fName, _ := fn.Constants[fNameImm].(string)
				fields[fName] = vm.getMem(frame, fValMem)
				fieldOrder = append(fieldOrder, fName)
			}
			st := NewVMStruct(vm, typeName, fields, fieldOrder)
			vm.setMem(frame, dst, st)
			st.Release()

		case OP_GET_FIELD:
			dst, objMem, fieldImm := code[ip], code[ip+1], code[ip+2]
			ip += 3
			obj := vm.getMem(frame, objMem)
			fieldName, _ := fn.Constants[fieldImm].(string)
			switch o := obj.(type) {
			case *VMStruct:
				val, ok := o.Fields[fieldName]
				if !ok {
					return nil, fmt.Errorf("struct %q has no field %q", o.TypeName, fieldName)
				}
				vm.setMem(frame, dst, val)
			case *VMModule:
				val, ok := o.Exports[fieldName]
				if !ok {
					return nil, fmt.Errorf("module %q has no export %q", o.Name, fieldName)
				}
				vm.setMem(frame, dst, val)
			default:
				return nil, fmt.Errorf("cannot access field %q on %T", fieldName, obj)
			}

		case OP_SET_FIELD:
			objMem, fieldImm, valMem := code[ip], code[ip+1], code[ip+2]
			ip += 3
			obj := vm.getMem(frame, objMem)
			fieldName, _ := fn.Constants[fieldImm].(string)
			val := vm.getMem(frame, valMem)
			st, ok := obj.(*VMStruct)
			if !ok {
				return nil, fmt.Errorf("cannot set field %q on %T", fieldName, obj)
			}
			old := st.Fields[fieldName]
			if !valuesEqual(old, val) {
				if rc, isRC := val.(RCObject); isRC {
					rc.AddRef()
				}
				if rc, isRC := old.(RCObject); isRC {
					rc.Release()
				}
				st.Fields[fieldName] = val
				if vm.cc != nil {
					vm.cc.PossibleRoot(st)
				}
			}

		case OP_NEW_LIST:
			dst, elemTypeImm, isRepeat := code[ip], code[ip+1], code[ip+2]
			ip += 3
			elemType, _ := fn.Constants[elemTypeImm].(string)
			if isRepeat != 0 {
				repValMem, repCntMem := code[ip], code[ip+1]
				ip += 2
				repVal := vm.getMem(frame, repValMem)
				cntAny := vm.getMem(frame, repCntMem)
				cnt, ok := cntAny.(int64)
				if !ok {
					return nil, fmt.Errorf("list repeat count must be int, got %T", cntAny)
				}
				if cnt < 0 {
					return nil, fmt.Errorf("list repeat count cannot be negative: %d", cnt)
				}
				elems := make([]any, cnt)
				for i := int64(0); i < cnt; i++ {
					elems[i] = repVal
				}
				listObj := NewVMList(vm, elemType, elems)
				vm.setMem(frame, dst, listObj)
				listObj.Release()
			} else {
				elemCount := code[ip]
				ip++
				elems := make([]any, int(elemCount))
				for i := uint64(0); i < elemCount; i++ {
					elems[i] = vm.getMem(frame, code[ip])
					ip++
				}
				listObj := NewVMList(vm, elemType, elems)
				vm.setMem(frame, dst, listObj)
				listObj.Release()
			}

		case OP_INDEX_GET:
			dst, listMem, idxMem := code[ip], code[ip+1], code[ip+2]
			ip += 3
			listAny := vm.getMem(frame, listMem)
			idxAny := vm.getMem(frame, idxMem)
			listObj, ok := listAny.(*VMList)
			if !ok {
				return nil, fmt.Errorf("cannot index non-list value of type %T", listAny)
			}
			idx, ok := idxAny.(int64)
			if !ok {
				return nil, fmt.Errorf("list index must be int, got %T", idxAny)
			}
			if idx < 0 || int(idx) >= len(listObj.Elements) {
				return nil, fmt.Errorf("list index out of bounds: index %d, length %d", idx, len(listObj.Elements))
			}
			vm.setMem(frame, dst, listObj.Elements[idx])

		case OP_INDEX_SET:
			listMem, idxMem, valMem := code[ip], code[ip+1], code[ip+2]
			ip += 3
			listAny := vm.getMem(frame, listMem)
			idxAny := vm.getMem(frame, idxMem)
			val := vm.getMem(frame, valMem)
			listObj, ok := listAny.(*VMList)
			if !ok {
				return nil, fmt.Errorf("cannot index-assign non-list value of type %T", listAny)
			}
			idx, ok := idxAny.(int64)
			if !ok {
				return nil, fmt.Errorf("list index must be int, got %T", idxAny)
			}
			if idx < 0 || int(idx) >= len(listObj.Elements) {
				return nil, fmt.Errorf("list index out of bounds: index %d, length %d", idx, len(listObj.Elements))
			}
			old := listObj.Elements[idx]
			if !valuesEqual(old, val) {
				if rc, isRC := val.(RCObject); isRC {
					rc.AddRef()
				}
				if rc, isRC := old.(RCObject); isRC {
					rc.Release()
				}
				listObj.Elements[idx] = val
				if vm.cc != nil {
					vm.cc.PossibleRoot(listObj)
				}
			}

		case OP_LIST_LEN:
			dst, listMem := code[ip], code[ip+1]
			ip += 2
			listAny := vm.getMem(frame, listMem)
			listObj, ok := listAny.(*VMList)
			if !ok {
				return nil, fmt.Errorf("len requires a list, got %T", listAny)
			}
			vm.setMem(frame, dst, int64(len(listObj.Elements)))

		case OP_GET_UPVALUE:
			dst, upImm := code[ip], code[ip+1]
			ip += 2
			if int(upImm) >= len(frame.closure.Upvalues) {
				return nil, fmt.Errorf("upvalue index out of range: %d", upImm)
			}
			uv := frame.closure.Upvalues[upImm]
			if uv.Closed {
				vm.setMem(frame, dst, uv.Val)
			} else if uv.Slot != nil {
				vm.setMem(frame, dst, *uv.Slot)
			}

		case OP_SET_UPVALUE:
			upImm, src := code[ip], code[ip+1]
			ip += 2
			if int(upImm) >= len(frame.closure.Upvalues) {
				return nil, fmt.Errorf("upvalue index out of range: %d", upImm)
			}
			uv := frame.closure.Upvalues[upImm]
			val := vm.getMem(frame, src)
			if uv.Closed {
				old := uv.Val
				if !valuesEqual(old, val) {
					if rc, isRC := val.(RCObject); isRC {
						rc.AddRef()
					}
					if rc, isRC := old.(RCObject); isRC {
						rc.Release()
					}
					uv.Val = val
					if vm.cc != nil {
						vm.cc.PossibleRoot(frame.closure)
					}
				}
			} else if uv.Slot != nil {
				old := *uv.Slot
				if !valuesEqual(old, val) {
					if rc, isRC := val.(RCObject); isRC {
						rc.AddRef()
					}
					if rc, isRC := old.(RCObject); isRC {
						rc.Release()
					}
					*uv.Slot = val
					if vm.cc != nil {
						vm.cc.PossibleRoot(frame.closure)
					}
				}
			}

		case OP_JMP:
			relPos := ip
			rel := DecodeRel(code[relPos])
			ip = int(int64(relPos) + rel)

		case OP_JMP_IF_FALSE:
			cond := code[ip]
			relPos := ip + 1
			rel := DecodeRel(code[relPos])
			ip += 2
			cAny := vm.getMem(frame, cond)
			b, ok := cAny.(bool)
			if !ok {
				return nil, fmt.Errorf("condition must be bool, got %T", cAny)
			}
			if !b {
				ip = int(int64(relPos) + rel)
			}

		case OP_JMP_IF_TRUE:
			cond := code[ip]
			relPos := ip + 1
			rel := DecodeRel(code[relPos])
			ip += 2
			cAny := vm.getMem(frame, cond)
			b, ok := cAny.(bool)
			if !ok {
				return nil, fmt.Errorf("condition must be bool, got %T", cAny)
			}
			if b {
				ip = int(int64(relPos) + rel)
			}

		case OP_CLOSURE:
			dst, fnImm := code[ip], code[ip+1]
			ip += 2
			innerFn, ok := fn.Constants[fnImm].(*CompiledFunction)
			if !ok {
				return nil, fmt.Errorf("invalid closure function constant at %d", fnImm)
			}
			var upvals []*VMUpvalue
			for _, desc := range innerFn.Upvalues {
				if desc.IsLocal {
					idx := int(desc.Index)
					if idx >= len(frame.mem) {
						newMem := make([]any, idx+1)
						copy(newMem, frame.mem)
						frame.mem = newMem
					}
					uv := &VMUpvalue{
						Closed: false,
						Slot:   &frame.mem[idx],
					}
					upvals = append(upvals, uv)
				} else {
					if int(desc.Index) < len(frame.closure.Upvalues) {
						upvals = append(upvals, frame.closure.Upvalues[desc.Index])
					}
				}
			}
			clos := NewVMClosure(vm, innerFn, upvals)
			vm.setMem(frame, dst, clos)
			clos.Release()

		case OP_CALL:
			fnMem, dstCount, argCount := code[ip], code[ip+1], code[ip+2]
			ip += 3
			var dsts []uint64
			if dstCount > 0 {
				dsts = make([]uint64, int(dstCount))
				for i := uint64(0); i < dstCount; i++ {
					dsts[i] = code[ip]
					ip++
				}
			}
			args := make([]any, int(argCount))
			for i := uint64(0); i < argCount; i++ {
				args[i] = vm.getMem(frame, code[ip])
				ip++
			}
			frame.ip = ip

			firstDst := uint64(0)
			if len(dsts) > 0 {
				firstDst = dsts[0]
			}
			callee := vm.getMem(frame, fnMem)

			switch c := callee.(type) {
			case *VMClosure:
				nextMemLen := int(c.Fn.MemCount)
				if len(args) > nextMemLen {
					nextMemLen = len(args)
				}
				nextFrame := vm.acquireFrame(c, nextMemLen, firstDst, dsts, true)
				for i, arg := range args {
					vm.setMem(nextFrame, uint64(i), arg)
				}
				vm.frames = append(vm.frames, nextFrame)
				continue

			case NativeFunc:
				argAddrs := make([]uint64, len(args))
				for i, a := range args {
					argAddrs[i] = vm.allocHeapSlot(a)
				}
				resAddr, err := c(vm, argAddrs, nil)
				if err != nil {
					return nil, err
				}
				resVal, _ := vm.derefHeap(resAddr)
				vm.storeReturnToCaller(frame, firstDst, dsts, resVal)

			case func(*VM, []uint64, []Type) (uint64, error):
				argAddrs := make([]uint64, len(args))
				for i, a := range args {
					argAddrs[i] = vm.allocHeapSlot(a)
				}
				resAddr, err := c(vm, argAddrs, nil)
				if err != nil {
					return nil, err
				}
				resVal, _ := vm.derefHeap(resAddr)
				vm.storeReturnToCaller(frame, firstDst, dsts, resVal)

			case func(*VM, []any) (any, error):
				res, err := c(vm, args)
				if err != nil {
					return nil, err
				}
				vm.storeReturnToCaller(frame, firstDst, dsts, res)

			default:
				return nil, fmt.Errorf("cannot call value of type %T", callee)
			}

		case OP_RET:
			valCount := code[ip]
			ip++
			var retVal any
			if valCount > 1 {
				mr := make([]any, int(valCount))
				for i := uint64(0); i < valCount; i++ {
					mr[i] = vm.getMem(frame, code[ip])
					ip++
				}
				retVal = multiReturnVals{vals: mr}
			} else if valCount == 1 {
				retVal = vm.getMem(frame, code[ip])
				ip++
			}
			_ = ip
			lastResult = retVal

			for _, uv := range frame.closure.Upvalues {
				if uv != nil && !uv.Closed && uv.Slot != nil {
					uv.Val = *uv.Slot
					if rc, isRC := uv.Val.(RCObject); isRC {
						rc.AddRef()
					}
					uv.Closed = true
					uv.Slot = nil
				}
			}

			callerHasRet := frame.hasRet
			callerRetDst := frame.retDst
			callerRetDsts := frame.retDsts
			vm.releaseFrame(frame)
			vm.frames = vm.frames[:len(vm.frames)-1]

			if callerHasRet && len(vm.frames) > 0 {
				callerFrame := vm.frames[len(vm.frames)-1]
				vm.storeReturnToCaller(callerFrame, callerRetDst, callerRetDsts, retVal)
			}
			continue

		case OP_FOR_STEP:
			idxMem, limMem, stepRaw, jcc := code[ip], code[ip+1], code[ip+2], code[ip+3]
			relPos := ip + 4
			rel := DecodeRel(code[relPos])
			ip += 5

			idxVal, okIdx := frame.mem[idxMem].(int64)
			if !okIdx {
				return nil, fmt.Errorf("OP_FOR_STEP index must be int64, got %T", frame.mem[idxMem])
			}
			limVal, okLim := frame.mem[limMem].(int64)
			if !okLim {
				return nil, fmt.Errorf("OP_FOR_STEP limit must be int64, got %T", frame.mem[limMem])
			}

			nextIdx := idxVal + int64(stepRaw)
			frame.mem[idxMem] = nextIdx

			var matched bool
			switch jcc {
			case JCC_LT:
				matched = nextIdx < limVal
			case JCC_LTE:
				matched = nextIdx <= limVal
			case JCC_GT:
				matched = nextIdx > limVal
			case JCC_GTE:
				matched = nextIdx >= limVal
			default:
				return nil, fmt.Errorf("unsupported JCC in OP_FOR_STEP: %d", jcc)
			}

			if matched {
				ip = int(int64(relPos) + rel)
			}

		case OP_HALT:
			vm.releaseFrame(frame)
			vm.frames = vm.frames[:len(vm.frames)-1]
			if vm.cc != nil {
				vm.cc.Collect()
			}
			return lastResult, nil

		default:
			return nil, fmt.Errorf("unknown opcode: %d", uint64(op))
		}

		frame.ip = ip
	}

	if vm.cc != nil {
		vm.cc.Collect()
	}
	return lastResult, nil
}

func (vm *VM) evalAdd(lhs, rhs any) (any, error) {
	switch l := lhs.(type) {
	case int64:
		r, ok := rhs.(int64)
		if !ok {
			return nil, fmt.Errorf("type mismatch in '+': int64 and %T", rhs)
		}
		return l + r, nil
	case float64:
		r, ok := rhs.(float64)
		if !ok {
			return nil, fmt.Errorf("type mismatch in '+': float64 and %T", rhs)
		}
		return l + r, nil
	case string:
		r, ok := rhs.(string)
		if !ok {
			return nil, fmt.Errorf("type mismatch in '+': string and %T", rhs)
		}
		return l + r, nil
	default:
		return nil, fmt.Errorf("operator '+' not supported for %T", lhs)
	}
}

func (vm *VM) evalSub(lhs, rhs any) (any, error) {
	switch l := lhs.(type) {
	case int64:
		r, ok := rhs.(int64)
		if !ok {
			return nil, fmt.Errorf("type mismatch in '-': int64 and %T", rhs)
		}
		return l - r, nil
	case float64:
		r, ok := rhs.(float64)
		if !ok {
			return nil, fmt.Errorf("type mismatch in '-': float64 and %T", rhs)
		}
		return l - r, nil
	default:
		return nil, fmt.Errorf("operator '-' not supported for %T", lhs)
	}
}

func (vm *VM) evalMul(lhs, rhs any) (any, error) {
	switch l := lhs.(type) {
	case int64:
		r, ok := rhs.(int64)
		if !ok {
			return nil, fmt.Errorf("type mismatch in '*': int64 and %T", rhs)
		}
		return l * r, nil
	case float64:
		r, ok := rhs.(float64)
		if !ok {
			return nil, fmt.Errorf("type mismatch in '*': float64 and %T", rhs)
		}
		return l * r, nil
	default:
		return nil, fmt.Errorf("operator '*' not supported for %T", lhs)
	}
}

func (vm *VM) evalDiv(lhs, rhs any) (any, error) {
	switch l := lhs.(type) {
	case int64:
		r, ok := rhs.(int64)
		if !ok {
			return nil, fmt.Errorf("type mismatch in '/': int64 and %T", rhs)
		}
		if r == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return l / r, nil
	case float64:
		r, ok := rhs.(float64)
		if !ok {
			return nil, fmt.Errorf("type mismatch in '/': float64 and %T", rhs)
		}
		if r == 0 || math.IsNaN(r) {
			return nil, fmt.Errorf("division by zero")
		}
		return l / r, nil
	default:
		return nil, fmt.Errorf("operator '/' not supported for %T", lhs)
	}
}

func (vm *VM) evalCmp(lhs, rhs any, jcc uint64) (bool, error) {
	switch l := lhs.(type) {
	case int64:
		r, ok := rhs.(int64)
		if !ok {
			return false, fmt.Errorf("cannot compare int64 with %T", rhs)
		}
		switch jcc {
		case JCC_EQ:
			return l == r, nil
		case JCC_NEQ:
			return l != r, nil
		case JCC_LT:
			return l < r, nil
		case JCC_LTE:
			return l <= r, nil
		case JCC_GT:
			return l > r, nil
		case JCC_GTE:
			return l >= r, nil
		}
	case float64:
		r, ok := rhs.(float64)
		if !ok {
			return false, fmt.Errorf("cannot compare float64 with %T", rhs)
		}
		switch jcc {
		case JCC_EQ:
			return l == r, nil
		case JCC_NEQ:
			return l != r, nil
		case JCC_LT:
			return l < r, nil
		case JCC_LTE:
			return l <= r, nil
		case JCC_GT:
			return l > r, nil
		case JCC_GTE:
			return l >= r, nil
		}
	case string:
		r, ok := rhs.(string)
		if !ok {
			return false, fmt.Errorf("cannot compare string with %T", rhs)
		}
		switch jcc {
		case JCC_EQ:
			return l == r, nil
		case JCC_NEQ:
			return l != r, nil
		}
	case bool:
		r, ok := rhs.(bool)
		if !ok {
			return false, fmt.Errorf("cannot compare bool with %T", rhs)
		}
		switch jcc {
		case JCC_EQ:
			return l == r, nil
		case JCC_NEQ:
			return l != r, nil
		}
	}
	return false, fmt.Errorf("unsupported comparison between %T and %T", lhs, rhs)
}
