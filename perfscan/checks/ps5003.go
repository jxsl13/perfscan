package checks

import (
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5003 reports w.Write([]byte{c}) — a one-element []byte composite
// literal built just to hand a single byte to Write — on a receiver whose
// method set has WriteByte(byte) error.
var PS5003 = register(&lint.Check{
	ID:       "PS5003",
	Category: "alloc",
	Slug:     "write-single-byte-literal",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "Write([]byte{c}) copies one byte through a throwaway slice; WriteByte(c) appends it directly",
		Text: `Write([]byte{c}) materializes a one-element []byte composite
literal — a slice header plus a backing array the compiler must construct —
and then Write does its length bookkeeping and copies that single byte into
the buffer. WriteByte(c) appends the byte through the direct single-byte
path: no slice construction, no copy loop, and (behind an interface, where
the argument escapes) no heap allocation for the throwaway slice.

The check matches a method call w.Write(x) whose single argument is a
composite literal of type exactly []byte (an alias is looked through, a
named slice type is not matched) with EXACTLY ONE element that is not a
keyed entry — []byte{a, b}, []byte{5: c} (length 6) and []byte{} write a
different byte count and are never reported. The receiver's method set must
also carry WriteByte(byte) error, verified through the type checker with
the same addressability Write itself required.

The automatic fix rewrites w.Write([]byte{c}) to w.WriteByte(c) when three
proofs hold. First, the Write/WriteByte pair is the standard library's own
bytes.Buffer or strings.Builder — pinned via the type checker's method
objects, not by name, so a user type shadowing either method is excluded;
both writers never error and always append the full input, so the two forms
append the identical byte and both errors are always nil. bufio.Writer is
deliberately excluded from the fix (its Write can flush and return an
error mid-stream; the check still reports it as advisory). Second, the
call's results are discarded (an expression statement, or the call of a go
or defer statement): Write returns (int, error) but WriteByte returns only
error, so a used result would change the statement's shape. Third, the
element expression carries no comment the rewrite would delete. The element
c is evaluated exactly once in both forms, and its assignability to byte is
guaranteed by the original code compiling — []byte{c} already required c to
be assignable to byte, which is WriteByte's parameter type. The rewrite
keeps c's source text in place and touches no imports.`,
		Before: `buf.Write([]byte{c})`,
		After:  `buf.WriteByte(c)`,
		MeasuredWin: `BenchmarkPS5003 (1024 single bytes into a reset
bytes.Buffer, Apple M2 Pro): 3.38 µs/op -> 2.10 µs/op (~1.6x, ~1.2 ns per
write), zero allocations either way (the non-escaping literal stays on the
stack; behind an io.Writer interface it would heap-allocate too). The
saving is the composite-literal construction and Write's copy machinery
that the direct byte append skips.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5003",
		Doc:  "Write of a one-element []byte literal instead of WriteByte",
		Run:  runPS5003,
	},
})

func runPS5003(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps5003Match(pass, call)
			if m == nil {
				return true
			}
			msg := "Write of the one-element slice literal []byte{" + m.eltText +
				"} builds and copies a throwaway slice; WriteByte(" + m.eltText +
				") appends the byte directly"
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: msg,
			}
			switch {
			case !m.stdPair:
				// A custom (or bufio.Writer) Write/WriteByte pair: nothing
				// proves the two methods append identically — advisory only.
				diag.Message = msg + "; this Write/WriteByte pair is not bytes.Buffer's or strings.Builder's — verify WriteByte appends identically before rewriting"
			case !ps5003ResultDiscarded(call, stack):
				// Write returns (int, error), WriteByte only error: a used
				// result changes the statement's shape — advisory only.
				diag.Message = msg + "; Write returns (int, error) but WriteByte returns only error — adjust the result handling by hand"
			case ps5003CommentIn(f, m.lit.Pos(), m.elt.Pos()) || ps5003CommentIn(f, m.elt.End(), m.lit.End()):
				// A comment inside the deleted "[]byte{" or "}" text would be
				// destroyed — keep the advisory report without a fix.
			default:
				// Rewrite the selected name and delete only the literal's
				// "[]byte{" and "}" around the element, leaving the element
				// expression untouched in place — same evaluation, same
				// order, import-neutral.
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace Write([]byte{c}) with WriteByte(c)",
					TextEdits: []analysis.TextEdit{
						{Pos: m.sel.Sel.Pos(), End: m.sel.Sel.End(), NewText: []byte("WriteByte")},
						{Pos: m.lit.Pos(), End: m.elt.Pos()},
						{Pos: m.elt.End(), End: m.lit.End()},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps5003Site is a matched w.Write([]byte{c}) call: the selector to rewrite,
// the composite literal, its single element, and whether the method pair is
// bytes.Buffer's or strings.Builder's (fix-eligible).
type ps5003Site struct {
	sel     *ast.SelectorExpr
	lit     *ast.CompositeLit
	elt     ast.Expr
	eltText string
	stdPair bool
}

// ps5003Match matches a method call w.Write(L) with L a composite literal
// of type exactly []byte holding exactly one non-keyed element, on a
// receiver whose method set carries WriteByte(byte) error. Type information
// pins every part: the selection must be a real method (not a
// function-typed field or a package function), Write must have io.Writer's
// exact shape, and WriteByte is looked up on the receiver's type with the
// addressability Write itself required.
func ps5003Match(pass *analysis.Pass, call *ast.CallExpr) *ps5003Site {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Write" || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil
	}
	selection, ok := pass.TypesInfo.Selections[sel]
	if !ok || selection.Kind() != types.MethodVal {
		return nil
	}
	wr, ok := selection.Obj().(*types.Func)
	if !ok || !ps5003IsWriteSig(wr) {
		return nil
	}
	lit := ps5003SingleByteLit(pass, call.Args[0])
	if lit == nil {
		return nil
	}
	// WriteByte on the receiver expression's type. Write on a pointer
	// receiver means the checker already had an address for w, so a
	// pointer-receiver WriteByte is reachable too; a value-receiver Write
	// proves nothing about addressability, so the lookup then demands
	// WriteByte without one.
	addressable := false
	if r := ps5003Recv(wr); r != nil {
		_, addressable = r.Type().(*types.Pointer)
	}
	// LookupFieldOrMethod returns a nil object when the only WriteByte has
	// a pointer receiver and the receiver is neither a pointer nor
	// addressable — exactly the case where the rewrite would not compile.
	wbObj, _, _ := types.LookupFieldOrMethod(selection.Recv(), addressable, pass.Pkg, "WriteByte")
	wb, ok := wbObj.(*types.Func)
	if !ok || !ps5003IsWriteByteSig(wb) {
		return nil
	}
	elt := lit.Elts[0]
	eltText, ok := ps5003ExprText(elt)
	if !ok {
		return nil
	}
	return &ps5003Site{
		sel:     sel,
		lit:     lit,
		elt:     elt,
		eltText: eltText,
		stdPair: ps5003StdPair(wr, wb),
	}
}

// ps5003SingleByteLit matches arg (parens stripped) as a composite literal
// of type exactly []byte — an unnamed slice of the predeclared byte, an
// alias thereof included, a NAMED slice type rejected — with exactly one
// element that is not a KeyValueExpr. []byte{5: c} has length 6 and
// []byte{} length 0: both write a different byte count and never match.
func ps5003SingleByteLit(pass *analysis.Pass, arg ast.Expr) *ast.CompositeLit {
	for {
		p, ok := arg.(*ast.ParenExpr)
		if !ok {
			break
		}
		arg = p.X
	}
	lit, ok := arg.(*ast.CompositeLit)
	if !ok || len(lit.Elts) != 1 {
		return nil
	}
	if _, keyed := lit.Elts[0].(*ast.KeyValueExpr); keyed {
		return nil
	}
	lt := pass.TypesInfo.TypeOf(lit)
	if lt == nil {
		return nil
	}
	sl, ok := types.Unalias(lt).(*types.Slice)
	if !ok {
		return nil
	}
	eb, ok := types.Unalias(sl.Elem()).(*types.Basic)
	if !ok || eb.Kind() != types.Uint8 {
		return nil
	}
	return lit
}

// ps5003IsWriteSig reports whether wr is exactly Write([]byte) (int, error).
func ps5003IsWriteSig(wr *types.Func) bool {
	sig, ok := wr.Type().(*types.Signature)
	if !ok || sig.Variadic() || sig.Params().Len() != 1 || sig.Results().Len() != 2 {
		return false
	}
	sl, ok := types.Unalias(sig.Params().At(0).Type()).(*types.Slice)
	if !ok {
		return false
	}
	pb, ok := types.Unalias(sl.Elem()).(*types.Basic)
	if !ok || pb.Kind() != types.Uint8 {
		return false
	}
	rb, ok := sig.Results().At(0).Type().(*types.Basic)
	if !ok || rb.Kind() != types.Int {
		return false
	}
	return types.Identical(sig.Results().At(1).Type(), types.Universe.Lookup("error").Type())
}

// ps5003IsWriteByteSig reports whether wb is exactly WriteByte(byte) error.
func ps5003IsWriteByteSig(wb *types.Func) bool {
	sig, ok := wb.Type().(*types.Signature)
	if !ok || sig.Variadic() || sig.Params().Len() != 1 || sig.Results().Len() != 1 {
		return false
	}
	pb, ok := types.Unalias(sig.Params().At(0).Type()).(*types.Basic)
	if !ok || pb.Kind() != types.Uint8 {
		return false
	}
	return types.Identical(sig.Results().At(0).Type(), types.Universe.Lookup("error").Type())
}

// ps5003StdPair reports whether wr and wb are the standard library's own
// Write/WriteByte pair on the same fix-eligible writer — bytes.Buffer or
// strings.Builder, the two whose Write and WriteByte are proven to append
// identically and never error. Promotion through an embedded std writer
// still resolves both methods to the std objects, so embedding matches
// too; a shadowing custom method would resolve shallower and fail the
// pairing. bufio.Writer is deliberately absent: its Write may flush and
// surface an error mid-stream, so the pairing proof does not carry.
func ps5003StdPair(wr, wb *types.Func) bool {
	name, ok := ps5003StdRecv(wr)
	if !ok {
		return false
	}
	wbName, ok := ps5003StdRecv(wb)
	return ok && wbName == name
}

// ps5003StdRecv returns the "path.Type" of f's receiver when f is a method
// of one of the two fix-eligible standard writers.
func ps5003StdRecv(f *types.Func) (string, bool) {
	if f.Pkg() == nil {
		return "", false
	}
	r := ps5003Recv(f)
	if r == nil {
		return "", false
	}
	t := r.Type()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return "", false
	}
	key := f.Pkg().Path() + "." + named.Obj().Name()
	switch key {
	case "bytes.Buffer", "strings.Builder":
		return key, true
	}
	return "", false
}

// ps5003Recv returns f's receiver variable, or nil for a non-method.
func ps5003Recv(f *types.Func) *types.Var {
	sig, ok := f.Type().(*types.Signature)
	if !ok {
		return nil
	}
	return sig.Recv()
}

// ps5003ResultDiscarded reports whether the call's results are unused: the
// call is directly an expression statement, or the callee of a go/defer
// statement. Parentheses around the call are looked through.
func ps5003ResultDiscarded(call *ast.CallExpr, stack []ast.Node) bool {
	i := len(stack) - 1
	for ; i >= 0; i-- {
		if _, ok := stack[i].(*ast.ParenExpr); !ok {
			break
		}
	}
	if i < 0 {
		return false
	}
	switch p := stack[i].(type) {
	case *ast.ExprStmt:
		return true
	case *ast.GoStmt:
		return p.Call == call
	case *ast.DeferStmt:
		return p.Call == call
	}
	return false
}

// ps5003CommentIn reports whether any comment of f overlaps [pos, end) —
// text the fix would delete.
func ps5003CommentIn(f *ast.File, pos, end token.Pos) bool {
	for _, cg := range f.Comments {
		if cg.End() > pos && cg.Pos() < end {
			return true
		}
	}
	return false
}

// ps5003ExprText renders e for splicing into the diagnostic message.
func ps5003ExprText(e ast.Expr) (string, bool) {
	var b strings.Builder
	if err := printer.Fprint(&b, token.NewFileSet(), e); err != nil {
		return "", false
	}
	return b.String(), true
}
