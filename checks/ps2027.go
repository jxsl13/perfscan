package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2027 reports emptiness tests that materialize a bytes.Buffer's whole
// contents just to answer a one-word question: buf.String() == "" (resp.
// != ""), and len(buf.String()) compared against an integer constant.
// Buffer.String() heap-allocates and byte-copies the entire unread
// contents; buf.Len() reads the same answer in O(1) with no allocation.
// The automatic fix applies only when the receiver is provably non-nil
// (a value-typed bytes.Buffer, or &x / new(bytes.Buffer)); for other
// *bytes.Buffer receivers the report stays advisory, because
// (*bytes.Buffer)(nil).String() is "<nil>" while Len panics.
var PS2027 = register(&lint.Check{
	ID:       "PS2027",
	Category: "alloc",
	Slug:     "bytesbuffer-string-emptiness-test",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `buf.String() == "" copies the whole bytes.Buffer just to test emptiness; buf.Len() == 0 answers it in O(1)`,
		Text: `bytes.Buffer.String() heap-allocates a fresh string and
byte-copies the buffer's entire unread contents into it. Feeding that
string to an emptiness test — buf.String() == "", buf.String() != "",
or len(buf.String()) compared against a constant — pays a full-buffer
copy (and its allocation) for a one-word answer that buf.Len() reads
directly: Len() is a single O(1) integer subtraction, len(b.buf)-b.off,
with no copy and no allocation. The cost of the Before shape grows
linearly with the buffered data; the After shape is constant. This is
the common "has anything been written yet?" idiom, so the wasted copy
often sits inside a loop or on a hot logging/rendering path.

The rewrite is BIT-IDENTICAL under the fix's non-nil gate. For a
non-nil receiver, String() returns string(b.buf[b.off:]), so
len(buf.String()) and buf.Len() are the same int for every buffer
state — freshly zero, written, partially or fully drained by reads,
Reset, Truncate, arbitrary (including invalid-UTF-8) bytes — and a
string compares equal to "" exactly when its length is 0. Therefore
buf.String() == "" is true iff buf.Len() == 0, buf.String() != "" iff
buf.Len() != 0, and any comparison of len(buf.String()) against an
integer constant keeps its exact truth value with len(buf.String())
replaced by buf.Len(). Both sides are pure reads, evaluate the receiver
exactly once in the same position, and yield a plain bool, so side
effects and evaluation order are untouched. The one divergent input is
a nil *bytes.Buffer: String() special-cases it to "<nil>" (so
buf.String() == "" is false) while Len() dereferences and panics. The
fix therefore applies only when the receiver is provably non-nil: its
static type is the value type bytes.Buffer (a method call on an
addressable value takes the address of that value, which is never nil),
or it is an address-of expression &x, or a direct new(bytes.Buffer)
call. Any other *bytes.Buffer receiver keeps the report advisory —
rewrite by hand once the pointer is known non-nil.

The automatic fix applies only when type information proves the shape:
the callee resolves to the standard library's (*bytes.Buffer).String
method (a same-named method on another type, strings.Builder, a
fmt.Stringer interface, or a shadowed identifier never matches), the
receiver expression's static type is exactly bytes.Buffer or
*bytes.Buffer (types that merely embed a Buffer are out of scope: a
pointer embed can hide a nil), the len identifier is the builtin, and
the far operand is a compile-time constant — the empty string for the
String() comparison (matched by constant value, so a named empty-string
constant counts), an integer constant for the len form. The receiver
and the far operand of the len form carry over byte-verbatim; the fix
only renames String to Len, unwraps the len(...) call, and replaces the
empty-string operand with 0, so the replacement is a primary expression
that never needs parentheses. No imports change (bytes stays imported
for the receiver's type). A comment inside the syntax the fix would
delete withholds the fix (the report stays advisory) rather than
destroying the comment.`,
		Before: `if buf.String() == "" {
	return
}
for len(buf.String()) > 0 { drain(&buf) }`,
		After: `if buf.Len() == 0 {
	return
}
for buf.Len() > 0 { drain(&buf) }`,
		MeasuredWin: `BenchmarkPS2027 (bytes.Buffer holding 4 KiB, Apple M2
Pro, go1.26): buf.String() == "" 530.5 ns/op, 4096 B/op, 1 alloc/op ->
buf.Len() == 0 0.50 ns/op, 0 B/op, 0 allocs/op (~1000x,
allocation-free); the len form len(buf.String()) > 0 454.3 ns/op,
4096 B/op, 1 alloc/op -> buf.Len() > 0 0.46 ns/op, 0 B/op, 0 allocs/op
— the compiler does not elide the String() copy in either shape, and
the saving grows linearly with the buffered bytes.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2027",
		Doc:  `buf.String() == "" / len(buf.String()) compared against a constant copies the whole bytes.Buffer just to test emptiness; buf.Len() reads the same answer in O(1) with no allocation`,
		Run:  runPS2027,
	},
})

func runPS2027(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			be, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			m := ps2027Match(pass, be)
			if m == nil {
				return true
			}
			msg := m.beforeText + " copies the whole bytes.Buffer just to test emptiness; " +
				m.afterText + " reads the same answer in O(1) with no copy and no allocation"
			diag := analysis.Diagnostic{
				Pos:     be.Pos(),
				End:     be.End(),
				Message: msg,
			}
			switch {
			case !m.nonNil:
				// The receiver is a *bytes.Buffer we cannot prove non-nil:
				// (*bytes.Buffer)(nil).String() returns "<nil>" while Len
				// panics, so the rewrite is withheld.
				diag.Message = msg + `; the *bytes.Buffer receiver is not provably non-nil ((*bytes.Buffer)(nil).String() is "<nil>" while Len panics) — the automatic fix is withheld; rewrite by hand once the pointer is known non-nil`
			case ps2027CommentInSpans(f, m.edits):
				// A comment sits inside syntax the fix would rewrite:
				// withhold the fix rather than destroy the comment.
				diag.Message = msg + "; a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand"
			default:
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "test emptiness with " + m.recvText + ".Len() instead of materializing the contents",
					TextEdits: m.edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2027Site is a matched emptiness test: the fix edits, the receiver's
// source text, whether the receiver is provably non-nil, and the
// message's before/after renderings in the operands' original order.
type ps2027Site struct {
	edits      []analysis.TextEdit
	recvText   string
	nonNil     bool
	beforeText string
	afterText  string
}

// ps2027Match matches a binary comparison whose one operand is
// buf.String() (against a constant empty string, == / != only) or
// len(buf.String()) (against an integer constant, any comparison
// operator), with buf's static type exactly bytes.Buffer or
// *bytes.Buffer. Both operand orders are matched.
func ps2027Match(pass *analysis.Pass, be *ast.BinaryExpr) *ps2027Site {
	switch be.Op {
	case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
	default:
		return nil
	}
	if m := ps2027MatchSide(pass, be, be.X, be.Y, false); m != nil {
		return m
	}
	return ps2027MatchSide(pass, be, be.Y, be.X, true)
}

// ps2027MatchSide tries to match side as the buf.String() /
// len(buf.String()) operand and far as the constant operand. swapped
// records that side is be.Y, for rendering the message in source order.
func ps2027MatchSide(pass *analysis.Pass, be *ast.BinaryExpr, side, far ast.Expr, swapped bool) *ps2027Site {
	farTV, ok := pass.TypesInfo.Types[far]
	if !ok || farTV.Value == nil {
		return nil
	}

	// String-comparison form: buf.String() == "" / != "" (either order).
	if call, isCall := ps2108Unparen(side).(*ast.CallExpr); isCall {
		if sel, recvText, nonNil, isBuf := ps2027BufferStringCall(pass, call); isBuf {
			if (be.Op == token.EQL || be.Op == token.NEQ) &&
				farTV.Value.Kind() == constant.String && constant.StringVal(farTV.Value) == "" {
				farText, okText := ps5004ExprText(far)
				if !okText {
					return nil
				}
				sideText, okText := ps5004ExprText(side)
				if !okText {
					return nil
				}
				return &ps2027Site{
					edits: []analysis.TextEdit{
						{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Len")},
						{Pos: far.Pos(), End: far.End(), NewText: []byte("0")},
					},
					recvText:   recvText,
					nonNil:     nonNil,
					beforeText: ps2027Render(sideText, be.Op, farText, swapped),
					afterText:  ps2027Render(recvText+".Len()", be.Op, "0", swapped),
				}
			}
			return nil
		}
	}

	// len form: len(buf.String()) compared against an integer constant,
	// any comparison operator — buf.Len() is the identical int.
	lenCall, isCall := ps2108Unparen(side).(*ast.CallExpr)
	if !isCall || len(lenCall.Args) != 1 || lenCall.Ellipsis.IsValid() {
		return nil
	}
	lenIdent, isIdent := ps2108Unparen(lenCall.Fun).(*ast.Ident)
	if !isIdent || pass.TypesInfo.Uses[lenIdent] != types.Universe.Lookup("len") {
		return nil
	}
	inner, isCall := ps2108Unparen(lenCall.Args[0]).(*ast.CallExpr)
	if !isCall {
		return nil
	}
	sel, recvText, nonNil, isBuf := ps2027BufferStringCall(pass, inner)
	if !isBuf || farTV.Value.Kind() != constant.Int {
		return nil
	}
	farText, okText := ps5004ExprText(far)
	if !okText {
		return nil
	}
	sideText, okText := ps5004ExprText(side)
	if !okText {
		return nil
	}
	return &ps2027Site{
		edits: []analysis.TextEdit{
			// Delete the "len(" scaffolding (with any redundant parens
			// wrapped around the inner call) and its closing ")", then
			// rename String to Len; the receiver stays byte-verbatim.
			{Pos: lenCall.Pos(), End: inner.Pos()},
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Len")},
			{Pos: inner.End(), End: lenCall.End()},
		},
		recvText:   recvText,
		nonNil:     nonNil,
		beforeText: ps2027Render(sideText, be.Op, farText, swapped),
		afterText:  ps2027Render(recvText+".Len()", be.Op, farText, swapped),
	}
}

// ps2027BufferStringCall reports whether call is a nullary method call
// resolving to the standard library's (*bytes.Buffer).String on a
// receiver expression whose static type is exactly bytes.Buffer or
// *bytes.Buffer. It returns the selector (for the rename edit), the
// receiver's source text, and whether the receiver is provably non-nil:
// a value-typed bytes.Buffer (the implicit address of an addressable
// value is never nil), an address-of expression, or a direct
// new(bytes.Buffer) call. Types that merely embed a Buffer never match:
// a pointer embed can hide a nil receiver behind a non-pointer static
// type.
func ps2027BufferStringCall(pass *analysis.Pass, call *ast.CallExpr) (sel *ast.SelectorExpr, recvText string, nonNil, ok bool) {
	if len(call.Args) != 0 || call.Ellipsis.IsValid() {
		return nil, "", false, false
	}
	sel, isSel := ps2108Unparen(call.Fun).(*ast.SelectorExpr)
	if !isSel {
		return nil, "", false, false
	}
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || fn.Name() != "String" || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
		return nil, "", false, false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() == nil {
		return nil, "", false, false
	}
	recvTV, found := pass.TypesInfo.Types[sel.X]
	if !found || recvTV.Type == nil {
		return nil, "", false, false
	}
	switch t := types.Unalias(recvTV.Type).(type) {
	case *types.Named:
		if !ps2027IsBytesBuffer(t) {
			return nil, "", false, false
		}
		nonNil = true
	case *types.Pointer:
		named, isNamed := types.Unalias(t.Elem()).(*types.Named)
		if !isNamed || !ps2027IsBytesBuffer(named) {
			return nil, "", false, false
		}
		nonNil = ps2027ProvablyNonNil(pass, sel.X)
	default:
		return nil, "", false, false
	}
	recvText, okText := ps5004ExprText(sel.X)
	if !okText {
		return nil, "", false, false
	}
	return sel, recvText, nonNil, true
}

// ps2027IsBytesBuffer reports whether t is the standard library's
// bytes.Buffer named type.
func ps2027IsBytesBuffer(t *types.Named) bool {
	obj := t.Obj()
	return obj != nil && obj.Name() == "Buffer" && obj.Pkg() != nil && obj.Pkg().Path() == "bytes"
}

// ps2027ProvablyNonNil reports whether the pointer expression e can be
// proven non-nil syntactically: an address-of expression (the operand
// of & is addressable, and the address of an addressable operand is
// never nil) or a direct call to the builtin new.
func ps2027ProvablyNonNil(pass *analysis.Pass, e ast.Expr) bool {
	switch x := ps2108Unparen(e).(type) {
	case *ast.UnaryExpr:
		return x.Op == token.AND
	case *ast.CallExpr:
		id, isIdent := ps2108Unparen(x.Fun).(*ast.Ident)
		return isIdent && pass.TypesInfo.Uses[id] == types.Universe.Lookup("new")
	}
	return false
}

// ps2027Render renders one side of the comparison message in the
// operands' original source order.
func ps2027Render(sideText string, op token.Token, farText string, swapped bool) string {
	if swapped {
		return farText + " " + op.String() + " " + sideText
	}
	return sideText + " " + op.String() + " " + farText
}

// ps2027CommentInSpans reports whether a comment overlaps any span an
// edit would delete or replace — such a comment withholds the fix.
func ps2027CommentInSpans(f *ast.File, edits []analysis.TextEdit) bool {
	for _, e := range edits {
		if ps2111CommentIn(f, e.Pos, e.End) {
			return true
		}
	}
	return false
}
