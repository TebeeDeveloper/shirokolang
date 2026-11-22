package main

type Node interface {
	Line() int
	Column() int
}

type Stmt interface {
	Node
	stmtNode()
}

type Expr interface {
	Node
	exprNode()
}

type BasePos struct {
	TokLine int
	TokCol  int
}

func (p BasePos) Line() int   { return p.TokLine }
func (p BasePos) Column() int { return p.TokCol }

type Program struct {
	BasePos
	Items []Stmt
}

// ===== Module =====

type UseStmt struct {
	BasePos
	Module string
}

func (*UseStmt) stmtNode() {}

// ===== Struct =====

type StructDecl struct {
	BasePos
	Name   string
	Fields []*FieldDecl
}

func (*StructDecl) stmtNode() {}

type FieldDecl struct {
	BasePos
	Name string
	Type string
}

// ===== Function =====

type Param struct {
	BasePos
	Name     string
	Type     string
	Variadic bool
}

type FuncDecl struct {
	BasePos
	Name       string
	Params     []*Param
	ReturnType string
	Body       *BlockStmt
}

func (*FuncDecl) stmtNode() {}

// ===== Statements =====

type LetStmt struct {
	BasePos
	IsConst bool
	Name    string
	Names   []string
	Type    string
	Types   []string
	Init    Expr
}

func (*LetStmt) stmtNode() {}

type AssignStmt struct {
	BasePos
	Target Expr
	Value  Expr
}

func (*AssignStmt) stmtNode() {}

type ReturnStmt struct {
	BasePos
	Value  Expr
	Values []Expr
}

func (*ReturnStmt) stmtNode() {}

type ContinueStmt struct {
	BasePos
}

func (*ContinueStmt) stmtNode() {}

type BreakStmt struct {
	BasePos
}

func (*BreakStmt) stmtNode() {}

type IfBranch struct {
	BasePos
	Cond Expr
	Body *BlockStmt
}

type IfStmt struct {
	BasePos
	Branches []IfBranch
	ElseBody *BlockStmt
}

func (*IfStmt) stmtNode() {}

type ForStmt struct {
	BasePos
	Init Stmt
	Cond Expr
	Post Stmt
	Body *BlockStmt
}

func (*ForStmt) stmtNode() {}

type ForEachStmt struct {
	BasePos
	VarName  string
	Iterable Expr
	Body     *BlockStmt
}

func (*ForEachStmt) stmtNode() {}

type BlockStmt struct {
	BasePos
	Stmts []Stmt
}

func (*BlockStmt) stmtNode() {}

type ExprStmt struct {
	BasePos
	Expr Expr
}

func (*ExprStmt) stmtNode() {}

// ===== Expressions =====

type IntLit struct {
	BasePos
	Value string
}

func (*IntLit) exprNode() {}

type FloatLit struct {
	BasePos
	Value string
}

func (*FloatLit) exprNode() {}

type StringLit struct {
	BasePos
	Value string
}

func (*StringLit) exprNode() {}

type BoolLit struct {
	BasePos
	Value bool
}

func (*BoolLit) exprNode() {}

type IdentExpr struct {
	BasePos
	Name string
}

func (*IdentExpr) exprNode() {}

type StructFieldInit struct {
	BasePos
	Name  string
	Value Expr
}

type StructLit struct {
	BasePos
	TypeName string
	Fields   []*StructFieldInit
}

func (*StructLit) exprNode() {}

type ListLit struct {
	BasePos
	ElemTypeName string
	Elements     []Expr
	RepeatVal    Expr
	RepeatCount  Expr
}

func (*ListLit) exprNode() {}

type IndexExpr struct {
	BasePos
	Target Expr
	Index  Expr
}

func (*IndexExpr) exprNode() {}

type FuncLit struct {
	BasePos
	Params     []*Param
	ReturnType string
	Body       *BlockStmt
}

func (*FuncLit) exprNode() {}

type UnaryExpr struct {
	BasePos
	Op    string
	Right Expr
}

func (*UnaryExpr) exprNode() {}

type BinaryExpr struct {
	BasePos
	Left  Expr
	Op    string
	Right Expr
}

func (*BinaryExpr) exprNode() {}

type CallExpr struct {
	BasePos
	Callee Expr
	Args   []Expr
}

func (*CallExpr) exprNode() {}

type FieldExpr struct {
	BasePos
	Target Expr
	Field  string
}

func (*FieldExpr) exprNode() {}
