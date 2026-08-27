package rewrite

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

const (
	rtImportPath = "github.com/arshnah/detsim/rt"
	schedVarName = "detsimSched"
	fsVarName    = "detsimFS"
	netVarName   = "detsimNet"
	syncPkgPath  = "sync"
	timePkgPath  = "time"
	randPkgPath  = "math/rand"
	osPkgPath    = "os"
	netPkgPath   = "net"
)

// Warning is a non-fatal note about a construct Rewrite left unrewritten.
type Warning struct {
	Pos     token.Position
	Message string
}

type fileContext struct {
	pkg   *packages.Package
	fset  *token.FileSet
	file  *ast.File
	info  *types.Info
	warns *[]Warning

	tmpCount int
}

func (c *fileContext) warn(pos token.Pos, format string, args ...any) {
	*c.warns = append(*c.warns, Warning{
		Pos:     c.fset.Position(pos),
		Message: fmt.Sprintf(format, args...),
	})
}

func (c *fileContext) typeOf(e ast.Expr) types.Type {
	if t, ok := c.info.Types[e]; ok {
		return t.Type
	}
	return nil
}

// importPathOf resolves an identifier that names a package (possibly through an alias)
// to its import path, or "" if the identifier isn't a package name.
func importPathOf(info *types.Info, id *ast.Ident) string {
	if info == nil || id == nil {
		return ""
	}
	pn, ok := info.Uses[id].(*types.PkgName)
	if !ok {
		return ""
	}
	return pn.Imported().Path()
}

func (c *fileContext) importPathOf(id *ast.Ident) string {
	return importPathOf(c.info, id)
}

// isConversion reports whether call is a type conversion (e.g. time.Duration(x))
// rather than a function call, so passes leave it untouched.
func (c *fileContext) isConversion(call *ast.CallExpr) bool {
	if tv, ok := c.info.Types[call.Fun]; ok {
		return tv.IsType()
	}
	return false
}

// isBuiltinCall reports whether call is a call of the predeclared builtin named name.
// Checking types.Info rather than just the spelling keeps a user variable shadowing the
// builtin from being rewritten as if it were the builtin.
func isBuiltinCall(ctx *fileContext, call *ast.CallExpr, name string) bool {
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Name != name {
		return false
	}
	if ctx.info == nil {
		return true
	}
	_, isBuiltin := ctx.info.Uses[id].(*types.Builtin)
	return isBuiltin
}
