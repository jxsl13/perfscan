package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5019 reports bytes.Replace(b, old, new, bytes.Count(b, old)) —
// an explicit Count of the very slice Replace is about to Count again
// internally, so b is walked twice for one replace-all — and rewrites it
// to bytes.ReplaceAll(b, old, new), the identical replacement in a
// single scan. This is the bytes twin of PS5012: the count-to-cap
// equality n == Count(b, old) makes the two forms byte-identical on
// every input (see the Doc and the runtime differential in
// equiv_PS5019_test.go).
var PS5019 = register(&lint.Check{
	ID:       "PS5019",
	Category: "arith",
	Slug:     "bytes-replace-counted-all",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "bytes.Replace(b, old, new, bytes.Count(b, old)) scans b twice for one replace-all; bytes.ReplaceAll(b, old, new) is the identical substitution in a single scan",
		Text: `bytes.Replace(b, old, new, n) begins by computing
bytes.Count(b, old) ITSELF to size its output and clamp n. Passing
bytes.Count(b, old) as the explicit n therefore buys nothing: the
argument is a full O(len(b)) substring scan whose only effect is to
hand Replace a cap it immediately re-derives, so every call walks b
twice (plus spins old's matcher state up twice). bytes.ReplaceAll(b,
old, new) — literally Replace(b, old, new, -1) — performs the single
internal Count and the replacement: one scan, the same one allocation,
about half the pre-replacement work. This is the bytes twin of PS5012;
the relative win is largest exactly where the pattern hurts most, on
long inputs with few matches, where the redundant Count dominates the
whole call.

The rewrite is byte-identical on every input. Replace clamps its n to
m = Count(b, old) whenever n < 0 or n > m and then replaces exactly
min(n, m) leftmost non-overlapping occurrences; passing n == m replaces
all m of them, which is precisely what ReplaceAll's n = -1 resolves to.
The equality holds on every edge: no match (m == 0 — with an explicit
n == 0 Replace skips its internal Count entirely and both forms return
a fresh copy of b, so unlike the strings twin there is not even an
aliasing wrinkle: NEITHER form ever returns a slice sharing memory
with b), old == new (both produce the same fresh copy), overlapping
candidates (Count and Replace share the same non-overlapping
left-to-right match walk, so the count can never overshoot or
undershoot the sites Replace visits), and even old == nil/empty —
Count(b, nil) is utf8.RuneCount(b)+1, exactly the k+1 boundary
insertions ReplaceAll performs, with identical invalid-UTF-8 treatment
(both decode RuneError width 1). bytes.* is pure literal byte-substring
matching, so arbitrary bytes and invalid UTF-8 round through
identically. The runtime differential test pins the equality (bytes,
length, AND the never-aliases-b property) over exhaustive short operand
triples, targeted seeds and randomized full-byte-range inputs.

The one semantic difference is evaluation COUNT: the Before evaluates b
and old twice each (once inside the explicit Count, once for Replace)
and the After once each. The automatic fix therefore applies only when
the shape is provably order-insensitive: the callee is the standard
library's package-level bytes.Replace with its n argument exactly a
package-level bytes.Count call (both pinned by type information
through the same import — a shadowed or dot-imported bytes never
matches), the two b expressions and the two old expressions are
syntactically identical, and b, old AND new are all free of calls,
conversions and channel receives — so dropping the second evaluation of
b and old is unobservable, and no expression between the first and
second evaluations can mutate what they denote. A match whose operands
do contain calls (bytes.Replace(f(), old, new, bytes.Count(f(), old))
and friends, including []byte(s) conversions) is still reported — it
does strictly redundant double scanning, and worse — but stays
advisory: f() could return different values or carry side effects, so
no mechanical rewrite is bit-identical there. The fix keeps b, old and
new byte-verbatim in place and rewrites only the punctuation around
them (the callee name and the trailing ", bytes.Count(b, old)" tail),
reusing the file's own bytes qualifier, alias included — no import
surgery is ever needed since the rewritten call still references the
same package. A comment anywhere in the replaced punctuation keeps the
report advisory.`,
		Before: `out := bytes.Replace(b, old, new, bytes.Count(b, old))`,
		After:  `out := bytes.ReplaceAll(b, old, new)`,
		MeasuredWin: `BenchmarkPS5019 (4 KiB log payload, 68 "msg=" matches, Apple
M2 Pro, go1.26): bytes.Replace(b, old, new, bytes.Count(b, old))
8132 ns/op, 4864 B/op, 1 alloc/op -> bytes.ReplaceAll(b, old, new)
5687 ns/op, 4864 B/op, 1 alloc/op (~1.43x faster, identical single
allocation: the whole delta is the redundant explicit Count scan of
all 4 KiB — each match forces the substring searcher to stop and
restart, work Count does in full before Replace re-does it). When the
searcher can skip in long SIMD strides (sparse single matches) both
scans are memchr-fast and the gap narrows; it never inverts, since the
After does strictly less work on every input.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5019",
		Doc:  "bytes.Replace(b, old, new, bytes.Count(b, old)) walks b twice — the explicit Count only re-derives the cap Replace computes internally; bytes.ReplaceAll(b, old, new) is the identical replace-all in a single scan",
		Run:  runPS5019,
	},
})

const ps5019Msg = "bytes.Replace(b, old, new, bytes.Count(b, old)) walks b twice: the explicit Count is a full scan whose result Replace re-derives internally anyway; bytes.ReplaceAll(b, old, new) performs the identical replace-all in a single scan"

func runPS5019(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			count, matched := ps5019Match(pass, call)
			if !matched {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: ps5019Msg,
			}
			if fix := ps5019Fix(pass, f, call, count); fix != nil {
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

// ps5019BytesFunc reports whether call invokes the standard library's
// package-level bytes.<name> with exactly nargs non-variadic arguments,
// returning the selector when it does. Pinning through the *types.Func
// object means a shadowed bytes, a dot import or a same-named method
// never matches.
func ps5019BytesFunc(pass *analysis.Pass, call *ast.CallExpr, name string, nargs int) (*ast.SelectorExpr, bool) {
	if len(call.Args) != nargs || call.Ellipsis.IsValid() {
		return nil, false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return nil, false
	}
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || fn.Name() != name || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
		return nil, false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, false
	}
	return sel, true
}

// ps5019Match matches call against
// bytes.Replace(b, old, new, bytes.Count(b, old)) — the outer callee
// and the n argument both pinned to the standard library by type
// information, the two b expressions and the two old expressions
// syntactically identical (parens allowed around the Count call itself,
// nowhere else). It returns the inner Count call.
func ps5019Match(pass *analysis.Pass, call *ast.CallExpr) (count *ast.CallExpr, ok bool) {
	if _, isReplace := ps5019BytesFunc(pass, call, "Replace", 4); !isReplace {
		return nil, false
	}
	count, isCall := ps2108Unparen(call.Args[3]).(*ast.CallExpr)
	if !isCall {
		return nil, false
	}
	if _, isCount := ps5019BytesFunc(pass, count, "Count", 2); !isCount {
		return nil, false
	}
	// The count must be of the SAME haystack and needle, syntactically:
	// Count(b, sep) over any other operand pair is a genuine different
	// cap, not the replace-all idiom.
	if !exprEqual(call.Args[0], count.Args[0]) || !exprEqual(call.Args[1], count.Args[1]) {
		return nil, false
	}
	return count, true
}

// ps5019Fix builds the bytes.ReplaceAll(b, old, new) rewrite for one
// site, or nil when a guard fails and the report must stay advisory. The
// b, old and new expressions are kept byte-verbatim in place (single
// evaluation, original spelling); only the punctuation around them is
// replaced, so the replacement is a call wherever the original call was
// legal and never needs parentheses. No import surgery is ever needed:
// the rewritten call reuses the outer call's own bytes qualifier.
func ps5019Fix(pass *analysis.Pass, f *ast.File, call, count *ast.CallExpr) *analysis.SuggestedFix {
	// Dropping the Count call removes the second evaluation of b and old.
	// That is unobservable only when b, old and new (which is evaluated
	// BETWEEN the two, and could otherwise mutate what they denote
	// through a call) are all free of calls, conversions and channel
	// receives — value-identical re-reads with no effects to lose.
	b, old, new := call.Args[0], call.Args[1], call.Args[2]
	if !ps3110CallFree(b) || !ps3110CallFree(old) || !ps3110CallFree(new) {
		return nil
	}
	// Both qualifiers must be the same import object: with duplicate
	// bytes imports under different names, deleting the Count call
	// could orphan the alias it referenced (the rewritten call only
	// keeps the outer qualifier alive).
	sel := call.Fun.(*ast.SelectorExpr)   // shape established by ps5019Match
	csel := count.Fun.(*ast.SelectorExpr) // shape established by ps5019Match
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
	// including the whole ", bytes.Count(b, old))" tail; a comment
	// anywhere there would be silently destroyed — advisory then.
	if ps2111CommentIn(f, call.Pos(), b.Pos()) ||
		ps2111CommentIn(f, b.End(), old.Pos()) ||
		ps2111CommentIn(f, old.End(), new.Pos()) ||
		ps2111CommentIn(f, new.End(), call.End()) {
		return nil
	}
	qualifier := exprTextRendered(sel.X)
	return &analysis.SuggestedFix{
		Message: "replace with " + qualifier + ".ReplaceAll(b, old, new)",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: b.Pos(), NewText: []byte(qualifier + ".ReplaceAll(")},
			{Pos: b.End(), End: old.Pos(), NewText: []byte(", ")},
			{Pos: old.End(), End: new.Pos(), NewText: []byte(", ")},
			{Pos: new.End(), End: call.End(), NewText: []byte(")")},
		},
	}
}
