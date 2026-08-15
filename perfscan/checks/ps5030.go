package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5030 reports strings.IndexAny / strings.ContainsAny whose cutset is a
// compile-time constant string of exactly ONE valid multi-byte rune: for
// such a cutset len(chars) >= 2, so IndexAny skips its one-byte fast path
// AND its ASCII-set fast path (the cutset is not ASCII) and falls into
// the general fallback that UTF-8-decodes EVERY rune of the haystack,
// probing the cutset per rune. strings.IndexRune / strings.ContainsRune
// run a single optimized substring scan over the rune's UTF-8 encoding
// instead. The direct multi-byte sibling of PS5022, which deliberately
// stops at a one-ASCII-byte cutset.
var PS5030 = register(&lint.Check{
	ID:       "PS5030",
	Category: "arith",
	Slug:     "indexany-single-multibyte-rune-cutset",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "IndexAny/ContainsAny with a one-multi-byte-rune cutset decodes every haystack rune; IndexRune/ContainsRune run one substring scan",
		Text: `strings.IndexAny(s, chars) — and strings.ContainsAny, which is
literally defined as IndexAny(s, chars) >= 0 — dispatches on the CUTSET:
an empty cutset is a constant -1, a one-BYTE cutset delegates to
IndexRune (PS5022's territory when that byte is ASCII), and an all-ASCII
cutset of a haystack longer than 8 bytes builds a 256-bit ASCII set and
scans bytewise. A cutset that is a single MULTI-BYTE rune — "—", "€",
"あ", "😀" — hits none of those: len(chars) >= 2 skips the one-byte
path, makeASCIISet fails on the non-ASCII bytes, and the call lands in
the general fallback

    for i, c := range s {
        if IndexRune(chars, c) >= 0 { return i }
    }

which UTF-8-DECODES EVERY RUNE of the haystack and, per rune, pays a
non-inlined IndexRune probe into the cutset. strings.IndexRune(s, '—')
for a valid multi-byte rune instead runs a single substring scan for the
rune's UTF-8 encoding (a bytealg/SIMD-assisted search keyed on the
encoding's last byte — no per-rune decode of the haystack at all): the
O(runes) decode-and-probe loop collapses to one memchr-style pass.
ContainsRune(s, r) is IndexRune(s, r) >= 0, so the membership twin
inherits the same saving. Neither form allocates — the win is pure scan
cost, and it GROWS with the haystack. This is the direct multi-byte
sibling of PS5022 (IndexAny/ContainsAny of a one-ASCII-byte cutset ->
IndexByte), which deliberately leaves multi-byte cutsets out.

Only a cutset that is a compile-time constant string holding EXACTLY one
rune whose UTF-8 encoding is 2..4 bytes long, decodes cleanly, and is
not utf8.RuneError is matched. Every bound is load-bearing:

  - exactly one rune, >= 2 bytes: a multi-character cutset is a genuine
    SET search IndexRune cannot express; an empty cutset is a constant
    -1/false; a one-byte cutset is PS5022's (ASCII) or excluded
    entirely (a lone non-ASCII byte makes IndexAny search for invalid
    UTF-8 — see PS5022's divergence witness);
  - the encoding must decode cleanly as one rune: constants holding
    truncated ("\xe2\x80"), overlong ("\xc0\xaf") or surrogate
    ("\xed\xa0\x80") byte sequences decode as RuneError-per-byte, take
    per-byte membership semantics in the fallback, and have no
    single-rune equivalent;
  - rune != utf8.RuneError: IndexRune's DOCUMENTED contract for
    RuneError is a different question — "the first instance of any
    invalid UTF-8 byte sequence" — so a "�" cutset is excluded
    conservatively rather than tied to the current coincidence of the
    two implementations (the equivalence suite pins today's behavior so
    a stdlib change is caught by a test, not a user).

Under those guards the identity is exact for ALL inputs, valid UTF-8 or
not. IndexAny's fallback returns the first 'range s' offset whose
decoded rune equals the cutset rune r (no other rune can match: every
byte of r's encoding is >= 0x80, so an ASCII haystack rune never probes
into it; UTF-8 is self-synchronizing, so no other rune's encoding is a
substring of r's; and an invalid haystack byte decodes to RuneError,
which is not in the cutset). IndexRune returns the first BYTE offset of
r's encoding. These coincide: r's lead byte is never a UTF-8
continuation byte, so no valid haystack rune can span across an
occurrence of r, and Go's decoder consumes exactly one byte on every
invalid sequence — 'range s' therefore always lands a fresh decode
exactly on the first occurrence's start and never matches earlier. The
haystack expression passes through untouched, evaluated exactly once in
the same position, and the callee is pinned by type information — a
shadowed strings identifier or a same-named method never matches.
bytes.IndexAny/ContainsAny are a different implementation and are out
of this check's scope.

The automatic fix renames the callee (IndexAny -> IndexRune,
ContainsAny -> ContainsRune; an aliased strings import keeps its
qualifier verbatim, and both replacements live in the same package, so
the import can never be orphaned) and replaces the string-literal
cutset with the equivalent rune literal rendered by strconv.QuoteRune —
'—' for a printable rune, a backslash-u escape such as '\u00a0' for a
non-printable one — always a single token denoting exactly the
decoded rune, whatever the source spelling ("—", "\xe2\x80\x94" and a raw backquoted literal all
land on the same rune). Both rewrites are CALL-FOR-CALL with the same
result type (int -> int, bool -> bool), so every syntactic position is
safe: conditions, operands of ! / == / && (no parenthesization is ever
introduced), go and defer statements (the replacement is still a call
expression), and bare call statements. As in PS5022, a named constant
or constant expression cutset is advisory-only, because spelling out a
copy of its value would discard the symbolic name — decode the constant
by hand when rewriting. LastIndexAny is out of scope (a separate
backward-scan pattern). The fix touches no imports.`,
		Before: `i := strings.IndexAny(s, "—")
if strings.ContainsAny(line, "€") { ... }`,
		After: `i := strings.IndexRune(s, '—')
if strings.ContainsRune(line, '€') { ... }`,
		MeasuredWin: `BenchmarkPS5030 (64-byte haystack, the rune in the last
field, Apple M2 Pro, go1.26): strings.IndexAny(s, "—") 200.3 ns/op ->
strings.IndexRune(s, '—') 8.9 ns/op (~23x), and
strings.ContainsAny(s, "€") 171.6 ns/op ->
strings.ContainsRune(s, '€') 9.2 ns/op (~19x). 0 B/op and 0 allocs/op
on every side: IndexAny's fallback decodes all ~60 runes of the
haystack and pays a non-inlined IndexRune probe per rune, while
IndexRune runs one substring scan keyed on the encoding's last byte —
and the gap widens with the haystack.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5030",
		Doc:  "strings.IndexAny/ContainsAny with a one-multi-byte-rune constant cutset instead of the direct IndexRune/ContainsRune scan",
		Run:  runPS5030,
	},
})

func runPS5030(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps5030Match(pass, call)
			if m == nil {
				return true
			}
			repl := "IndexRune"
			result := "index"
			if m.fn == "ContainsAny" {
				repl = "ContainsRune"
				result = "boolean"
			}
			msg := "strings." + m.fn + " of the one-multi-byte-rune cutset " + m.argText +
				" falls into the fallback that decodes every rune of the haystack; strings." + repl +
				"(s, " + m.runeText + ") is a single substring scan over the rune's UTF-8 encoding — identical " +
				result + " for every input"
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: msg,
			}
			if m.lit == nil {
				// A named constant or constant expression: spelling out a
				// copy of its value as a rune literal would discard the
				// symbolic name — advisory only, decode the constant by
				// hand.
				diag.Message = msg + "; the cutset is a constant expression, not a string literal — rewrite to strings." + repl + " by hand"
			} else {
				// Rename the callee identifier and replace the literal
				// token with the equivalent rune literal. Both rewrites are
				// call-for-call with the same result type, so no context
				// bookkeeping (parens, statements, go/defer) is ever
				// needed, and no deleted span can hold a comment (both
				// replaced spans are single tokens).
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace " + m.fn + " with " + repl,
					TextEdits: []analysis.TextEdit{
						{Pos: m.sel.Sel.Pos(), End: m.sel.Sel.End(), NewText: []byte(repl)},
						{Pos: m.lit.Pos(), End: m.lit.End(), NewText: []byte(m.runeText)},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps5030Site is a matched strings.IndexAny/ContainsAny call with a
// one-multi-byte-rune constant cutset: the selector to rewrite, the
// function name, the cutset's literal token (nil when the constant is not
// a direct string literal — advisory only), the cutset's source text for
// the message, and the rune literal the fix substitutes.
type ps5030Site struct {
	sel      *ast.SelectorExpr
	fn       string
	lit      *ast.BasicLit
	argText  string
	runeText string
}

// ps5030Match matches a call to the standard library's package-level
// strings.IndexAny or strings.ContainsAny whose second argument is a
// compile-time constant string holding exactly ONE valid rune whose UTF-8
// encoding is 2..4 bytes and which is not utf8.RuneError. Type
// information pins the callee (a shadowed strings identifier, a
// same-named method, or a third-party package never matches). The decoded
// length must consume the WHOLE constant (one rune exactly), must be >= 2
// (one ASCII byte is PS5022's; a lone non-ASCII byte or a truncated /
// overlong / surrogate sequence decodes as RuneError and has per-byte
// fallback semantics no single-rune call expresses), and the rune must
// not be RuneError itself (IndexRune's documented RuneError contract is a
// different question — the first INVALID sequence). bytes.IndexAny /
// bytes.ContainsAny are a different implementation, out of scope here;
// LastIndexAny is a separate backward-scan pattern.
func ps5030Match(pass *analysis.Pass, call *ast.CallExpr) *ps5030Site {
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
	case "IndexAny", "ContainsAny":
	default:
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
	r, size := utf8.DecodeRuneInString(val)
	if size < 2 || size != len(val) || r == utf8.RuneError {
		// size < 2 excludes the empty cutset (constant -1/false), the
		// one-ASCII-byte cutset (PS5022's), and every invalid leading
		// sequence (the decoder consumes exactly 1 byte and yields
		// RuneError); size != len(val) excludes multi-rune cutsets
		// (genuine SET searches) and trailing garbage; r == RuneError
		// excludes a real "�" cutset, whose IndexRune contract is a
		// different question.
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
	return &ps5030Site{sel: sel, fn: fn.Name(), lit: lit, argText: argText, runeText: strconv.QuoteRune(r)}
}
