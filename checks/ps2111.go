package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2111 reports w.Write([]byte(s)) — a string converted to a byte slice
// only to be written — when w's type also provides io.StringWriter's
// WriteString, which writes the string without the copy.
var PS2111 = register(&lint.Check{
	ID:       "PS2111",
	Category: "alloc",
	Slug:     "write-bytes-of-string",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "w.Write([]byte(s)) allocates; w.WriteString(s) writes the string directly",
		Text: `Converting a string to []byte just to pass it to Write allocates
and copies the whole string on every call (the compiler's 32-byte stack
buffer only saves conversions of short, non-escaping strings). Writers
that implement io.StringWriter — bytes.Buffer, strings.Builder,
bufio.Writer, and most others — expose WriteString(s string) (int, error)
for exactly this case: it writes the string's bytes without materializing
a slice copy first. The io.StringWriter contract requires WriteString to
behave like Write with []byte(s), so the rewrite writes the same bytes
and returns the same (int, error).

The check matches a method call w.Write(x) whose single argument is a
conversion []byte(s), verified with type information: s must be a plain
(unnamed) string — a named string type is not assignable to WriteString's
parameter — and the receiver's method set must contain BOTH
Write([]byte) (int, error) and WriteString(string) (int, error), i.e. it
satisfies io.Writer and io.StringWriter with the exact signatures, so the
call's results keep their types. When Write was only reachable through an
automatic &w (a pointer-receiver method on an addressable value), the
pointer method set is consulted instead — the same auto-address makes
WriteString equally callable. A value whose Write is value-receiver but
whose WriteString is pointer-receiver is skipped: on a non-addressable
receiver the rewrite would not compile.

One position is excluded entirely: w.Write([]byte(s)) lexically inside
w's own WriteString method — the standard WriteString-delegates-to-Write
implementation. There the rewrite would make WriteString call itself
(unbounded recursion, a stack overflow that still compiles), and the
original delegation is already the correct code, so the check reports
nothing. Writing to a different object (a field like w.buf, another
variable) inside WriteString is still reported.

The automatic fix renames the selected method to WriteString and deletes
only the conversion's "[]byte(" and ")" around the argument, leaving the
receiver and the argument expression untouched in place — same
evaluation, same order, import-neutral (a []byte conversion references no
package). A comment inside the deleted conversion text suppresses the fix
and keeps the advisory report.`,
		Before: `buf.Write([]byte(s))`,
		After:  `buf.WriteString(s)`,
		MeasuredWin: `BenchmarkPS2111 (64 strings of ~83 bytes written to a
bytes.Buffer behind a Writer+StringWriter interface, Apple M2 Pro):
1368 ns/op -> 237 ns/op (~5.7x) and 6144 B/op, 64 allocs/op -> 0 B/op,
0 allocs/op — each []byte(s) heap-allocated and copied the string that
WriteString writes directly. (On a devirtualized concrete receiver
modern compilers may already elide the copy for non-escaping
arguments; behind an indirect call the allocation always happens.)`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2111",
		Doc:  "Write([]byte(s)) on an io.StringWriter instead of WriteString(s)",
		Run:  runPS2111,
	},
})

