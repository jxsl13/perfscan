package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5007 reports strings.Index(s, "z") and strings.LastIndex(s, "z")
// where the needle is a one-byte string literal: both route a single
// byte through the generic substring-search machinery, while
// strings.IndexByte / strings.LastIndexByte scan for the byte directly.
var PS5007 = register(&lint.Check{
	ID:       "PS5007",
	Category: "arith",
	Slug:     "index-single-byte-literal",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "Index/LastIndex of a one-byte string runs the substring machinery; IndexByte/LastIndexByte scan for the byte directly",
		Text: `strings.Index(s, "z") pays a per-call dispatch over the needle
length before it discovers len(substr) == 1 and delegates to the byte
scan; strings.IndexByte(s, 'z') skips straight to the SIMD single-byte
scan. strings.LastIndex is far worse: it has NO single-byte fast path at
all and runs its full Rabin-Karp reverse substring search for a one-byte
needle, while strings.LastIndexByte is a plain backward byte scan.
Neither form allocates — the win is pure instruction count, which
matters in the parsing and splitting loops where these calls cluster.

Only a needle that is a compile-time constant string of byte-length
EXACTLY 1 is matched. The length rule is bytes, not runes: a multi-byte
literal such as "é" (two bytes of UTF-8) is out of scope, while a raw
single-byte escape such as "\xff" or "\x00" — even when it is not valid
UTF-8 — is in scope, because for a one-byte needle Index/LastIndex
match that raw byte exactly as IndexByte/LastIndexByte do. The empty
needle is excluded by the same rule (strings.Index(s, "") is 0 and
LastIndex(s, "") is len(s) — semantics IndexByte cannot express).

The rewrite is BIT-IDENTICAL on every input. For len(substr) == 1,
strings.Index(s, substr) is specified (and implemented) as
IndexByte(s, substr[0]), and strings.LastIndex performs the same raw
byte-wise comparison backwards — no rune decoding, no case folding, no
UTF-8 validation anywhere on either path — so the returned index is
equal for every haystack: empty, needle absent, needle at either end,
repeated needles (first vs last occurrence covered separately per
function), NUL bytes, and invalid UTF-8 (pinned by the equivalence
suite over all 256 needle bytes crossed with adversarial and randomized
haystacks). The haystack argument passes through byte-verbatim —
Index's first parameter is already the plain string type IndexByte
takes, so there is no conversion, no named-type involvement, and no
method dispatch — and both arguments are evaluated exactly once, in the
original order.

The automatic fix rewrites the callee name (Index -> IndexByte,
LastIndex -> LastIndexByte; an aliased strings import keeps its
qualifier verbatim) and wraps the ORIGINAL needle literal as
"z"[0] — reusing the literal token byte-for-byte, so no re-escaping is
ever performed and the byte value is exact whatever the source
spelling (escape sequences and raw backquoted literals included). The
compiler constant-folds the literal index to an immediate byte, so the
rewritten form's codegen matches a hand-written byte literal — no
bounds check and no extra work at runtime (measured identical to
IndexByte(s, 'z')). The callee is pinned by type information to the
standard library's package-level strings.Index/strings.LastIndex — a
shadowed strings identifier or a same-named method never matches. The
fix applies only when the needle is a string literal in the source; a
named constant or constant expression of byte-length 1 is reported
advisory-only, because wrapping a spelled-out copy of its value would
discard the symbolic name — index the constant itself (c[0]) when
rewriting by hand. The fix touches no imports (strings stays imported)
and deletes no source text but the callee identifier, so no comment can
be destroyed.`,
		Before: `i := strings.Index(s, "z")
j := strings.LastIndex(s, "/")`,
		After: `i := strings.IndexByte(s, "z"[0])
j := strings.LastIndexByte(s, "/"[0])`,
		MeasuredWin: `BenchmarkPS5007 (61-byte haystack, needle in the last
field, Apple M2 Pro, go1.26): strings.Index(s, "z") 3.79 ns/op ->
strings.IndexByte(s, "z"[0]) 3.11 ns/op (~1.2x), and
strings.LastIndex(s, "z") 2.15 ns/op ->
strings.LastIndexByte(s, "z"[0]) 0.63 ns/op (~3.4x — LastIndex runs
its full Rabin-Karp reverse search even for one byte). 0 B/op and
0 allocs/op on every side: the win is pure instruction count. The
"z"[0] spelling is free — measured identical to a hand-written 'z'
(2.99 vs 3.02 ns), since the compiler folds it to a byte constant.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5007",
		Doc:  "strings.Index/LastIndex of a single-byte string literal instead of the direct IndexByte/LastIndexByte scan",
		Run:  runPS5007,
	},
})

func runPS5007(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps5007Match(pass, call)
			if m == nil {
				return true
			}
			msg := "strings." + m.fn + " of the single-byte string " + m.argText +
				" runs the generic substring search; strings." + m.fn + "Byte(s, " +
				m.argText + "[0]) finds the same byte directly — identical index for every input"
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: msg,
			}
			if m.lit == nil {
				// A named constant or constant expression: wrapping a
				// spelled-out copy of its value would discard the symbolic
				// name — advisory only, index the constant by hand.
				diag.Message = msg + "; the needle is a constant expression, not a string literal — rewrite to strings." + m.fn + "Byte by hand"
			} else {
				// Rename the callee identifier and replace the literal
				// token with ITSELF plus the [0] index — the literal's
				// source bytes carry over verbatim (lit.Value is the raw
				// token), so no re-escaping happens and no comment can sit
				// inside either replaced span.
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace " + m.fn + " with " + m.fn + "Byte",
					TextEdits: []analysis.TextEdit{
						{Pos: m.sel.Sel.Pos(), End: m.sel.Sel.End(), NewText: []byte(m.fn + "Byte")},
						{Pos: m.lit.Pos(), End: m.lit.End(), NewText: []byte(m.lit.Value + "[0]")},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps5007Site is a matched strings.Index/LastIndex call with a one-byte
// constant needle: the selector to rewrite, the function name, the
// needle's literal token (nil when the constant is not a direct string
// literal — advisory only), and the needle's source text for the message.
type ps5007Site struct {
	sel     *ast.SelectorExpr
	fn      string
	lit     *ast.BasicLit
	argText string
}

// ps5007Match matches a call to the standard library's package-level
// strings.Index or strings.LastIndex whose second argument is a
// compile-time constant string of byte-length exactly 1. Type information
// pins the callee (a shadowed strings identifier, a same-named method, or
// a third-party package never matches), and the constant's DECODED byte
// length is measured — so "é" (two bytes) is out and "\xff" (one raw
// byte) is in, matching the byte-wise semantics of the rewrite exactly.
func ps5007Match(pass *analysis.Pass, call *ast.CallExpr) *ps5007Site {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "strings" {
		return nil
	}
	switch fn.Name() {
	case "Index", "LastIndex":
	default:
		// IndexAny/LastIndexAny cut differently on multi-byte sets and
		// have their own byte forms only via this same length-1 argument
		// shape — but their needle is a SET, a separate pattern.
		return nil
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil
	}
	arg := ps2108Unparen(call.Args[1])
	tv, found := pass.TypesInfo.Types[arg]
	if !found || tv.Value == nil || tv.Value.Kind() != constant.String {
		return nil
	}
	if len(constant.StringVal(tv.Value)) != 1 {
		return nil
	}
	argText, ok := ps5004ExprText(arg)
	if !ok {
		return nil
	}
	lit, _ := arg.(*ast.BasicLit)
	if lit != nil && lit.Kind != token.STRING {
		lit = nil
	}
	return &ps5007Site{sel: sel, fn: fn.Name(), lit: lit, argText: argText}
}
