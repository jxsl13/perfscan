package checks

import (
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2106 reports runs of two or more strictly adjacent statements of the
// form `x = append(x, a); x = append(x, b)` and combines them into a
// single `x = append(x, a, b)` — one length/capacity check and at most
// one growth instead of one per statement.
var PS2106 = register(&lint.Check{
	ID:       "PS2106",
	Category: "alloc",
	Slug:     "append-combine",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "consecutive appends to the same slice can be combined into one call",
		Text: `Every x = append(x, v) statement re-reads the slice header,
checks capacity, and may grow the backing array — a chain of consecutive
single-value appends repeats that work per statement and can pay several
growth reallocations where one multi-value append performs a single check
and at most one growth. append(x, a, b, ...) produces the same elements in
the same order, and the argument expressions are still evaluated left to
right in their original statement order.

The automatic fix merges the argument lists when every statement in the
run assigns the same identifier from a non-spread builtin append on that
same identifier. Arguments of the statements AFTER the first are moved
before the first assignment, so the fix additionally requires them to be
side-effect-free, non-panicking expressions (identifiers, literals,
value-field selections, composite literals, simple arithmetic) that do not
reference x — such expressions cannot observe the intermediate slice
value, making the reordering unobservable. Runs whose later arguments
contain calls, indexing, or pointer dereferences are reported without a
fix: combine manually only after checking the expressions cannot observe
x. Statements whose later arguments mention x itself (e.g. len(x)) end
the run entirely — combining those would change the value they see.
Spread appends (append(x, ys...)) are never combined. As with pre-sizing
(PS2101), intermediate writes into spare capacity during growth are not
treated as observable.`,
		Before: `args = append(args, "--verbose")
args = append(args, "--name")
args = append(args, name)`,
		After:       `args = append(args, "--verbose", "--name", name)`,
		MeasuredWin: `combining 8 consecutive single-value appends into one call: 97.4ns/op, 240B/op, 4 allocs/op → 31.4ns/op, 128B/op, 1 alloc/op (≈3.1× faster, 4× fewer allocations; BenchmarkPS2106 in benchmarks/, Apple M2 Pro)`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2106",
		Doc:  "consecutive appends to the same slice combinable into one call",
		Run:  runPS2106,
	},
})

func runPS2106(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		file := f
		ast.Inspect(f, func(n ast.Node) bool {
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for i := 0; i < len(block.List); {
				first, obj := ps2106AppendStmt(pass, block.List[i])
				if first == nil {
					i++
					continue
				}
				run := []*ast.AssignStmt{first}
				fixable := true
				j := i + 1
				for ; j < len(block.List); j++ {
					next, nobj := ps2106AppendStmt(pass, block.List[j])
					if next == nil || nobj != obj {
						break
					}
					values := next.Rhs[0].(*ast.CallExpr).Args[1:]
					// A later argument mentioning x (len(x), x[0]) must see
					// the value after the earlier appends; combining would
					// change what it sees. Such a statement ends the run —
					// even manual combining would be wrong.
					if ps2106MentionsObj(pass, values, obj) {
						break
					}
					// Later arguments are moved before the first
					// assignment; only provably pure, non-panicking
					// expressions keep that reordering unobservable.
					for _, v := range values {
						if !ps2106PureArg(pass, v, obj) {
							fixable = false
						}
					}
					run = append(run, next)
				}
				if len(run) >= 2 {
					ps2106Report(pass, file, run, obj.Name(), fixable)
				}
				i = j
			}
			return true
		})
	}
	return nil, nil
}

// ps2106AppendStmt matches `x = append(x, a, ...)` — plain assignment of
// the builtin append over the same identifier it assigns, with at least
// one fixed (non-spread) value — returning the statement and x's object.
func ps2106AppendStmt(pass *analysis.Pass, s ast.Stmt) (*ast.AssignStmt, types.Object) {
	as, ok := s.(*ast.AssignStmt)
	if !ok || as.Tok != token.ASSIGN || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return nil, nil
	}
	lhs, ok := as.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name == "_" {
		return nil, nil
	}
	call, ok := as.Rhs[0].(*ast.CallExpr)
	if !ok || call.Ellipsis.IsValid() || len(call.Args) < 2 {
		return nil, nil
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "append" {
		return nil, nil
	}
	// The builtin, not a shadowing local named append.
	if _, isBuiltin := pass.TypesInfo.Uses[fun].(*types.Builtin); !isBuiltin {
		return nil, nil
	}
	arg0, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return nil, nil
	}
	lobj := pass.TypesInfo.ObjectOf(lhs)
	if lobj == nil || lobj != pass.TypesInfo.ObjectOf(arg0) {
		return nil, nil
	}
	return as, lobj
}