func runPS2111(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Write" {
				return true
			}
			// A method value selection only: package-qualified functions
			// (pkg.Write) and struct fields of function type do not appear
			// in Selections as MethodVal and are rejected.
			selInfo, ok := pass.TypesInfo.Selections[sel]
			if !ok || selInfo.Kind() != types.MethodVal {
				return true
			}
			conv, inner := ps2111StringConv(pass, call.Args[0])
			if conv == nil {
				return true
			}
			wt := pass.TypesInfo.TypeOf(sel.X)
			if wt == nil || !ps2111CanWriteString(wt) {
				return true
			}
			// Inside the receiver's own WriteString method the rewrite
			// w.WriteString(s) would call the enclosing method itself —
			// the classic WriteString-delegates-to-Write implementation.
			// The original code is the correct delegation: stay silent.
			if writeFixSelfDispatches(pass, call, sel.X, "WriteString") {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "w.Write([]byte(s)) allocates and copies the string; the receiver implements io.StringWriter, so w.WriteString(s) writes it directly",
			}
			// The fix keeps the receiver and argument expressions in place
			// (same evaluation, same order): it renames the selected method
			// and deletes only the conversion's "[]byte(" and ")". A comment
			// inside either deleted range would be destroyed — keep the
			// report advisory then.
			if !ps2111CommentIn(f, conv.Pos(), inner.Pos()) && !ps2111CommentIn(f, inner.End(), conv.End()) {
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace Write([]byte(...)) with WriteString(...)",
					TextEdits: []analysis.TextEdit{
						{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("WriteString")},
						{Pos: conv.Pos(), End: inner.Pos()},
						{Pos: inner.End(), End: conv.End()},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2111StringConv matches arg (parens stripped) as the conversion
// []byte(s) of a plain string: the conversion type must be an (alias of
// an) unnamed slice of the predeclared byte, and s must be a string whose
// static type is the unnamed basic string (an untyped string constant
// defaults to it). A NAMED string type is rejected — it is not assignable
// to WriteString's string parameter, so the rewrite would not compile.
func ps2111StringConv(pass *analysis.Pass, arg ast.Expr) (conv *ast.CallExpr, inner ast.Expr) {
	for {
		p, ok := arg.(*ast.ParenExpr)
		if !ok {
			break
		}
		arg = p.X
	}
	c, ok := arg.(*ast.CallExpr)
	if !ok || len(c.Args) != 1 || c.Ellipsis.IsValid() {
		return nil, nil
	}
	tv, ok := pass.TypesInfo.Types[c.Fun]
	if !ok || !tv.IsType() {
		return nil, nil
	}
	sl, ok := types.Unalias(tv.Type).(*types.Slice)
	if !ok {
		return nil, nil
	}
	eb, ok := types.Unalias(sl.Elem()).(*types.Basic)
	if !ok || eb.Kind() != types.Byte {
		return nil, nil
	}
	it := pass.TypesInfo.TypeOf(c.Args[0])
	if it == nil {
		return nil, nil
	}
	ib, ok := types.Default(it).(*types.Basic)
	if !ok || ib.Info()&types.IsString == 0 {
		return nil, nil
	}
	return c, c.Args[0]
}

// ps2111WriterStringWriter is the interface { Write([]byte) (int, error);
// WriteString(string) (int, error) } — io.Writer AND io.StringWriter with
// the exact signatures. Requiring both at once pins the called Write to
// io.Writer's shape, so swapping it for WriteString preserves the result
// types, and guarantees WriteString exists to swap to. Both names are
// exported, so satisfaction is package-independent.
var ps2111WriterStringWriter = func() *types.Interface {
	errType := types.Universe.Lookup("error").Type()
	sig := func(param types.Type) *types.Signature {
		return types.NewSignatureType(nil, nil, nil,
			types.NewTuple(types.NewVar(token.NoPos, nil, "", param)),
			types.NewTuple(
				types.NewVar(token.NoPos, nil, "", types.Typ[types.Int]),
				types.NewVar(token.NoPos, nil, "", errType)),
			false)
	}
	i := types.NewInterfaceType([]*types.Func{
		types.NewFunc(token.NoPos, nil, "Write", sig(types.NewSlice(types.Typ[types.Byte]))),
		types.NewFunc(token.NoPos, nil, "WriteString", sig(types.Typ[types.String])),
	}, nil)
	i.Complete()
	return i
}()

// ps2111CanWriteString reports whether a receiver of static type wt on
// which .Write was called can equally call .WriteString, with both
// methods in io.Writer/io.StringWriter shape.
//
//   - wt's own method set has both: always callable.
//   - wt's own method set lacks Write entirely, yet the Write call
//     compiled: the compiler auto-addressed an addressable receiver, so
//     the pointer method set applies — and makes WriteString callable
//     through the same auto-address.
//   - anything else (e.g. value-receiver Write with pointer-receiver
//     WriteString on a possibly non-addressable value): not provably
//     compilable, skip.
func ps2111CanWriteString(wt types.Type) bool {
	if types.Implements(wt, ps2111WriterStringWriter) {
		return true
	}
	if types.NewMethodSet(wt).Lookup(nil, "Write") == nil &&
		types.Implements(types.NewPointer(wt), ps2111WriterStringWriter) {
		return true
	}
	return false
}

// ps2111CommentIn reports whether any comment of f overlaps [pos, end) —
// text the fix would delete.
func ps2111CommentIn(f *ast.File, pos, end token.Pos) bool {
	for _, cg := range f.Comments {
		if cg.End() > pos && cg.Pos() < end {
			return true
		}
	}
	return false
}
