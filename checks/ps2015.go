package checks

import (
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2015 reports strings.Join(strings.Split(s, sep), new) with a constant,
// NON-EMPTY separator — rebuilding a replacement through a throwaway
// []string of every fragment — and rewrites it to
// strings.ReplaceAll(s, sep, new), the identical result in one scan.
var PS2015 = register(&lint.Check{
	ID:       "PS2015",
	Category: "alloc",
	Slug:     "join-split-replaceall",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.Join(strings.Split(s, sep), new) rebuilds a replacement via a throwaway []string; strings.ReplaceAll(s, sep, new) is the identical result in one scan",
		Text: `strings.Split(s, sep) materializes a full []string of every
fragment — one slice header plus a backing array sized
Count(s, sep)+1 — purely as an intermediate; strings.Join then walks
that slice and allocates the result. strings.ReplaceAll(s, sep, new)
produces the identical string in a single left-to-right pass with only
the result allocation, dropping the intermediate slice and its
per-fragment bookkeeping: fewer allocations and one less pass over the
pieces. When sep does not occur at all, ReplaceAll additionally
returns the input unchanged with ZERO allocations, where Split+Join
still allocates the one-element slice and a fresh copy of s.

The rewrite is bit-identical for every input as long as sep is
NON-EMPTY. Split cuts s at exactly the non-overlapping occurrences of
sep, found left-to-right via Index (byte-oriented, so invalid UTF-8 is
matched identically), and Join re-inserts new between the fragments —
the Split/Join inverse identity. Replace finds the same non-overlapping
occurrences with the same Index scan and substitutes new at each, so
the concatenation p0+new+p1+...+new+pk is byte-for-byte ReplaceAll's
output. s, sep and new are each evaluated exactly once in both forms
and in the same left-to-right order (Split and ReplaceAll are pure, so
moving the call boundary is unobservable); neither form can panic, and
there is no named-type/Stringer surface — both operands are plain
strings.

The ONLY divergence is the empty separator: Split(s, "") explodes s
into its k runes and Join puts new in the k-1 gaps, while
ReplaceAll(s, "", new) matches the empty string BEFORE and AFTER every
rune and inserts new k+1 times ("ab" -> "a;b" vs ";a;b;"). The fix is
therefore gated on sep being a compile-time string constant whose
value is non-empty — a purely local check; a variable separator may be
"" at run time, so it is never reported at all rather than reported
with a wrong suggestion. The replacement string new needs no gate: the
identity holds for every value including the empty string (deletion).

The callee pair is pinned with type information to the package-level
strings.Join and strings.Split (a shadowed strings, a local helper or
a method does not resolve there; SplitN, SplitAfter and Fields shape
their pieces differently and are never matched). The Split call must
be DIRECTLY the first Join argument (parentheses allowed): a slice
stored in a variable first may have other consumers. The fix keeps all
three argument expressions byte-verbatim — it renames Join to
ReplaceAll, deletes the inner "strings.Split(" wrapper, and re-joins
the separator argument to the replacement argument with ", " — so
evaluation order and side-effect count are untouched. No import
changes: ReplaceAll lives in the same strings package the code
already names.`,
		Before: `out := strings.Join(strings.Split(s, ","), "; ")`,
		After:  `out := strings.ReplaceAll(s, ",", "; ")`,
		MeasuredWin: `BenchmarkPS2015 (a ~1.3KB line of 64 comma-separated
~19-byte fields, "," -> "; ", Apple M2 Pro): Join(Split) 1360 ns/op,
2560 B/op, 2 allocs/op vs ReplaceAll 1014 ns/op, 1408 B/op,
1 alloc/op (~1.3x faster, ~45% less memory, half the allocations).
With no separator occurrence the gap is starker: Split+Join still
allocates the one-piece slice and a copy of s, ReplaceAll returns s
unchanged with 0 allocs.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2015",
		Doc:  "strings.Join(strings.Split(s, sep), new) with a constant non-empty sep allocates a throwaway []string of every fragment; strings.ReplaceAll(s, sep, new) is bit-identical in one scan",
		Run:  runPS2015,
	},
})

func runPS2015(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			join, ok := n.(*ast.CallExpr)
			if !ok || len(join.Args) != 2 || join.Ellipsis.IsValid() {
				return true
			}
			// PS5112 owns exact inverse Split/Join compositions and removes
			// both calls in one pass. Do not overlap it with the intermediate
			// ReplaceAll(s, sep, sep) rewrite.
			if _, owned := ps5112SplitJoinIdentity(pass, join); owned {
				return true
			}
			joinSel, ok := join.Fun.(*ast.SelectorExpr)
			if !ok || !ps2015IsStringsFunc(pass, joinSel, "Join") {
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
			if !ok || !ps2015IsStringsFunc(pass, splitSel, "Split") {
				return true
			}
			// The separator must be a compile-time string constant with a
			// NON-EMPTY value: Split(s, "") rune-explodes and Join puts new
			// in the k-1 gaps, while ReplaceAll(s, "", new) inserts new k+1
			// times — the one divergence shape. A variable separator may be
			// "" at run time, so it is never reported at all.
			sep := split.Args[1]
			tv, ok := pass.TypesInfo.Types[sep]
			if !ok || tv.Value == nil || tv.Value.Kind() != constant.String ||
				constant.StringVal(tv.Value) == "" {
				return true
			}
			// Three edits keep s, sep and new byte-verbatim: rename Join to
			// ReplaceAll, delete the "strings.Split(" wrapper (and any
			// parentheses around it) in front of s, and replace the span
			// between sep and new — the Split-closing parenthesis plus the
			// argument comma — with ", ". The three argument expressions
			// already appear in source in exactly ReplaceAll's order.
			pass.Report(analysis.Diagnostic{
				Pos: join.Pos(),
				End: join.End(),
				Message: "strings.Join(strings.Split(s, sep), new) with a constant non-empty separator " +
					"allocates a throwaway []string of every fragment; strings.ReplaceAll(s, sep, new) is bit-identical in one scan",
				SuggestedFixes: []analysis.SuggestedFix{{
					Message: "replace with strings.ReplaceAll(s, sep, new)",
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

// ps2015IsStringsFunc reports whether sel resolves to the package-level
// strings function with the given name — not a shadowed strings, not a
// local helper, not a method.
func ps2015IsStringsFunc(pass *analysis.Pass, sel *ast.SelectorExpr, name string) bool {
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != name || fn.Pkg() == nil || fn.Pkg().Path() != "strings" {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	return ok && sig.Recv() == nil
}
