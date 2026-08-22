package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5055 reports slices.Equal(a, b) and slices.Compare(a, b) where a and b are
// byte slices: the generic element-by-element loop (comparing one byte per
// iteration) where bytes.Equal / bytes.Compare dispatch to the architecture's
// SIMD memequal / memcmp. slices.Equal -> bytes.Equal and slices.Compare ->
// bytes.Compare return the identical result an order of magnitude faster. The
// Equal/Compare companions of PS5050 (slices.Index/Contains -> bytes.IndexByte).
var PS5055 = register(&lint.Check{
	ID:       "PS5055",
	Category: "arith",
	Slug:     "slices-equal-compare-bytes",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.Equal/Compare over byte slices runs the generic element loop; bytes.Equal/Compare runs the SIMD routine for the identical result",
		Text: `slices.Equal(a, b) and slices.Compare(a, b), when a and b are []byte,
resolve to the generic slices loop: a length check then an element-by-element
comparison, one byte per iteration. bytes.Equal / bytes.Compare dispatch to the
architecture-optimized routine (memequal / memcmp), comparing many bytes per
instruction. slices.Equal(a, b) -> bytes.Equal(a, b) returns the identical bool
and slices.Compare(a, b) -> bytes.Compare(a, b) the identical -1/0/+1.

The rewrite is BYTE-IDENTICAL: both compare raw bytes with the same total order
(shorter is less for Compare; equal length and content for Equal), so the result
agrees for every pair — empty or nil, equal, prefix, differing — pinned by the
equivalence suite over adversarial and randomized slices. Both arguments are
evaluated once.

The match is deliberately narrow — it is the whole safety story:
  - the callee is the package-level slices.Equal or slices.Compare, pinned by
    type information (a shadowed slices, an aliased import kept verbatim, or a
    same-named method never matches), with two arguments and no spread;
  - the first argument's element type is EXACTLY byte (uint8). A []byte or a
    named slice whose underlying type is []byte both qualify (assignable to the
    bytes routine's []byte parameters, and slices.Equal/Compare return the
    unnamed bool/int regardless); a defined byte element type is excluded.
    slices.Equal/Compare require both arguments to share the slice type, so the
    second is a byte slice whenever the first is;
  - bytes must be importable at the site and, because the rewrite drops these
    slices references, the fix is withheld file-wide unless slices retains
    another use afterward — so it never orphans the slices import (that residual
    case is advisory).
The fix renames the callee (slices.Equal -> bytes.Equal, slices.Compare ->
bytes.Compare; an aliased bytes import keeps its qualifier), keeping both
arguments verbatim. A comment inside the renamed selector withholds the fix.`,
		Before: `if slices.Equal(a, b) {
	n := slices.Compare(a, b)`,
		After: `if bytes.Equal(a, b) {
	n := bytes.Compare(a, b)`,
		MeasuredWin: `1281-byte slices (Apple M2 Pro, go1.26): slices.Equal ~404 ns/op -> ` +
			`bytes.Equal ~0.3 ns/op (equal inputs; the SIMD path plus early exits), and ` +
			`slices.Compare ~1190 ns/op -> bytes.Compare ~38 ns/op (~31x); both 0 B/op, ` +
			`0 allocs/op. The generic element loop compares one byte per iteration; the bytes ` +
			`routine compares a machine word at a time.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5055",
		Doc:  "slices.Equal/Compare over byte slices instead of bytes.Equal/Compare",
		Run:  runPS5055,
	},
})

func runPS5055(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		type site struct {
			diag analysis.Diagnostic
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		importAdded := false
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "slices" {
				return true
			}
			name := fn.Name()
			if name != "Equal" && name != "Compare" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// The element type must be exactly byte, so the slices are assignable
			// to the bytes routine's []byte parameters and the comparisons are
			// byte-wise identical.
			st, ok := pass.TypesInfo.TypeOf(call.Args[0]).Underlying().(*types.Slice)
			if !ok || !types.Identical(st.Elem(), types.Typ[types.Uint8]) {
				return true
			}

			msg := "slices." + name + " over byte slices runs the generic element loop; bytes." +
				name + " runs the SIMD routine for the identical result"
			s := site{diag: analysis.Diagnostic{Pos: call.Pos(), End: call.End(), Message: msg}}

			if ps2111CommentIn(f, sel.Pos(), sel.End()) {
				s.diag.Message = msg + "; a comment inside the renamed selector withholds the automatic fix — rewrite by hand"
			} else {
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "bytes", "bytes")
				if usable && !(needImport && ps2107ImportsC(f)) {
					edits := []analysis.TextEdit{
						{Pos: sel.Pos(), End: sel.End(), NewText: []byte(useName + "." + name)},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "bytes"))
						importAdded = true
					}
					s.fix = &analysis.SuggestedFix{
						Message:   "replace slices." + name + " with " + useName + "." + name,
						TextEdits: edits,
					}
					fixable++
				}
			}
			sites = append(sites, s)
			return true
		})

		// Withhold every fix in the file unless slices retains a use after the
		// rewrite drops these references — never orphan the slices import.
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "slices") > fixable
		for _, s := range sites {
			d := s.diag
			if emitFixes && s.fix != nil {
				d.SuggestedFixes = []analysis.SuggestedFix{*s.fix}
			}
			pass.Report(d)
		}
	}
	return nil, nil
}
