package rewrite

import (
	"go/ast"

	"golang.org/x/tools/go/ast/astutil"
)

func rewriteMakeChan(ctx *fileContext) {
	astutil.Apply(ctx.file, nil, func(cur *astutil.Cursor) bool {
		call, ok := cur.Node().(*ast.CallExpr)
		if !ok {
			return true
		}
		if !isBuiltinCall(ctx, call, "make") || len(call.Args) == 0 {
			return true
		}
		chanType, ok := call.Args[0].(*ast.ChanType)
		if !ok {
			return true
		}

		capExpr := ast.Expr(&ast.BasicLit{Kind: intLit, Value: "0"})
		if len(call.Args) > 1 {
			capExpr = call.Args[1]
		}

		newCall := callExpr(
			&ast.IndexExpr{X: rtSelector("NewChan"), Index: chanType.Value},
			schedIdent(),
			capExpr,
		)
		cur.Replace(newCall)
		return true
	})
}
