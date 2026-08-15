package checks

import (
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2043 reports len(hex.EncodeToString(b)) and len(enc.EncodeToString(b))
// (enc an encoding/base64 or encoding/base32 *Encoding): EncodeToString
// allocates and fills the WHOLE encoded string — 2*len(b) bytes for hex,
// ~4/3*len(b) for base64, ~8/5*len(b) for base32 — purely to read its length
// off the top. hex.EncodedLen(len(b)) / enc.EncodedLen(len(b)) returns that
// same integer with a couple of arithmetic ops, no allocation and no encode
// pass. The encoding sibling of PS2125's len-through-a-throwaway-conversion
// family.
var PS2043 = register(&lint.Check{
	ID:       "PS2043",
	Category: "alloc",
	Slug:     "len-encodetostring",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "len(EncodeToString(b)) encodes the whole input just to take a length; EncodedLen(len(b)) is the identical integer, allocation-free",
		Text: `hex.EncodeToString(b) allocates a 2*len(b)-byte buffer, encodes
every input byte into it and converts it to a string; taking only the
len of that string discards all of the work. hex.EncodedLen(len(b))
computes the identical integer with one multiplication. The same holds
for the method form on an encoding/base64 or encoding/base32 *Encoding:
enc.EncodeToString(b) allocates its output via EXACTLY
enc.EncodedLen(len(b)) — the stdlib implementation is literally
"buf := make([]byte, enc.EncodedLen(len(src))); enc.Encode(buf, src);
return string(buf)" and Encode always fills the whole buffer — so
len(enc.EncodeToString(b)) == enc.EncodedLen(len(b)) for EVERY input
(empty and nil included: both 0 with padding disabled or not) and EVERY
encoder variant, because the rewrite reuses the SAME receiver expression:
StdEncoding, URLEncoding, RawStdEncoding, a custom alphabet or a
WithPadding derivative each pair their own EncodedLen with their own
EncodeToString, padded or not.

Evaluation order and count are unchanged. The original evaluates the
receiver (the hex qualifier or the enc expression), then b, then calls;
the rewrite evaluates the same receiver expression, then len(b) — b once
— in the same order, so any side effect in the receiver or the argument
happens exactly once, unchanged. b is only measured, never encoded or
mutated. A nil *Encoding receiver panics identically in both forms
(EncodeToString's first act is calling EncodedLen on the receiver).

The match is pinned by type information: the outer callee must be the
predeclared builtin len (a shadowed len is rejected), and the inner
callee must resolve to encoding/hex's package-level EncodeToString
(reached through the package qualifier, alias-aware) or to the
EncodeToString METHOD of encoding/base64's or encoding/base32's
Encoding. For the method form the receiver expression's type must be
EXACTLY (a pointer to) that package's Encoding: a wrapper type embedding
*Encoding is skipped deliberately, because the wrapper could declare its
own EncodedLen that shadows the promoted one and the rewrite would
silently change callees. A same-named method on any other type never
matches.

The fix keeps both the receiver and b byte-verbatim in place — same
text, same single evaluation — and edits only the scaffolding:
len(X.EncodeToString(b)) becomes X.EncodedLen(len(b)). The replacement
is a call — a primary expression — so no outer parentheses are ever
needed, and the qualifier/receiver keeps referencing its package, so no
import is ever added or orphaned. Two shapes keep an advisory report
with no fix: an untyped nil argument (hex.EncodeToString(nil) compiles
but len(nil) does not — the rewrite must not manufacture a typed
expression) and a comment inside the rewritten scaffolding (it would be
destroyed).

An honest caveat on unreachable-length inputs: EncodedLen's arithmetic
can overflow int for len(b) near MaxInt/2 (hex) — inputs on which
EncodeToString itself can never return (its make would panic or exhaust
memory first, on 64-bit such a slice cannot exist at all). On those
crash-only inputs the rewrite returns the wrapped integer instead of
crashing; every input the original can actually complete on agrees
exactly.`,
		Before: `n := len(hex.EncodeToString(b))
m := len(base64.StdEncoding.EncodeToString(b))`,
		After: `n := hex.EncodedLen(len(b))
m := base64.StdEncoding.EncodedLen(len(b))`,
		MeasuredWin: `BenchmarkPS2043 (a 1 KiB []byte, Apple M2 Pro, go1.26):
len(hex.EncodeToString(b)) ~1083 ns/op, 4096 B/op, 2 allocs/op vs
hex.EncodedLen(len(b)) ~0.31 ns/op, 0 B/op, 0 allocs/op (~3500x faster;
the full encode pass and both allocations — the 2 KiB buffer and its
string conversion — disappear, and the win scales linearly with len(b)).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2043",
		Doc:  "len(EncodeToString(b)) allocates and encodes the whole input just to read a length; EncodedLen(len(b)) is the identical integer, allocation-free",
		Run:  runPS2043,
	},
})

func runPS2043(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			lenCall, ok := n.(*ast.CallExpr)
			if !ok || len(lenCall.Args) != 1 || lenCall.Ellipsis.IsValid() {
				return true
			}
			// The outer callee must be the predeclared builtin len — a
			// shadowed len resolves to some other object and is rejected.
			id, ok := lenCall.Fun.(*ast.Ident)
			if !ok || id.Name != "len" || pass.TypesInfo.Uses[id] != types.Universe.Lookup("len") {
				return true
			}
			// The argument must be DIRECTLY the EncodeToString call — a
			// result stored in a variable first may have other consumers.
			inner, ok := ast.Unparen(lenCall.Args[0]).(*ast.CallExpr)
			if !ok || len(inner.Args) != 1 || inner.Ellipsis.IsValid() {
				return true
			}
			sel, ok := ast.Unparen(inner.Fun).(*ast.SelectorExpr)
			if !ok {
				return true
			}
			kind, ok := ps2043Callee(pass, sel)
			if !ok {
				return true
			}
			arg := inner.Args[0]
			recvText := ps2043ExprText(sel.X)
			argText := ps2043ExprText(arg)
			diag := analysis.Diagnostic{
				Pos: lenCall.Pos(),
				End: lenCall.End(),
				Message: "len(" + recvText + ".EncodeToString(" + argText + ")) allocates and fills the whole " +
					kind + "-encoded string just to read its length; " + recvText + ".EncodedLen(len(" + argText +
					")) is the identical integer with no encode pass and no allocation",
			}
			if fix := ps2043Fix(pass, f, lenCall, sel, arg); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2043Callee pins sel to one of the three EncodeToString flavors via type
// information and returns the encoding's name for the diagnostic:
//
//   - encoding/hex's package-level func EncodeToString (receiver-less,
//     reached through the package qualifier — the only selector shape that
//     can reference a package-level func, so sel.X is the alias-aware
//     package name and sel.X.EncodedLen resolves to hex.EncodedLen);
//   - the EncodeToString METHOD of encoding/base64's or encoding/base32's
//     Encoding, with the receiver expression's type EXACTLY (a pointer to)
//     that Encoding — a wrapper type embedding *Encoding is rejected because
//     its own EncodedLen could shadow the promoted one and the rewrite would
//     change callees.
//
// A shadowed package, a same-named method on any other type, or a func-typed
// field selector all resolve to different objects and never match.
func ps2043Callee(pass *analysis.Pass, sel *ast.SelectorExpr) (kind string, ok bool) {
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != "EncodeToString" || fn.Pkg() == nil {
		return "", false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return "", false
	}
	switch fn.Pkg().Path() {
	case "encoding/hex":
		if sig.Recv() != nil {
			return "", false
		}
		return "hex", true
	case "encoding/base64":
		if sig.Recv() == nil || !ps2043RecvIsEncoding(pass, sel.X, "encoding/base64") {
			return "", false
		}
		return "base64", true
	case "encoding/base32":
		if sig.Recv() == nil || !ps2043RecvIsEncoding(pass, sel.X, "encoding/base32") {
			return "", false
		}
		return "base32", true
	}
	return "", false
}

// ps2043RecvIsEncoding reports whether x's type is exactly pkgPath's Encoding
// or a pointer to it, so that x.EncodedLen resolves to the stdlib method (no
// user code can add methods to a stdlib type, and no embedding wrapper can
// interpose a shadowing EncodedLen on the exact type).
func ps2043RecvIsEncoding(pass *analysis.Pass, x ast.Expr, pkgPath string) bool {
	t := pass.TypesInfo.TypeOf(x)
	if p, isPtr := t.(*types.Pointer); isPtr {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Name() == "Encoding" && obj.Pkg() != nil && obj.Pkg().Path() == pkgPath
}

// ps2043Fix builds the rewrite len(X.EncodeToString(b)) -> X.EncodedLen(len(b)),
// or returns nil when a guard fails and the report must stay advisory. X and b
// stay byte-verbatim in place — same expressions, same single evaluation in
// the same order — and only the scaffolding around them is edited, so no
// import is ever touched (the qualifier/receiver keeps referencing its
// package). The replacement is a call — a primary expression — so no outer
// parentheses are ever needed.
func ps2043Fix(pass *analysis.Pass, f *ast.File, lenCall *ast.CallExpr, sel *ast.SelectorExpr, arg ast.Expr) *analysis.SuggestedFix {
	// hex.EncodeToString(nil) compiles ([]byte parameter), but len(nil) does
	// not — an untyped nil argument cannot be kept verbatim inside len.
	if t, ok := pass.TypesInfo.TypeOf(arg).(*types.Basic); ok && t.Kind() == types.UntypedNil {
		return nil
	}
	// A comment inside the rewritten scaffolding would be destroyed — the
	// fix is withheld then and the report stays advisory.
	if ps2111CommentIn(f, lenCall.Pos(), sel.X.Pos()) ||
		ps2111CommentIn(f, sel.X.End(), arg.Pos()) ||
		ps2111CommentIn(f, arg.End(), lenCall.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + ps2043ExprText(sel.X) + ".EncodedLen(len(...))",
		TextEdits: []analysis.TextEdit{
			{Pos: lenCall.Pos(), End: sel.X.Pos(), NewText: nil},                    // drop "len("
			{Pos: sel.X.End(), End: arg.Pos(), NewText: []byte(".EncodedLen(len(")}, // ".EncodeToString(" -> ".EncodedLen(len("
			{Pos: arg.End(), End: lenCall.End(), NewText: []byte("))")},             // close both calls
		},
	}
}

// ps2043ExprText renders e for use in diagnostic messages.
func ps2043ExprText(e ast.Expr) string {
	var b strings.Builder
	if err := printer.Fprint(&b, token.NewFileSet(), e); err != nil {
		return "..."
	}
	return b.String()
}
