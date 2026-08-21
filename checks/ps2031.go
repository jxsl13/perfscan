package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2031 reports comparisons that materialize a bytes.Buffer's whole
// contents just to compare them against a compile-time string constant:
// buf.String() == "OK" (resp. != "OK", either operand order).
// Buffer.String() heap-allocates and byte-copies the entire unread
// contents on every call; bytes.Equal(buf.Bytes(), []byte("OK")) reads
// the buffer through the zero-copy Bytes() view, converts the tiny
// constant on the stack (the conversion does not escape into Equal),
// and bails out immediately on a length mismatch — O(1) with zero heap
// allocation versus O(len(buf)) plus a guaranteed allocation. The empty
// string is deliberately excluded: buf.String() == "" belongs to PS2027,
// whose buf.Len() == 0 rewrite is stronger. The automatic fix applies
// only when the receiver is provably non-nil (a value-typed
// bytes.Buffer, or &x / new(bytes.Buffer)), the comparison's result is
// used as a plain bool, and the file has a usable import of package
// bytes; otherwise the report stays advisory.
var PS2031 = register(&lint.Check{
	ID:       "PS2031",
	Category: "alloc",
	Slug:     "bytesbuffer-string-constant-compare",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `buf.String() == "lit" copies the whole bytes.Buffer just to compare it; bytes.Equal(buf.Bytes(), []byte("lit")) is allocation-free`,
		Text: `bytes.Buffer.String() heap-allocates a fresh string and
byte-copies the buffer's entire unread contents into it. Comparing that
string against a short compile-time constant — buf.String() == "OK",
buf.String() != "OK", in either operand order — pays a full-buffer copy
(and its allocation) for an answer the == usually short-circuits on the
length mismatch anyway, so almost the whole copy is pure waste.
bytes.Equal(buf.Bytes(), []byte("OK")) computes the identical answer
with none of it: Bytes() returns the zero-copy b.buf[b.off:] slice
header, the []byte conversion of the constant is stack-allocated (its
operand is a constant and the argument does not escape into Equal), and
Equal returns immediately when the lengths differ — O(1) with zero heap
allocation versus O(len(buf)) plus one guaranteed allocation on every
evaluation. Note gc's string(b) == "lit" no-allocation optimization
does NOT apply to the Before shape: buf.String() is a method call, not
a string([]byte) conversion, so its allocation is real. The empty
string is deliberately excluded — buf.String() == "" is PS2027's
pattern, whose buf.Len() == 0 rewrite is stronger still. A
non-constant right-hand side is also deliberately excluded: []byte(s)
of a non-constant string would merely trade the buffer copy for an
operand copy.

The rewrite is BIT-IDENTICAL under the fix's gates. For a non-nil
receiver, String() returns string(b.buf[b.off:]) and Bytes() returns
b.buf[b.off:] — the same bytes — and both a string comparison and
bytes.Equal are pure byte-for-byte equality tests with no UTF-8 or
case interpretation, so buf.String() == "lit" is true iff
bytes.Equal(buf.Bytes(), []byte("lit")) is, for every buffer state
(fresh, written, partially or fully drained, Reset, Truncate,
arbitrary — including invalid-UTF-8 — bytes) and every constant. The
receiver is evaluated exactly once in the same position in both forms,
and the other operand is a compile-time constant, so side effects and
evaluation order are untouched. The one divergent input is a nil
*bytes.Buffer: String() returns the sentinel "<nil>" (so a comparison
against "<nil>" is even true) while Bytes() dereferences and panics.
The fix therefore applies only when the receiver is provably non-nil:
its static type is the value type bytes.Buffer (a method call on an
addressable value takes the address of that value, which is never
nil), or it is an address-of expression &x, or a direct
new(bytes.Buffer) call. Any other *bytes.Buffer receiver keeps the
report advisory — rewrite by hand once the pointer is known non-nil.

Two further gates keep the rewrite compile-safe. A comparison yields
an UNTYPED bool, which Go assigns to any type whose underlying type is
bool; bytes.Equal returns the typed bool. When the comparison's
type-checked result is anything but plain bool (a named bool type, a
bool type parameter), the rewrite would not compile, and the report
stays advisory. And the replacement names the bytes package, so the
fix requires the file to import "bytes" under a usable name (its alias
when aliased) that is not shadowed at the comparison — a file that
sees the Buffer only through another package's type, or that
dot-imports bytes, keeps the report advisory rather than gaining an
import edit.

The automatic fix applies only when type information proves the shape:
the callee resolves to the standard library's (*bytes.Buffer).String
method (a same-named method on another type, strings.Builder, a
fmt.Stringer interface, or a shadowed identifier never matches), the
receiver expression's static type is exactly bytes.Buffer or
*bytes.Buffer (types that merely embed a Buffer are out of scope: a
pointer embed can hide a nil), and the far operand is a compile-time
non-empty string constant (matched by constant value, so a named
constant counts and carries over verbatim into the []byte conversion).
The whole comparison is replaced by a single call expression — a
primary expression, so no parentheses are ever needed; the != form
becomes !bytes.Equal(...), and a prefixed ! binds tighter than every
binary operator, so the replacement needs no parentheses there either.
A comment inside the syntax the fix would replace withholds the fix
(the report stays advisory) rather than destroying the comment.`,
		Before: `var buf bytes.Buffer
// ...
if buf.String() == "OK" {
	return
}`,
		After: `var buf bytes.Buffer
// ...
if bytes.Equal(buf.Bytes(), []byte("OK")) {
	return
}`,
		MeasuredWin: `BenchmarkPS2031 (bytes.Buffer holding 4 KiB, Apple M2
Pro, go1.26): buf.String() == "OK" 676.4 ns/op, 4096 B/op, 1 alloc/op
-> bytes.Equal(buf.Bytes(), []byte("OK")) 0.90 ns/op, 0 B/op,
0 allocs/op (~750x, allocation-free) — the comparison short-circuits
on the length mismatch either way, but the Before shape copies the
whole buffer first, so the saving grows linearly with the buffered
bytes.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2031",
		Doc:  `buf.String() compared against a compile-time string constant copies the whole bytes.Buffer; bytes.Equal(buf.Bytes(), []byte(lit)) tests the same bytes with no copy and no allocation`,
		Run:  runPS2031,
	},
})

func runPS2031(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			be, ok := n.(*ast.BinaryExpr)
			if !ok || (be.Op != token.EQL && be.Op != token.NEQ) {
				return true
			}
			m := ps2031Match(pass, be)
			if m == nil {
				return true
			}

			qual, qualOK := ps2031BytesQualifier(pass, f, be.Pos())
			after := qual + ".Equal(" + m.recvText + ".Bytes(), []byte(" + m.farText + "))"
			if be.Op == token.NEQ {
				after = "!" + after
			}
			msg := m.beforeText + " copies the whole bytes.Buffer just to compare it against a constant string; " +
				after + " tests the same bytes with no copy and no allocation"
			diag := analysis.Diagnostic{
				Pos:     be.Pos(),
				End:     be.End(),
				Message: msg,
			}
			switch {
			case !m.nonNil:
				// The receiver is a *bytes.Buffer we cannot prove non-nil:
				// (*bytes.Buffer)(nil).String() returns "<nil>" while
				// Bytes panics, so the rewrite is withheld.
				diag.Message = msg + `; the *bytes.Buffer receiver is not provably non-nil ((*bytes.Buffer)(nil).String() is "<nil>" while Bytes panics) — the automatic fix is withheld; rewrite by hand once the pointer is known non-nil`
			case !ps2031PlainBoolResult(pass, be):
				// The comparison's untyped-bool result flows into a
				// non-basic bool type (named bool, bool type parameter):
				// bytes.Equal's typed bool would not compile there.
				diag.Message = msg + "; the comparison's untyped bool result flows into a named bool type, which bytes.Equal's typed bool would not satisfy — the automatic fix is withheld; rewrite by hand with an explicit conversion"
			case !qualOK:
				// The replacement must name package bytes, but this file
				// has no usable import of it at this position.
				diag.Message = msg + "; this file has no usable import of package bytes at this position (missing, blank, dot, or shadowed) — the automatic fix is withheld; rewrite by hand"
			case ps2111CommentIn(f, be.Pos(), be.End()):
				// A comment sits inside the syntax the fix would replace:
				// withhold the fix rather than destroy the comment.
				diag.Message = msg + "; a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand"
			default:
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "compare the raw bytes with " + qual + ".Equal instead of materializing the contents",
					TextEdits: []analysis.TextEdit{{Pos: be.Pos(), End: be.End(), NewText: []byte(after)}},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2031Site is a matched constant-string comparison: the receiver's
// source rendering (parenthesized when not primary, so appending
// .Bytes() keeps the parse), the []byte operand's rendering, whether
// the receiver is provably non-nil, and the message's before rendering
// in the operands' original order.
type ps2031Site struct {
	recvText   string
	farText    string
	nonNil     bool
	beforeText string
}

// ps2031Match matches buf.String() == / != a non-empty compile-time
// string constant, with buf's static type exactly bytes.Buffer or
// *bytes.Buffer. Both operand orders are matched. The empty string is
// excluded — that comparison is PS2027's, with the stronger
// buf.Len() == 0 rewrite.
func ps2031Match(pass *analysis.Pass, be *ast.BinaryExpr) *ps2031Site {
	if m := ps2031MatchSide(pass, be, be.X, be.Y, false); m != nil {
		return m
	}
	return ps2031MatchSide(pass, be, be.Y, be.X, true)
}

// ps2031MatchSide tries to match side as the buf.String() operand and
// far as the constant operand. swapped records that side is be.Y, for
// rendering the message in source order.
func ps2031MatchSide(pass *analysis.Pass, be *ast.BinaryExpr, side, far ast.Expr, swapped bool) *ps2031Site {
	farTV, ok := pass.TypesInfo.Types[far]
	if !ok || farTV.Value == nil || farTV.Value.Kind() != constant.String ||
		constant.StringVal(farTV.Value) == "" {
		return nil
	}
	call, isCall := ps2108Unparen(side).(*ast.CallExpr)
	if !isCall {
		return nil
	}
	sel, recvText, nonNil, isBuf := ps2027BufferStringCall(pass, call)
	if !isBuf {
		return nil
	}
	if !ps2108Primary(sel.X) {
		// Defensive: every receiver that parses as sel.X is already a
		// primary expression or parenthesized, but keep the appended
		// .Bytes() selector safe regardless.
		recvText = "(" + recvText + ")"
	}
	farText, okText := ps5004ExprText(ps2108Unparen(far))
	if !okText {
		return nil
	}
	farAsIs, okText := ps5004ExprText(far)
	if !okText {
		return nil
	}
	sideText, okText := ps5004ExprText(side)
	if !okText {
		return nil
	}
	return &ps2031Site{
		recvText: recvText,
		farText:  farText,
		nonNil:   nonNil,
		beforeText: func() string {
			if swapped {
				return farAsIs + " " + be.Op.String() + " " + sideText
			}
			return sideText + " " + be.Op.String() + " " + farAsIs
		}(),
	}
}

// ps2031PlainBoolResult reports whether the comparison's type-checked
// result is a plain (possibly untyped) bool. A comparison yields an
// untyped bool, which go/types records with its FINAL type — the named
// bool type or bool type parameter it was assigned to, if any — and
// bytes.Equal's typed bool result would not compile in those contexts.
func ps2031PlainBoolResult(pass *analysis.Pass, be ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[be]
	if !ok || tv.Type == nil {
		return false
	}
	_, isBasic := types.Unalias(tv.Type).(*types.Basic)
	return isBasic
}

// ps2031BytesQualifier returns the identifier this file can use to name
// package bytes at pos: the import's name (its alias when aliased),
// provided it is not blank or dot and not shadowed at pos. ok is false
// when no usable import exists — the fix is withheld rather than
// adding one.
func ps2031BytesQualifier(pass *analysis.Pass, f *ast.File, pos token.Pos) (name string, ok bool) {
	for _, imp := range f.Imports {
		if imp.Path.Value != `"bytes"` {
			continue
		}
		local := "bytes"
		if imp.Name != nil {
			local = imp.Name.Name
		}
		if local == "_" || local == "." {
			continue
		}
		if ps2031NameIsBytes(pass, pos, local) {
			return local, true
		}
	}
	return "bytes", false
}

// ps2031NameIsBytes reports whether name resolves to package bytes at
// pos (not shadowed there by a local or other declaration).
func ps2031NameIsBytes(pass *analysis.Pass, pos token.Pos, name string) bool {
	scope := pass.Pkg.Scope().Innermost(pos)
	if scope == nil {
		return false
	}
	_, obj := scope.LookupParent(name, pos)
	pn, isPkg := obj.(*types.PkgName)
	return isPkg && pn.Imported() != nil && pn.Imported().Path() == "bytes"
}
