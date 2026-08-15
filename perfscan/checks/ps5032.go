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

// PS5032 reports bytes.IndexAny / bytes.ContainsAny whose cutset is a
// compile-time constant string of exactly ONE valid multi-byte rune: for
// such a cutset len(chars) >= 2, so IndexAny skips its one-byte-cutset
// fast path AND its ASCII-set fast path (the cutset is not ASCII) and
// falls into the general loop that UTF-8-decodes EVERY rune of the
// haystack, probing the cutset per rune. bytes.IndexRune /
// bytes.ContainsRune run a single optimized substring scan over the
// rune's UTF-8 encoding instead. The bytes twin of PS5030 (strings side).
var PS5032 = register(&lint.Check{
	ID:       "PS5032",
	Category: "arith",
	Slug:     "bytes-indexany-single-multibyte-rune-cutset",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "bytes.IndexAny/ContainsAny with a one-multi-byte-rune cutset decodes every haystack rune; IndexRune/ContainsRune run one substring scan",
		Text: `bytes.IndexAny(b, chars) — and bytes.ContainsAny, which is
literally defined as IndexAny(b, chars) >= 0 — dispatches on the CUTSET:
an empty cutset is a constant -1, a one-BYTE cutset delegates to
IndexRune, and an all-ASCII cutset of a haystack longer than 8 bytes
builds a 256-bit ASCII set and scans bytewise. A cutset that is a single
MULTI-BYTE rune — "—", "€", "あ", "😀" — hits none of those:
len(chars) >= 2 skips the one-byte path, makeASCIISet fails on the
non-ASCII bytes, and the call lands in the general loop

    for i := 0; i < len(s); i += width {
        ... utf8.DecodeRune(s[i:]) ...
        if bytealg.IndexString(chars, string(r)) >= 0 { return i }
    }

which UTF-8-DECODES EVERY multi-byte rune of the haystack (and pays a
bytealg.IndexByteString probe into the cutset for every ASCII byte),
comparing the cutset per decoded rune. bytes.IndexRune(b, '—') for a
valid multi-byte rune instead runs a single substring scan for the
rune's UTF-8 encoding (a bytealg/SIMD-assisted search keyed on the
encoding's last byte — no per-rune decode of the haystack at all): the
O(runes) decode-and-probe loop collapses to one memchr-style pass.
ContainsRune(b, r) is IndexRune(b, r) >= 0, so the membership twin
inherits the same saving. Neither form allocates — the win is pure scan
cost, and it GROWS with the haystack. This is the bytes twin of PS5030,
which covers the identical rewrite on the strings side.

Only a cutset that is a compile-time constant string holding EXACTLY one
rune whose UTF-8 encoding is 2..4 bytes long, decodes cleanly, and is
not utf8.RuneError is matched. Every bound is load-bearing:

  - exactly one rune, >= 2 bytes: a multi-character cutset is a genuine
    SET search IndexRune cannot express; an empty cutset is a constant
    -1/false; a one-byte cutset delegates differently (an ASCII byte is
    IndexByte territory, and a lone non-ASCII byte makes IndexAny
    search for RuneError — per-byte semantics no single-rune call
    expresses);
  - the encoding must decode cleanly as one rune: constants holding
    truncated ("\xe2\x80"), overlong ("\xc0\xaf") or surrogate
    ("\xed\xa0\x80") byte sequences decode as RuneError-per-byte, take
    per-byte membership semantics in the general loop, and have no
    single-rune equivalent;
  - rune != utf8.RuneError: IndexRune's DOCUMENTED contract for
    RuneError is a different question — "the first instance of any
    invalid UTF-8 byte sequence" — so a "�" cutset is excluded
    conservatively rather than tied to the current coincidence of the
    two implementations (the equivalence suite pins today's behavior so
    a stdlib change is caught by a test, not a user).

Under those guards the identity is exact for ALL inputs, valid UTF-8 or
not. IndexAny's general loop returns the first byte offset whose FRESH
decode equals the cutset rune r (no other position can match: every
byte of r's encoding is >= 0x80, so an ASCII haystack byte never probes
into it; UTF-8 is self-synchronizing, so no other rune's encoding is a
substring of r's; and an invalid haystack byte decodes to RuneError,
which is not the cutset rune). The bytes-specific len(b)==1 fast path
returns -1 on both sides (a one-byte haystack cannot hold a 2..4-byte
encoding, and its RuneError probe never finds the non-RuneError cutset).
IndexRune returns the first BYTE offset of r's encoding. These coincide:
r's lead byte is never a UTF-8 continuation byte, so no valid haystack
rune can span across an occurrence of r, and Go's decoder consumes
exactly one byte on every invalid sequence — the scan therefore always
lands a fresh decode exactly on the first occurrence's start and never
matches earlier. The haystack expression passes through untouched,
evaluated exactly once in the same position, and the callee is pinned by
type information — a shadowed bytes identifier or a same-named method
never matches. strings.IndexAny/ContainsAny are PS5030's territory.

The automatic fix renames the callee (IndexAny -> IndexRune,
ContainsAny -> ContainsRune; an aliased bytes import keeps its
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
expression), and bare call statements. As in PS5030, a named constant
or constant expression cutset is advisory-only, because spelling out a
copy of its value would discard the symbolic name — decode the constant
by hand when rewriting. LastIndexAny is out of scope (a separate
backward-scan pattern). The fix touches no imports.`,
		Before: `i := bytes.IndexAny(b, "—")
if bytes.ContainsAny(line, "€") { ... }`,
		After: `i := bytes.IndexRune(b, '—')
if bytes.ContainsRune(line, '€') { ... }`,
		MeasuredWin: `BenchmarkPS5032 (64-byte haystack, the rune in the last
field, Apple M2 Pro, go1.26): bytes.IndexAny(b, "—") 168.2 ns/op ->
bytes.IndexRune(b, '—') 7.3 ns/op (~23x), and
bytes.ContainsAny(b, "€") 124.6 ns/op ->
bytes.ContainsRune(b, '€') 7.4 ns/op (~17x). 0 B/op and 0 allocs/op
on every side: IndexAny's general loop walks all ~60 runes of the
haystack, paying a per-byte cutset probe for ASCII bytes and a
decode-plus-compare for multi-byte runes, while IndexRune runs one
substring scan keyed on the encoding's last byte — and the gap widens
with the haystack.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5032",
		Doc:  "bytes.IndexAny/ContainsAny with a one-multi-byte-rune constant cutset instead of the direct IndexRune/ContainsRune scan",
		Run:  runPS5032,
	},
})

func runPS5032(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps5032Match(pass, call)
			if m == nil {
				return true
			}
			repl := "IndexRune"
			result := "index"
			if m.fn == "ContainsAny" {
				repl = "ContainsRune"
				result = "boolean"
			}
			msg := "bytes." + m.fn + " of the one-multi-byte-rune cutset " + m.argText +
				" falls into the general loop that decodes every rune of the haystack; bytes." + repl +
				"(b, " + m.runeText + ") is a single substring scan over the rune's UTF-8 encoding — identical " +
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
				diag.Message = msg + "; the cutset is a constant expression, not a string literal — rewrite to bytes." + repl + " by hand"
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

// ps5032Site is a matched bytes.IndexAny/ContainsAny call with a
// one-multi-byte-rune constant cutset: the selector to rewrite, the
// function name, the cutset's literal token (nil when the constant is not
// a direct string literal — advisory only), the cutset's source text for
// the message, and the rune literal the fix substitutes.
type ps5032Site struct {
	sel      *ast.SelectorExpr
	fn       string
	lit      *ast.BasicLit
	argText  string
	runeText string
}

// ps5032Match matches a call to the standard library's package-level
// bytes.IndexAny or bytes.ContainsAny whose second argument is a
// compile-time constant string holding exactly ONE valid rune whose UTF-8
// encoding is 2..4 bytes and which is not utf8.RuneError. Type
// information pins the callee (a shadowed bytes identifier, a same-named
// method, or a third-party package never matches). The decoded length
// must consume the WHOLE constant (one rune exactly), must be >= 2 (a
// one-byte cutset delegates through IndexAny's own fast path; a lone
// non-ASCII byte or a truncated / overlong / surrogate sequence decodes
// as RuneError and has per-byte semantics no single-rune call
// expresses), and the rune must not be RuneError itself (IndexRune's
// documented RuneError contract is a different question — the first
// INVALID sequence). strings.IndexAny / strings.ContainsAny are PS5030's
// territory; LastIndexAny is a separate backward-scan pattern.
func ps5032Match(pass *analysis.Pass, call *ast.CallExpr) *ps5032Site {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
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
		// size < 2 excludes the empty cutset (constant -1/false), every
		// one-byte cutset (IndexAny's own delegation path), and every
		// invalid leading sequence (the decoder consumes exactly 1 byte
		// and yields RuneError); size != len(val) excludes multi-rune
		// cutsets (genuine SET searches) and trailing garbage; r ==
		// RuneError excludes a real "�" cutset, whose IndexRune contract
		// is a different question.
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
	return &ps5032Site{sel: sel, fn: fn.Name(), lit: lit, argText: argText, runeText: strconv.QuoteRune(r)}
}
