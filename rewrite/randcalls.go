package rewrite

import (
	"go/ast"

	"golang.org/x/tools/go/ast/astutil"
)

var randFuncToRandMethod = map[string]string{
	"Int":     "Int",
	"Intn":    "Intn",
	"Int31":   "Int31",
	"Int31n":  "Int31n",
	"Int63":   "Int63",
	"Int63n":  "Int63n",
	"Float64": "Float64",
	"Float32": "Float32",
	"Perm":    "Perm",
	"Shuffle": "Shuffle",
}

func rewriteRandCalls(ctx *fileContext) {
	if !fileImportsPackage(ctx.file, randPkgPath) {
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
		if !ok || ctx.importPathOf(pkgIdent) != randPkgPath {
			return true
		}

		if sel.Sel.Name == "Seed" {
			ctx.warn(call.Pos(), "rand.Seed is a no-op under detsim/rt, the scheduler seed controls all randomness")
			cur.Replace(callExpr(&ast.FuncLit{
				Type: &ast.FuncType{Params: &ast.FieldList{}},
				Body: &ast.BlockStmt{},
			}))
			return true
		}

		method, known := randFuncToRandMethod[sel.Sel.Name]
		if !known {
			if !ctx.isConversion(call) {
				ctx.warn(call.Pos(), "unsupported rand.%s call, leaving unrewritten", sel.Sel.Name)
			}
			return true
		}

		randField := &ast.SelectorExpr{X: schedIdent(), Sel: ast.NewIdent("Rand")}
		cur.Replace(callExpr(&ast.SelectorExpr{X: randField, Sel: ast.NewIdent(method)}, call.Args...))
		return true
	})
}
