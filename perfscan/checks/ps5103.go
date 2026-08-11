package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5103 reports case-insensitive string comparison spelled as
// strings.ToLower(a) == strings.ToLower(b) (or ToUpper, or !=), which
// allocates two converted strings; strings.EqualFold answers the same
// question allocation-free. Advisory only: EqualFold uses Unicode simple
// case-folding, which is NOT bit-identical to ToLower/ToUpper equality
// for all inputs, so no mechanical rewrite is attached.
var PS5103 = register(&lint.Check{
	ID:       "PS5103",
	Category: "arith",
	Slug:     "case-insensitive-compare-via-tolower",
	Level:    lint.LevelIdiomatic,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "case-insensitive compare via ToLower/ToUpper equality, where strings.EqualFold is allocation-free",
		Text: `strings.ToLower(a) == strings.ToLower(b) builds two fully
converted throwaway strings — two allocations plus two O(n) transform
passes — only to compare them and discard both. strings.EqualFold walks
the two operands in lockstep, folding rune by rune, allocates nothing,
and bails at the first mismatching rune instead of always converting
both strings to the end. The same applies to ToUpper on both sides and
to the != spelling.

Why this is advisory and not an auto-fix: the two spellings are NOT
bit-identical on all inputs. EqualFold uses Unicode simple case-folding
while ToLower applies the simple lowercase mapping, and the two
relations genuinely disagree on special-cased runes — in both
directions. Measured on this Go toolchain:

    strings.ToLower("ſ") == strings.ToLower("S")  → false
    strings.EqualFold("ſ", "S")                   → true   (U+017F LATIN SMALL LETTER LONG S folds to s)

    strings.ToLower("İ") == strings.ToLower("i")  → true
    strings.EqualFold("İ", "i")                   → false  (U+0130 LATIN CAPITAL LETTER I WITH DOT ABOVE folds only to itself)

A rewrite would therefore change observable behavior for non-ASCII
data, and whether both operands are ASCII-only is not statically
provable for ordinary variables. Under perfscan's bit-identical bar the
check only reports; apply the change manually where you either want
EqualFold's (more standard) case-insensitive semantics or know the data
is ASCII — for pure-ASCII inputs the two spellings agree exactly.

Ordering comparisons (ToLower(a) < ToLower(b)) and one-sided forms
(ToLower(a) == b) are deliberately not flagged: EqualFold answers only
equality of two folded operands. Mixed ToLower-vs-ToUpper operands are
also skipped — that is not a case-insensitive equality test.`,
		Before: `if strings.ToLower(name) == strings.ToLower(want) {
	return true
}`,
		After: `// Manual change — verify Unicode semantics (see check text):
if strings.EqualFold(name, want) {
	return true
}`,
		MeasuredWin: `BenchmarkPS5103 (1024 pairs of ~7-byte ASCII words,
equal case-insensitively, Apple M2 Pro): 48.9 µs/op, 8.4 KB, 1024
allocs per pass -> 8.2 µs/op, 0 B, 0 allocs (~5.9x). Only one side
allocated in Before because ToLower returns its input unchanged when
nothing needs converting; a truly mixed-case pair pays two allocations,
and mismatching pairs win by more still — EqualFold stops at the first
differing rune while ToLower always converts both strings entirely.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5103",
		Doc:  "case-insensitive compare via ToLower/ToUpper equality instead of strings.EqualFold",
		Run:  runPS5103,
	},
})

func runPS5103(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
				return true
			}
			// Both operands must be calls to the SAME strings case
			// function: ToLower==ToLower or ToUpper==ToUpper. Mixed
			// operands are not a case-insensitive equality test.
			name := ps5103CaseCallName(pass, bin.X)
			if name == "" || name != ps5103CaseCallName(pass, bin.Y) {
				return true
			}
			op, repl := "==", "strings.EqualFold(a, b)"
			if bin.Op == token.NEQ {
				op, repl = "!=", "!strings.EqualFold(a, b)"
			}
			pass.Report(analysis.Diagnostic{
				Pos: bin.Pos(),
				End: bin.End(),
				Message: "strings." + name + "(a) " + op + " strings." + name + "(b) allocates two converted strings; " +
					repl + " is allocation-free (manual change: EqualFold's Unicode simple case-folding is not bit-identical to " + name + " equality)",
			})
			return true
		})
	}
	return nil, nil
}

// ps5103CaseCallName returns "ToLower" or "ToUpper" if e (possibly
// parenthesized) is a direct call of that package-level function from the
// standard library strings package, and "" otherwise. Type information
// pins the callee: a method or local function spelled ToLower, or a
// shadowed `strings` identifier, does not resolve to a receiver-less
// *types.Func with package path "strings" and is rejected.
func ps5103CaseCallName(pass *analysis.Pass, e ast.Expr) string {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			break
		}
		e = p.X
	}
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis != token.NoPos {
		return ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "strings" {
		return ""
	}
	if fn.Name() != "ToLower" && fn.Name() != "ToUpper" {
		return ""
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return ""
	}
	return fn.Name()
}
