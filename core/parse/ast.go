// shiroko/core/parse/ast.go

package parse

type ASTNode interface{
	IsASTNode()
}

type ASTNodes struct {
	Node []ASTNode
}

// Literal

type Ident struct {
	Type string
	Name string
}

type VarName struct {
	Name string
}

type List struct {
	Name string
}

// Constant

type FloatNumber struct {
	Value string
}

type IntNumber struct {
	Value string
}

type String struct {
	Value string
}

// Expr

type OrExpr struct {
	Left ASTNode
	Right ASTNode
}

type AndExpr struct {
	Left ASTNode
	Right ASTNode
}

type CompareExpr struct {
	Left ASTNode
	Op string
	Right ASTNode
}

type SumExpr struct {
	Left ASTNode
	Op string
	Right ASTNode
}

type ProductExpr struct {
	Left ASTNode
	Op string
	Right ASTNode
}

type UnaryExpr struct {
	Op string
	Right ASTNode
}

type PrimaryExpr struct {
	Ex ASTNode
}

type Expr struct {
	Ex ASTNode
}

// List

type ContainList struct {
	Left ASTNode
	Right ASTNode
}

type DifferenceList struct {
	Left ASTNode
	Right ASTNode
}

type IntersectList struct {
	Left ASTNode
	Right ASTNode
}

type UnionList struct {
	Left ASTNode
	Right ASTNode
}

type ListExpr struct {
	Ex ASTNode
}

// Statement

type ContractDecl struct {
	Modules []string
}

type VarDecl struct {
	Id Ident
	Ex Expr
}

type ListStmt struct {
	Id Ident
	Ex ListExpr
}

type Assign struct {
	Id Ident
	Ex Expr
}

type ForLoop struct {
	Var VarName
	Init ASTNode
	Cond ASTNode
	Step ASTNode
	Body []ASTNode
}

type ForList struct {
	Var VarName
	List VarName
	Cond ASTNode
	Body []ASTNode
}

type FuncDecl struct {
	Name Ident
	Args []Ident
	Body []ASTNode
}

type FuncCall struct {
	Name Ident
	Args ASTNode
}

type ForAll struct {
	Var VarName
	List VarName
	Cond ASTNode
	Body []ASTNode
}

type Exist struct {
	Var VarName
	List VarName
	Cond ASTNode
	Body []ASTNode
}

// Program
type Program struct {
	All []ASTNode
}

func (self ASTNodes) IsASTNode() {}
func (self Ident) IsASTNode() {}
func (self VarName) IsASTNode() {}
func (self List) IsASTNode() {}
func (self IntNumber) IsASTNode() {}
func (self FloatNumber) IsASTNode() {}
func (self String) IsASTNode() {}
func (self OrExpr) IsASTNode() {}
func (self AndExpr) IsASTNode() {}
func (self CompareExpr) IsASTNode() {}
func (self SumExpr) IsASTNode() {}
func (self ProductExpr) IsASTNode() {}
func (self UnaryExpr) IsASTNode() {}
func (self PrimaryExpr) IsASTNode() {}
func (self Expr) IsASTNode() {}
func (self ContainList) IsASTNode() {}
func (self IntersectList) IsASTNode() {}
func (self DifferenceList) IsASTNode() {}
func (self UnionList) IsASTNode() {}
func (self ListExpr) IsASTNode() {}
func (self ContractDecl) IsASTNode() {}
func (self VarDecl) IsASTNode() {}
func (self ListStmt) IsASTNode() {}
func (self Assign) IsASTNode() {}
func (self ForLoop) IsASTNode() {}
func (self ForList) IsASTNode() {}
func (self FuncDecl) IsASTNode() {}
func (self FuncCall) IsASTNode() {}
func (self ForAll) IsASTNode() {}
func (self Exist) IsASTNode() {}
func (self Program) IsASTNode() {}
