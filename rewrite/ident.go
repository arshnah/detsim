package rewrite

import (
	"go/ast"
	"go/token"
)

const intLit = token.INT

func schedIdent() *ast.Ident {
	return ast.NewIdent(schedVarName)
}

func rtSelector(name string) *ast.SelectorExpr {
	return &ast.SelectorExpr{
		X:   ast.NewIdent("rt"),
		Sel: ast.NewIdent(name),
	}
}

func rtChanType(elem ast.Expr) ast.Expr {
	return &ast.StarExpr{
		X: &ast.IndexExpr{
			X:     rtSelector("Chan"),
			Index: elem,
		},
	}
}

func callExpr(fn ast.Expr, args ...ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{Fun: fn, Args: args}
}

func methodCall(recv ast.Expr, method string, args ...ast.Expr) *ast.CallExpr {
	return callExpr(&ast.SelectorExpr{X: recv, Sel: ast.NewIdent(method)}, args...)
}
