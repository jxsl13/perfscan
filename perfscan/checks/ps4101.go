package checks

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS4101 reports element-copy loops replaceable by the copy builtin.
var PS4101 = register(&lint.Check{
	ID:       "PS4101",
	Category: "vector",
	Slug:     "loop-copy",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "an element-copy loop replaceable by the copy builtin",
		Text: `for i := range src { dst[i] = src[i] } performs one
bounds-checked load/store pair per element; the copy builtin is a single
memmove. The rewrite is mechanical.

One semantic caveat the fix inherits from copy itself: for overlapping
slices copy behaves like memmove, while a forward loop where dst starts
after src inside the SAME array propagates elements. Such aliased
self-copies are exceedingly rare and intentional ones deserve a comment;
review the fix where dst and src could share a backing array.`,
		Before: `for i := range src {
	dst[i] = src[i]
}`,
		After: `copy(dst, src)`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS4101",
		Doc:  "element-copy loop replaceable by copy()",
		Run:  runPS4101,
	},
})

func runPS4101(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			dst, src, ok := copyLoopShape(n)
			if !ok {
				return true
			}
			// Both sides must be slices (not maps/strings), same element
			// type.
			dt := pass.TypesInfo.TypeOf(dstExprOf(n))
			st := pass.TypesInfo.TypeOf(srcExprOf(n))
			ds, ok1 := underlyingSlice(dt)
			ss, ok2 := underlyingSlice(st)
			if !ok1 || !ok2 || !types.Identical(ds.Elem(), ss.Elem()) {
				return true
			}
			loop := n.(ast.Stmt)
			pass.Report(analysis.Diagnostic{
				Pos:     loop.Pos(),
				End:     loop.End(),
				Message: fmt.Sprintf("element-copy loop from %s to %s is a single memmove; replace with copy(%s, %s)", src, dst, dst, src),
				SuggestedFixes: []analysis.SuggestedFix{{
					Message: fmt.Sprintf("replace loop with copy(%s, %s)", dst, src),
					TextEdits: []analysis.TextEdit{
						{Pos: loop.Pos(), End: loop.End(), NewText: fmt.Appendf(nil, "copy(%s, %s)", dst, src)},
					},
				}},
			})
			return true
		})
	}
	return nil, nil
}

// copyLoopShape matches
//
//	for i := range src { dst[i] = src[i] }
//	for i, v := range src { dst[i] = v }
//	for i := 0; i < len(src); i++ { dst[i] = src[i] }
//
// returning the dst and src identifier names.
func copyLoopShape(n ast.Node) (dst, src string, ok bool) {
	switch l := n.(type) {
	case *ast.RangeStmt:
		srcID, o := l.X.(*ast.Ident)
		if !o {
			return "", "", false
		}
		key, o := l.Key.(*ast.Ident)
		if !o || key.Name == "_" || len(l.Body.List) != 1 {
			return "", "", false
		}
		as, o := l.Body.List[0].(*ast.AssignStmt)
		if !o || len(as.Lhs) != 1 || len(as.Rhs) != 1 || as.Tok.String() != "=" {
			return "", "", false
		}
		ix, o := as.Lhs[0].(*ast.IndexExpr)
		if !o {
			return "", "", false
		}
		dstID, o := ix.X.(*ast.Ident)
		if !o || dstID.Name == srcID.Name {
			return "", "", false
		}
		if idxID, o := ix.Index.(*ast.Ident); !o || idxID.Name != key.Name {
			return "", "", false
		}
		// RHS: src[i], or the range value variable.
		switch rhs := as.Rhs[0].(type) {
		case *ast.IndexExpr:
			rx, o := rhs.X.(*ast.Ident)
			if !o || rx.Name != srcID.Name {
				return "", "", false
			}
			if ri, o := rhs.Index.(*ast.Ident); !o || ri.Name != key.Name {
				return "", "", false
			}
			return dstID.Name, srcID.Name, true
		case *ast.Ident:
			if val, o := l.Value.(*ast.Ident); o && val.Name == rhs.Name && rhs.Name != "_" {
				return dstID.Name, srcID.Name, true
			}
		}
		return "", "", false
	case *ast.ForStmt:
		// i := 0; i < len(src); i++
		init, o := l.Init.(*ast.AssignStmt)
		if !o || len(init.Lhs) != 1 {
			return "", "", false
		}
		iv, o := init.Lhs[0].(*ast.Ident)
		if !o {
			return "", "", false
		}
		lit, o := init.Rhs[0].(*ast.BasicLit)
		if !o || lit.Value != "0" {
			return "", "", false
		}
		cond, o := l.Cond.(*ast.BinaryExpr)
		if !o || cond.Op.String() != "<" {
			return "", "", false
		}
		lenCall, o := cond.Y.(*ast.CallExpr)
		if !o || !calleeIsLen(lenCall) || len(lenCall.Args) != 1 {
			return "", "", false
		}
		srcID, o := lenCall.Args[0].(*ast.Ident)
		if !o || len(l.Body.List) != 1 {
			return "", "", false
		}
		as, o := l.Body.List[0].(*ast.AssignStmt)
		if !o || len(as.Lhs) != 1 || len(as.Rhs) != 1 || as.Tok.String() != "=" {
			return "", "", false
		}
		ix, o := as.Lhs[0].(*ast.IndexExpr)
		if !o {
			return "", "", false
		}
		dstID, o := ix.X.(*ast.Ident)
		if !o || dstID.Name == srcID.Name {
			return "", "", false
		}
		idxID, o := ix.Index.(*ast.Ident)
		if !o || idxID.Name != iv.Name {
			return "", "", false
		}
		rhs, o := as.Rhs[0].(*ast.IndexExpr)
		if !o {
			return "", "", false
		}
		rx, o := rhs.X.(*ast.Ident)
		if !o || rx.Name != srcID.Name {
			return "", "", false
		}
		if ri, o := rhs.Index.(*ast.Ident); !o || ri.Name != iv.Name {
			return "", "", false
		}
		return dstID.Name, srcID.Name, true
	}
	return "", "", false
}

// dstExprOf / srcExprOf re-locate the dst/src expressions for typing.
func dstExprOf(n ast.Node) ast.Expr {
	body := loopBodyOf(n.(ast.Stmt))
	as := body.List[0].(*ast.AssignStmt)
	return as.Lhs[0].(*ast.IndexExpr).X
}

func srcExprOf(n ast.Node) ast.Expr {
	switch l := n.(type) {
	case *ast.RangeStmt:
		return l.X
	case *ast.ForStmt:
		return l.Cond.(*ast.BinaryExpr).Y.(*ast.CallExpr).Args[0]
	}
	return nil
}

func underlyingSlice(t types.Type) (*types.Slice, bool) {
	if t == nil {
		return nil, false
	}
	s, ok := t.Underlying().(*types.Slice)
	return s, ok
}
