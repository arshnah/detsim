package rewrite

import (
	"go/ast"

	"golang.org/x/tools/go/ast/astutil"
)

type netTypeMapping struct {
	rtName    string
	pointerRT bool
}

var netTypeToRT = map[string]netTypeMapping{
	"Conn":     {rtName: "Conn", pointerRT: true},
	"Listener": {rtName: "Listener", pointerRT: true},
	"Addr":     {rtName: "Addr", pointerRT: true},
}

func rewriteNetCalls(ctx *fileContext) {
	if !fileImportsPackage(ctx.file, netPkgPath) {
		return
	}
	astutil.Apply(ctx.file, nil, func(cur *astutil.Cursor) bool {
		if rewriteNetType(ctx, cur) {
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
		if !ok || ctx.importPathOf(pkgIdent) != netPkgPath {
			return true
		}

		switch sel.Sel.Name {
		case "Listen":
			cur.Replace(callExpr(&ast.SelectorExpr{X: netIdent(), Sel: ast.NewIdent("Listen")}, lastArg(call.Args)))
		case "Dial":
			cur.Replace(callExpr(&ast.SelectorExpr{X: netIdent(), Sel: ast.NewIdent("Dial")}, lastArg(call.Args)))
		default:
			if !ctx.isConversion(call) {
				ctx.warn(call.Pos(), "unsupported net.%s call, leaving unrewritten", sel.Sel.Name)
			}
		}
		return true
	})
}

func lastArg(args []ast.Expr) ast.Expr {
	return args[len(args)-1]
}

func rewriteNetType(ctx *fileContext, cur *astutil.Cursor) bool {
	sel, ok := cur.Node().(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok || ctx.importPathOf(pkgIdent) != netPkgPath {
		return false
	}
	mapping, known := netTypeToRT[sel.Sel.Name]
	if !known {
		return false
	}

	if mapping.pointerRT {
		cur.Replace(&ast.StarExpr{X: rtSelector(mapping.rtName)})
	} else {
		cur.Replace(rtSelector(mapping.rtName))
	}
	return true
}

func netIdent() *ast.Ident {
	return ast.NewIdent(netVarName)
}
