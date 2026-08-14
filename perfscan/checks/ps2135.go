package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2135 reports w.WriteString(string(b)) — a byte slice converted to a
// string only to be written — when w's type also provides io.Writer's
// Write, which writes the bytes without the copy. The method-form mirror
// of PS2111.
var PS2135 = register(&lint.Check{
	ID:       "PS2135",
	Category: "alloc",
	Slug:     "write-string-of-bytes",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "w.WriteString(string(b)) allocates; w.Write(b) writes the bytes directly",
		Text: `Converting a []byte to a string just to pass it to WriteString
allocates and copies the whole slice on every call (unlike some
string-to-[]byte conversions, a []byte-to-string conversion is never
elided when the result is passed to an interface-shaped callee). Writers
that implement io.StringWriter — bytes.Buffer, strings.Builder,
bufio.Writer, and most others — also implement io.Writer, whose
Write(p []byte) (int, error) writes the bytes without materializing a
string copy first. The io.StringWriter contract requires WriteString(s)
to behave like Write([]byte(s)), so WriteString(string(b)) and Write(b)
write the same bytes and return the same (int, error): both write
exactly b's bytes and report (len(b), err) — bit-identical.

The check matches a method call w.WriteString(x) whose single argument
is a conversion string(b), verified with type information: the
conversion target must be the plain (unnamed) predeclared string — a
named string type is not assignable to WriteString's parameter and
would never have compiled here anyway — and b's type must have an
(unaliased) underlying *types.Slice whose element is the predeclared
byte. That excludes string(rs) for a []rune (element rune, not byte —
and Write(rs) would not compile) and string(i) for an integer (not a
slice at all). The receiver's method set must contain BOTH
Write([]byte) (int, error) and WriteString(string) (int, error), i.e.
it satisfies io.Writer and io.StringWriter with the exact signatures,
so the call's results keep their types. When WriteString was only
reachable through an automatic &w (a pointer-receiver method on an
addressable value), the pointer method set is consulted instead — the
same auto-address makes Write equally callable. A value whose
WriteString is value-receiver but whose Write is pointer-receiver is
skipped: on a non-addressable receiver the rewrite would not compile.

One position is excluded entirely: w.WriteString(string(p)) lexically
inside w's own Write method — the standard Write-delegates-to-
WriteString implementation. There the rewrite would make Write call
itself (unbounded recursion, a stack overflow that still compiles), and
the original delegation is already the correct code, so the check
reports nothing. Writing to a different object (a field like w.buf,
another variable) inside Write is still reported.

The automatic fix renames the selected method to Write and deletes only
the conversion's "string(" and ")" around the argument, leaving the
receiver and the argument expression untouched in place — same
evaluation, same order, import-neutral (a string conversion references
no package). A comment inside the deleted conversion text suppresses
the fix and keeps the advisory report.`,
		Before: `buf.WriteString(string(b))`,
		After:  `buf.Write(b)`,
		MeasuredWin: `BenchmarkPS2135 (64 byte slices of ~83 bytes written to a
bytes.Buffer behind a Writer+StringWriter interface, Apple M2 Pro):
1456 ns/op -> 234 ns/op (~6.2x) and 6144 B/op, 64 allocs/op -> 0 B/op,
0 allocs/op — each string(b) heap-allocated and copied the slice that
Write writes directly. (Behind an indirect call the conversion always
escapes and allocates; on a devirtualized concrete receiver the copy
may still be needed because the callee retains the bytes.)`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2135",
		Doc:  "WriteString(string(b)) on an io.Writer instead of Write(b)",
		Run:  runPS2135,
	},
})

func runPS2135(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "WriteString" {
				return true
			}
			// A method value selection only: package-qualified functions
			// (pkg.WriteString) and struct fields of function type do not
			// appear in Selections as MethodVal and are rejected.
			selInfo, ok := pass.TypesInfo.Selections[sel]
			if !ok || selInfo.Kind() != types.MethodVal {
				return true
			}
			conv, inner := ps2135BytesConv(pass, call.Args[0])
			if conv == nil {
				return true
			}
			wt := pass.TypesInfo.TypeOf(sel.X)
			if wt == nil || !ps2135CanWrite(wt) {
				return true
			}
			// Inside the receiver's own Write method the rewrite w.Write(p)
			// would call the enclosing method itself — the classic
			// Write-delegates-to-WriteString implementation. The original
			// code is the correct delegation: stay silent.
			if writeFixSelfDispatches(pass, call, sel.X, "Write") {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "w.WriteString(string(b)) allocates and copies the byte slice; the receiver implements io.Writer, so w.Write(b) writes it directly",
			}
			// The fix keeps the receiver and argument expressions in place
			// (same evaluation, same order): it renames the selected method
			// and deletes only the conversion's "string(" and ")". A comment
			// inside either deleted range would be destroyed — keep the
			// report advisory then.
			if !ps2111CommentIn(f, conv.Pos(), inner.Pos()) && !ps2111CommentIn(f, inner.End(), conv.End()) {
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace WriteString(string(...)) with Write(...)",
					TextEdits: []analysis.TextEdit{
						{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Write")},
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

// ps2135BytesConv matches arg (parens stripped) as the conversion string(b)
// of a byte slice: the conversion type must be (an alias of) the unnamed
// predeclared string — a NAMED string type would not have been assignable
// to WriteString's string parameter in the first place — and b's type,
// unaliased, must have an underlying unnamed-or-named slice of the
// predeclared byte, so that b is assignable to Write's []byte parameter.
// A []rune (element rune, not byte) and an integer (not a slice) are
// rejected: for those, string(x) is a genuine transformation, not a copy.
func ps2135BytesConv(pass *analysis.Pass, arg ast.Expr) (conv *ast.CallExpr, inner ast.Expr) {
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
	tb, ok := types.Unalias(tv.Type).(*types.Basic)
	if !ok || tb.Kind() != types.String || tb.Info()&types.IsString == 0 {
		return nil, nil
	}
	it := pass.TypesInfo.TypeOf(c.Args[0])
	if it == nil {
		return nil, nil
	}
	sl, ok := types.Unalias(it).Underlying().(*types.Slice)
	if !ok {
		return nil, nil
	}
	eb, ok := types.Unalias(sl.Elem()).(*types.Basic)
	if !ok || eb.Kind() != types.Byte {
		return nil, nil
	}
	return c, c.Args[0]
}

// ps2135CanWrite reports whether a receiver of static type wt on which
// .WriteString was called can equally call .Write, with both methods in
// io.Writer/io.StringWriter shape. The exact mirror of
// ps2111CanWriteString, keyed on the method that was actually CALLED:
//
//   - wt's own method set has both: always callable.
//   - wt's own method set lacks WriteString entirely, yet the WriteString
//     call compiled: the compiler auto-addressed an addressable receiver,
//     so the pointer method set applies — and makes Write callable through
//     the same auto-address.
//   - anything else (e.g. value-receiver WriteString with pointer-receiver
//     Write on a possibly non-addressable value): not provably compilable,
//     skip.
func ps2135CanWrite(wt types.Type) bool {
	if types.Implements(wt, ps2111WriterStringWriter) {
		return true
	}
	if types.NewMethodSet(wt).Lookup(nil, "WriteString") == nil &&
		types.Implements(types.NewPointer(wt), ps2111WriterStringWriter) {
		return true
	}
	return false
}
