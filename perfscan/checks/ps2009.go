package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2009 reports strings.Split(s, sep)[0] — scanning the whole input and
// allocating a []string of every piece just to read the head — where
// strings.SplitN(s, sep, 2)[0] stops at the first separator and allocates
// at most a two-element slice. The bytes twin is matched the same way.
var PS2009 = register(&lint.Check{
	ID:       "PS2009",
	Category: "alloc",
	Slug:     "split-head",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.Split(s, sep)[0] allocates every piece just to read the head; strings.SplitN(s, sep, 2)[0] stops at the first separator",
		Text: `strings.Split scans the ENTIRE input for every separator
occurrence and allocates a []string holding every piece; indexing [0]
then discards all but the head. strings.SplitN(s, sep, 2) stops
scanning after the FIRST separator and materializes at most two
pieces, so the work is O(prefix) instead of O(len(s)) and the slice is
fixed-tiny. The same holds for bytes.Split versus bytes.SplitN.

The rewrite is bit-identical for ALL inputs — unlike strings.Cut,
which diverges on the empty separator (Cut(s, "") returns "" while
Split(s, "")[0] is the first rune). Split is SplitN with n = -1, and
the first piece never depends on n: with a separator present both
return the prefix up to it, with none both return the whole input,
and for the empty separator both take the same rune-explode path and
return the first rune (verified across multibyte and invalid UTF-8).
Panic behavior is preserved too: the only empty result is
Split("", "") vs SplitN("", "", 2) — both yield an empty slice and
both panic identically on [0]. For bytes, the head is the same capped
subslice of the same backing array (identical len, cap, aliasing, and
nil-ness). Both arguments are kept verbatim, so evaluation order and
side-effect count are unchanged.

The callee is pinned with type information to the package-level
strings.Split / bytes.Split (a shadowed strings/bytes or a local or
method Split does not resolve there; SplitAfter and Fields are never
matched). The index must be a constant integer 0 — [1], [i] or [n]
need pieces beyond the head and are out of scope. The Split call must
be indexed DIRECTLY: a result stored in a variable first may have
other consumers. SplitN takes the same two arguments plus the literal
limit, so the fix only renames Split to SplitN and inserts ", 2"
after the separator argument — both argument expressions stay
byte-verbatim, no import changes, and the indexed call remains a
primary expression, so no parenthesization is ever needed.`,
		Before: `first := strings.Split(s, sep)[0]`,
		After:  `first := strings.SplitN(s, sep, 2)[0]`,
		MeasuredWin: `BenchmarkPS2009 (a ~1.3KB line of 64 comma-separated
~19-byte fields, head taken once per op, Apple M2 Pro): Split[0]
975 ns/op, 1152 B/op, 1 alloc/op vs SplitN(2)[0] 28.7 ns/op,
32 B/op, 1 alloc/op (~34x faster, 36x less memory). The win grows
with the number of separators after the first: Split pays a scan and
a slice header for every piece, SplitN(2) pays for the prefix only.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2009",
		Doc:  "strings.Split(s, sep)[0] scans and allocates every piece just for the head; strings.SplitN(s, sep, 2)[0] stops at the first separator",
		Run:  runPS2009,
	},
})

func runPS2009(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			idx, ok := n.(*ast.IndexExpr)
			if !ok {
				return true
			}
			// The index must be a constant integer 0: any other index needs
			// pieces beyond the head, which SplitN(..., 2) does not provide.
			if !ps2009IndexIsZero(pass, idx.Index) {
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
			// Type info pins the callee to the package-level strings.Split or
			// bytes.Split: a shadowed strings/bytes resolves sel.Sel to some
			// other object, and a method carries a receiver. SplitAfter and
			// Fields shape their pieces differently and are never matched;
			// SplitN is already limited.
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
			// INSERT ", 2" right after the separator argument, so any comment
			// or trailing comma between it and the closing parenthesis
			// survives untouched. No import ever changes and the indexed call
			// stays a primary expression, so the fix is always attached.
			pass.Report(analysis.Diagnostic{
				Pos: idx.Pos(),
				End: idx.End(),
				Message: fmt.Sprintf(
					"%[1]s.Split(...)[0] scans the whole input and allocates every piece just to read the head; %[1]s.SplitN(..., 2)[0] stops at the first separator and is bit-identical",
					pkgPath),
				SuggestedFixes: []analysis.SuggestedFix{{
					Message: "replace with " + pkgPath + ".SplitN(..., 2)[0]",
					TextEdits: []analysis.TextEdit{
						{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("SplitN")},
						{Pos: call.Args[1].End(), End: call.Args[1].End(), NewText: []byte(", 2")},
					},
				}},
			})
			return true
		})
	}
	return nil, nil
}

// ps2009IndexIsZero reports whether the index expression is a constant
// integer with the exact value 0 (a literal 0 or a named constant that
// evaluates to it) — the precondition for Split(...)[i] == SplitN(..., 2)[i].
func ps2009IndexIsZero(pass *analysis.Pass, index ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[index]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return false
	}
	v, exact := constant.Int64Val(tv.Value)
	return exact && v == 0
}
