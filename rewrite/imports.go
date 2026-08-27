package rewrite

import (
	"go/ast"
	"path/filepath"
	"strconv"

	"golang.org/x/tools/go/ast/astutil"
)

func fileImportsPackage(file *ast.File, path string) bool {
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err == nil && p == path {
			return true
		}
	}
	return false
}

func identUsedAsSelectorBase(file *ast.File, name string) bool {
	used := false
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == name {
			used = true
		}
		return true
	})
	return used
}

// finalizeImports drops stdlib imports whose every use was rewritten away and adds the
// rt import if rewritten code needs it. Alias-aware: an aliased import is pruned only
// when its own local name has no remaining selector-base uses.
func finalizeImports(ctx *fileContext) {
	// Collect before deleting: deleting mutates ctx.file.Imports in place, so deleting
	// while ranging over it corrupts the iteration.
	type prune struct{ name, path string }
	var prunes []prune
	for _, imp := range ctx.file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		switch path {
		case syncPkgPath, timePkgPath, randPkgPath, osPkgPath, netPkgPath:
		default:
			continue
		}

		name := filepath.Base(path)
		if imp.Name != nil {
			name = imp.Name.Name
			if name == "_" || name == "." {
				continue
			}
		}
		if identUsedAsSelectorBase(ctx.file, name) {
			continue
		}
		importName := ""
		if imp.Name != nil {
			importName = imp.Name.Name
		}
		prunes = append(prunes, prune{name: importName, path: path})
	}
	for _, p := range prunes {
		astutil.DeleteNamedImport(ctx.fset, ctx.file, p.name, p.path)
	}

	if identUsedAsSelectorBase(ctx.file, "rt") {
		astutil.AddImport(ctx.fset, ctx.file, rtImportPath)
	}
}
