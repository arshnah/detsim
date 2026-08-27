package rewrite

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/ast/astutil"
)

func rewriteChanSend(ctx *fileContext) {
	astutil.Apply(ctx.file, nil, func(cur *astutil.Cursor) bool {
		send, ok := cur.Node().(*ast.SendStmt)
		if !ok {
			return true
		}
		cur.Replace(&ast.ExprStmt{X: methodCall(send.Chan, "Send", send.Value)})
		return true
	})
}

func isRecvExpr(e ast.Expr) (*ast.UnaryExpr, bool) {
	u, ok := e.(*ast.UnaryExpr)
	if !ok || u.Op != token.ARROW {
		return nil, false
	}
	return u, true
}

func rewriteChanRecvOKAssign(ctx *fileContext) {
	astutil.Apply(ctx.file, nil, func(cur *astutil.Cursor) bool {
		assign, ok := cur.Node().(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 2 || len(assign.Rhs) != 1 {
			return true
		}
		recv, ok := isRecvExpr(assign.Rhs[0])
		if !ok {
			return true
		}
		cur.Replace(&ast.AssignStmt{
			Lhs: assign.Lhs,
			Tok: assign.Tok,
			Rhs: []ast.Expr{methodCall(recv.X, "RecvOK")},
		})
		return true
	})
}

func rewriteChanRecvExpr(ctx *fileContext) {
	astutil.Apply(ctx.file, nil, func(cur *astutil.Cursor) bool {
		unary, ok := cur.Node().(*ast.UnaryExpr)
		if !ok {
			return true
		}
		recv, ok := isRecvExpr(unary)
		if !ok {
			return true
		}
		cur.Replace(methodCall(recv.X, "Recv"))
		return true
	})
}
