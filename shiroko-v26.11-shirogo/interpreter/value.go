package interpreter

// ---------- tagged union ----------

type ValueKind uint8

const (
	KindNil ValueKind = iota
	KindBool
	KindInt
	KindByte
	KindFloat
	KindString
	KindList
	KindMap
	KindSet
	KindStruct
	KindFunction
	KindBoundMethod
	KindBuiltin
	KindPackage
	KindRef
	KindErr
)

type Value struct {
	Kind  ValueKind
	Int   int
	Float float64
	Str   string
	Bool  bool
	Obj   any
}

func NilVal() Value                 { return Value{Kind: KindNil} }
func BoolVal(b bool) Value          { return Value{Kind: KindBool, Bool: b} }
func IntVal(n int) Value            { return Value{Kind: KindInt, Int: n} }
func FloatVal(f float64) Value      { return Value{Kind: KindFloat, Float: f} }
func StrVal(s string) Value         { return Value{Kind: KindString, Str: s} }
func ListVal(l *List) Value         { return Value{Kind: KindList, Obj: l} }
func MapVal(m *Map) Value           { return Value{Kind: KindMap, Obj: m} }
func SetVal(s *Set) Value           { return Value{Kind: KindSet, Obj: s} }
func StructVal(s *Struct) Value     { return Value{Kind: KindStruct, Obj: s} }
func FnVal(f *Function) Value       { return Value{Kind: KindFunction, Obj: f} }
func BoundVal(b *BoundMethod) Value { return Value{Kind: KindBoundMethod, Obj: b} }
func BuiltinVal(b *Builtin) Value   { return Value{Kind: KindBuiltin, Obj: b} }
func PkgVal(p *pkgValue) Value      { return Value{Kind: KindPackage, Obj: p} }
func RefVal(r Ref) Value            { return Value{Kind: KindRef, Obj: r} }
func ErrVal(e *Err) Value           { return Value{Kind: KindErr, Obj: e} }

func (v Value) Truthy() bool {
	switch v.Kind {
	case KindNil:
		return false
	case KindBool:
		return v.Bool
	}
	return true
}

func (v Value) deref() Value {
	if v.Kind == KindRef {
		return v.Obj.(Ref).Get()
	}
	return v
}

func looseEq(a, b Value) bool {
	if a.Kind == b.Kind {
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
		default:
			return a.Obj == b.Obj
		}
	}
	if a.Kind == KindInt && b.Kind == KindFloat {
		return float64(a.Int) == b.Float
	}
	if a.Kind == KindFloat && b.Kind == KindInt {
		return a.Float == float64(b.Int)
	}
	return false
}

// ---------- runtime objects ----------

type List struct{ Items []Value }

type Map struct {
	KeyTy string
	ValTy string
	Data  map[Value]Value
}

type Set struct {
	ElemTy string
	Items  []Value
}

func (s *Set) Add(v Value) {
	for _, x := range s.Items {
		if looseEq(x, v) {
			return
		}
	}
	s.Items = append(s.Items, v)
}

func (s *Set) Contains(v Value) bool {
	for _, x := range s.Items {
		if looseEq(x, v) {
			return true
		}
	}
	return false
}

type Struct struct {
	TypeName string
	Fields   map[string]Value
}

type StructSchema struct {
	Name   string
	Fields map[string]string
}

type Function struct {
	Name     string
	Params   []Param
	RecvTy   string
	Compiled *CompiledFn
}

type Param struct {
	Name     string
	Variadic bool
}

type BoundMethod struct {
	Recv Value
	Fn   *Function
}

type Builtin struct {
	Name string
	Fn   func(ip *Interpreter, args []Value) Value
}

type pkgValue struct {
	Name    string
	Members map[string]Value
}

type Err struct{ Msg string }

func (e *Err) Error() string { return e.Msg }

// ---------- references ----------

type Ref interface {
	Get() Value
	Set(Value)
}

type Slot struct {
	Name  string
	Value Value
}

func (s *Slot) Get() Value  { return s.Value }
func (s *Slot) Set(v Value) { s.Value = v }

type FieldRef struct {
	Owner *Struct
	Name  string
}

func (f *FieldRef) Get() Value  { return f.Owner.Fields[f.Name] }
func (f *FieldRef) Set(v Value) { f.Owner.Fields[f.Name] = v }

type mapRef struct {
	m   *Map
	key Value
}

func (r *mapRef) Get() Value  { return r.m.Data[r.key] }
func (r *mapRef) Set(v Value) { r.m.Data[r.key] = v }

type listRef struct {
	l *List
	i int
}

func (r *listRef) Get() Value  { return r.l.Items[r.i] }
func (r *listRef) Set(v Value) { r.l.Items[r.i] = v }

type localRef struct {
	ip  *Interpreter
	idx int
}

func (r *localRef) Get() Value  { return r.ip.mem[r.idx] }
func (r *localRef) Set(v Value) { r.ip.mem[r.idx] = v }

// ---------- helpers ----------

func kindName(k ValueKind) string {
	switch k {
	case KindNil:
		return "nil"
	case KindBool:
		return "bool"
	case KindInt:
		return "int"
	case KindFloat:
		return "float"
	case KindString:
		return "string"
	case KindList:
		return "list"
	case KindMap:
		return "map"
	case KindSet:
		return "set"
	case KindStruct:
		return "struct"
	case KindFunction:
		return "function"
	case KindBoundMethod:
		return "method"
	case KindBuiltin:
		return "builtin"
	case KindPackage:
		return "package"
	case KindRef:
		return "pointer"
	case KindErr:
		return "error"
	}
	return "unknown"
}

func typeNodeName(node any) string {
	n, ok := node.([]any)
	if !ok || len(n) < 2 {
		return "any"
	}
	switch n[0].(string) {
	case "NamedType":
		return n[1].(string)
	case "PtrType":
		return "*" + typeNodeName(n[1])
	case "ListType":
		return "[]" + typeNodeName(n[1])
	case "SetType":
		return "{}" + typeNodeName(n[1])
	case "MapType":
		return "map[" + typeNodeName(n[1]) + "]" + typeNodeName(n[2])
	case "TupleType":
		return "any"
	}
	return "any"
}