// ps2106MentionsObj reports whether any of the expressions references the
// given object.
func ps2106MentionsObj(pass *analysis.Pass, exprs []ast.Expr, obj types.Object) bool {
	for _, e := range exprs {
		found := false
		ast.Inspect(e, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == obj {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

// ps2106PureArg reports whether e is provably side-effect-free and
// non-panicking, so its evaluation can move before the earlier append
// assignments without any observable difference: no calls (which could
// observe x through a capture or global), no indexing or dereferences
// (which could read memory aliasing x's backing array), no operators that
// can panic (/, %, shifts, interface comparison).
func ps2106PureArg(pass *analysis.Pass, e ast.Expr, x types.Object) bool {
	switch v := e.(type) {
	case *ast.BasicLit:
		return true
	case *ast.Ident:
		obj := pass.TypesInfo.ObjectOf(v)
		if obj == nil || obj == x {
			return false
		}
		switch obj.(type) {
		case *types.Var, *types.Const, *types.Func, *types.Nil:
			return true
		}
		return false
	case *ast.ParenExpr:
		return ps2106PureArg(pass, v.X, x)
	case *ast.UnaryExpr:
		switch v.Op {
		case token.ADD, token.SUB, token.NOT, token.XOR:
			return ps2106PureArg(pass, v.X, x)
		}
		return false
	case *ast.BinaryExpr:
		switch v.Op {
		case token.ADD, token.SUB, token.MUL, token.AND, token.OR,
			token.XOR, token.AND_NOT, token.LAND, token.LOR:
			return ps2106PureArg(pass, v.X, x) && ps2106PureArg(pass, v.Y, x)
		}
		return false
	case *ast.SelectorExpr:
		if sel, ok := pass.TypesInfo.Selections[v]; ok {
			// A value-field selection reads the base variable's own
			// storage; an indirect one dereferences a pointer, which can
			// both panic (nil) and alias x's backing array.
			return sel.Kind() == types.FieldVal && !sel.Indirect() && ps2106PureArg(pass, v.X, x)
		}
		// Qualified identifier pkg.Name.
		switch pass.TypesInfo.Uses[v.Sel].(type) {
		case *types.Var, *types.Const, *types.Func:
			return true
		}
		return false
	case *ast.CompositeLit:
		for _, el := range v.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				if !ps2106PureArg(pass, kv.Key, x) || !ps2106PureArg(pass, kv.Value, x) {
					return false
				}
				continue
			}
			if !ps2106PureArg(pass, el, x) {
				return false
			}
		}
		return true
	}
	return false
}

func ps2106Report(pass *analysis.Pass, f *ast.File, run []*ast.AssignStmt, name string, fixable bool) {
	n := len(run)
	msg := fmt.Sprintf("%d consecutive appends to %s can be combined into one append call — a single length/growth check instead of %d", n, name, n)
	if !fixable {
		msg += " (left advisory: an appended argument may have side effects or read shared memory; combine manually only if it cannot observe " + name + ")"
	}
	diag := analysis.Diagnostic{
		Pos:     run[0].Pos(),
		End:     run[n-1].End(),
		Message: msg,
	}
	if fixable {
		if fix := ps2106Fix(pass, f, run, name); fix != nil {
			diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
		}
	}
	pass.Report(diag)
}

// ps2106Fix splices the later statements' values into the first call's
// argument list and deletes the later statements line by line, preserving
// comments between the statements. Any comment overlapping a deleted span
// withholds the fix rather than destroying it.
func ps2106Fix(pass *analysis.Pass, f *ast.File, run []*ast.AssignStmt, name string) *analysis.SuggestedFix {
	firstCall := run[0].Rhs[0].(*ast.CallExpr)
	if !firstCall.Rparen.IsValid() {
		return nil
	}
	tf := pass.Fset.File(run[0].Pos())
	if tf == nil {
		return nil
	}
	var extra strings.Builder
	extra.Grow(16 * len(run)) // rough lower bound: ", " + a short arg per combined statement
	edits := make([]analysis.TextEdit, 0, len(run))
	edits = append(edits, analysis.TextEdit{Pos: firstCall.Rparen, End: firstCall.Rparen})
	// One throwaway FileSet for every render: the printed args carry no
	// positions relative to it, so a single empty set gives identical
	// output without allocating a fresh FileSet per argument.
	fset := token.NewFileSet()
	for k := 1; k < len(run); k++ {
		stmt := run[k]
		for _, v := range stmt.Rhs[0].(*ast.CallExpr).Args[1:] {
			extra.WriteString(", ")
			if err := printer.Fprint(&extra, fset, v); err != nil {
				return nil
			}
		}
		// Delete the whole statement plus either the "; " separator (same
		// line as the previous statement) or its line's leading newline and
		// indentation, so no blank line is left behind.
		delStart := run[k-1].End()
		if tf.Line(stmt.Pos()) != tf.Line(run[k-1].End()) {
			delStart = tf.LineStart(tf.Line(stmt.Pos())) - 1
		}
		if ps2106CommentBetween(f, delStart, stmt.End()) {
			return nil
		}
		edits = append(edits, analysis.TextEdit{Pos: delStart, End: stmt.End()})
	}
	edits[0].NewText = []byte(extra.String())
	return &analysis.SuggestedFix{
		Message:   fmt.Sprintf("combine %d appends to %s into one call", len(run), name),
		TextEdits: edits,
	}
}

// ps2106CommentBetween reports whether any comment overlaps (from, to).
func ps2106CommentBetween(f *ast.File, from, to token.Pos) bool {
	for _, cg := range f.Comments {
		if cg.End() > from && cg.Pos() < to {
			return true
		}
	}
	return false
}
