package checks

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2045 reports comparisons that materialize BOTH bytes.Buffers'
// whole contents just to compare them with each other:
// buf1.String() == buf2.String() (resp. !=). Buffer.String()
// heap-allocates and byte-copies the entire unread contents on every
// call, and here the comparison pays that twice — both copies exist
// before the == even runs. bytes.Equal(buf1.Bytes(), buf2.Bytes())
// reads each buffer through the zero-copy Bytes() view, bails out
// immediately on a length mismatch, and does a single memory compare —
// zero heap allocations versus two, and O(1) instead of
// O(len(buf1)+len(buf2)) on a length mismatch. The automatic fix
// applies only when BOTH receivers are provably non-nil (a value-typed
// bytes.Buffer, or &x / new(bytes.Buffer)), the comparison's result is
// used as a plain bool, and the file has a usable import of package
// bytes; otherwise the report stays advisory.
var PS2045 = register(&lint.Check{
	ID:       "PS2045",
	Category: "alloc",
	Slug:     "bytesbuffer-string-string-compare",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `buf1.String() == buf2.String() copies both whole bytes.Buffers just to compare them; bytes.Equal(buf1.Bytes(), buf2.Bytes()) is allocation-free`,
		Text: `bytes.Buffer.String() heap-allocates a fresh string and
byte-copies the buffer's entire unread contents into it. Comparing two
buffers via buf1.String() == buf2.String() (or !=) pays that
full-buffer copy-and-allocation TWICE — the length short-circuit of the
string comparison happens only after both copies already exist.
bytes.Equal(buf1.Bytes(), buf2.Bytes()) computes the identical answer
with none of it: each Bytes() returns the zero-copy b.buf[b.off:]
slice header (no allocation), Equal returns immediately when the
lengths differ, and otherwise runs a single memory compare — zero heap
allocations versus two, and O(1) instead of O(len(buf1)+len(buf2))
work on a length mismatch. This is strictly a bigger win than PS2031's
constant comparison (which removes ONE allocation): here two
whole-buffer allocations collapse to none, and neither operand gains a
conversion copy — both stay zero-copy Bytes() views. A mixed
comparison — buf.String() against a strings.Builder, fmt.Stringer, or
any other type's String() — is out of scope: only the bytes.Buffer
side has a zero-copy byte view.

The rewrite is BIT-IDENTICAL under the fix's gates. For a non-nil
receiver, String() returns string(b.buf[b.off:]) and Bytes() returns
b.buf[b.off:] — the same bytes — and both a string comparison and
bytes.Equal are pure byte-for-byte equality tests with no UTF-8 or
case interpretation, so buf1.String() == buf2.String() is true iff
bytes.Equal(buf1.Bytes(), buf2.Bytes()) is, for every pair of buffer
states (fresh, written, partially or fully drained, Reset, Truncate,
arbitrary — including invalid-UTF-8 — bytes). Neither String() nor
Bytes() mutates its receiver, so both receivers are evaluated exactly
once, in the unchanged left-to-right order, with no side-effect
change. The one divergent input is a nil *bytes.Buffer: String()
returns the sentinel "<nil>" (so nilBuf.String() == nilBuf2.String()
can even be TRUE) while Bytes() dereferences and panics. The fix
therefore applies only when BOTH receivers are provably non-nil: the
static type is the value type bytes.Buffer (a method call on an
addressable value takes the address of that value, which is never
nil), or the receiver is an address-of expression &x, or a direct
new(bytes.Buffer) call. Any other *bytes.Buffer receiver on either
side keeps the report advisory — rewrite by hand once the pointers
are known non-nil.

Two further gates keep the rewrite compile-safe, exactly as in
PS2031. A comparison yields an UNTYPED bool, which Go assigns to any
type whose underlying type is bool; bytes.Equal returns the typed
bool. When the comparison's type-checked result is anything but plain
bool (a named bool type, a bool type parameter), the rewrite would
not compile, and the report stays advisory. And the replacement names
the bytes package, so the fix requires the file to import "bytes"
under a usable name (its alias when aliased) that is not shadowed at
the comparison — a file that sees the Buffers only through another
package's values, or that dot-imports bytes, keeps the report
advisory rather than gaining an import edit.

The automatic fix applies only when type information proves the
shape: BOTH callees resolve to the standard library's
(*bytes.Buffer).String method (a same-named method on another type,
strings.Builder, a fmt.Stringer interface, or a shadowed identifier
never matches), and both receiver expressions' static types are
exactly bytes.Buffer or *bytes.Buffer (types that merely embed a
Buffer are out of scope: a pointer embed can hide a nil). The whole
comparison is replaced by a single call expression — a primary
expression, so no parentheses are ever needed; the != form becomes
!bytes.Equal(...), and a prefixed ! binds tighter than every binary
operator, so the replacement needs no parentheses there either. A
comment inside the syntax the fix would replace withholds the fix
(the report stays advisory) rather than destroying the comment.`,
		Before: `var buf1, buf2 bytes.Buffer
// ...
if buf1.String() == buf2.String() {
	return
}`,
		After: `var buf1, buf2 bytes.Buffer
// ...
if bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
	return
}`,
		MeasuredWin: `BenchmarkPS2045 (two bytes.Buffers holding 4 KiB each,
equal contents so the compare runs to the end, Apple M2 Pro, go1.26):
buf1.String() == buf2.String() 754 ns/op, 8192 B/op, 2 allocs/op ->
bytes.Equal(buf1.Bytes(), buf2.Bytes()) 76.0 ns/op, 0 B/op,
0 allocs/op (~10x, allocation-free) — the After time is the worst case
(an equal-bytes full compare); on a length mismatch it is O(1) while
the Before shape still copies both whole buffers first, so the saving
grows linearly with the buffered bytes.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2045",
		Doc:  `buf1.String() == buf2.String() copies both whole bytes.Buffers just to compare them; bytes.Equal(buf1.Bytes(), buf2.Bytes()) tests the same bytes with no copy and no allocation`,
		Run:  runPS2045,
	},
})

func runPS2045(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			be, ok := n.(*ast.BinaryExpr)
			if !ok || (be.Op != token.EQL && be.Op != token.NEQ) {
				return true
			}
			m := ps2045Match(pass, be)
			if m == nil {
				return true
			}

			qual, qualOK := ps2031BytesQualifier(pass, f, be.Pos())
			after := qual + ".Equal(" + m.leftText + ".Bytes(), " + m.rightText + ".Bytes())"
			if be.Op == token.NEQ {
				after = "!" + after
			}
			msg := m.beforeText + " copies both whole bytes.Buffers just to compare them; " +
				after + " tests the same bytes with no copy and no allocation"
			diag := analysis.Diagnostic{
				Pos:     be.Pos(),
				End:     be.End(),
				Message: msg,
			}
			switch {
			case !m.nonNil:
				// At least one receiver is a *bytes.Buffer we cannot prove
				// non-nil: (*bytes.Buffer)(nil).String() returns "<nil>"
				// while Bytes panics, so the rewrite is withheld.
				diag.Message = msg + `; a *bytes.Buffer receiver is not provably non-nil ((*bytes.Buffer)(nil).String() is "<nil>" while Bytes panics) — the automatic fix is withheld; rewrite by hand once the pointers are known non-nil`
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
					Message:   "compare the raw bytes with " + qual + ".Equal instead of materializing both contents",
					TextEdits: []analysis.TextEdit{{Pos: be.Pos(), End: be.End(), NewText: []byte(after)}},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2045Site is a matched two-buffer comparison: each receiver's source
// rendering (parenthesized when not primary, so appending .Bytes()
// keeps the parse), whether BOTH receivers are provably non-nil, and
// the message's before rendering in source order.
type ps2045Site struct {
	leftText   string
	rightText  string
	nonNil     bool
	beforeText string
}

// ps2045Match matches buf1.String() == / != buf2.String(), with both
// callees resolving to (*bytes.Buffer).String and both receivers'
// static types exactly bytes.Buffer or *bytes.Buffer. A mixed
// comparison (only one side a Buffer.String call) does not match —
// rewriting it would reintroduce a conversion copy on the other side.
func ps2045Match(pass *analysis.Pass, be *ast.BinaryExpr) *ps2045Site {
	leftCall, ok := ps2108Unparen(be.X).(*ast.CallExpr)
	if !ok {
		return nil
	}
	rightCall, ok := ps2108Unparen(be.Y).(*ast.CallExpr)
	if !ok {
		return nil
	}
	leftSel, leftText, leftNonNil, isBuf := ps2027BufferStringCall(pass, leftCall)
	if !isBuf {
		return nil
	}
	rightSel, rightText, rightNonNil, isBuf := ps2027BufferStringCall(pass, rightCall)
	if !isBuf {
		return nil
	}
	if !ps2108Primary(leftSel.X) {
		// Defensive: every receiver that parses as sel.X is already a
		// primary expression or parenthesized, but keep the appended
		// .Bytes() selector safe regardless.
		leftText = "(" + leftText + ")"
	}
	if !ps2108Primary(rightSel.X) {
		rightText = "(" + rightText + ")"
	}
	xText, okText := ps5004ExprText(be.X)
	if !okText {
		return nil
	}
	yText, okText := ps5004ExprText(be.Y)
	if !okText {
		return nil
	}
	return &ps2045Site{
		leftText:   leftText,
		rightText:  rightText,
		nonNil:     leftNonNil && rightNonNil,
		beforeText: xText + " " + be.Op.String() + " " + yText,
	}
}
