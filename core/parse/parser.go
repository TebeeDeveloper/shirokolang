package parse

import (
	"fmt"
	"shiroko/core/tokenize"
	"strings"
)

type Parser struct { // size=184 (0xb8), class=192 (0xc0)
	Lexer tokenize.Tokenizer
	Prev tokenize.Token
	Cur tokenize.Token
	Line int
	Col int
	Error []string
}

type ASTResult struct { // size=16 (0x10)
	Node ASTNode
}

func Init(Lexer tokenize.Tokenizer) (p Parser) {
	p.Lexer = Lexer
	p.Cur = Lexer.NextToken()
	p.Line = 1
	p.Col = 1

	return
}

func (p* Parser) PrintError() {
	var e string
	for _, e = range p.Error {
		fmt.Println(e)
	}
}

func (p *Parser) ParseAll() (a ASTResult) {
	var prgm Program
	for p.Cur.Type != "EOF" {
		var ast ASTResult = p.parseStmt()

		if ast.Node != nil {
			prgm.All = append(prgm.All, ast.Node)
		}
	}
	a.Node = prgm
	return
}

func (p *Parser) parseStmt() ASTResult {
	for p.Cur.Type == "NewLine" {
		p.nextToken()
	}
	if p.Cur.Type == "KW" {
		switch p.Cur.Value {
			case "contract":
				return p.parseContract()
			case "var":
				return p.parseVar()
			case "for":
				return p.parseFor()
			case "fn":
				return p.parseFunc()
			case "V":
				return p.parseForAll()
			case "E":
				return p.parseExist()
		}
	}
	if p.Cur.Type == "ID" {
		return p.parseAssign()
	}
	p.addErrorln(fmt.Sprintf("unexpected token \"%s\" (%s)", p.Cur.Value, p.Cur.Type))
	p.nextToken()
	return ASTResult{Node: nil}
}

