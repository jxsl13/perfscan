package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5070 reports w.WriteString(string(b)) — a []byte converted to a string
// only to be written — when w's type also provides io.Writer's Write, which
// writes the bytes without the copy. The method-call inverse of PS2111
// (w.Write([]byte(s)) -> w.WriteString(s)) and the method-receiver
// counterpart of PS2118 (io.WriteString(w, string(b)) -> w.Write(b)).
var PS5070 = register(&lint.Check{
	ID:       "PS5070",
	Category: "alloc",
	Slug:     "writestring-of-byte-conversion",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "w.WriteString(string(b)) allocates; w.Write(b) writes the bytes directly",
		Text: `Converting a []byte to a string just to pass it to a WriteString
method allocates and copies the whole slice on every call (the compiler's
small-buffer stack optimization only saves conversions of short,
non-escaping strings — behind an interface the copy always happens). When
the receiver also implements io.Writer — bytes.Buffer, strings.Builder,
bufio.Writer, os.File, and most others do — the direct w.Write(b) writes
exactly the same bytes with no copy.

The rewrite is BIT-IDENTICAL under the check's gate. It matches a method
call w.WriteString(x) whose single argument is a conversion string(b),
verified with type information: the conversion target is the predeclared
string type (not a named string type or a function named string), and b's
type is assignable to Write's []byte parameter (an unnamed []byte, or a
named slice of byte — []rune, a string, or a slice of a NAMED byte type
are excluded). The receiver's method set must contain BOTH
Write([]byte) (int, error) and WriteString(string) (int, error), i.e. it
satisfies io.Writer and io.StringWriter with the exact signatures — the
io.StringWriter contract requires WriteString to behave like Write with
the same bytes, so w.WriteString(string(b)) and w.Write(b) write the
identical bytes and return the identical (int, error) (the count is the
byte length either way, equal for string(b) and b). io.Writer forbids
Write from retaining or modifying its argument, so handing w the slice b
directly instead of the string's private copy changes nothing for a
conforming writer. When Write was only reachable through an automatic &w
(a pointer-receiver method on an addressable value), the pointer method
set is consulted — the same auto-address makes Write equally callable.

One position is excluded entirely: w.WriteString(string(b)) lexically
inside w's own Write method — a Write that delegates through WriteString.
There the rewrite w.Write(b) would make Write call itself (unbounded
recursion that still compiles), and the original delegation is already
correct, so the check reports nothing. Writing to a different object (a
field, another variable) inside Write is still reported.

The automatic fix renames the selected method to Write and deletes only
the conversion's "string(" and ")" around the argument, leaving the
receiver and argument expression untouched in place — same evaluation,
same order, import-neutral (a string conversion references no package). A
comment inside the deleted conversion text suppresses the fix and keeps
the advisory report.`,
		Before: `buf.WriteString(string(b))`,
		After:  `buf.Write(b)`,
		MeasuredWin: `64 slices of ~83 bytes written to a bytes.Buffer behind a
Writer+StringWriter interface (Apple M2 Pro, go1.26): 2241 ns/op ->
252 ns/op (~8.9x) and 6144 B/op, 64 allocs/op -> 0 B/op, 0 allocs/op —
each string(b) heap-allocated and copied the bytes that Write sends
directly. (On a devirtualized concrete receiver modern compilers may
already elide the copy for non-escaping arguments; behind an indirect
call the allocation always happens.)`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5070",
		Doc:  "WriteString(string(b)) on an io.Writer instead of Write(b)",
		Run:  runPS5070,
	},
})

func runPS5070(pass *analysis.Pass) (any, error) {
	byteSlice := types.NewSlice(types.Typ[types.Byte])
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
			// A method value selection only: package-qualified functions and
			// struct fields of function type are not MethodVal and are rejected.
			selInfo, ok := pass.TypesInfo.Selections[sel]
			if !ok || selInfo.Kind() != types.MethodVal {
				return true
			}
			conv, inner := ps5070ByteConv(pass, call.Args[0], byteSlice)
			if conv == nil {
				return true
			}
			wt := pass.TypesInfo.TypeOf(sel.X)
			if wt == nil || !ps2111CanWriteString(wt) {
				return true
			}
			// Inside the receiver's own Write method the rewrite w.Write(b)
			// would call the enclosing method itself — the classic
			// Write-delegates-to-WriteString implementation. The original code
			// is the correct delegation: stay silent.
			if writeFixSelfDispatches(pass, call, sel.X, "Write") {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "w.WriteString(string(b)) allocates and copies the slice; the receiver implements io.Writer, so w.Write(b) writes the bytes directly",
			}
			// The fix keeps the receiver and argument expressions in place
			// (same evaluation, same order): it renames the selected method and
			// deletes only the conversion's "string(" and ")". A comment inside
			// either deleted range would be destroyed — keep the report
			// advisory then.
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

// ps5070ByteConv matches arg (parens stripped) as the conversion string(b):
// the conversion target must be the predeclared string type (not a named
// string type, not a function named string), and b's type must be assignable
// to Write's []byte parameter — an unnamed []byte or a named slice of byte,
// so w.Write(b) compiles and writes the identical bytes. []rune, a plain
// string, or a slice of a NAMED byte type is rejected.
func ps5070ByteConv(pass *analysis.Pass, arg ast.Expr, byteSlice types.Type) (conv *ast.CallExpr, inner ast.Expr) {
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
	// The conversion target must be the predeclared string (an unnamed
	// *types.Basic of kind string); a named type with underlying string is
	// *types.Named and is rejected.
	b, ok := types.Unalias(tv.Type).(*types.Basic)
	if !ok || b.Kind() != types.String {
		return nil, nil
	}
	it := pass.TypesInfo.TypeOf(c.Args[0])
	if it == nil || !types.AssignableTo(it, byteSlice) {
		return nil, nil
	}
	return c, c.Args[0]
}
