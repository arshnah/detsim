package rewrite

import (
	"go/ast"

	"golang.org/x/tools/go/ast/astutil"
)

var osFuncToFileSystemMethod = map[string]string{
	"Open":      "Open",
	"Create":    "Create",
	"Remove":    "Remove",
	"Rename":    "Rename",
	"Stat":      "Stat",
	"ReadFile":  "ReadFile",
	"WriteFile": "WriteFile",
}

var osFuncMaxArgs = map[string]int{
	"WriteFile": 2,
}

var osTypeToRT = map[string]string{
	"FileInfo": "FileInfo",
}

var osFuncsSafeUnrewritten = map[string]bool{
	"IsNotExist":   true,
	"IsExist":      true,
	"IsPermission": true,
	"IsTimeout":    true,
}

func rewriteOSFileCalls(ctx *fileContext) {
	if !fileImportsPackage(ctx.file, osPkgPath) {
		return
	}
	astutil.Apply(ctx.file, nil, func(cur *astutil.Cursor) bool {
		if rewriteOSFileType(ctx, cur) {
			return true
		}

		call, ok := cur.Node().(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok || ctx.importPathOf(pkgIdent) != osPkgPath {
			return true
		}

		method, known := osFuncToFileSystemMethod[sel.Sel.Name]
		if !known {
			if !osFuncsSafeUnrewritten[sel.Sel.Name] && !ctx.isConversion(call) {
				ctx.warn(call.Pos(), "unsupported os.%s call, leaving unrewritten", sel.Sel.Name)
			}
			return true
		}

		args := call.Args
		if max, capped := osFuncMaxArgs[sel.Sel.Name]; capped && len(args) > max {
			args = args[:max]
		}
		cur.Replace(callExpr(&ast.SelectorExpr{X: fsIdent(), Sel: ast.NewIdent(method)}, args...))
		return true
	})
}

func rewriteOSFileType(ctx *fileContext, cur *astutil.Cursor) bool {
	if star, ok := cur.Node().(*ast.StarExpr); ok {
		sel, ok := star.X.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok || ctx.importPathOf(pkgIdent) != osPkgPath || sel.Sel.Name != "File" {
			return false
		}
		cur.Replace(&ast.StarExpr{X: rtSelector("File")})
		return true
	}

	sel, ok := cur.Node().(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok || ctx.importPathOf(pkgIdent) != osPkgPath {
		return false
	}
	rtName, known := osTypeToRT[sel.Sel.Name]
	if !known {
		return false
	}
	cur.Replace(rtSelector(rtName))
	return true
}

func fsIdent() *ast.Ident {
	return ast.NewIdent(fsVarName)
}
