package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// ps2023MaxIndex is the largest constant index the check rewrites. The
// win is the tail of the input NOT scanned past piece i, so it is
// biggest for small i; a huge constant index both shrinks the win and
// suggests the surrounding code wants most of the pieces anyway. The
// bound also keeps the inserted literal n = i+2 trivially overflow-free
// (it mirrors ps2014MaxIndex for the Split sibling).
const ps2023MaxIndex = 16

// PS2023 reports strings.SplitAfter(s, sep)[i] for a constant index
// 0 <= i <= 16 — scanning the whole input and allocating a []string of
// every piece just to read one early field — where
// strings.SplitAfterN(s, sep, i+2)[i] stops scanning after the (i+1)-th
// separator and materializes at most i+2 pieces. The bytes twin is
// matched the same way. This is the SplitAfter sibling of PS2009/PS2014,
// which deliberately never match SplitAfter; index 0 is in scope here
// because no other check covers the SplitAfter head read.
var PS2023 = register(&lint.Check{
	ID:       "PS2023",
	Category: "alloc",
	Slug:     "splitafter-index",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.SplitAfter(s, sep)[i] with a small constant i allocates every piece just to read one field; strings.SplitAfterN(s, sep, i+2)[i] stops after that field",
		Text: `strings.SplitAfter scans the ENTIRE input for every separator
occurrence and allocates a []string holding every piece; indexing a
constant [i] then discards all but one field. strings.SplitAfterN(s,
sep, i+2) stops scanning after the (i+1)-th separator and materializes
at most i+2 pieces, so the work is O(prefix through field i) instead
of O(len(s)) and the slice is fixed-tiny. This is the PS2009/PS2014
reduction applied to the SplitAfter member those checks deliberately
skip; the same holds for bytes.SplitAfter versus bytes.SplitAfterN.

The rewrite is bit-identical for ALL inputs. SplitAfter and
SplitAfterN share one core: the ONLY thing SplitAfter changes over
Split — keeping the separator attached to the end of each piece — is
applied identically in the eager and N-limited forms, so the
piece-boundary relationship is structurally the one PS2014 already
proves for Split. SplitAfterN(s, sep, i+2) produces min(k+1, i+2)
pieces for k separator occurrences, and ONLY the LAST produced piece
can be the un-split remainder; every earlier piece is
boundary-for-boundary identical to SplitAfter's. Since the code
indexes i < i+1, piece i is never the truncated remainder, so it
equals SplitAfter(s, sep)[i] exactly — same bytes for strings, and
for bytes the same capped subslice of the same backing array
(identical len, cap, aliasing and nil-ness; when piece i happens to
be the FINAL piece, i.e. exactly i separators, both forms return the
same uncapped remainder). Panic behavior is preserved: SplitAfter
yields k+1 pieces and SplitAfterN yields min(k+1, i+2); [i] is in
range iff there are at least i+1 pieces, and min(k+1, i+2) >= i+1 iff
k+1 >= i+1, so both forms panic — or both succeed — on identical
inputs. The empty separator is fine too: BOTH forms reduce to the
same rune-explode path (the separator-attachment tweak is unused
there), so piece i is the i-th rune in either form (verified across
multibyte and invalid UTF-8, which only move rune boundaries
identically for the two forms). n = i+2 is exactly right: i+1 would
make piece i the remainder and CHANGE the value.

The callee is pinned with type information to the package-level
strings.SplitAfter / bytes.SplitAfter (a shadowed strings/bytes or a
local or method SplitAfter does not resolve there; Split and Fields
are never matched — Split is PS2009/PS2014's domain). The index must
be a constant integer with 0 <= i <= 16 — a variable index may take
any value at run time, and a huge constant index shrinks the win
while suggesting the code wants most pieces anyway. The SplitAfter
call must be indexed DIRECTLY: a result stored in a variable first
may have other consumers. Both argument expressions stay
byte-verbatim (evaluation order and side-effect count unchanged) —
the fix only renames SplitAfter to SplitAfterN and inserts the
literal ", i+2" after the separator argument. No import changes, and
the indexed call remains a primary expression, so no parenthesization
is ever needed.`,
		Before: `third := strings.SplitAfter(s, ",")[2]`,
		After:  `third := strings.SplitAfterN(s, ",", 4)[2]`,
		MeasuredWin: `BenchmarkPS2023 (a ~1.3KB line of 64 comma-separated
~19-byte fields, field [2] taken once per op, Apple M2 Pro):
SplitAfter[2] 875 ns/op, 1152 B/op, 1 alloc/op vs SplitAfterN(4)[2]
34.6 ns/op, 64 B/op, 1 alloc/op (~25x faster, 18x less memory). The
win grows with the number of separators after field i: SplitAfter
pays a scan and a slice header for every piece, SplitAfterN(i+2) pays
for the prefix through field i only.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2023",
		Doc:  "strings.SplitAfter(s, sep)[i] with a small constant i scans and allocates every piece just for one field; strings.SplitAfterN(s, sep, i+2)[i] stops after that field",
		Run:  runPS2023,
	},
})

func runPS2023(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			idx, ok := n.(*ast.IndexExpr)
			if !ok {
				return true
			}
			// The index must be a constant integer in [0, ps2023MaxIndex]:
			// a variable may be anything at run time, and a negative
			// constant index does not compile. Unlike PS2014, index 0 is in
			// scope — PS2009 covers only Split, never SplitAfter. The bound
			// keeps the rewrite where the win is real (small early fields).
			i, ok := ps2023ConstIndex(pass, idx.Index)
			if !ok {
				return true
			}
			// The indexed operand must be DIRECTLY the SplitAfter call
			// (parens allowed): a slice stored in a variable first may have
			// other consumers — out of scope.
			call, ok := ps2121Unparen(idx.X).(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// Type info pins the callee to the package-level
			// strings.SplitAfter or bytes.SplitAfter: a shadowed
			// strings/bytes resolves sel.Sel to some other object, and a
			// method carries a receiver. Split and Fields shape their
			// pieces differently and are never matched here (Split is
			// PS2009/PS2014's domain); SplitAfterN is already limited.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Name() != "SplitAfter" || fn.Pkg() == nil {
				return true
			}
			pkgPath := fn.Pkg().Path()
			if pkgPath != "strings" && pkgPath != "bytes" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// Two edits keep both arguments byte-verbatim: rename the
			// SplitAfter selector (a bare identifier — it cannot contain a
			// comment) and INSERT ", i+2" right after the separator
			// argument, so any comment or trailing comma between it and the
			// closing parenthesis survives untouched. No import ever
			// changes and the indexed call stays a primary expression, so
			// the fix is always attached.
			nLim := i + 2
			pass.Report(analysis.Diagnostic{
				Pos: idx.Pos(),
				End: idx.End(),
				Message: fmt.Sprintf(
					"%[1]s.SplitAfter(...)[%[2]d] scans the whole input and allocates every piece just to read one field; %[1]s.SplitAfterN(..., %[3]d)[%[2]d] stops after that field and is bit-identical",
					pkgPath, i, nLim),
				SuggestedFixes: []analysis.SuggestedFix{{
					Message: fmt.Sprintf("replace with %s.SplitAfterN(..., %d)[%d]", pkgPath, nLim, i),
					TextEdits: []analysis.TextEdit{
						{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("SplitAfterN")},
						{Pos: call.Args[1].End(), End: call.Args[1].End(), NewText: fmt.Appendf(nil, ", %d", nLim)},
					},
				}},
			})
			return true
		})
	}
	return nil, nil
}

// ps2023ConstIndex returns the constant integer value of the index
// expression when it is in [0, ps2023MaxIndex] — the precondition for
// SplitAfter(...)[i] == SplitAfterN(..., i+2)[i] being both
// bit-identical and a worthwhile rewrite.
func ps2023ConstIndex(pass *analysis.Pass, index ast.Expr) (int64, bool) {
	tv, ok := pass.TypesInfo.Types[index]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return 0, false
	}
	v, exact := constant.Int64Val(tv.Value)
	if !exact || v < 0 || v > ps2023MaxIndex {
		return 0, false
	}
	return v, true
}
