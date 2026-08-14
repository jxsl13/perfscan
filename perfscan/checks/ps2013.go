package checks

import (
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2013 reports strings.NewReplacer(old, new).Replace(s) with exactly ONE
// constant, non-empty pair — a heavyweight per-call replacer machine built and
// discarded for what is a plain single-substitution — and rewrites it to
// strings.ReplaceAll(s, old, new), the same substitution in one scan.
var PS2013 = register(&lint.Check{
	ID:       "PS2013",
	Category: "alloc",
	Slug:     "single-pair-newreplacer-replaceall",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "single-pair strings.NewReplacer(...).Replace(s) rebuilds a replacer machine per call; strings.ReplaceAll(s, old, new) is the identical substitution in one scan",
		Text: `strings.NewReplacer(old, new).Replace(s) constructs a full replacer
(byte, single-string or generic-trie machine depending on the pair shape),
uses it once, and throws it away — per call. For a SINGLE pair the whole
machine is pure overhead: strings.ReplaceAll(s, old, new) performs the
identical replacement with one strings.Count-sized output allocation and
one linear scan. Measured for a short-key substitution: ~485 ns, 8 allocs
and ~2.6 KB per call for the inline replacer versus ~134 ns and 1 alloc
for ReplaceAll — about 3.6x; for a single-byte pair with no match the gap
is ~9x (~186 ns / 3 allocs vs ~21 ns / 0 allocs, ReplaceAll returns the
input unchanged without allocating). Against a HOISTED single-pair
replacer (the PS2132 shape) ReplaceAll is time-parity with fewer
allocations (1 vs 3: genericReplacer.Replace routes its output through an
append-writer buffer, ReplaceAll writes the exact-sized result directly)
— so the rewrite never trades away the amortized alternative. This is
work reduction, not a readability swap.

The rewrite is bit-identical whenever old is a NON-EMPTY constant string:
both forms replace all non-overlapping occurrences of old left-to-right,
resuming immediately after each replacement, over arbitrary bytes (invalid
UTF-8 included) — pinned by a runtime differential over 2,000,000
randomized adversarial inputs with zero diffs. The single divergence in
the whole API is old == "": NewReplacer("", x) inserts x between every
BYTE while ReplaceAll(s, "", x) inserts between every RUNE, so the check
simply does not match an empty old. Both arguments must be compile-time
constant strings: the rewrite moves them after s (the constructor
evaluates old and new before the receiver argument, ReplaceAll evaluates
s first), and constants make that reorder unobservable; s itself stays in
place, evaluated exactly once in both forms. Both forms return string,
accept the same argument types (anything assignable to string compiles
identically in both), and neither can panic, so no call site changes
meaning.

The match is pinned by type information: the constructor must be the
standard library's package-level strings.NewReplacer resolved by import
path (an aliased import matches and keeps its alias; a shadowed or fake
"strings" never matches), the method must be Replace (WriteString has no
single-call ReplaceAll counterpart and stays with PS2132), the constructor
must have exactly two non-variadic constant-string arguments, and old must
have length >= 1. The automatic fix keeps s byte-verbatim in place and
re-renders only the constant arguments after it; a comment anywhere in the
replaced punctuation keeps the report advisory. A multi-pair or
runtime-value replacer is PS2132's territory (hoist to package scope) and
is deliberately not reported here.`,
		Before: `out := strings.NewReplacer("the", "THE").Replace(s)`,
		After:  `out := strings.ReplaceAll(s, "the", "THE")`,
		MeasuredWin: `BenchmarkPS2013 (80-byte sentence, 4 matches, Apple M2 Pro,
go1.26): strings.NewReplacer("the", "THE").Replace(s) 485 ns/op, 2632 B/op,
8 allocs/op -> strings.ReplaceAll(s, "the", "THE") 134 ns/op, 80 B/op,
1 alloc/op (~3.6x faster, 7 allocations fewer). A hoisted single-pair
replacer measures time-parity with ReplaceAll but 3 allocs vs 1, so the
rewrite also never loses to the amortized alternative; for a single-byte
pair the inline-vs-ReplaceAll gap is ~9x (186 ns/3 allocs vs 21 ns/0).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2013",
		Doc:  "single constant-pair strings.NewReplacer(...).Replace(s) builds and discards a replacer machine per call; strings.ReplaceAll(s, old, new) is the identical substitution in one scan",
		Run:  runPS2013,
	},
})

const ps2013Msg = "strings.NewReplacer with a single constant pair builds and discards a replacer machine on every Replace call; strings.ReplaceAll(s, old, new) performs the identical substitution in one scan with one allocation (~3.6-9x faster)"

func runPS2013(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			outer, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			inner, matched := ps2013Match(pass, outer)
			if !matched {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     outer.Pos(),
				End:     outer.End(),
				Message: ps2013Msg,
			}
			if fix := ps2013Fix(f, outer, inner); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			// Keep descending: further matches can only sit inside the
			// verbatim s argument, whose edits never overlap this site's
			// punctuation edits.
			return true
		})
	}
	return nil, nil
}

// ps2013Match matches outer against strings.NewReplacer(old, new).Replace(s)
// with old and new compile-time constant strings and old non-empty, everything
// pinned by type information. It returns the inner constructor call.
func ps2013Match(pass *analysis.Pass, outer *ast.CallExpr) (inner *ast.CallExpr, ok bool) {
	// .Replace(s): exactly one argument, no ellipsis, and the method is the
	// standard library's (*strings.Replacer).Replace.
	if len(outer.Args) != 1 || outer.Ellipsis.IsValid() {
		return nil, false
	}
	sel, isSel := outer.Fun.(*ast.SelectorExpr)
	if !isSel {
		return nil, false
	}
	method, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || method.Name() != "Replace" || method.Pkg() == nil || method.Pkg().Path() != "strings" {
		return nil, false
	}
	if sig, isSig := method.Type().(*types.Signature); !isSig || sig.Recv() == nil {
		// A package-level strings.Replace (4 args) can never reach here via
		// the arity gate, but pin the receiver anyway: only the METHOD
		// (*strings.Replacer).Replace is the matched shape.
		return nil, false
	}
	// The receiver must be an inline strings.NewReplacer(...) call — a
	// replacer stored in a variable is out of scope (and already amortized).
	inner, isCall := ps2108Unparen(sel.X).(*ast.CallExpr)
	if !isCall || len(inner.Args) != 2 || inner.Ellipsis.IsValid() {
		return nil, false
	}
	innerSel, isSel := inner.Fun.(*ast.SelectorExpr)
	if !isSel {
		return nil, false
	}
	ctor, isFunc := pass.TypesInfo.Uses[innerSel.Sel].(*types.Func)
	if !isFunc || ctor.Name() != "NewReplacer" || ctor.Pkg() == nil || ctor.Pkg().Path() != "strings" {
		return nil, false
	}
	if sig, isSig := ctor.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, false
	}
	// Both pair arguments must be compile-time constant strings: the rewrite
	// moves them after s, and only side-effect-free constants make the
	// evaluation reorder unobservable.
	for _, a := range inner.Args {
		tv, found := pass.TypesInfo.Types[a]
		if !found || tv.Value == nil || tv.Value.Kind() != constant.String {
			return nil, false
		}
	}
	// old == "" is the one divergent input: NewReplacer("", x) inserts x
	// between every BYTE, ReplaceAll(s, "", x) between every RUNE. Never
	// match it.
	if constant.StringVal(pass.TypesInfo.Types[inner.Args[0]].Value) == "" {
		return nil, false
	}
	return inner, true
}

// ps2013Fix builds the strings.ReplaceAll(s, old, new) rewrite for one site,
// or nil when a guard fails and the report must stay advisory. The s argument
// is kept byte-verbatim in place (single evaluation preserved); the punctuation
// around it becomes the ReplaceAll call, with the constant pair re-rendered
// after s. The strings qualifier is reused from the source (alias included),
// so no import surgery is ever needed.
func ps2013Fix(f *ast.File, outer, inner *ast.CallExpr) *analysis.SuggestedFix {
	s := outer.Args[0]
	// The replaced spans are everything around s; a comment there (between
	// the constructor arguments, before .Replace, ...) would be silently
	// destroyed — advisory then.
	if ps2111CommentIn(f, outer.Pos(), s.Pos()) || ps2111CommentIn(f, s.End(), outer.End()) {
		return nil
	}
	innerSel := inner.Fun.(*ast.SelectorExpr) // shape established by ps2013Match
	qualifier := exprTextRendered(innerSel.X)
	oldText := exprTextRendered(inner.Args[0])
	newText := exprTextRendered(inner.Args[1])
	return &analysis.SuggestedFix{
		Message: "replace with " + qualifier + ".ReplaceAll(s, old, new)",
		TextEdits: []analysis.TextEdit{
			{Pos: outer.Pos(), End: s.Pos(), NewText: []byte(qualifier + ".ReplaceAll(")},
			{Pos: s.End(), End: outer.End(), NewText: []byte(", " + oldText + ", " + newText + ")")},
		},
	}
}
