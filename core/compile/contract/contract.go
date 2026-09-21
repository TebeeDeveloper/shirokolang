// shiroko/core/compile/contract/contract.go
package contract

import (
	"fmt"
	"shiroko/core/parse"
	"slices"
)

type ContractChecker struct {
	Error []string
}

func Validate(prgm parse.Program) ContractChecker {
	var node parse.ASTNode
	var funcToModules map[string]string = map[string]string{
		"print": "print",

		"sqrt": "math",
		"pow": "math",
		"abs": "math",
		"sin": "math",
		"cos": "math",
		"tan": "math",
		"asin": "math",
		"acos": "math",
		"atan": "math",

		"read": "io",
		"write": "io",

		"len": "list",
		"strlen": "string",
	}
	var declaredModules []string
	var usedModules map[string]bool = make(map[string]bool)

	for _, node = range prgm.All {
		switch node.(type) {
		case parse.ContractDecl:
			var n parse.ContractDecl = node.(parse.ContractDecl)
			declaredModules = n.Modules
		default:
			walkNode(node, func(current parse.ASTNode){
				var call parse.FuncCall
				var ok bool
				call, ok = current.(parse.FuncCall)
				if ok {
					var fnName = call.Name.Name
					var mod string
					var exists bool
					mod, exists = funcToModules[fnName]
					if exists {
						usedModules[mod] = true
					}
				}
			})
		}
	}

	var errs []string

	var usedMod string
	for usedMod = range usedModules {
		var isDeclared bool = slices.Contains(declaredModules, usedMod)
		if !isDeclared {
			errs = append(errs, fmt.Sprintf("module |%s| could not in contract", usedMod))
		}
	}

	var declMod string
	for _, declMod = range declaredModules {
		var isUsed bool = usedModules[declMod]
		if !isUsed {
			errs = append(errs, fmt.Sprintf("module |%s| unusued", declMod))
		}
	}

	return ContractChecker{
		Error: errs,
	}
}

func walkNode(node parse.ASTNode, callback func(parse.ASTNode)) {
	callback(node)


	var stmt parse.ASTNode
	switch node.(type) {
	case parse.FuncDecl:
		var n parse.FuncDecl = node.(parse.FuncDecl)
		for _, stmt = range n.Body {
			walkNode(stmt, callback)
		}
	case parse.ForLoop:
		var n parse.ForLoop = node.(parse.ForLoop)
		walkNode(n.Init, callback)
		walkNode(n.Cond, callback)
		walkNode(n.Step, callback)
		for _, stmt = range n.Body {
			walkNode(stmt, callback)
		}
	case parse.ForList:
		var n parse.ForList = node.(parse.ForList)
		walkNode(n.Cond, callback)
		for _, stmt = range n.Body {
			walkNode(stmt, callback)
		}
	case parse.ForAll:
		var n parse.ForAll = node.(parse.ForAll)
		walkNode(n.Cond, callback)
		for _, stmt = range n.Body {
			walkNode(stmt, callback)
		}
	case parse.Exist:
		var n parse.Exist = node.(parse.Exist)
		walkNode(n.Cond, callback)
		for _, stmt = range n.Body {
			walkNode(stmt, callback)
		}
	case parse.VarDecl:
		var n parse.VarDecl = node.(parse.VarDecl)
		walkNode(n.Ex, callback)
	case parse.Assign:
		var n parse.Assign = node.(parse.Assign)
		walkNode(n.Ex, callback)
	case parse.ListStmt:
		var n parse.ListStmt = node.(parse.ListStmt)
		walkNode(n.Ex, callback)
	case parse.Expr:
		var n parse.Expr = node.(parse.Expr)
		walkNode(n.Ex, callback)
	case parse.PrimaryExpr:
		var n parse.PrimaryExpr = node.(parse.PrimaryExpr)
		walkNode(n.Ex, callback)
	case parse.ListExpr:
		var n parse.ListExpr = node.(parse.ListExpr)
		walkNode(n.Ex, callback)
	case parse.OrExpr:
		var n parse.OrExpr = node.(parse.OrExpr)
		walkNode(n.Left, callback)
		walkNode(n.Right, callback)
	case parse.AndExpr:
		var n parse.AndExpr = node.(parse.AndExpr)
		walkNode(n.Left, callback)
		walkNode(n.Right, callback)
	case parse.CompareExpr:
		var n parse.CompareExpr = node.(parse.CompareExpr)
		walkNode(n.Left, callback)
		walkNode(n.Right, callback)
	case parse.SumExpr:
		var n parse.SumExpr = node.(parse.SumExpr)
		walkNode(n.Left, callback)
		walkNode(n.Right, callback)
	case parse.ProductExpr:
		var n parse.ProductExpr = node.(parse.ProductExpr)
		walkNode(n.Left, callback)
		walkNode(n.Right, callback)
	case parse.UnaryExpr:
		var n parse.UnaryExpr = node.(parse.UnaryExpr)
		walkNode(n.Right, callback)
	case parse.FuncCall:
		var n parse.FuncCall = node.(parse.FuncCall)
		walkNode(n.Args, callback)
	case parse.ASTNodes:
		var n parse.ASTNodes = node.(parse.ASTNodes)
		var arg parse.ASTNode
		for _, arg = range n.Node {
			walkNode(arg, callback)
		}
	}
}
