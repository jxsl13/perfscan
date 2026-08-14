package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2011 reports []byte(strings.Repeat(s, n)) — building the repetition as
// a throwaway string and then copying every byte of it into a second
// buffer — where bytes.Repeat([]byte(s), n) writes the repeated content
// into a single []byte allocation directly. ADVISORY BY DESIGN: the two
// spellings produce the same bytes but not always the same CAPACITY (and
// they panic with different messages), so no mechanical rewrite is
// bit-identical — see the Doc for the exact divergences.
var PS2011 = register(&lint.Check{
	ID:       "PS2011",
	Category: "alloc",
	Slug:     "byte-slice-of-strings-repeat",
	Level:    lint.LevelIdiomatic,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "[]byte(strings.Repeat(s, n)) builds the repetition twice; bytes.Repeat([]byte(s), n) fills one buffer",
		Text: `strings.Repeat(s, n) allocates a string of len(s)*n bytes and
fills it; the surrounding []byte(...) conversion then allocates a SECOND
len(s)*n buffer and copies every byte into it. bytes.Repeat([]byte(s), n)
converts only the short seed (len(s) bytes) and writes the repetition
into a single buffer: one large allocation instead of two, and half the
bytes touched. The saving scales with the output length.

The byte CONTENT is identical on every input: bytes.Repeat([]byte(s), n)
produces exactly the bytes of s repeated n times — the same bytes
[]byte(strings.Repeat(s, n)) produces, for every s (empty, multi-byte
UTF-8, invalid UTF-8 — both forms are byte-level) and every n, with the
same length and non-nil-ness (pinned by the equivalence suite). So why is
this ADVISORY rather than an auto-fix? Two observable divergences survive
every static guard:

  - CAPACITY. When the converted slice escapes to the heap, gc gives
    []byte(string) a capacity rounded up to the allocator's size class
    (e.g. cap 16 for a 9-byte result), while bytes.Repeat clamps its
    result to exactly cap == len via a three-index slice. cap(b) is
    observable directly, and indirectly through append: with spare
    capacity an append extends the SAME backing array (later writes
    through the appended slice alias the original), with cap == len it
    reallocates. Whether the rounding happens depends on escape analysis,
    so no source-level condition can prove the capacities equal (the same
    divergence that keeps the bytes.SplitSeq rewrite advisory).

  - PANIC MESSAGE. strings.Repeat panics "strings: negative Repeat
    count" / "strings: Repeat output length overflow" where bytes.Repeat
    says "bytes: ..." — a recovered and inspected panic string differs.
    This one could be guarded away with a provably non-negative,
    non-overflowing constant count, but the capacity divergence above
    cannot, so the check stays advisory for every shape.

Apply the rewrite manually when the result's capacity is not load-bearing
— which is nearly always: the typical consumer reads the bytes, writes
them out, or hashes them, and never appends through an alias. The match
is strict, so every report is the real pattern: the conversion target
must be exactly the predeclared unnamed []byte (an alias matches, a
defined slice type does not), and the callee is pinned by type
information to the standard library's package-level strings.Repeat
reached through the imported package qualifier (a shadowed strings, a
local Repeat, or a method never matches).`,
		Before: `b := []byte(strings.Repeat(s, 64))`,
		After:  `b := bytes.Repeat([]byte(s), 64)`,
		MeasuredWin: `BenchmarkPS2011 (s = "ab", n = 64, 128-byte output,
Apple M2 Pro, go1.26): []byte(strings.Repeat(s, 64)) 77.5 ns/op,
256 B/op, 2 allocs/op -> bytes.Repeat([]byte(s), 64) 39.9 ns/op,
128 B/op, 1 alloc/op (~1.9x faster, half the memory — the eliminated
alloc and copy grow with the output length).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2011",
		Doc:  "[]byte(strings.Repeat(s, n)) allocates and fills the repetition twice; bytes.Repeat([]byte(s), n) fills a single buffer",
		Run:  runPS2011,
	},
})

const ps2011Msg = "[]byte(strings.Repeat(s, n)) allocates the repeated string and then a second []byte buffer it is copied into; bytes.Repeat([]byte(s), n) fills a single buffer — one allocation and one copy instead of two (advisory: the result's capacity and the panic message can differ)"

func runPS2011(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			conv, ok := n.(*ast.CallExpr)
			if !ok || !ps2011Match(pass, conv) {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     conv.Pos(),
				End:     conv.End(),
				Message: ps2011Msg,
			})
			return true
		})
	}
	return nil, nil
}

// ps2011Match matches conv against []byte(strings.Repeat(s, n)): a
// conversion to exactly the predeclared unnamed []byte whose sole operand
// is a direct call of the standard library's package-level strings.Repeat
// reached through the imported package qualifier.
func ps2011Match(pass *analysis.Pass, conv *ast.CallExpr) bool {
	if len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return false
	}
	if !ps2108IsByteSliceConv(pass, conv.Fun) {
		return false
	}
	call, isCall := ps2108Unparen(conv.Args[0]).(*ast.CallExpr)
	if !isCall || len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return false
	}
	// The qualifier must BE the imported strings package: a shadowing
	// value or type spelled `strings` is some other pattern entirely.
	xid, isIdent := sel.X.(*ast.Ident)
	if !isIdent {
		return false
	}
	if pn, isPkg := pass.TypesInfo.Uses[xid].(*types.PkgName); !isPkg || pn.Imported().Path() != "strings" {
		return false
	}
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || fn.Name() != "Repeat" || fn.Pkg() == nil || fn.Pkg().Path() != "strings" {
		return false
	}
	sig, isSig := fn.Type().(*types.Signature)
	return isSig && sig.Recv() == nil
}
