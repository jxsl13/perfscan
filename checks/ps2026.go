package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2026 reports len(buf.String()) on a bytes.Buffer — a call that
// heap-allocates and byte-copies the ENTIRE unread contents of the buffer
// only to take the length of the throwaway string — and rewrites it to
// buf.Len(), the identical int computed by one O(1) subtraction with zero
// allocation.
var PS2026 = register(&lint.Check{
	ID:       "PS2026",
	Category: "alloc",
	Slug:     "len-buffer-string",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "len(buf.String()) copies the whole bytes.Buffer just to measure it; buf.Len() is the same int in O(1)",
		Text: `bytes.Buffer.String is 'return string(b.buf[b.off:])': it
heap-allocates a fresh string and byte-copies the ENTIRE unread contents
of the buffer — O(n) time and O(n) allocation — and len(buf.String())
then throws that string away after reading its length. Buffer.Len is
'return len(b.buf) - b.off': a single integer subtraction, zero
allocation. The rewrite replaces a full-length malloc+memcpy with one
integer op; the saving grows linearly with the buffer size. (This win is
specific to bytes.Buffer: strings.Builder.String is a zero-copy unsafe
cast, so a Builder receiver is out of scope here.)

The rewrite is BIT-IDENTICAL under the check's gate.
len(string(b.buf[b.off:])) == len(b.buf)-b.off == Len() for every buffer
state, including the zero value (buf==nil, off==0: String() is "" with
len 0, Len() is 0). Both methods are pure reads — String does not
advance b.off or mutate anything — the receiver expression is evaluated
exactly once in both spellings, and both results have static type int.
The ONE divergence in the whole method pair is a nil *bytes.Buffer
receiver: (*Buffer)(nil).String() has an explicit nil-guard returning
"<nil>" (length 5) while (*Buffer)(nil).Len() dereferences nil and
panics. The automatic fix is therefore gated to receivers that cannot be
nil: an expression whose static type is the VALUE type bytes.Buffer
(the pointer-receiver call auto-addresses it; a compiling receiver is
addressable, and a value shape that faults while being addressed — a
deref (*p) or a p.field with nil p — faults identically in both
spellings, because the spec guarantees &x panics whenever evaluating x
would, before either method body runs), or a *bytes.Buffer expression
that is syntactically &x, new(bytes.Buffer), bytes.NewBuffer(...), or
bytes.NewBufferString(...) (all provably non-nil). Any other
*bytes.Buffer receiver — a plain pointer variable, field, parameter, or
call result — still gets a diagnostic, but ADVISORY only, spelling out
the nil divergence; the human proves non-nilness and rewrites by hand. Type information pins the
callee to the standard library's (*bytes.Buffer).String — a same-named
method on another type, a fmt.Stringer interface call, a shadowed len,
or a method promoted through an embedded field never matches — and the
receiver sub-expression is kept byte-verbatim: only the 'len('
scaffolding around it is deleted and 'String' renamed to 'Len', so the
replacement is a call expression legal wherever the original call was
and never needs parentheses. A comment inside the deleted scaffolding
withholds the fix (the report stays advisory) rather than destroying
the comment.`,
		Before: `var buf bytes.Buffer
buf.WriteString(payload)
n := len(buf.String())`,
		After: `var buf bytes.Buffer
buf.WriteString(payload)
n := buf.Len()`,
		MeasuredWin: `BenchmarkPS2026 (64 KiB buffer, Apple M2 Pro, go1.26):
len(buf.String()) 7198 ns/op, 73728 B/op, 1 alloc/op -> buf.Len()
0.38 ns/op, 0 B/op, 0 allocs/op (~18000x faster, allocation-free). The
Before side is one full-length malloc+memcpy of the buffer's unread
contents per call and scales O(n) with buffer size; the After side is a
single subtraction regardless of size.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2026",
		Doc:  "len(buf.String()) on a bytes.Buffer heap-allocates and copies the whole unread contents just to measure them; buf.Len() returns the identical int in O(1) with zero allocation",
		Run:  runPS2026,
	},
})

func runPS2026(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			lenCall, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps2026Match(pass, lenCall)
			if m == nil {
				return true
			}
			msg := "len(" + m.recvText + ".String()) heap-allocates and copies the whole bytes.Buffer just to measure it; " +
				m.recvText + ".Len() returns the identical int in O(1) with zero allocation"
			diag := analysis.Diagnostic{
				Pos:     lenCall.Pos(),
				End:     lenCall.End(),
				Message: msg,
			}
			switch {
			case !m.provablyNonNil:
				// A *bytes.Buffer receiver that is not provably non-nil:
				// the ONE divergence of the pair. Report, but advisory.
				diag.Message = msg + "; the automatic fix is withheld because a nil *bytes.Buffer diverges (String() returns \"<nil>\" — length 5 — where Len() panics) — prove the receiver pointer non-nil and rewrite by hand"
			case ps2026CommentInSpans(f, m.del):
				// A comment sits inside the scaffolding the fix would
				// delete: withhold the fix rather than destroy the comment.
				diag.Message = msg + "; a comment inside the syntax the fix would delete withholds the automatic fix — rewrite by hand"
			default:
				// Delete the len(...) scaffolding around the kept receiver
				// and rename the method: the receiver's source bytes carry
				// over verbatim and are evaluated exactly once, as before.
				edits := []analysis.TextEdit{
					{Pos: m.sel.Sel.Pos(), End: m.sel.Sel.End(), NewText: []byte("Len")},
				}
				for _, d := range m.del {
					edits = append(edits, analysis.TextEdit{Pos: d.pos, End: d.end})
				}
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "replace len(" + m.recvText + ".String()) with " + m.recvText + ".Len()",
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2026Span is a source range of len(...) scaffolding the fix deletes.
type ps2026Span struct {
	pos, end token.Pos
}

// ps2026Site is a matched len(<recv>.String()) call on a bytes.Buffer:
// the String selector to rename, the scaffolding spans to delete, the
// receiver's source text for the message, and whether the receiver is
// provably non-nil (fix) or not (advisory).
type ps2026Site struct {
	sel            *ast.SelectorExpr
	del            []ps2026Span
	recvText       string
	provablyNonNil bool
}

// ps2026Match matches a call to the predeclared builtin len whose single
// argument is a direct call to the standard library's
// (*bytes.Buffer).String on a receiver whose static type is bytes.Buffer
// or *bytes.Buffer. Type information pins every part: a shadowed len, a
// same-named method on another type (including strings.Builder, whose
// String is zero-copy), a fmt.Stringer interface call, and a method
// promoted through an embedded field are all rejected — for a promoted
// method the outer type may define its own Len, or the promotion may run
// through a nil-able pointer field, so promoted shapes are out of scope
// entirely.
func ps2026Match(pass *analysis.Pass, lenCall *ast.CallExpr) *ps2026Site {
	if len(lenCall.Args) != 1 || lenCall.Ellipsis.IsValid() {
		return nil
	}
	// The outer callee must be the predeclared builtin len — a shadowed
	// len resolves to some other object and is rejected.
	id, ok := lenCall.Fun.(*ast.Ident)
	if !ok || id.Name != "len" || pass.TypesInfo.Uses[id] != types.Universe.Lookup("len") {
		return nil
	}
	inner, ok := ps2108Unparen(lenCall.Args[0]).(*ast.CallExpr)
	if !ok || len(inner.Args) != 0 || inner.Ellipsis.IsValid() {
		return nil
	}
	sel, ok := inner.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	// The callee must be the standard library's (*bytes.Buffer).String —
	// checked via the used object, so shadowing and same-named methods on
	// other types never match.
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != "String" || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
		return nil
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return nil
	}
	// The receiver expression's STATIC type must itself be bytes.Buffer
	// or *bytes.Buffer: a method promoted through an embedded field (the
	// receiver being some outer type) is out of scope.
	tv, found := pass.TypesInfo.Types[sel.X]
	if !found || tv.Type == nil {
		return nil
	}
	var provablyNonNil bool
	switch {
	case ps2026IsBuffer(tv.Type):
		// A VALUE of type bytes.Buffer: the pointer-receiver call
		// auto-addresses it (x.String() is (&x).String()), and the
		// receiver expression is necessarily addressable or the String
		// call would not have compiled. Every such shape is safe: a
		// variable, field, or element yields a real, non-nil address; a
		// shape that can panic while evaluating the receiver — (*p) or
		// p.field with a nil p — panics IDENTICALLY in both spellings,
		// because the spec guarantees &x panics whenever evaluating x
		// would (so (*p).String() and (*p).Len() both fault on the &*p
		// evaluation before either method body runs; String's nil-guard
		// is reachable only through an undereferenced nil POINTER
		// receiver, which is the *bytes.Buffer case below).
		provablyNonNil = true
	case ps2026IsBufferPtr(tv.Type):
		provablyNonNil = ps2026NonNilPtrExpr(pass, sel.X)
	default:
		return nil
	}
	recvText, okText := ps5004ExprText(sel.X)
	if !okText {
		return nil
	}
	return &ps2026Site{
		sel: sel,
		del: []ps2026Span{
			{pos: lenCall.Pos(), end: sel.X.Pos()}, // "len(", plus any wrapping "("s around the String call
			{pos: inner.End(), end: lenCall.End()}, // len's ")", plus any wrapping ")"s
		},
		recvText:       recvText,
		provablyNonNil: provablyNonNil,
	}
}

// ps2026IsBuffer reports whether t (through aliases) is the standard
// library's named type bytes.Buffer.
func ps2026IsBuffer(t types.Type) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == "bytes" && obj.Name() == "Buffer"
}

// ps2026IsBufferPtr reports whether t (through aliases) is *bytes.Buffer.
func ps2026IsBufferPtr(t types.Type) bool {
	ptr, ok := types.Unalias(t).(*types.Pointer)
	return ok && ps2026IsBuffer(ptr.Elem())
}

// ps2026NonNilPtrExpr reports whether the *bytes.Buffer expression e is
// PROVABLY non-nil by its syntax: &x (an address is never nil),
// new(bytes.Buffer) (new never returns nil), or a call to the standard
// library's bytes.NewBuffer / bytes.NewBufferString (documented
// constructors returning an initialized, non-nil buffer). Anything else —
// a pointer variable, field, parameter, or arbitrary call result — is not
// provable here, and the report stays advisory: a nil receiver is the one
// input where String() (len 5, "<nil>") and Len() (panic) diverge.
func ps2026NonNilPtrExpr(pass *analysis.Pass, e ast.Expr) bool {
	switch x := ps2108Unparen(e).(type) {
	case *ast.UnaryExpr:
		return x.Op == token.AND
	case *ast.CallExpr:
		switch fun := ps2108Unparen(x.Fun).(type) {
		case *ast.Ident:
			// new(bytes.Buffer): the predeclared builtin new never
			// returns nil. (The result type is already known to be
			// *bytes.Buffer from the caller's type check.)
			return fun.Name == "new" && pass.TypesInfo.Uses[fun] == types.Universe.Lookup("new")
		case *ast.SelectorExpr:
			fn, ok := pass.TypesInfo.Uses[fun.Sel].(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
				return false
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return false
			}
			return fn.Name() == "NewBuffer" || fn.Name() == "NewBufferString"
		}
	}
	return false
}

// ps2026CommentInSpans reports whether a comment overlaps any of the
// scaffolding spans the fix would delete.
func ps2026CommentInSpans(f *ast.File, spans []ps2026Span) bool {
	for _, d := range spans {
		if ps2111CommentIn(f, d.pos, d.end) {
			return true
		}
	}
	return false
}
