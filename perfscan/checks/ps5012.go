package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5012 reports strings.Replace(s, old, new, strings.Count(s, old)) —
// an explicit Count of the very string Replace is about to Count again
// internally, so s is walked twice for one replace-all — and rewrites it
// to strings.ReplaceAll(s, old, new), the identical replacement in a
// single scan. The count-to-cap equality n == Count(s, old) makes the
// two forms bit-identical on every input (see the Doc and the runtime
// differential in equiv_PS5012_test.go).
var PS5012 = register(&lint.Check{
	ID:       "PS5012",
	Category: "arith",
	Slug:     "replace-counted-all",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.Replace(s, old, new, strings.Count(s, old)) scans s twice for one replace-all; strings.ReplaceAll(s, old, new) is the identical substitution in a single scan",
		Text: `strings.Replace(s, old, new, n) begins by computing
strings.Count(s, old) ITSELF to size its output and clamp n. Passing
strings.Count(s, old) as the explicit n therefore buys nothing: the
argument is a full O(len(s)) substring scan whose only effect is to
hand Replace a cap it immediately re-derives, so every call walks s
twice (plus walks old's matcher state up twice). strings.ReplaceAll(s,
old, new) — literally Replace(s, old, new, -1) — performs the single
internal Count and the replacement: one scan, the same one allocation,
about half the pre-replacement work. The relative win is largest
exactly where the pattern hurts most, on long inputs with few matches,
where the redundant Count dominates the whole call.

The rewrite is bit-identical on every input. Replace clamps its n to
m = Count(s, old) whenever n < 0 or n > m and then replaces exactly
min(n, m) leftmost non-overlapping occurrences; passing n == m replaces
all m of them, which is precisely what ReplaceAll's n = -1 resolves to.
The equality holds on every edge: no match (m == 0, both return s
itself, zero allocations), old == new (both return s), overlapping
candidates (Count and Replace share the same non-overlapping
left-to-right match walk, so the count can never overshoot or
undershoot the sites Replace visits), and even old == "" — Count(s, "")
is utf8.RuneCountInString(s)+1, exactly the k+1 boundary insertions
ReplaceAll performs, with identical invalid-UTF-8 treatment (both
decode RuneError width 1). Both forms are byte-oriented throughout, so
arbitrary bytes and invalid UTF-8 round through identically. The
runtime differential test pins the equality over exhaustive short
operand triples, targeted seeds and randomized full-byte-range inputs.

The one semantic difference is evaluation COUNT: the Before evaluates s
and old twice each (once inside the explicit Count, once for Replace)
and the After once each. The automatic fix therefore applies only when
the shape is provably order-insensitive: the callee is the standard
library's package-level strings.Replace with its n argument exactly a
package-level strings.Count call (both pinned by type information
through the same import — a shadowed or dot-imported strings never
matches), the two s expressions and the two old expressions are
syntactically identical, and s, old AND new are all free of calls,
conversions and channel receives — so dropping the second evaluation of
s and old is unobservable, and no expression between the first and
second evaluations can mutate what they denote. A match whose operands
do contain calls (strings.Replace(f(), old, new, strings.Count(f(),
old)) and friends) is still reported — it does strictly redundant
double scanning, and worse — but stays advisory: f() could return
different values or carry side effects, so no mechanical rewrite is
bit-identical there. The fix keeps s, old and new byte-verbatim in
place and rewrites only the punctuation around them (the callee name
and the trailing ", strings.Count(s, old)" tail), reusing the file's
own strings qualifier, alias included — no import surgery is ever
needed since the rewritten call still references the same package. A
comment anywhere in the replaced punctuation keeps the report advisory.`,
		Before: `out := strings.Replace(s, old, new, strings.Count(s, old))`,
		After:  `out := strings.ReplaceAll(s, old, new)`,
		MeasuredWin: `BenchmarkPS5012 (4 KiB log payload, 68 "msg=" matches, Apple
M2 Pro, go1.26): strings.Replace(s, old, new, strings.Count(s, old))
9300 ns/op, 4864 B/op, 1 alloc/op -> strings.ReplaceAll(s, old, new)
6920 ns/op, 4864 B/op, 1 alloc/op (~1.35x faster, identical single
allocation: the whole delta is the redundant explicit Count scan of
all 4 KiB — each match forces the substring searcher to stop and
restart, work Count does in full before Replace re-does it). When the
searcher can skip in long SIMD strides (sparse single matches) both
scans are memchr-fast and the gap narrows; it never inverts, since the
After does strictly less work on every input.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5012",
		Doc:  "strings.Replace(s, old, new, strings.Count(s, old)) walks s twice — the explicit Count only re-derives the cap Replace computes internally; strings.ReplaceAll(s, old, new) is the identical replace-all in a single scan",
		Run:  runPS5012,
	},
})

const ps5012Msg = "strings.Replace(s, old, new, strings.Count(s, old)) walks s twice: the explicit Count is a full scan whose result Replace re-derives internally anyway; strings.ReplaceAll(s, old, new) performs the identical replace-all in a single scan"

func runPS5012(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			count, matched := ps5012Match(pass, call)
			if !matched {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: ps5012Msg,
			}
			if fix := ps5012Fix(pass, f, call, count); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			// Keep descending: a nested match can only sit inside the
			// verbatim operand spans, whose edits never overlap this
			// site's punctuation edits.
			return true
		})
	}
	return nil, nil
}

// ps5012StringsFunc reports whether call invokes the standard library's
// package-level strings.<name> with exactly nargs non-variadic arguments,
// returning the selector when it does. Pinning through the *types.Func
// object means a shadowed strings, a dot import or a same-named method
// never matches.
func ps5012StringsFunc(pass *analysis.Pass, call *ast.CallExpr, name string, nargs int) (*ast.SelectorExpr, bool) {
	if len(call.Args) != nargs || call.Ellipsis.IsValid() {
		return nil, false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return nil, false
	}
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || fn.Name() != name || fn.Pkg() == nil || fn.Pkg().Path() != "strings" {
		return nil, false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, false
	}
	return sel, true
}

// ps5012Match matches call against
// strings.Replace(s, old, new, strings.Count(s, old)) — the outer callee
// and the n argument both pinned to the standard library by type
// information, the two s expressions and the two old expressions
// syntactically identical (parens allowed around the Count call itself,
// nowhere else). It returns the inner Count call.
func ps5012Match(pass *analysis.Pass, call *ast.CallExpr) (count *ast.CallExpr, ok bool) {
	if _, isReplace := ps5012StringsFunc(pass, call, "Replace", 4); !isReplace {
		return nil, false
	}
	count, isCall := ps2108Unparen(call.Args[3]).(*ast.CallExpr)
	if !isCall {
		return nil, false
	}
	if _, isCount := ps5012StringsFunc(pass, count, "Count", 2); !isCount {
		return nil, false
	}
	// The count must be of the SAME haystack and needle, syntactically:
	// Count(s, sep) over any other operand pair is a genuine different
	// cap, not the replace-all idiom.
	if !exprEqual(call.Args[0], count.Args[0]) || !exprEqual(call.Args[1], count.Args[1]) {
		return nil, false
	}
	return count, true
}

// ps5012Fix builds the strings.ReplaceAll(s, old, new) rewrite for one
// site, or nil when a guard fails and the report must stay advisory. The
// s, old and new expressions are kept byte-verbatim in place (single
// evaluation, original spelling); only the punctuation around them is
// replaced, so the replacement is a call wherever the original call was
// legal and never needs parentheses. No import surgery is ever needed:
// the rewritten call reuses the outer call's own strings qualifier.
func ps5012Fix(pass *analysis.Pass, f *ast.File, call, count *ast.CallExpr) *analysis.SuggestedFix {
	// Dropping the Count call removes the second evaluation of s and old.
	// That is unobservable only when s, old and new (which is evaluated
	// BETWEEN the two, and could otherwise mutate what they denote
	// through a call) are all free of calls, conversions and channel
	// receives — value-identical re-reads with no effects to lose.
	s, old, new := call.Args[0], call.Args[1], call.Args[2]
	if !ps3110CallFree(s) || !ps3110CallFree(old) || !ps3110CallFree(new) {
		return nil
	}
	// Both qualifiers must be the same import object: with duplicate
	// strings imports under different names, deleting the Count call
	// could orphan the alias it referenced (the rewritten call only
	// keeps the outer qualifier alive).
	sel := call.Fun.(*ast.SelectorExpr)   // shape established by ps5012Match
	csel := count.Fun.(*ast.SelectorExpr) // shape established by ps5012Match
	qid, qok := ps2108Unparen(sel.X).(*ast.Ident)
	cid, cok := ps2108Unparen(csel.X).(*ast.Ident)
	if !qok || !cok {
		return nil
	}
	qpkg, qIsPkg := pass.TypesInfo.Uses[qid].(*types.PkgName)
	cpkg, cIsPkg := pass.TypesInfo.Uses[cid].(*types.PkgName)
	if !qIsPkg || !cIsPkg || qpkg != cpkg {
		return nil
	}
	// The replaced spans are the punctuation around the kept operands —
	// including the whole ", strings.Count(s, old))" tail; a comment
	// anywhere there would be silently destroyed — advisory then.
	if ps2111CommentIn(f, call.Pos(), s.Pos()) ||
		ps2111CommentIn(f, s.End(), old.Pos()) ||
		ps2111CommentIn(f, old.End(), new.Pos()) ||
		ps2111CommentIn(f, new.End(), call.End()) {
		return nil
	}
	qualifier := exprTextRendered(sel.X)
	return &analysis.SuggestedFix{
		Message: "replace with " + qualifier + ".ReplaceAll(s, old, new)",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: s.Pos(), NewText: []byte(qualifier + ".ReplaceAll(")},
			{Pos: s.End(), End: old.Pos(), NewText: []byte(", ")},
			{Pos: old.End(), End: new.Pos(), NewText: []byte(", ")},
			{Pos: new.End(), End: call.End(), NewText: []byte(")")},
		},
	}
}
