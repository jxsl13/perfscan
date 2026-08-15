package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5023 reports strings.IndexRune / bytes.IndexRune whose rune argument
// is a compile-time constant in [0, utf8.RuneSelf): for such a rune
// IndexRune's entire body is `return IndexByte(s, byte(r))` behind a
// multi-branch switch that keeps it above the inliner's cost budget, so
// every call pays a non-inlined wrapper frame plus the range check before
// reaching the assembly-optimized IndexByte intrinsic. The CONSTANT-rune
// sibling of PS5007/PS5013 (one-byte substring needles) and PS5022
// (one-ASCII-byte cutsets), which deliberately leave IndexRune out.
var PS5023 = register(&lint.Check{
	ID:       "PS5023",
	Category: "arith",
	Slug:     "indexrune-const-ascii",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "IndexRune with a constant ASCII rune pays a non-inlined range-check wrapper; IndexByte jumps straight to the scan",
		Text: `strings.IndexRune(s, 'z') — and bytes.IndexRune — dispatches over
the rune's range in a multi-branch switch (ASCII, utf8.RuneError,
invalid rune, genuine multi-byte search) whose very first case, for
0 <= r < utf8.RuneSelf, is nothing but "return IndexByte(s, byte(r))".
That switch keeps IndexRune above the inliner's cost budget, so a call
with a constant ASCII rune pays a full non-inlined call frame plus the
range comparison on every invocation before delegating to the
SIMD/bytealg byte scan IndexByte jumps to directly. Neither form
allocates — the win is pure instruction count, in exactly the parsing
and splitting loops where these probes cluster. This is the
CONSTANT-rune sibling of PS5007/PS5013 (Index/LastIndex of a one-byte
substring) and PS5022 (IndexAny/ContainsAny of a one-ASCII-byte
cutset), which deliberately leave IndexRune out.

Only a rune operand that is a compile-time constant with value in
[0, utf8.RuneSelf = 0x80) is matched. Every part of that gate is
load-bearing, because byte(r) TRUNCATES anything wider:

  - a NON-CONSTANT rune never matches, not even advisory: a variable
    can hold a non-ASCII value at runtime, where IndexRune searches for
    the rune's multi-byte UTF-8 encoding (or the first invalid-UTF-8
    position, for utf8.RuneError) while IndexByte(s, byte(r)) searches
    for a single truncated byte — genuinely different answers;
  - a constant >= 0x80 is excluded entirely: IndexRune encodes it to
    multi-byte UTF-8 ('é' searches for the two bytes C3 A9, never the
    single byte E9), and utf8.RuneError (0xFFFD) does not search at all
    but returns the first INVALID-UTF-8 position — neither is a byte
    search IndexByte can express;
  - a negative constant is excluded too: IndexRune(s, -1) hits the
    !utf8.ValidRune branch and is a constant -1, while byte(-1) of a
    runtime value would be 0xFF and could FIND something.

The equivalence suite pins a concrete divergence witness for each
excluded shape. An ASCII rune literal such as 'z', '\t', '\x00' or
'\x7f' is always in [0, 0x80), so it never touches the RuneError or
invalid-rune branches, and a single ASCII byte is its own UTF-8
encoding — UTF-8 validity of the haystack is irrelevant on both sides
(both do a raw byte scan; the equivalence suite crosses all 128 ASCII
runes with adversarial invalid-UTF-8 haystacks to pin exactly that).

Under that gate the identity is exact for ALL inputs: IndexRune's
first case returns IndexByte(s, byte(r)) verbatim — no rune decoding,
no UTF-8 validation, no case folding — so the returned index is
bit-identical for every haystack: nil or empty (both -1), needle
absent, needle at either end, repeated needles, NUL bytes, and invalid
UTF-8. Both return int. The haystack expression passes through
untouched, evaluated exactly once in the same position; the rune is a
compile-time constant, so it has no evaluation order or side-effect
concerns at all. The callee is pinned by type information — a shadowed
strings/bytes identifier or a same-named method never matches.

The automatic fix renames the callee (IndexRune -> IndexByte; an
aliased strings/bytes import keeps its qualifier verbatim, and
IndexByte lives in the same package, so the import can never be
orphaned) and leaves the rune literal token COMPLETELY untouched: an
untyped rune constant in [0, 0x80) is representable as byte, so 'z',
'\t', '\x00', 'A', an octal escape, or a plain integer literal
like 47 converts implicitly in IndexByte's argument position — the
compiler folds it to the same immediate byte, with no re-spelling and
no conversion syntax added. The replacement is still a call returning
the same int, so it drops into every syntactic position — comparisons,
index expressions, go/defer statements — with no parenthesization
concerns, and the only replaced span is the selected identifier
itself, so no comment can ever sit inside deleted syntax.

The fix applies only when the rune operand is a literal in the source
(rune or integer). A NAMED constant (or constant expression) in
[0, 0x80) is reported advisory-only: rewriting would either discard
the symbolic name by spelling out its value or — for a TYPED rune
constant — require inserting an explicit byte(...) conversion, so the
rewrite is left to a human (write IndexByte(s, byte(c)); drop the
conversion when c is untyped). ContainsRune is out of scope (its
rewrite splices in a >= 0 comparison — a separate pattern), and there
is no LastIndexRune in the standard library. The fix touches no
imports.`,
		Before: `i := strings.IndexRune(s, 'z')
j := bytes.IndexRune(b, '/')`,
		After: `i := strings.IndexByte(s, 'z')
j := bytes.IndexByte(b, '/')`,
		MeasuredWin: `BenchmarkPS5023 (61-byte haystack, needle in the last
field, Apple M2 Pro, go1.26): strings.IndexRune(s, 'z') 3.50 ns/op ->
strings.IndexByte(s, 'z') 3.09 ns/op (~1.1x); bytes.IndexRune(b, 'z')
4.27 ns/op -> bytes.IndexByte(b, 'z') 3.10 ns/op (~1.4x). 0 B/op and
0 allocs/op on every side: the win is pure instruction count — the
non-inlined IndexRune wrapper frame and its range-dispatch switch
disappear, leaving only the intrinsic byte scan.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5023",
		Doc:  "strings/bytes IndexRune with a constant ASCII rune instead of the direct IndexByte scan",
		Run:  runPS5023,
	},
})

func runPS5023(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps5023Match(pass, call)
			if m == nil {
				return true
			}
			msg := m.pkg + ".IndexRune of the constant ASCII rune " + m.argText +
				" pays a non-inlined range-check wrapper before delegating to the byte scan; " +
				m.pkg + ".IndexByte(" + m.hay + ", " + m.argText +
				") jumps straight to the same scan — identical index for every input"
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: msg,
			}
			if m.lit == nil {
				// A named constant or constant expression: spelling out its
				// value would discard the symbolic name, and a TYPED rune
				// constant would additionally need an inserted byte(...)
				// conversion — advisory only, rewrite by hand.
				diag.Message = msg + "; the rune is a constant expression, not a literal — rewrite to " + m.pkg + ".IndexByte by hand (a typed rune constant needs an explicit byte(...) conversion)"
			} else {
				// Rename the callee identifier and nothing else: an untyped
				// rune/integer literal in [0, 0x80) is representable as byte,
				// so the original token converts implicitly in IndexByte's
				// argument position — no re-spelling, no added conversion,
				// and the single replaced span is one identifier, so no
				// comment can ever sit inside deleted syntax.
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace IndexRune with IndexByte",
					TextEdits: []analysis.TextEdit{
						{Pos: m.sel.Sel.Pos(), End: m.sel.Sel.End(), NewText: []byte("IndexByte")},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps5023Site is a matched strings/bytes IndexRune call with a constant
// ASCII rune: the selector to rewrite, the package name (which doubles as
// the message qualifier — "strings" or "bytes"), the conventional
// haystack placeholder for the message ("s" or "b"), the rune's literal
// token (nil when the constant is not a direct rune/int literal —
// advisory only), and the rune's source text for the message.
type ps5023Site struct {
	sel     *ast.SelectorExpr
	pkg     string
	hay     string
	lit     *ast.BasicLit
	argText string
}

// ps5023Match matches a call to the standard library's package-level
// strings.IndexRune or bytes.IndexRune whose second argument is a
// compile-time constant with integer value in [0, utf8.RuneSelf = 0x80).
// Type information pins the callee (a shadowed strings/bytes identifier,
// a same-named method, or a third-party package never matches). Every
// bound is load-bearing: a non-constant rune can hold a non-ASCII value
// at runtime where byte(r) truncates and diverges; a constant >= 0x80 is
// a multi-byte UTF-8 search (or, for utf8.RuneError, an invalid-UTF-8
// probe); and a negative constant is IndexRune's constant -1 via
// !utf8.ValidRune — none of which IndexByte can express, so such
// operands never match at all, not even advisory.
func ps5023Match(pass *analysis.Pass, call *ast.CallExpr) *ps5023Site {
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
	if fn.Name() != "IndexRune" {
		// ContainsRune splices in a comparison (a separate pattern), and
		// the byte/substring/set needles are PS5007/PS5013/PS5016/PS5014/
		// PS5022's territory.
		return nil
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil
	}
	arg := ps2108Unparen(call.Args[1])
	tv, found := pass.TypesInfo.Types[arg]
	if !found || tv.Value == nil {
		return nil
	}
	v, exact := constant.Int64Val(constant.ToInt(tv.Value))
	if !exact || v < 0 || v >= 0x80 {
		return nil
	}
	argText, ok := ps5004ExprText(arg)
	if !ok {
		return nil
	}
	lit, _ := arg.(*ast.BasicLit)
	if lit != nil && lit.Kind != token.CHAR && lit.Kind != token.INT {
		// A FLOAT/IMAG spelling of an integral value ("IndexRune(s, 122.0)")
		// is legal but bizarre; keep it advisory rather than pinning that
		// the same token converts to byte.
		lit = nil
	}
	hay := "s"
	if pkg == "bytes" {
		hay = "b"
	}
	return &ps5023Site{sel: sel, pkg: pkg, hay: hay, lit: lit, argText: argText}
}
