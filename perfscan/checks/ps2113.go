package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2113 reports w.Write([]byte(fmt.Sprintf(...))) — formatting into a
// throwaway string, copying it into a throwaway []byte, and only then
// handing the bytes to the writer — where fmt.Fprintf(w, ...) formats
// straight into w.
var PS2113 = register(&lint.Check{
	ID:       "PS2113",
	Category: "alloc",
	Slug:     "write-bytes-of-sprintf",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "w.Write([]byte(fmt.Sprintf(...))) builds a throwaway string+slice; fmt.Fprintf writes straight to w",
		Text: `fmt.Sprintf allocates the formatted string, the []byte
conversion copies it into a second allocation, and only then does the
writer see the bytes. fmt.Fprintf(w, format, args...) formats into an
internal buffer and passes it to w.Write directly — the string and the
copied slice never exist. fmt.Sprint and fmt.Sprintln map to fmt.Fprint
and fmt.Fprintln the same way.

The result is bit-identical: fmt.Fprintf returns exactly what the single
w.Write call returned (fmt calls w.Write(buf) once and forwards its
(n, err) untouched), and the bytes written are the same bytes Sprintf
would have produced. Operand evaluation order is unchanged: w, then the
format, then the arguments. The io.Writer contract forbids retaining the
passed slice, so handing the writer fmt's reused buffer instead of a
fresh copy is not observable for conforming writers.

One position is excluded entirely: w.Write([]byte(fmt.Sprintf(...)))
lexically inside w's own Write method. fmt.Fprintf writes through
w.Write, so the rewrite would dispatch back to the enclosing method
itself — unbounded recursion that still compiles. The check reports
nothing there; writing to a different object (a field, another variable)
inside Write is still reported.

The automatic fix requires the argument to be exactly a predeclared
[]byte conversion of a call type-pinned to the standard library's
fmt.Sprintf/Sprint/Sprintln, and the callee to be a method with
io.Writer's exact Write signature. The receiver's own type must be
assignable to io.Writer — a value whose Write has a pointer receiver
(e.g. a bytes.Buffer variable rather than *bytes.Buffer) satisfies the
call via auto-address but could not be passed to fmt.Fprintf, so it is
reported without a fix. The format string and arguments are kept verbatim
in place, the fmt qualifier (alias included) is reused, and fmt stays
imported — the rewrite is import-neutral. A comment inside the rewritten
scaffolding also downgrades the fix to an advisory report.`,
		Before: `w.Write([]byte(fmt.Sprintf("user=%s id=%d", name, id)))`,
		After:  `fmt.Fprintf(w, "user=%s id=%d", name, id)`,
		MeasuredWin: `BenchmarkPS2113 (formatting "word=%s idx=%d" into a reset
bytes.Buffer, Apple M2 Pro): 93 ns/op, 54 B/op, 3 allocs/op ->
81 ns/op, 24 B/op, 2 allocs/op (~1.15x faster; the intermediate string
and its []byte copy are gone, the remaining allocations are the boxed
operands). The win grows with the formatted length: the copies removed
scale with the output, the boxing does not.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2113",
		Doc:  "w.Write([]byte(fmt.Sprintf(...))) instead of fmt.Fprintf(w, ...)",
		Run:  runPS2113,
	},
})

// ps2113Fprint maps each recognized fmt formatter to its writer-directed
// counterpart.
var ps2113Fprint = map[string]string{
	"Sprintf":  "Fprintf",
	"Sprint":   "Fprint",
	"Sprintln": "Fprintln",
}

// ps2113ByteSlice is the predeclared []byte.
var ps2113ByteSlice = types.NewSlice(types.Typ[types.Byte])

// ps2113Writer is io.Writer built structurally (interface satisfaction is
// structural, so no import of io's type data is needed):
// interface{ Write(p []byte) (n int, err error) }.
var ps2113Writer = func() *types.Interface {
	sig := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "p", ps2113ByteSlice)),
		types.NewTuple(
			types.NewVar(token.NoPos, nil, "n", types.Typ[types.Int]),
			types.NewVar(token.NoPos, nil, "err", types.Universe.Lookup("error").Type()),
		),
		false)
	iface := types.NewInterfaceType([]*types.Func{
		types.NewFunc(token.NoPos, nil, "Write", sig),
	}, nil)
	iface.Complete()
	return iface
}()

func runPS2113(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			outer, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			recv, inner, qual, sname, ok := ps2113Match(pass, outer)
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
			fname := ps2113Fprint[sname]
			diag := analysis.Diagnostic{
				Pos: outer.Pos(),
				End: outer.End(),
				Message: "Write([]byte(" + qual + "." + sname + "(...))) builds a throwaway string and []byte; " +
					qual + "." + fname + "(w, ...) writes the formatted bytes directly to w",
			}
			// The fix keeps the receiver and every formatting operand
			// verbatim in place; only the scaffolding around them is
			// rewritten. That requires the receiver's own type to be
			// assignable to io.Writer (a pointer-receiver Write reached
			// via auto-address is callable here but the value could not
			// be passed to Fprintf), and no comment may sit inside the
			// deleted scaffolding text.
			recvType := pass.TypesInfo.TypeOf(recv)
			if recvType != nil && types.AssignableTo(recvType, ps2113Writer) &&
				!ps2113CommentIn(f, recv.End(), inner.Lparen+1) &&
				!ps2113CommentIn(f, inner.Rparen, outer.End()) {
				// w.Write([]byte(fmt.Sprintf(f, args))) -> fmt.Fprintf(w, f, args)
				//   insert "fmt.Fprintf(" before w,
				//   replace ".Write([]byte(fmt.Sprintf(" with ", ",
				//   replace ")))" with ")".
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

// ps2113Match matches outer as w.Write([]byte(fmt.S*(...))): a method call
// with io.Writer's exact Write signature whose sole argument is a
// predeclared []byte conversion of a direct stdlib
// fmt.Sprintf/Sprint/Sprintln call. It returns the receiver expression,
// the inner fmt call, the fmt qualifier as written (alias preserved), and
// the fmt function name.
func ps2113Match(pass *analysis.Pass, outer *ast.CallExpr) (recv ast.Expr, inner *ast.CallExpr, qual, sname string, ok bool) {
	if len(outer.Args) != 1 || outer.Ellipsis.IsValid() {
		return nil, nil, "", "", false
	}
	sel, ok := outer.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Write" {
		return nil, nil, "", "", false
	}
	// The callee must be a METHOD (a package-level or field func named
	// Write is rejected) with exactly io.Writer's signature:
	// Write(p []byte) (n int, err error).
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok {
		return nil, nil, "", "", false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil || sig.Variadic() ||
		sig.Params().Len() != 1 || sig.Results().Len() != 2 ||
		!types.Identical(sig.Params().At(0).Type(), ps2113ByteSlice) ||
		!types.Identical(sig.Results().At(0).Type(), types.Typ[types.Int]) ||
		!types.Identical(sig.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return nil, nil, "", "", false
	}
	// The argument must be a conversion to the predeclared []byte — a
	// NAMED byte-slice type is a different conversion and is left alone.
	conv, ok := ps2113Unparen(outer.Args[0]).(*ast.CallExpr)
	if !ok || len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return nil, nil, "", "", false
	}
	tv, ok := pass.TypesInfo.Types[conv.Fun]
	if !ok || !tv.IsType() {
		return nil, nil, "", "", false
	}
	sl, ok := types.Unalias(tv.Type).(*types.Slice)
	if !ok {
		return nil, nil, "", "", false
	}
	if eb, ok := sl.Elem().(*types.Basic); !ok || eb.Kind() != types.Byte {
		return nil, nil, "", "", false
	}
	// The converted operand must be a direct call of the standard
	// library's fmt.Sprintf/Sprint/Sprintln — type information pins the
	// callee, so a shadowed fmt or a same-named method never matches.
	inner, ok = ps2113Unparen(conv.Args[0]).(*ast.CallExpr)
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

// ps2113Unparen strips any parenthesis layers around e.
func ps2113Unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// ps2113CommentIn reports whether any comment overlaps [pos, end) — text
// the fix would delete, which must not swallow a comment.
func ps2113CommentIn(f *ast.File, pos, end token.Pos) bool {
	for _, cg := range f.Comments {
		if cg.End() > pos && cg.Pos() < end {
			return true
		}
	}
	return false
}
