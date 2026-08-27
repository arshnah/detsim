package rewrite

import (
	"go/ast"

	"golang.org/x/tools/go/ast/astutil"
)

var timeFuncToSchedMethod = map[string]string{
	"Sleep": "Sleep",
	"Now":   "Now",
	"After": "After",
}

func rewriteTimeCalls(ctx *fileContext) {
	if !fileImportsPackage(ctx.file, timePkgPath) {
		return
	}
	astutil.Apply(ctx.file, nil, func(cur *astutil.Cursor) bool {
		call, ok := cur.Node().(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok || ctx.importPathOf(pkgIdent) != timePkgPath {
			return true
		}
		method, known := timeFuncToSchedMethod[sel.Sel.Name]
		if !known {
			if !ctx.isConversion(call) {
				ctx.warn(call.Pos(), "unsupported time.%s call, leaving unrewritten", sel.Sel.Name)
			}
			return true
		}
		cur.Replace(callExpr(&ast.SelectorExpr{X: schedIdent(), Sel: ast.NewIdent(method)}, call.Args...))
		return true
	})
}
