package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"shiroko/core/ast"
	"shiroko/core/parser"
)

func Load(entry, stdlibRoot string) (*ast.Program, []string) {
	var errs []string
	seen := map[string]bool{}
	pkgs := map[string]*ast.Package{}
	var order []string

	var load func(path, expectName string)
	load = func(path, expectName string) {
		if expectName != "" && seen[expectName] {
			return
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, err.Error())
			return
		}
		prog, perrs := parser.Parse(string(raw))
		if len(perrs) > 0 {
			errs = append(errs, perrs...)
			return
		}
		for name, p := range prog.Packages {
			if expectName != "" && name != expectName {
				errs = append(errs, fmt.Sprintf(
					"%s: package clause is %q, expected %q", path, name, expectName))
				return
			}
			if seen[name] {
				return
			}
			seen[name] = true
			pkgs[name] = p
			order = append(order, name)

			for _, im := range p.Imports {
				for _, mod := range im.Modules {
					child := resolveImport(stdlibRoot, mod)
					if child == "" {
						continue
					}
					load(child, lastSeg(mod))
				}
			}
		}
	}

	entryName := lastSeg(strings.TrimSuffix(filepath.Base(entry), ".shrko"))
	load(entry, "")

	// Collect imports from every loaded package (in load order).
	var allImports []ast.ImportStmt
	for _, name := range order {
		allImports = append(allImports, pkgs[name].Imports...)
	}

	return &ast.Program{
		Packages: pkgs,
		Entry:    entryName,
		Prog:     flatten(pkgs, order),
		Imports:  allImports,
	}, errs
}

func resolveImport(root, path string) string {
	for _, base := range []string{root, filepath.Join(root, "..")} {
		full := filepath.Join(base, filepath.FromSlash(path)+".shrko")
		if _, err := os.Stat(full); err == nil {
			return full
		}
	}
	return ""
}

func lastSeg(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func flatten(pkgs map[string]*ast.Package, order []string) []ast.Stmt {
	prefixes := map[string]string{}
	for name := range pkgs {
		if name == "main" {
			continue
		}
		prefixes[name] = name + "_"
	}

	var out []ast.Stmt
	for _, name := range order {
		p := pkgs[name]
		prefix := ""
		if name != "main" {
			prefix = name + "_"
		}
		for _, node := range p.Prog {
			prefixToplevel(node, prefix)
			walkRewrite(node, prefixes)
			out = append(out, node)
		}
	}
	return out
}

func prefixToplevel(node ast.Stmt, prefix string) {
	if prefix == "" {
		return
	}
	switch n := node.(type) {
		case *ast.FuncDeclStmt:
			n.Name = prefix + n.Name
		case *ast.StructDeclStmt:
			n.Name = prefix + n.Name
		case *ast.InterfaceDeclStmt:
			n.Name = prefix + n.Name
		case *ast.StructFuncDeclStmt:
			n.Struct = prefix + n.Struct
	}
}

func walkRewrite(node any, prefixes map[string]string) {
	switch n := node.(type) {
		case nil, *ast.Ident, *ast.Bool, *ast.Number, *ast.String:
		case *ast.SelectorExpr:
			walkRewrite(n.X, prefixes)
			if id, ok := n.X.(*ast.Ident); ok {
				if pre, ok := prefixes[id.Name]; ok {
					n.X = &ast.Ident{Name: pre + n.Sel, Pos: n.Pos}
					n.Sel = ""
				}
			}
		case *ast.CallExpr:
			walkRewrite(n.Func, prefixes)
			for _, a := range n.Args {
				walkRewrite(a, prefixes)
			}
		case *ast.BinaryExpr:
			walkRewrite(n.Left, prefixes)
			walkRewrite(n.Right, prefixes)
		case *ast.UnaryExpr:
			walkRewrite(n.Right, prefixes)
		case *ast.IndexExpr:
			walkRewrite(n.X, prefixes)
			walkRewrite(n.Index, prefixes)
		case *ast.BoundsExpr:
			walkRewrite(n.Start, prefixes)
			walkRewrite(n.End, prefixes)
		case *ast.SetCompExpr:
			walkRewrite(n.Cond, prefixes)
			walkRewrite(n.Bound, prefixes)
		case *ast.ListLit:
			for _, v := range n.Values {
				walkRewrite(v, prefixes)
			}
		case *ast.MapLit:
			for _, p := range n.Pairs {
				walkRewrite(p.Key, prefixes)
				walkRewrite(p.Value, prefixes)
			}
		case *ast.StructLit:
			for _, f := range n.Fields {
				walkRewrite(f.Value, prefixes)
			}
		case *ast.FuncLit:
			for _, s := range n.Body {
				walkRewrite(s, prefixes)
			}
		case *ast.FuncDeclStmt:
			for _, s := range n.Body {
				walkRewrite(s, prefixes)
			}
		case *ast.StructFuncDeclStmt:
			for _, s := range n.Body {
				walkRewrite(s, prefixes)
			}
		case *ast.StructDeclStmt:
			for _, f := range n.Fields {
				walkRewrite(f, prefixes)
			}
		case *ast.ConstDeclStmt:
			walkRewrite(n.Init, prefixes)
		case *ast.AssignStmt:
			walkRewrite(n.Expr, prefixes)
		case *ast.ExprStmt:
			walkRewrite(n.Expr, prefixes)
		case *ast.ReturnStmt:
			walkRewrite(n.Expr, prefixes)
		case *ast.IfStmt:
			walkRewrite(n.Cond, prefixes)
			for _, s := range n.Body {
				walkRewrite(s, prefixes)
			}
			walkRewrite(n.Next, prefixes)
		case *ast.ElseStmt:
			for _, s := range n.Body {
				walkRewrite(s, prefixes)
			}
		case *ast.ForeverStmt:
			for _, s := range n.Body {
				walkRewrite(s, prefixes)
			}
		case *ast.ForCondStmt:
			walkRewrite(n.Cond, prefixes)
			for _, s := range n.Body {
				walkRewrite(s, prefixes)
			}
		case *ast.ForRangeStmt:
			walkRewrite(n.Init, prefixes)
			walkRewrite(n.End, prefixes)
			walkRewrite(n.Step, prefixes)
			for _, s := range n.Body {
				walkRewrite(s, prefixes)
			}
		case *ast.ForRangeRefStmt:
			walkRewrite(n.Init, prefixes)
			walkRewrite(n.End, prefixes)
			walkRewrite(n.Step, prefixes)
			for _, s := range n.Body {
				walkRewrite(s, prefixes)
			}
		case *ast.ForIterStmt:
			for _, s := range n.Body {
				walkRewrite(s, prefixes)
			}
		case *ast.ForIterRefStmt:
			for _, s := range n.Body {
				walkRewrite(s, prefixes)
			}
		case *ast.MatchStmt:
			walkRewrite(n.Cond, prefixes)
			for _, arm := range n.Arms {
				walkRewrite(arm.Pattern, prefixes)
				for _, s := range arm.Body {
					walkRewrite(s, prefixes)
				}
			}
			case *ast.InterfaceDeclStmt, *ast.ImportStmt, *ast.VarDeclStmt,
			*ast.BreakStmt, *ast.ContinueStmt:
	}
}
