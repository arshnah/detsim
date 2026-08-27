package rewrite

import (
	"go/ast"

	"golang.org/x/tools/go/ast/astutil"
)

func rewriteChanTypes(ctx *fileContext) {
	astutil.Apply(ctx.file, nil, func(cur *astutil.Cursor) bool {
		chanType, ok := cur.Node().(*ast.ChanType)
		if !ok {
			return true
		}
		cur.Replace(rtChanType(chanType.Value))
		return true
	})
}
