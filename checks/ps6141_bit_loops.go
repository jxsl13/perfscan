package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/analysis"
)

// A source-derived ranking proof, not a decoder-name exception. A nonzero
// unsigned word initially masked below one target bit reaches that bit after
// at most log2(target) single-bit shifts, without word overflow.
func ps6141FiniteBitLoops(pass *analysis.Pass, decl *ast.FuncDecl) map[*ast.ForStmt]bool {
	result := map[*ast.ForStmt]bool{}
	number := func(e ast.Expr) int64 {
		v := pass.TypesInfo.Types[e].Value
		if v == nil || v.Kind() != constant.Int {
			return -1
		}
		n, ok := constant.Int64Val(v)
		if !ok {
			return -1
		}
		return n
	}
	same := func(e ast.Expr, o types.Object) bool {
		id, ok := ps2110Unparen(e).(*ast.Ident)
		return ok && identObject(pass, id) == o
	}
	check := func(list []ast.Stmt) {
		for at, stmt := range list {
			loop, ok := stmt.(*ast.ForStmt)
			if !ok || loop.Init != nil || loop.Post != nil || len(loop.Body.List) != 2 {
				continue
			}
			cond, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
			if !ok || cond.Op != token.EQL || number(cond.Y) != 0 {
				continue
			}
			test, ok := ps2110Unparen(cond.X).(*ast.BinaryExpr)
			if !ok || test.Op != token.AND {
				continue
			}
			id, ok := test.X.(*ast.Ident)
			if !ok {
				continue
			}
			word := pass.TypesInfo.Uses[id]
			basic, ok := types.Unalias(word.Type()).Underlying().(*types.Basic)
			target := number(test.Y)
			if !ok || basic.Info()&types.IsUnsigned == 0 || target < 2 || target > 1<<20 || target&(target-1) != 0 || pass.TypesSizes == nil {
				continue
			}
			width := pass.TypesSizes.Sizeof(word.Type()) * 8
			if width < 1 || width > 64 || constant.Compare(constant.MakeInt64(target), token.GEQ, constant.Shift(constant.MakeInt64(1), token.SHL, uint(width))) {
				continue
			}
			shift, ok := loop.Body.List[0].(*ast.AssignStmt)
			if !ok || shift.Tok != token.SHL_ASSIGN || len(shift.Lhs) != 1 || len(shift.Rhs) != 1 || !same(shift.Lhs[0], word) || number(shift.Rhs[0]) != 1 {
				continue
			}
			decrement, ok := loop.Body.List[1].(*ast.IncDecStmt)
			if !ok || decrement.Tok != token.DEC || same(decrement.X, word) {
				continue
			}
			initialized, unchanged, guarded := false, true, false
			ast.Inspect(decl.Body, func(n ast.Node) bool {
				if increment, ok := n.(*ast.IncDecStmt); ok && increment.Pos() < loop.Pos() && same(increment.X, word) {
					unchanged = false
				}
				assignment, ok := n.(*ast.AssignStmt)
				if !ok || assignment.Pos() >= loop.Pos() {
					return true
				}
				for _, lhs := range assignment.Lhs {
					if !same(lhs, word) {
						continue
					}
					if assignment.Tok != token.DEFINE || len(assignment.Rhs) != 1 {
						unchanged = false
						continue
					}
					mask, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.BinaryExpr)
					if ok && mask.Op == token.AND && number(mask.Y) == target-1 {
						initialized = true
					} else {
						unchanged = false
					}
				}
				return true
			})
			for _, prefix := range list[:at] {
				guard, ok := prefix.(*ast.IfStmt)
				if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 {
					continue
				}
				comparison, ok := ps2110Unparen(guard.Cond).(*ast.BinaryExpr)
				if !ok || comparison.Op != token.EQL || !same(comparison.X, word) || number(comparison.Y) != 0 {
					continue
				}
				if _, ok := guard.Body.List[0].(*ast.ReturnStmt); ok {
					guarded = true
				}
			}
			if initialized && unchanged && guarded {
				result[loop] = true
			}
		}
	}
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BlockStmt:
			check(node.List)
		case *ast.CaseClause:
			check(node.Body)
		}
		return true
	})
	return result
}
