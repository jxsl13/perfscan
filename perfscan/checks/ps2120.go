package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2120 reports w.WriteString(fmt.Sprintf(...)) — formatting into a
// throwaway string and only then handing its bytes to the writer — where
// fmt.Fprintf(w, ...) formats straight into w. The exact sibling of
// PS2113 for the WriteString method instead of Write([]byte(...)).
var PS2120 = register(&lint.Check{
	ID:       "PS2120",
	Category: "alloc",
	Slug:     "writestring-of-sprintf",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "w.WriteString(fmt.Sprintf(...)) builds a throwaway string; fmt.Fprintf writes straight to w",
		Text: `fmt.Sprintf allocates the formatted string, and WriteString
copies its bytes into the writer — the string exists only to be copied
once and dropped. fmt.Fprintf(w, format, args...) formats into an
internal buffer and passes it to w.Write directly — the intermediate
string never exists. fmt.Sprint and fmt.Sprintln map to fmt.Fprint and
fmt.Fprintln the same way.

The result is bit-identical: the bytes reaching the writer are exactly
the bytes Sprintf would have produced, and both calls return
(n int, err error) with the count of bytes written and the writer's
error. The io.StringWriter contract requires WriteString(s) to behave
exactly like Write([]byte(s)) (io.WriteString relies on that
equivalence), so routing the same bytes through Write instead is not
observable for conforming writers. Operand evaluation order is
unchanged: w, then the format, then the arguments.

One position is excluded entirely: w.WriteString(fmt.Sprintf(...))
lexically inside w's own Write method. fmt.Fprintf writes through
w.Write, so the rewrite would dispatch back to the enclosing method
itself — unbounded recursion that still compiles. The check reports
nothing there; writing to a different object (a field, another variable)
inside Write is still reported.

The automatic fix requires the argument to be exactly a call type-pinned
to the standard library's fmt.Sprintf/Sprint/Sprintln, and the callee to
be a method with io.StringWriter's exact WriteString signature. Because
fmt.Fprintf takes an io.Writer, the receiver's own type must be
assignable to io.Writer — a value whose Write has a pointer receiver
(e.g. a strings.Builder variable rather than *strings.Builder) satisfies
the WriteString call via auto-address but could not be passed to
fmt.Fprintf, so it is reported without a fix. A type with WriteString
but no Write method at all cannot be handed to fmt.Fprintf in any form,
so it is not reported. The format string and arguments are kept verbatim
in place, the fmt qualifier (alias included) is reused, and fmt stays
imported — the rewrite is import-neutral. A comment inside the rewritten
scaffolding also downgrades the fix to an advisory report.`,
		Before: `w.WriteString(fmt.Sprintf("user=%s id=%d", name, id))`,
		After:  `fmt.Fprintf(w, "user=%s id=%d", name, id)`,
		MeasuredWin: `BenchmarkPS2120 (formatting "word=%s idx=%d" into a reset
bytes.Buffer, Apple M2 Pro): 91 ns/op, 55 B/op, 3 allocs/op ->
79 ns/op, 24 B/op, 2 allocs/op (~1.16x faster; the intermediate string
is gone, the remaining allocations are the boxed operands). The win
grows with the formatted length: the copy removed scales with the
output, the boxing does not.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2120",
		Doc:  "w.WriteString(fmt.Sprintf(...)) instead of fmt.Fprintf(w, ...)",
		Run:  runPS2120,
	},
})

func runPS2120(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			outer, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			recv, inner, qual, sname, ok := ps2120Match(pass, outer)
			if !ok {
				return true
			}
			// fmt.Fprintf(w, ...) writes through w.Write: inside the
			// receiver's own Write method the rewrite would dispatch back
			// to the enclosing method itself — stay silent, no valid
			// rewrite exists there.
			if writeFixSelfDispatches(pass, outer, recv, "Write") {
				return true
			}
			// fmt.Fprintf needs an io.Writer. Unlike PS2113 (whose match
			// on w.Write already guarantees one), WriteString proves
			// nothing about Write: a type may implement io.StringWriter
			// only. If neither the receiver's type nor its pointer type
			// is assignable to io.Writer, the rewrite recommendation
			// itself would be wrong — stay silent.
			recvType := pass.TypesInfo.TypeOf(recv)
			if recvType == nil {
				return true
			}
			fixable := types.AssignableTo(recvType, ps2113Writer)
			if !fixable && !types.AssignableTo(types.NewPointer(recvType), ps2113Writer) {
				return true
			}
			fname := ps2113Fprint[sname]
			diag := analysis.Diagnostic{
				Pos: outer.Pos(),
				End: outer.End(),
				Message: "WriteString(" + qual + "." + sname + "(...)) builds a throwaway string; " +
					qual + "." + fname + "(w, ...) writes the formatted bytes directly to w",
			}
			// The fix keeps the receiver and every formatting operand
			// verbatim in place; only the scaffolding around them is
			// rewritten. That requires the receiver's own type to be
			// assignable to io.Writer (a pointer-receiver Write reached
			// via auto-address is not enough: the value could not be
			// passed to Fprintf), and no comment may sit inside the
			// deleted scaffolding text.
			if fixable &&
				!ps2113CommentIn(f, recv.End(), inner.Lparen+1) &&
				!ps2113CommentIn(f, inner.Rparen, outer.End()) {
				// w.WriteString(fmt.Sprintf(f, args)) -> fmt.Fprintf(w, f, args)
				//   insert "fmt.Fprintf(" before w,
				//   replace ".WriteString(fmt.Sprintf(" with ", ",
				//   replace "))" with ")".
				edits := []analysis.TextEdit{
					{Pos: outer.Pos(), End: outer.Pos(), NewText: []byte(qual + "." + fname + "(")},
				}
				sep := []byte(", ")
				if len(inner.Args) == 0 {
					// fmt.Sprint() with no operands: no separator, the
					// writer becomes the only argument.
					sep = nil
				}
				edits = append(edits,
					analysis.TextEdit{Pos: recv.End(), End: inner.Lparen + 1, NewText: sep},
					analysis.TextEdit{Pos: inner.Rparen, End: outer.End(), NewText: []byte(")")},
				)
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "rewrite to " + qual + "." + fname + "(w, ...)",
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2120Match matches outer as w.WriteString(fmt.S*(...)): a method call
// with io.StringWriter's exact WriteString signature whose sole argument
// is a direct stdlib fmt.Sprintf/Sprint/Sprintln call. It returns the
// receiver expression, the inner fmt call, the fmt qualifier as written
// (alias preserved), and the fmt function name.
func ps2120Match(pass *analysis.Pass, outer *ast.CallExpr) (recv ast.Expr, inner *ast.CallExpr, qual, sname string, ok bool) {
	if len(outer.Args) != 1 || outer.Ellipsis.IsValid() {
		return nil, nil, "", "", false
	}
	sel, ok := outer.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "WriteString" {
		return nil, nil, "", "", false
	}
	// The callee must be a METHOD (the package function io.WriteString is
	// rejected) with exactly io.StringWriter's signature:
	// WriteString(s string) (n int, err error).
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok {
		return nil, nil, "", "", false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil || sig.Variadic() ||
		sig.Params().Len() != 1 || sig.Results().Len() != 2 ||
		!types.Identical(sig.Params().At(0).Type(), types.Typ[types.String]) ||
		!types.Identical(sig.Results().At(0).Type(), types.Typ[types.Int]) ||
		!types.Identical(sig.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return nil, nil, "", "", false
	}
	// The argument must be a direct call of the standard library's
	// fmt.Sprintf/Sprint/Sprintln — type information pins the callee, so
	// a shadowed fmt or a same-named method never matches.
	inner, ok = ps2113Unparen(outer.Args[0]).(*ast.CallExpr)
	if !ok {
		return nil, nil, "", "", false
	}
	isel, ok := inner.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, nil, "", "", false
	}
	iq, ok := isel.X.(*ast.Ident)
	if !ok {
		return nil, nil, "", "", false
	}
	ifn, ok := pass.TypesInfo.Uses[isel.Sel].(*types.Func)
	if !ok || ifn.Pkg() == nil || ifn.Pkg().Path() != "fmt" || ps2113Fprint[ifn.Name()] == "" {
		return nil, nil, "", "", false
	}
	if isig, ok := ifn.Type().(*types.Signature); !ok || isig.Recv() != nil {
		return nil, nil, "", "", false
	}
	return sel.X, inner, iq.Name, ifn.Name(), true
}
