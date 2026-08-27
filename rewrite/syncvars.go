package rewrite

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ast/astutil"
)

var syncTypeConstructors = map[string]string{
	"Mutex":     "NewMutex",
	"RWMutex":   "NewRWMutex",
	"WaitGroup": "NewWaitGroup",
	"Once":      "NewOnce",
}

func rewriteSyncLocals(ctx *fileContext) {
	if !fileImportsPackage(ctx.file, syncPkgPath) {
		return
	}
	astutil.Apply(ctx.file, nil, func(cur *astutil.Cursor) bool {
		declStmt, ok := cur.Node().(*ast.DeclStmt)
		if !ok {
			return true
		}
		genDecl, ok := declStmt.Decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR || len(genDecl.Specs) != 1 {
			return true
		}
		vspec, ok := genDecl.Specs[0].(*ast.ValueSpec)
		if !ok || len(vspec.Names) != 1 || len(vspec.Values) != 0 {
			return true
		}

		ctor, ok := syncConstructorFor(ctx.info, vspec.Type)
		if !ok {
			return true
		}

		cur.Replace(&ast.AssignStmt{
			Lhs: []ast.Expr{vspec.Names[0]},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{callExpr(rtSelector(ctor), schedIdent())},
		})
		return true
	})
}

func syncConstructorFor(info *types.Info, typeExpr ast.Expr) (ctor string, ok bool) {
	sel, ok := typeExpr.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok || importPathOf(info, pkgIdent) != syncPkgPath {
		return "", false
	}
	ctor, known := syncTypeConstructors[sel.Sel.Name]
	return ctor, known
}
