package rewrite

import (
	"go/ast"

	"golang.org/x/tools/go/ast/astutil"
)

func rewriteChanClose(ctx *fileContext) {
	astutil.Apply(ctx.file, nil, func(cur *astutil.Cursor) bool {
		call, ok := cur.Node().(*ast.CallExpr)
		if !ok || !isBuiltinCall(ctx, call, "close") || len(call.Args) != 1 {
			return true
		}
		cur.Replace(methodCall(call.Args[0], "Close"))
		return true
	})
}
