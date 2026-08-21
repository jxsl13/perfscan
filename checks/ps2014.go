package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// ps2014MaxIndex is the largest constant index the check rewrites. The
// win is the tail of the input NOT scanned past field i, so it is
// biggest for small i; a huge constant index both shrinks the win and
// suggests the surrounding code wants most of the pieces anyway. The
// bound also keeps the inserted literal n = i+2 trivially overflow-free.
const ps2014MaxIndex = 16

// PS2014 reports strings.Split(s, sep)[i] for a constant index i >= 1 —
// scanning the whole input and allocating a []string of every piece just
// to read one early field — where strings.SplitN(s, sep, i+2)[i] stops
// scanning after the (i+1)-th separator and materializes at most i+2
// pieces. The bytes twin is matched the same way. Index 0 is PS2009's
// domain and is never matched here.
var PS2014 = register(&lint.Check{
	ID:       "PS2014",
	Category: "alloc",
	Slug:     "split-index",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.Split(s, sep)[i] with a small constant i allocates every piece just to read one field; strings.SplitN(s, sep, i+2)[i] stops after that field",
		Text: `strings.Split scans the ENTIRE input for every separator
occurrence and allocates a []string holding every piece; indexing a
constant [i] then discards all but one field. strings.SplitN(s, sep,
i+2) stops scanning after the (i+1)-th separator and materializes at
most i+2 pieces, so the work is O(prefix through field i) instead of
O(len(s)) and the slice is fixed-tiny. This is PS2009 (the [0] head
read) generalized to any small constant index; the same holds for
bytes.Split versus bytes.SplitN.

The rewrite is bit-identical for ALL inputs. SplitN(s, sep, i+2)
produces min(k+1, i+2) pieces for k separator occurrences, and ONLY
the LAST produced piece can be the un-split remainder; every earlier
piece is boundary-for-boundary identical to Split's. Since the code
indexes i < i+1, piece i is never the truncated remainder, so it
equals Split(s, sep)[i] exactly — same bytes for strings, and for
bytes the same capped subslice of the same backing array (identical
len, cap, aliasing and nil-ness; when piece i happens to be the FINAL
piece, i.e. exactly i separators, both forms return the same uncapped
remainder). Panic behavior is preserved: Split yields k+1 pieces and
SplitN yields min(k+1, i+2); [i] is in range iff there are at least
i+1 pieces, and min(k+1, i+2) >= i+1 iff k+1 >= i+1, so both forms
panic — or both succeed — on identical inputs. The empty separator is
fine too: Split(s, "") rune-explodes while SplitN(s, "", i+2) yields
the first i+1 runes plus a remainder, and piece i is the i-th rune in
both (verified across multibyte and invalid UTF-8, which only move
rune boundaries identically for the two forms). n = i+2 is exactly
right: i+1 would make piece i the remainder and CHANGE the value.

The callee is pinned with type information to the package-level
strings.Split / bytes.Split (a shadowed strings/bytes or a local or
method Split does not resolve there; SplitAfter and Fields are never
matched). The index must be a constant integer with 1 <= i <= 16 —
[0] is PS2009, a variable index may take any value at run time, and a
huge constant index shrinks the win while suggesting the code wants
most pieces anyway. The Split call must be indexed DIRECTLY: a result
stored in a variable first may have other consumers. Both argument
expressions stay byte-verbatim (evaluation order and side-effect
count unchanged) — the fix only renames Split to SplitN and inserts
the literal ", i+2" after the separator argument. No import changes,
and the indexed call remains a primary expression, so no
parenthesization is ever needed.`,
		Before: `third := strings.Split(s, ",")[2]`,
		After:  `third := strings.SplitN(s, ",", 4)[2]`,
		MeasuredWin: `BenchmarkPS2014 (a ~1.3KB line of 64 comma-separated
~19-byte fields, field [2] taken once per op, Apple M2 Pro):
Split[2] 782 ns/op, 1152 B/op, 1 alloc/op vs SplitN(4)[2]
36.0 ns/op, 64 B/op, 1 alloc/op (~22x faster, 18x less memory). The
win grows with the number of separators after field i: Split pays a
scan and a slice header for every piece, SplitN(i+2) pays for the
prefix through field i only.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2014",
		Doc:  "strings.Split(s, sep)[i] with a small constant i scans and allocates every piece just for one field; strings.SplitN(s, sep, i+2)[i] stops after that field",
		Run:  runPS2014,
	},
})

func runPS2014(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			idx, ok := n.(*ast.IndexExpr)
			if !ok {
				return true
			}
			// The index must be a constant integer in [1, ps2014MaxIndex]:
			// 0 is PS2009's domain, a variable may be anything at run time,
			// and a negative constant index does not compile. The bound
			// keeps the rewrite where the win is real (small early fields).
			i, ok := ps2014ConstIndex(pass, idx.Index)
			if !ok {
				return true
			}
			// The indexed operand must be DIRECTLY the Split call (parens
			// allowed): a slice stored in a variable first may have other
			// consumers — out of scope.
			call, ok := ps2121Unparen(idx.X).(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// Type info pins the callee to the package-level strings.Split
			// or bytes.Split: a shadowed strings/bytes resolves sel.Sel to
			// some other object, and a method carries a receiver. SplitAfter
			// and Fields shape their pieces differently and are never
			// matched; SplitN is already limited.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Name() != "Split" || fn.Pkg() == nil {
				return true
			}
			pkgPath := fn.Pkg().Path()
			if pkgPath != "strings" && pkgPath != "bytes" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// Two edits keep both arguments byte-verbatim: rename the Split
			// selector (a bare identifier — it cannot contain a comment) and
			// INSERT ", i+2" right after the separator argument, so any
			// comment or trailing comma between it and the closing
			// parenthesis survives untouched. No import ever changes and the
			// indexed call stays a primary expression, so the fix is always
			// attached.
			nLim := i + 2
			pass.Report(analysis.Diagnostic{
				Pos: idx.Pos(),
				End: idx.End(),
				Message: fmt.Sprintf(
					"%[1]s.Split(...)[%[2]d] scans the whole input and allocates every piece just to read one field; %[1]s.SplitN(..., %[3]d)[%[2]d] stops after that field and is bit-identical",
					pkgPath, i, nLim),
				SuggestedFixes: []analysis.SuggestedFix{{
					Message: fmt.Sprintf("replace with %s.SplitN(..., %d)[%d]", pkgPath, nLim, i),
					TextEdits: []analysis.TextEdit{
						{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("SplitN")},
						{Pos: call.Args[1].End(), End: call.Args[1].End(), NewText: fmt.Appendf(nil, ", %d", nLim)},
					},
				}},
			})
			return true
		})
	}
	return nil, nil
}

// ps2014ConstIndex returns the constant integer value of the index
// expression when it is in [1, ps2014MaxIndex] — the precondition for
// Split(...)[i] == SplitN(..., i+2)[i] being both bit-identical and a
// worthwhile rewrite. Index 0 belongs to PS2009.
func ps2014ConstIndex(pass *analysis.Pass, index ast.Expr) (int64, bool) {
	tv, ok := pass.TypesInfo.Types[index]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return 0, false
	}
	v, exact := constant.Int64Val(tv.Value)
	if !exact || v < 1 || v > ps2014MaxIndex {
		return 0, false
	}
	return v, true
}