func (p *Parser) parseContract() ASTResult {
	p.nextToken()

	if p.Cur.Type != "Char" || p.Cur.Value != "=" {
		p.addError("Assign (\"=\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	if p.Cur.Type != "Char" || p.Cur.Value != "{" {
		p.addError("Open scope (\"{\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	if p.Cur.Type != "NewLine" {
		p.addErrorln("Cannot code contract in one line")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	var modules []string
	for p.Cur.Type != "Char" && p.Cur.Value != "}" {
		if p.Cur.Type == "EOF" {
			break
		}

		var module string

		if p.Cur.Type != "ID" {
			p.addError("ID (module name)")
			p.sync()
			return ASTResult{Node: nil}
		}
		module = p.Cur.Value
		p.nextToken()

		if p.Cur.Type != "NewLine" {
			p.addErrorln("Cannot write in one line")
			p.sync()
			return ASTResult{Node: nil}
		}
		p.nextToken()

		modules = append(modules, module)
	}

	if p.Cur.Type != "Char" || p.Cur.Value != "}" {
		p.addError("Close scope (\"}\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	if p.Cur.Type == "NewLine" {
		p.nextToken()
	}

	return ASTResult{
		Node: ContractDecl{
			Modules: modules,
		},
	}
}

func (p *Parser) parseVar() (varstmt ASTResult) {
	p.nextToken()

	var vname string
	var vtype string
	var expr ASTResult

	if p.Cur.Type != "ID" {
		p.addError("ID (variable name)")
		p.sync()
		return ASTResult{Node: nil}
	}
	vname = p.Cur.Value
	p.nextToken()

	if p.Cur.Type != "Type" {
		p.addError("Type (\"int\" | \"float\" | \"string\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	vtype = p.Cur.Value
	p.nextToken()

	if p.Cur.Type != "Char" || p.Cur.Value != "=" {
		p.addError("Assign (\"=\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	expr = p.parseExpr()

	if p.Cur.Type == "NewLine" {
		p.nextToken()
	}

	varstmt.Node = VarDecl{
		Id: Ident{
			Type: vtype,
			Name: vname,
		},
		Ex: Expr{
			Ex: expr.Node,
		},
	}
	return
}

func (p *Parser) parseAssign() (assign ASTResult) {
	var vname string
	var expr ASTResult

	if p.Cur.Type != "ID" {
		p.addError("ID (variable name)")
		p.sync()
		return ASTResult{Node: nil}
	}
	vname = p.Cur.Value
	p.nextToken()

	if p.Cur.Type == "Char" && p.Cur.Value == "(" {
		return p.parseCall(vname)
	}

	if p.Cur.Type != "Char" || p.Cur.Value != "=" {
		p.addError("Assign (\"=\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	expr = p.parseExpr()

	if p.Cur.Type == "NewLine" {
		p.nextToken()
	}

	assign.Node = Assign{
		Id: Ident{
			Name: vname,
			Type: "",
		},
		Ex: Expr{
			expr.Node,
		},
	}

	return
}

func (p *Parser) parseFor() ASTResult {
	p.nextToken()

	var vname string
	var vinit ASTResult
	var vcond ASTResult
	var vstep ASTResult

	if p.Cur.Type == "ID" {
		vname = p.Cur.Value
		p.nextToken()

		if p.Cur.Type == "Char" && p.Cur.Value == "@" {
			p.nextToken()

			var vlist string
			if p.Cur.Type == "ID" {
				vlist = p.Cur.Value
				p.nextToken()
			} else {
				p.addError("ID (list name)")
				p.sync()
				return ASTResult{Node: nil}
			}

			if p.Cur.Type == "Char" && p.Cur.Value == "|" {
				p.nextToken()

				vcond = p.parseExpr()
				if p.Cur.Type != "Char" || p.Cur.Value != "{" {
					p.addError("Open scope (\"{\")")
					p.sync()
					return ASTResult{Node: nil}
				}
				p.nextToken()

				if p.Cur.Type == "NewLine" {
					p.nextToken()
				} else {
					p.addErrorln("Cannot write in only one line")
					p.sync()
					return ASTResult{Node: nil}
				}

				var depth int = 1
				var block []ASTNode
				for {
					if p.Cur.Type == "EOF" {
						break
					}
					if p.Cur.Type == "Char" && p.Cur.Value == "{" {
						depth++
						p.nextToken()
						continue
					} else if p.Cur.Type == "Char" && p.Cur.Value == "}" {
						depth--
						if depth == 0 {
							break
						}
						p.nextToken()
						continue
					}
					if p.Cur.Type == "NewLine" {
						p.nextToken()
						continue
					}
					var stmt ASTResult = p.parseStmt()
					if stmt.Node != nil {
						block = append(block, stmt.Node)
					}
				}

				if p.Cur.Type != "Char" || p.Cur.Value != "}" {
					p.addError("Close scope (\"}\")")
					p.sync()
					return ASTResult{Node: nil}
				}
				p.nextToken()

				if p.Cur.Type == "NewLine" {
					p.nextToken()
				}

				return ASTResult{
					Node: ForList{
						Var: VarName{Name: vname},
						List: VarName{Name: vlist},
						Cond: vcond.Node,
						Body: block,
					},
				}
			} else {

				var depth int = 1
				var block []ASTNode
				if p.Cur.Type != "Char" || p.Cur.Value != "{" {
					p.addError("Open scope (\"{\")")
				}
				p.nextToken()

				if p.Cur.Type == "NewLine" {
					p.nextToken()
				} else {
					p.addErrorln("Cannot write in only one line")
					p.sync()
					return ASTResult{Node: nil}
				}

				for {
					if p.Cur.Type == "EOF" {
						break
					}

					if p.Cur.Type == "Char" && p.Cur.Value == "{" {
						depth++
						p.nextToken()
						continue
					} else if p.Cur.Type == "Char" && p.Cur.Value == "}" {
						depth--
						if depth == 0 {
							break
						}
						p.nextToken()
						continue
					}

					if p.Cur.Type == "NewLine" {
						p.nextToken()
						continue
					}

					var stmt ASTResult = p.parseStmt()
					if stmt.Node != nil {
						block = append(block, stmt.Node)
					}
				}
				if p.Cur.Type != "Char" || p.Cur.Value != "}" {
					p.addError("Close scope (\"}\")")
					p.sync()
					return ASTResult{Node: nil}
				}
				p.nextToken()

				if p.Cur.Type == "NewLine" {
					p.nextToken()
				}

				return ASTResult{
					Node: ForList{
						Var: VarName{Name: vname},
						List: VarName{Name: vlist},
						Cond: nil,
						Body: block,
					},
				}
			}
		} else {
			if p.Cur.Type != "Char" || p.Cur.Value != "=" {
				p.addError("Assign (\"=\")")
				p.sync()
				return ASTResult{Node: nil}
			}
			p.nextToken()

			vinit = p.parseExpr()

			if p.Cur.Type != "Char" || p.Cur.Value != ";" {
				p.addError("SemiColon (\";\")")
				p.sync()
				return ASTResult{Node: nil}
			}
			p.nextToken()

			vcond = p.parseExpr()

			if p.Cur.Type != "Char" || p.Cur.Value != ";" {
				p.addError("SemiColon (\";\")")
				p.sync()
				return ASTResult{Node: nil}
			}
			p.nextToken()

			vstep = p.parseExpr()

			if p.Cur.Type != "Char" || p.Cur.Value != "{" {
				p.addError("Open scope (\"{\")")
				p.sync()
				return ASTResult{Node: nil}
			}
			p.nextToken()

			if p.Cur.Type == "NewLine" {
				p.nextToken()
			} else {
				p.addErrorln("Cannot write in one line")
				p.sync()
				return ASTResult{Node: nil}
			}
		}
	} else {
		p.addError("ID (variable name)")
		p.sync()
		return ASTResult{Node: nil}
	}

	var depth int = 1
	var block []ASTNode
	for {
		if p.Cur.Type == "EOF" {
			break
		}

		if p.Cur.Type == "Char" && p.Cur.Value == "{" {
			depth++
			p.nextToken()
			continue
		} else if p.Cur.Type == "Char" && p.Cur.Value == "}" {
			depth--
			if depth == 0 {
				break
			}
			p.nextToken()
			continue
		}

		if p.Cur.Type == "NewLine" {
			p.nextToken()
			continue
		}

		var stmt ASTResult = p.parseStmt()

		if stmt.Node != nil {
			block = append(block, stmt.Node)
		}
	}

	return ASTResult{
		Node: ForLoop{
			Var: VarName{
				Name: vname,
			},
			Init: vinit.Node,
			Cond: vcond.Node,
			Step: vstep.Node,
			Body: block,
		},
	}
}

func (p *Parser) parseFunc() ASTResult {
	p.nextToken()

	var fname string
	var ftype string
	var fargs []Ident

	if p.Cur.Type != "ID" {
		p.addError("ID (function name)")
		p.sync()
		return ASTResult{Node: nil}
	}
	fname = p.Cur.Value
	p.nextToken()

	if p.Cur.Type != "Char" || p.Cur.Value != "(" {
		p.addError("Open parenthesis (\"(\")")
		p.sync()
		return  ASTResult{Node: nil}
	}
	p.nextToken()

	for p.Cur.Type != "Char" || p.Cur.Value != ")" {
		if p.Cur.Type != "ID" {
			p.addError("ID (argument name)")
			p.sync()
			return ASTResult{Node: nil}
		}

		var argName string  = p.Cur.Value
		p.nextToken()

		if p.Cur.Type != "Type" {
			p.addError("Type (argument type)")
			p.sync()
			return ASTResult{Node: nil}
		}

		var argType string = p.Cur.Value
		p.nextToken()

		fargs = append(fargs, Ident{Type: argType, Name: argName})

		if p.Cur.Type == "Char" && p.Cur.Value == "," {
			p.nextToken()
		} else {
			break
		}
	}

	if p.Cur.Type != "Char" || p.Cur.Value != ")" {
		p.addError("Close parenthesis (\")\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	if p.Cur.Type != "Char" || p.Cur.Value != "=" {
		p.addError("Assign (\"=\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	if p.Cur.Type == "Char" && p.Cur.Value == "{" {
		ftype = "void"
	} else if p.Cur.Type == "Type" {
		ftype = p.Cur.Value
		p.nextToken()
	} else {
		p.addError("Type (return type)")
		p.sync()
		return ASTResult{Node: nil}
	}

	if p.Cur.Type != "Char" || p.Cur.Value != "{" {
		p.addError("Open scope (\"{\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	var depth int = 1
	var block []ASTNode

	for {
		if p.Cur.Type == "EOF" {
			break
		}

		if p.Cur.Type == "Char" && p.Cur.Value == "{" {
			depth++
			p.nextToken()
			continue
		} else if p.Cur.Type == "Char" && p.Cur.Value == "}" {
			depth--
			if depth == 0 {
				break
			}
			p.nextToken()
			continue
		}

		if p.Cur.Type == "NewLine" {
			p.nextToken()
			continue
		}

		var stmt ASTResult = p.parseStmt()
		if stmt.Node != nil {
			block = append(block, stmt.Node)
		}
	}

	if p.Cur.Type != "Char" || p.Cur.Value != "}" {
		p.addError("Close scope (\"}\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	if p.Cur.Type == "NewLine" {
		p.nextToken()
	}

	return ASTResult{
		Node: FuncDecl{
			Name: Ident{Type: ftype, Name: fname},
			Args: fargs,
			Body: block,
		},
	}
}

func (p *Parser) parseCall(name string) ASTResult {
	if p.Cur.Type != "Char" || p.Cur.Value != "(" {
		return ASTResult{Node: nil}
	}
	p.nextToken()

	var args []ASTNode
	for {
		if p.Cur.Type == "EOF" {
			break
		}

		var expr ASTResult = p.parseExpr()
		if expr.Node != nil {
			args = append(args, expr.Node)
		}

		if p.Cur.Type == "Char" && p.Cur.Value == "," {
			p.nextToken()
		} else {
			break
		}

		if p.Cur.Type == "Char" && p.Cur.Value == ")" {
			break
		}
	}
	if p.Cur.Type != "Char" || p.Cur.Value != ")" {
		p.addError("Close parenthesis (\")\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	if p.Cur.Type == "NewLine" {
		p.nextToken()
	}

	return ASTResult{
		Node: FuncCall{
			Name: Ident{
				Name: name,
			},
			Args: ASTNodes{Node: args},
		},
	}
}

func (p *Parser) parseForAll() ASTResult {
	p.nextToken()

	var vname string
	var vlist string
	var vcond ASTResult

	if p.Cur.Type != "ID" {
		p.addError("ID (variable name)")
		p.sync()
		return ASTResult{Node: nil}
	}
	vname = p.Cur.Value
	p.nextToken()

	if p.Cur.Type != "Char" || p.Cur.Value != "@" {
		p.addError("At (@ is ∈)")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	if p.Cur.Type != "ID" {
		p.addError("ID (list name)")
		p.sync()
		return ASTResult{Node: nil}
	}
	vlist = p.Cur.Value
	p.nextToken()

	if p.Cur.Type != "Char" || p.Cur.Value != "|" {
		p.addError("with (\"|\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	vcond = p.parseExpr()

	if p.Cur.Type != "Char" || p.Cur.Value != "{" {
		p.addError("Open scope (\"{\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	var depth int = 1
	var block []ASTNode
	for {
		if p.Cur.Type == "EOF" {
			break
		}

		if p.Cur.Type == "Char" && p.Cur.Value == "{" {
			depth++
			p.nextToken()
			continue
		} else if p.Cur.Type == "Char" && p.Cur.Value == "}" {
			depth--
			if depth == 0 {
				break
			}
			p.nextToken()
			continue
		}

		if p.Cur.Type == "NewLine" {
			p.nextToken()
			continue
		}

		var stmt ASTResult = p.parseStmt()
		if stmt.Node != nil {
			block = append(block, stmt.Node)
		}
	}

	if p.Cur.Type != "Char" || p.Cur.Value != "}" {
		p.addError("Close scope (\"}\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	if p.Cur.Type == "NewLine" {
		p.nextToken()
	}

	return ASTResult{
		Node: ForAll{
			Var: VarName{Name: vname},
			List: VarName{Name: vlist},
			Cond: vcond.Node,
			Body: block,
		},
	}
}

func (p *Parser) parseExist() ASTResult {
	p.nextToken()

	var vname string
	var vlist string
	var vcond ASTResult

	if p.Cur.Type != "ID" {
		p.addError("ID (variable name)")
		p.sync()
		return ASTResult{Node: nil}
	}
	vname = p.Cur.Value
	p.nextToken()

	if p.Cur.Type != "Char" || p.Cur.Value != "@" {
		p.addError("At (@ is ∈)")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	if p.Cur.Type != "ID" {
		p.addError("ID (list name)")
		p.sync()
		return ASTResult{Node: nil}
	}
	vlist = p.Cur.Value
	p.nextToken()

	if p.Cur.Type != "Char" || p.Cur.Value != "|" {
		p.addError("with (\"|\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	vcond = p.parseExpr()

	if p.Cur.Type != "Char" || p.Cur.Value != "{" {
		p.addError("Open scope (\"{\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	var depth int = 1
	var block []ASTNode
	for {
		if p.Cur.Type == "EOF" {
			break
		}

		if p.Cur.Type == "Char" && p.Cur.Value == "{" {
			depth++
			p.nextToken()
			continue
		} else if p.Cur.Type == "Char" && p.Cur.Value == "}" {
			depth--
			if depth == 0 {
				break
			}
			p.nextToken()
		}

		if p.Cur.Type == "NewLine" {
			p.nextToken()
			continue
		}

		var stmt ASTResult = p.parseStmt()
		if stmt.Node != nil {
			block = append(block, stmt.Node)
		}
	}

	if p.Cur.Type != "Char" || p.Cur.Value != "}" {
		p.addError("Close scope (\"}\")")
		p.sync()
		return ASTResult{Node: nil}
	}
	p.nextToken()

	if p.Cur.Type == "NewLine" {
		p.nextToken()
	}

	return ASTResult{
		Node: Exist{
			Var: VarName{Name: vname},
			List: VarName{Name: vlist},
			Cond: vcond.Node,
			Body: block,
		},
	}
}

func (p *Parser) parseListExpr() ASTResult {
	return p.parseListContain()
}

func (p *Parser) parseListContain() (left ASTResult) {
	left = p.parseListDifference()
	for p.Cur.Type == "ListOp" && p.Cur.Value == "c" {
		p.nextToken()

		var right ASTResult
		right = p.parseListDifference()

		left.Node = ContainList{Left: left.Node, Right: right.Node}
	}

	return left
}

func (p *Parser) parseListDifference() (left ASTResult) {
	left = p.parseListIntersect()
	for p.Cur.Type == "ListOp" && p.Cur.Value == "\\" {
		p.nextToken()

		var right ASTResult = p.parseListIntersect()

		left.Node = DifferenceList{Left: left.Node, Right: right.Node}
	}

	return left
}

func (p *Parser) parseListIntersect() (left ASTResult) {
	left = p.parseListUnion()
	for p.Cur.Type == "ListOp" && p.Cur.Value == "n" {
		p.nextToken()

		var right ASTResult = p.parseListUnion()

		left.Node = IntersectList{Left: left.Node, Right: right.Node}
	}

	return left
}

func (p *Parser) parseListUnion() (left ASTResult) {
	left = p.parseListPrimary()
	for p.Cur.Type == "ListOp" && p.Cur.Value == "u" {
		p.nextToken()

		var right ASTResult = p.parseListPrimary()

		left.Node = UnionList{Left: left.Node, Right: right.Node}
	}
	return left
}

func (p *Parser) parseListPrimary() ASTResult {
	if p.Cur.Type == "ID" {
		return ASTResult{
			Node: List{
				Name: p.Cur.Value,
			},
		}
	}
	p.addError("ID (list name)")
	p.sync()
	return ASTResult{Node: nil}
}

func (p *Parser) parseExpr() ASTResult {
	return p.parseOr()
}

func (p* Parser) parseOr() (left ASTResult) {
	left = p.parseAnd()
	for p.Cur.Type == "OP" && p.Cur.Value == "||" {
		p.nextToken()

		var right ASTResult = p.parseAnd()

		left.Node = OrExpr{Left: left.Node, Right: right.Node}
	}
	return left
}

func (p *Parser) parseAnd() (left ASTResult) {
	left = p.parseCompare()
	for p.Cur.Type == "OP" && p.Cur.Value == "&&" {
		p.nextToken()

		var right ASTResult = p.parseCompare()

		left.Node = AndExpr{Left: left.Node, Right: right.Node}
	}
	return left
}

func (p *Parser) parseCompare() (left ASTResult) {
	left = p.parseSum()
	for (p.Cur.Type == "OP" || p.Cur.Type == "Char") && (p.Cur.Value == "==" || p.Cur.Value == "!=" || p.Cur.Value == "<=" || p.Cur.Value == ">=" || p.Cur.Value == "<" || p.Cur.Value == ">") {
		var op string = p.Cur.Value
		p.nextToken()

		var right ASTResult = p.parseSum()

		left.Node = CompareExpr{Left: left.Node, Op: op, Right: right.Node}
	}
	return left
}

func (p *Parser) parseSum() (left ASTResult) {
	left = p.parseProduct()
	for p.Cur.Type == "Char" && (p.Cur.Value == "+" || p.Cur.Value == "-") {
		var op string = p.Cur.Value
		p.nextToken()

		var right ASTResult = p.parseProduct()
		if op == "-" {
			var unaryexpr UnaryExpr = UnaryExpr{Op: "-", Right: right.Node}
			left.Node = SumExpr{Left: left.Node, Op: "+", Right: unaryexpr}
		} else {
			left.Node = SumExpr{Left: left.Node, Op: "+", Right: right.Node}
		}
	}
	return left
}

func (p *Parser) parseProduct() (left ASTResult) {
	left = p.parseUnary()
	for p.Cur.Type == "Char" && (p.Cur.Value == "*" || p.Cur.Value == "/") {
		var op string = p.Cur.Value
		p.nextToken()

		var right ASTResult = p.parseUnary()

		left.Node = ProductExpr{Left: left.Node, Op: op, Right: right.Node}
	}
	return left
}

func (p *Parser) parseUnary() (a ASTResult) {
	if p.Cur.Type == "Char" && p.Cur.Value == "-" {
		p.nextToken()

		var right ASTResult = p.parsePrimary()

		a.Node = UnaryExpr{Op: "-", Right: right.Node}
		return
	}
	return p.parsePrimary()
}

func (p *Parser) parsePrimary() (a ASTResult) {
	if p.Cur.Type == "String" {
		var s string = p.Cur.Value
		p.nextToken()
		a.Node = String{Value: s}
		return
	}

	if p.Cur.Type == "Number" {
		var v string = p.Cur.Value
		p.nextToken()
		if strings.Count(v, ".") > 1 {
			p.addError("Not a float number")
			p.sync()
			a.Node = nil
			return
		}
		if strings.Contains(v, ".") {
			a.Node = FloatNumber{Value: v}
			return
		} else {
			a.Node = IntNumber{Value: v}
			return
		}
	}

	if p.Cur.Type == "ID" {
		var vname string = p.Cur.Value
		p.nextToken()
		if p.Cur.Type == "Char" && p.Cur.Value == "(" {
			return p.parseCall(vname)
		}
		a.Node = Ident{Type: "", Name: vname}
		return
	}

	p.addErrorln(p.Cur.Message)
	a.Node = nil
	return
}

func (p *Parser) nextToken() {
	p.Prev = p.Cur
	p.Cur = p.Lexer.NextToken()
	p.Line = p.Lexer.Line
	p.Col = p.Lexer.Col
}

func (p *Parser) addError(msg string) {
	p.Error = append(p.Error, p.printError(msg))
}

func (p *Parser) addErrorln(msg string) {
	p.Error = append(p.Error, fmt.Sprintf("at line %d, col %d: %s", p.Line, p.Col, msg))
}

func (p *Parser) printError(msg string) string {
	return fmt.Sprintf("error at line %d, col %d: Expected %s, but got \"%s\"", p.Line, p.Col, msg, p.Cur.Value)
}

func (p *Parser) sync() {
	for p.Cur.Type != "EOF" {
		if p.Cur.Type == "NewLine" {
			p.nextToken()
			return
		}
		p.nextToken()
	}
}
