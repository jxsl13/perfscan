package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2020 reports bytes.Join(bytes.Split(b, sep), new) with a STATICALLY
// NON-EMPTY separator — rebuilding a replacement through a throwaway
// [][]byte of every fragment — and rewrites it to
// bytes.ReplaceAll(b, sep, new), the identical result in one scan. It is
// the bytes twin of PS2015 (strings.Join(strings.Split)).
var PS2020 = register(&lint.Check{
	ID:       "PS2020",
	Category: "alloc",
	Slug:     "bytes-join-split-replaceall",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "bytes.Join(bytes.Split(b, sep), new) rebuilds a replacement via a throwaway [][]byte; bytes.ReplaceAll(b, sep, new) is the identical result in one scan",
		Text: `bytes.Split(b, sep) materializes a full [][]byte of every
fragment — one slice header plus one subslice descriptor per fragment,
Count(b, sep)+1 of them — purely as an intermediate; bytes.Join then
walks that slice and allocates the result. bytes.ReplaceAll(b, sep, new)
produces the identical bytes in a single left-to-right pass with only
the result allocation, dropping the intermediate [][]byte and its
per-fragment bookkeeping: fewer allocations and one less pass over the
pieces. (PS2015 measures the same shape for strings: ~2 allocs -> 1 and
a time win; the bytes intermediate is heavier still, three words per
fragment instead of two.)

The rewrite is bit-identical for every input as long as sep is
NON-EMPTY. Split cuts b at exactly the non-overlapping occurrences of
sep, found left-to-right via Index (byte-oriented — arbitrary bytes and
invalid UTF-8 are matched literally, no rune interpretation anywhere),
and Join re-inserts new between the fragments — the Split/Join inverse
identity. Replace finds the same non-overlapping occurrences with the
same Index scan and substitutes new at each, so the concatenation
p0+new+p1+...+new+pk is byte-for-byte ReplaceAll's output. When sep does
not occur, Split returns the single fragment b and Join copies it, while
ReplaceAll returns a fresh copy of b — identical bytes either way, and
neither form ever aliases b, so no mutation-visibility changes. Even
nil-ness agrees: both forms build the result by copying, and a nil or
empty b yields the same nil result from both (pinned by the equivalence
suite). b, sep and new are each evaluated exactly once in both forms and
in the same left-to-right order (Split and ReplaceAll are pure, so
moving the call boundary is unobservable); neither form can panic, and
there is no named-type method surface — the operands are byte slices
passed straight through.

The ONLY divergence is the empty separator: Split(b, empty) explodes b
after each UTF-8 sequence into its k subslices and Join puts new in the
k-1 gaps, while ReplaceAll(b, empty, new) matches the empty slice BEFORE
and AFTER every rune and inserts new k+1 times ("ab" -> "a;b" vs
";a;b;"). The fix is therefore gated on sep being STATICALLY non-empty —
a []byte("...") conversion of a compile-time non-empty string constant,
or a []byte{...} composite literal with at least one element (a keyed
element still forces len > 0). A []byte VARIABLE may be empty or nil at
run time, so it is never reported at all rather than reported with a
wrong suggestion. The replacement new needs no gate: the identity holds
for every value including nil and empty (deletion).

The callee pair is pinned with type information to the package-level
bytes.Join and bytes.Split (a shadowed bytes, a local helper or a
method does not resolve there; SplitN, SplitAfter and Fields shape
their pieces differently and are never matched). The Split call must be
DIRECTLY the first Join argument (parentheses allowed): a slice stored
in a variable first may have other consumers. The fix keeps all three
argument expressions byte-verbatim — it renames Join to ReplaceAll,
deletes the inner "bytes.Split(" wrapper, and re-joins the separator
argument to the replacement argument with ", " — so evaluation order
and side-effect count are untouched. No import changes: ReplaceAll
lives in the same bytes package the code already names.`,
		Before: `out := bytes.Join(bytes.Split(b, []byte(",")), []byte("; "))`,
		After:  `out := bytes.ReplaceAll(b, []byte(","), []byte("; "))`,
		MeasuredWin: `BenchmarkPS2020 (a ~1.3KB line of 64 comma-separated
~19-byte fields, "," -> "; ", Apple M2 Pro, go1.26): Join(Split)
1310 ns/op, 3200 B/op, 2 allocs/op vs ReplaceAll 1093 ns/op, 1408 B/op,
1 alloc/op (~1.2x faster, ~56% less memory, half the allocations — the
dropped alloc is the 64-entry [][]byte, three words per fragment). With
no separator occurrence both forms still copy b, but ReplaceAll skips
the one-element [][]byte entirely.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2020",
		Doc:  "bytes.Join(bytes.Split(b, sep), new) with a statically non-empty sep allocates a throwaway [][]byte of every fragment; bytes.ReplaceAll(b, sep, new) is bit-identical in one scan",
		Run:  runPS2020,
	},
})

func runPS2020(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			join, ok := n.(*ast.CallExpr)
			if !ok || len(join.Args) != 2 || join.Ellipsis.IsValid() {
				return true
			}
			joinSel, ok := join.Fun.(*ast.SelectorExpr)
			if !ok || !ps2020IsBytesFunc(pass, joinSel, "Join") {
				return true
			}
			// The joined slice must be DIRECTLY the Split call (parens
			// allowed): a slice stored in a variable first may have other
			// consumers — out of scope.
			split, ok := ps2121Unparen(join.Args[0]).(*ast.CallExpr)
			if !ok || len(split.Args) != 2 || split.Ellipsis.IsValid() {
				return true
			}
			splitSel, ok := split.Fun.(*ast.SelectorExpr)
			if !ok || !ps2020IsBytesFunc(pass, splitSel, "Split") {
				return true
			}
			// The separator must be STATICALLY non-empty: Split(b, empty)
			// explodes b after each UTF-8 sequence and Join puts new in the
			// k-1 gaps, while ReplaceAll(b, empty, new) inserts new k+1
			// times — the one divergence shape. Provably non-empty means a
			// []byte("...") conversion of a non-empty string constant or a
			// []byte{...} literal with at least one element (the same gate
			// PS2121 uses for its bytes arm); a []byte variable may be empty
			// or nil at run time, so it is never reported at all.
			sep := split.Args[1]
			if !ps2121SepNonEmpty(pass, "bytes", sep) {
				return true
			}
			// Three edits keep b, sep and new byte-verbatim: rename Join to
			// ReplaceAll, delete the "bytes.Split(" wrapper (and any
			// parentheses around it) in front of b, and replace the span
			// between sep and new — the Split-closing parenthesis plus the
			// argument comma — with ", ". The three argument expressions
			// already appear in source in exactly ReplaceAll's order.
			pass.Report(analysis.Diagnostic{
				Pos: join.Pos(),
				End: join.End(),
				Message: "bytes.Join(bytes.Split(b, sep), new) with a statically non-empty separator " +
					"allocates a throwaway [][]byte of every fragment; bytes.ReplaceAll(b, sep, new) is bit-identical in one scan",
				SuggestedFixes: []analysis.SuggestedFix{{
					Message: "replace with bytes.ReplaceAll(b, sep, new)",
					TextEdits: []analysis.TextEdit{
						{Pos: joinSel.Sel.Pos(), End: joinSel.Sel.End(), NewText: []byte("ReplaceAll")},
						{Pos: join.Lparen + 1, End: split.Args[0].Pos()},
						{Pos: sep.End(), End: join.Args[1].Pos(), NewText: []byte(", ")},
					},
				}},
			})
			return true
		})
	}
	return nil, nil
}

// ps2020IsBytesFunc reports whether sel resolves to the package-level
// bytes function with the given name — not a shadowed bytes, not a
// local helper, not a method.
func ps2020IsBytesFunc(pass *analysis.Pass, sel *ast.SelectorExpr, name string) bool {
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != name || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	return ok && sig.Recv() == nil
}
