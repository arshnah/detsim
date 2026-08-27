package rewrite

import (
	"fmt"
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/ast/astutil"
)

// hoistChanTemp evaluates expr once, into a fresh temporary declared at the front of
// stmts, and returns the temporary's identifier. Names are unique per file so two
// selects in the same block can't redeclare.
func (c *fileContext) hoistChanTemp(expr ast.Expr, stmts *[]ast.Stmt) ast.Expr {
	name := fmt.Sprintf("__detsimCh%d", c.tmpCount)
	c.tmpCount++
	*stmts = append(*stmts, &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(name)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{expr},
	})
	return ast.NewIdent(name)
}

func fileHasUnsupportedSelect(ctx *fileContext) (reason string, unsupported bool) {
	found := ""
	ast.Inspect(ctx.file, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		sel, ok := n.(*ast.SelectStmt)
		if !ok {
			return true
		}
		for _, stmt := range sel.Body.List {
			comm := stmt.(*ast.CommClause)
			if caseBodyHasEscapingControlFlow(comm.Body) {
				found = fmt.Sprintf("select at %s has a case with a return/break/continue/goto", ctx.fset.Position(comm.Pos()))
				return false
			}
			if _, ok := buildSelectCase(comm); !ok {
				found = fmt.Sprintf("select at %s has an unsupported case form", ctx.fset.Position(comm.Pos()))
				return false
			}
		}
		return true
	})
	return found, found != ""
}

func rewriteSelectStatements(ctx *fileContext) {
	astutil.Apply(ctx.file, nil, func(cur *astutil.Cursor) bool {
		sel, ok := cur.Node().(*ast.SelectStmt)
		if !ok {
			return true
		}

		for _, stmt := range sel.Body.List {
			comm := stmt.(*ast.CommClause)

			if caseBodyHasEscapingControlFlow(comm.Body) {
				ctx.warn(comm.Pos(), "select case contains a return/break/continue/goto that would change meaning once moved into a closure, leaving the whole select statement unrewritten")
				return true
			}

			if _, ok := buildSelectCase(comm); !ok {
				ctx.warn(comm.Pos(), "unsupported select case form, leaving the whole select statement unrewritten")
				return true
			}
		}

		cur.Replace(rewriteSelectStmt(ctx, sel))
		return true
	})
}

// rewriteSelectStmt lowers one select statement into a block that first evaluates every
// case's channel expression exactly once, in source order, into a temporary, then hands
// the temporaries to Sched.Select. Without the hoist a case like `case <-time.After(d)`
// would evaluate its channel expression twice, once for readiness and once for commit,
// and the two evaluations can disagree.
func rewriteSelectStmt(ctx *fileContext, sel *ast.SelectStmt) ast.Stmt {
	var pre []ast.Stmt
	var caseExprs []ast.Expr

	for _, stmt := range sel.Body.List {
		comm := stmt.(*ast.CommClause)

		switch c := comm.Comm.(type) {
		case nil:
			caseExprs = append(caseExprs, defaultCaseExpr(comm.Body))
		case *ast.SendStmt:
			ch := ctx.hoistChanTemp(c.Chan, &pre)
			caseExprs = append(caseExprs, sendCaseExpr(ch, c.Value, comm.Body))
		case *ast.ExprStmt:
			recv, _ := isRecvExpr(c.X)
			ch := ctx.hoistChanTemp(recv.X, &pre)
			caseExprs = append(caseExprs, recvCaseExpr(ch, nil, comm.Body))
		case *ast.AssignStmt:
			recv, _ := isRecvExpr(c.Rhs[0])
			ch := ctx.hoistChanTemp(recv.X, &pre)
			caseExprs = append(caseExprs, recvCaseExpr(ch, c, comm.Body))
		}
	}

	pre = append(pre, &ast.ExprStmt{X: methodCall(schedIdent(), "Select", caseExprs...)})
	return &ast.BlockStmt{List: pre}
}

func buildSelectCase(comm *ast.CommClause) (ast.Expr, bool) {
	if comm.Comm == nil {
		return defaultCaseExpr(comm.Body), true
	}

	switch c := comm.Comm.(type) {
	case *ast.SendStmt:
		return sendCaseExpr(c.Chan, c.Value, comm.Body), true

	case *ast.ExprStmt:
		recv, ok := isRecvExpr(c.X)
		if !ok {
			return nil, false
		}
		return recvCaseExpr(recv.X, nil, comm.Body), true

	case *ast.AssignStmt:
		if len(c.Rhs) != 1 {
			return nil, false
		}
		recv, ok := isRecvExpr(c.Rhs[0])
		if !ok {
			return nil, false
		}
		return recvCaseExpr(recv.X, c, comm.Body), true
	}
	return nil, false
}

func recvCaseExpr(chanExpr ast.Expr, assign *ast.AssignStmt, body []ast.Stmt) ast.Expr {
	var recvStmt ast.Stmt
	switch {
	case assign == nil:
		recvStmt = &ast.ExprStmt{X: methodCall(chanExpr, "Recv")}
	case len(assign.Lhs) == 2:
		recvStmt = &ast.AssignStmt{Lhs: assign.Lhs, Tok: assign.Tok, Rhs: []ast.Expr{methodCall(chanExpr, "RecvOK")}}
	default:
		recvStmt = &ast.AssignStmt{Lhs: assign.Lhs, Tok: assign.Tok, Rhs: []ast.Expr{methodCall(chanExpr, "Recv")}}
	}

	fullBody := append([]ast.Stmt{recvStmt}, body...)
	return callExpr(rtSelector("RecvCase"), chanExpr, emptyFuncLit(fullBody))
}

func sendCaseExpr(ch ast.Expr, value ast.Expr, body []ast.Stmt) ast.Expr {
	sendStmt := &ast.ExprStmt{X: methodCall(ch, "Send", value)}
	fullBody := append([]ast.Stmt{sendStmt}, body...)
	return callExpr(rtSelector("SendCase"), ch, emptyFuncLit(fullBody))
}

func defaultCaseExpr(body []ast.Stmt) ast.Expr {
	return callExpr(rtSelector("DefaultCase"), emptyFuncLit(body))
}

func emptyFuncLit(body []ast.Stmt) *ast.FuncLit {
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: body},
	}
}

func caseBodyHasEscapingControlFlow(body []ast.Stmt) bool {
	found := false
	for _, stmt := range body {
		ast.Inspect(stmt, func(n ast.Node) bool {
			if found {
				return false
			}
			switch v := n.(type) {
			case *ast.FuncLit:
				return false
			case *ast.ReturnStmt:
				found = true
			case *ast.BranchStmt:
				if v.Tok == token.BREAK || v.Tok == token.CONTINUE || v.Tok == token.GOTO {
					found = true
				}
			}
			return true
		})
	}
	return found
}
