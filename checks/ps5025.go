package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5025 reports strings.LastIndexAny / bytes.LastIndexAny whose cutset
// is a compile-time constant string of exactly one ASCII byte: for that
// cutset the "any of a set" machinery collapses to a plain backward byte
// search, but only after the call has paid its per-call dispatch and —
// depending on the haystack length — either built a 32-byte ASCII bitset
// or run a reverse rune-decoding loop. strings.LastIndexByte /
// bytes.LastIndexByte dispatch straight into bytealg's reverse
// single-byte scan. The backward-scan sibling of PS5022 (IndexAny /
// ContainsAny), which deliberately leaves LastIndexAny out.
var PS5025 = register(&lint.Check{
	ID:       "PS5025",
	Category: "arith",
	Slug:     "lastindexany-single-ascii-cutset",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "LastIndexAny with a one-ASCII-byte cutset pays the set machinery for a plain backward byte search; LastIndexByte jumps straight to the scan",
		Text: `strings.LastIndexAny(s, chars) — and bytes.LastIndexAny — is a
non-inlined library call that, for a one-character cutset, still pays
its full per-call dispatch before doing any real work: the chars == ""
empty check, then for len(s) > 8 it BUILDS a makeASCIISet 32-byte
bitset (zeroing and populating it on every call) and runs a reverse
contains-loop over the haystack, and otherwise a rune conversion, a
utf8.RuneSelf comparison, and a reverse utf8.DecodeLastRune loop that
decodes every trailing rune just to compare it against a single ASCII
value. strings.LastIndexByte(s, "/"[0]) dispatches straight into
bytealg's reverse single-byte scan — the exact machinery the ASCII-set
path degenerates to — removing the set construction, the rune decoding
and one or two non-inlined call frames on every invocation. Neither
form allocates, so the win is pure instruction count, in exactly the
suffix-probing and path-splitting loops where these calls cluster.
This is the backward-scan sibling of PS5022 (IndexAny/ContainsAny of a
one-ASCII-byte cutset), which deliberately leaves LastIndexAny out.

Only a cutset that is a compile-time constant string of byte-length
EXACTLY 1 whose single byte is ASCII (< 0x80) is matched. Both bounds
are load-bearing:

  - the length rule is bytes, not runes: "é" (two bytes of UTF-8) and
    "" (zero bytes) are never matched — a multi-character cutset is a
    genuine SET search that LastIndexByte cannot express, and
    LastIndexAny(s, "") is a constant -1;
  - the ASCII bound exists because LastIndexAny REMAPS a non-ASCII
    cutset byte: for len(chars) == 1 it sets the target rune to
    rune(chars[0]) and, when that is >= utf8.RuneSelf (e.g. "\xff" or
    "\x80"), replaces it with utf8.RuneError and searches for the LAST
    INVALID-UTF-8 position — which is NOT LastIndexByte(s, 0xff).
    Those cutsets are excluded entirely (not even advisory: no
    byte-search rewrite exists for them). The equivalence suite pins a
    concrete divergence witness.

Under those constraints the identity is exact for ALL inputs, branch by
branch. The len(s) > 8 ASCII-set path checks the raw byte as.contains
(s[i]) back to front — for a one-byte set that is exactly the s[i] == c
comparison LastIndexByte performs. The reverse rune-decoding path
relies on ASCII being self-synchronizing in UTF-8: an ASCII byte can
never be a continuation byte (0x80–0xBF) or part of a multi-byte
sequence, so utf8.DecodeLastRune yields rune c at exactly the byte
offsets where s[i] == c, invalid UTF-8 elsewhere in the haystack
decodes to RuneError (never equal to an ASCII target), and the
backward walk finds the HIGHEST such offset first — precisely
LastIndexByte's answer. The len(s) == 1 fast paths agree too: both
sides return 0 iff s[0] == c (a non-ASCII s[0] is remapped toward
RuneError, which a one-ASCII-byte cutset never contains, so both
return -1). The haystack expression passes through untouched,
evaluated exactly once in the same position, and the callee is pinned
by type information — a shadowed strings/bytes identifier or a
same-named method never matches.

The automatic fix renames the callee (LastIndexAny -> LastIndexByte;
an aliased strings/bytes import keeps its qualifier verbatim, and
LastIndexByte lives in the same package, so the import can never be
orphaned) and wraps the ORIGINAL cutset literal as "/"[0] exactly like
PS5022 — reusing the literal token byte-for-byte, so no re-escaping is
ever performed and the byte value is exact whatever the source
spelling (escape sequences such as "\t" or "\x00" and raw backquoted
literals included; the compiler folds the literal index to an
immediate byte). The replacement stays a call returning the same int,
so it drops in everywhere — conditions, index expressions, go/defer —
with no parenthesization concerns, and both replaced spans are single
tokens, so no comment can ever sit inside deleted syntax. As in
PS5007/PS5022, a named constant or constant expression cutset is
advisory-only, because wrapping a spelled-out copy of its value would
discard the symbolic name — index the constant itself (c[0]) when
rewriting by hand. The forward IndexAny/ContainsAny direction is
PS5022's territory. The fix touches no imports.`,
		Before: `i := strings.LastIndexAny(s, "/")
j := bytes.LastIndexAny(b, "=")`,
		After: `i := strings.LastIndexByte(s, "/"[0])
j := bytes.LastIndexByte(b, "="[0])`,
		MeasuredWin: `BenchmarkPS5025 (61-byte haystack, needle in the last
field, Apple M2 Pro, go1.26): strings.LastIndexAny(s, "z") 3.15 ns/op
-> strings.LastIndexByte(s, "z"[0]) 0.65 ns/op (~4.9x), and
bytes.LastIndexAny(b, "z") 3.08 ns/op ->
bytes.LastIndexByte(b, "z"[0]) 0.65 ns/op (~4.7x); with the needle
only at byte 0 (a full backward scan): strings.LastIndexAny 32.4 ns/op
-> strings.LastIndexByte 21.2 ns/op (~1.5x), bytes.LastIndexAny
32.4 ns/op -> bytes.LastIndexByte 20.1 ns/op (~1.6x). 0 B/op and
0 allocs/op on every side: the win is pure instruction count — the
32-byte ASCII-set build and the reverse set-membership loop collapse
into bytealg's plain backward byte scan. The "z"[0] spelling is free —
the compiler folds it to a byte constant.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5025",
		Doc:  "strings/bytes LastIndexAny with a one-ASCII-byte constant cutset instead of the direct LastIndexByte backward scan",
		Run:  runPS5025,
	},
})

func runPS5025(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps5025Match(pass, call)
			if m == nil {
				return true
			}
			msg := m.pkg + ".LastIndexAny of the one-ASCII-byte cutset " + m.argText +
				" pays the cutset dispatch and the ASCII-set/rune-decoding machinery before the backward scan; " +
				m.pkg + ".LastIndexByte(" + m.hay + ", " + m.argText +
				"[0]) runs the same backward byte scan directly — identical index for every input"
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: msg,
			}
			if m.lit == nil {
				// A named constant or constant expression: wrapping a
				// spelled-out copy of its value would discard the symbolic
				// name — advisory only, index the constant by hand.
				diag.Message = msg + "; the cutset is a constant expression, not a string literal — rewrite to " + m.pkg + ".LastIndexByte by hand"
			} else {
				// Rename the callee identifier and replace the literal token
				// with ITSELF plus the [0] index — the literal's source bytes
				// carry over verbatim (lit.Value is the raw token), so no
				// re-escaping happens. Both replaced spans are single tokens,
				// so no comment can sit inside deleted syntax.
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace LastIndexAny with LastIndexByte",
					TextEdits: []analysis.TextEdit{
						{Pos: m.sel.Sel.Pos(), End: m.sel.Sel.End(), NewText: []byte("LastIndexByte")},
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

// ps5025Site is a matched strings/bytes LastIndexAny call with a
// one-ASCII-byte constant cutset: the selector to rewrite, the package
// name (which doubles as the message qualifier — "strings" or "bytes"),
// the conventional haystack placeholder for the message ("s" or "b"),
// the cutset's literal token (nil when the constant is not a direct
// string literal — advisory only), and the cutset's source text for the
// message.
type ps5025Site struct {
	sel     *ast.SelectorExpr
	pkg     string
	hay     string
	lit     *ast.BasicLit
	argText string
}

// ps5025Match matches a call to the standard library's package-level
// strings.LastIndexAny or bytes.LastIndexAny whose second argument (the
// cutset — a string in BOTH packages) is a compile-time constant string
// of byte-length exactly 1 whose single byte is ASCII (< 0x80). Type
// information pins the callee (a shadowed strings/bytes identifier, a
// same-named method, or a third-party package never matches). The
// constant's DECODED byte length is measured — "é" (two bytes) is out —
// and the ASCII bound is load-bearing: for a single byte >= 0x80
// LastIndexAny remaps the cutset to utf8.RuneError and searches for the
// LAST invalid-UTF-8 position, which LastIndexByte cannot express, so
// such cutsets never match at all.
func ps5025Match(pass *analysis.Pass, call *ast.CallExpr) *ps5025Site {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil {
		return nil
	}
	pkg := fn.Pkg().Path()
	if pkg != "strings" && pkg != "bytes" {
		return nil
	}
	if fn.Name() != "LastIndexAny" {
		// IndexAny/ContainsAny are PS5022's territory; LastIndex of a
		// one-byte SUBSTRING is PS5007/PS5013's.
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
	val := constant.StringVal(tv.Value)
	if len(val) != 1 || val[0] >= 0x80 {
		// Multi-byte and empty cutsets are genuine SET searches; a single
		// non-ASCII byte is remapped to utf8.RuneError by LastIndexAny —
		// no byte-search rewrite exists for either.
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
	hay := "s"
	if pkg == "bytes" {
		hay = "b"
	}
	return &ps5025Site{sel: sel, pkg: pkg, hay: hay, lit: lit, argText: argText}
}
