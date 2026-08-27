package rewrite

import (
	"fmt"
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/ast/astutil"
)

func rewriteGoStatements(ctx *fileContext) {
	astutil.Apply(ctx.file, nil, func(cur *astutil.Cursor) bool {
		goStmt, ok := cur.Node().(*ast.GoStmt)
		if !ok {
			return true
		}
		cur.Replace(rewriteGoStmt(goStmt))
		return true
	})
}

func rewriteGoStmt(goStmt *ast.GoStmt) ast.Stmt {
	call := goStmt.Call

	lhs := []ast.Expr{ast.NewIdent("__fn")}
	rhs := []ast.Expr{call.Fun}
	var innerArgs []ast.Expr
	for i, arg := range call.Args {
		name := fmt.Sprintf("__a%d", i)
		lhs = append(lhs, ast.NewIdent(name))
		rhs = append(rhs, arg)
		innerArgs = append(innerArgs, ast.NewIdent(name))
	}

	capture := &ast.AssignStmt{Lhs: lhs, Tok: token.DEFINE, Rhs: rhs}

	innerCall := &ast.CallExpr{
		Fun:      ast.NewIdent("__fn"),
		Args:     innerArgs,
		Ellipsis: call.Ellipsis,
	}

	spawn := &ast.ExprStmt{
		X: methodCall(schedIdent(), "Go", &ast.FuncLit{
			Type: &ast.FuncType{Params: &ast.FieldList{}},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: innerCall}}},
		}),
	}

	iife := &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{Params: &ast.FieldList{}},
			Body: &ast.BlockStmt{List: []ast.Stmt{capture, spawn}},
		},
	}
	return &ast.ExprStmt{X: iife}
}
