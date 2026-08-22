package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5075 reports repeated application of standard-library normalizers that
// are idempotent: once a value is normalized, applying the same function again
// cannot change it.
var PS5075 = register(&lint.Check{
	ID:       "PS5075",
	Category: "arith",
	Slug:     "nested-idempotent-stdlib-normalizer",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a standard-library normalizer is applied repeatedly even though one application is already stable",
		Text: `Several unary standard-library functions are idempotent: applying
them once produces a value on which the same function is a no-op. Nested calls
therefore repeat a scan, Unicode mapping, path normalization, floating-point
operation, or slice compaction without changing the result:

  strings.ToLower(strings.ToLower(s)) -> strings.ToLower(s)
  bytes.TrimSpace(bytes.TrimSpace(b)) -> bytes.TrimSpace(b)
  filepath.Clean(filepath.Clean(p))   -> filepath.Clean(p)
  math.Round(math.Round(x))           -> math.Round(x)
  slices.Compact(slices.Compact(xs))  -> slices.Compact(xs)

The complete family covers strings ToLower, ToUpper, ToTitle, and TrimSpace;
bytes.TrimSpace; path.Clean and filepath.Clean; math Abs, Ceil, Floor, Trunc,
Round, and RoundToEven; unicode ToLower, ToUpper, and ToTitle; and
slices.Compact. Arbitrarily deep runs collapse in one fix. The allocating
bytes case-mappers are deliberately excluded: a second bytes.ToLower/ToUpper/
ToTitle can change the observable slice capacity even when its bytes are
unchanged.

The rewrite is BIT-IDENTICAL. Case mapping outputs only runes already stable
under that same mapping; TrimSpace outputs no leading or trailing Unicode
space; Clean outputs canonical paths; each listed math function maps its
result to a fixed point while preserving its defined NaN, infinity, and signed
zero behavior; and Compact's output contains no adjacent equal pair. Invalid
UTF-8 normalization, nil/empty slice identity, slice length/capacity and
cleared tail, and filepath platform rules remain unchanged. The retained
innermost call still evaluates the original argument exactly once.

The shared repeated-call matcher resolves every layer through go/types.
Aliases and explicit generic instantiations with identical result types work;
shadowed functions, same-named user helpers, methods, cross-function
compositions, dot imports, a change of import binding, and mixed generic
instantiations whose result types differ do not match. The result-type guard
preserves the dynamic type when a collapsed expression is stored in an
interface. Independent repeated runs nested inside the retained argument
receive separate non-overlapping diagnostics.

The fix keeps the innermost call byte-for-byte and removes only redundant
outer scaffolding. A comment in removed scaffolding keeps the diagnostic
advisory so no comment is lost.`,
		Before: `normalized := strings.ToLower(strings.ToLower(payload))`,
		After:  `normalized := strings.ToLower(payload)`,
		MeasuredWin: `BenchmarkPS5075 on a 64 KiB uppercase ASCII string (Apple M2 Pro,
go1.26; five runs): nested strings.ToLower median 224849 ns/op,
65536 B/op, 1 alloc/op -> one strings.ToLower median 172614 ns/op,
65536 B/op, 1 alloc/op (~1.30x, -23.23%). The eliminated outer call allocates
nothing because its input is already lowercase, but still scans all 64 KiB.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5075",
		Doc:  "repeated application of an idempotent standard-library unary normalizer",
		Run:  runPS5075,
	},
})

var ps5075Functions = map[string]map[string]bool{
	"strings": {
		"ToLower": true, "ToUpper": true, "ToTitle": true, "TrimSpace": true,
	},
	"bytes": {
		"TrimSpace": true,
	},
	"path":          {"Clean": true},
	"path/filepath": {"Clean": true},
	"math": {
		"Abs": true, "Ceil": true, "Floor": true, "Trunc": true, "Round": true, "RoundToEven": true,
	},
	"unicode": {
		"ToLower": true, "ToUpper": true, "ToTitle": true,
	},
	"slices": {"Compact": true},
}

func runPS5075(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		covered := make(map[*ast.CallExpr]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			outer, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			// PS5111 owns a complete repeated-Clean -> fixed-point-producer
			// composition so all wrappers disappear in one non-overlapping fix.
			if _, ok := ps5111CleanedProducerChain(pass, outer); ok {
				return true
			}
			if covered[outer] {
				return true
			}
			matched, ok := matchRepeatedTypedUnaryPackageCall(pass, outer, ps5075Allowed)
			if !ok {
				return true
			}
			pkgPath := matched.fn.Pkg().Path()
			name := matched.fn.Name()
			diagnostic := analysis.Diagnostic{
				Pos:     outer.Pos(),
				End:     outer.End(),
				Message: fmt.Sprintf("%s.%s is applied %d times; it is idempotent, so the extra %d layer(s) repeat work without changing the result", pkgPath, name, matched.layers, matched.layers-1),
			}
			if !ps2111CommentIn(file, outer.Pos(), matched.keep.Pos()) &&
				!ps2111CommentIn(file, matched.keep.End(), outer.End()) {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "remove redundant normalizer calls",
					TextEdits: []analysis.TextEdit{
						{Pos: outer.Pos(), End: matched.keep.Pos()},
						{Pos: matched.keep.End(), End: outer.End()},
					},
				}}
			}
			pass.Report(diagnostic)
			markRepeatedTypedCall(covered, matched)
			return true
		})
	}
	return nil, nil
}

func ps5075Allowed(pkgPath, name string) bool {
	return ps5075Functions[pkgPath][name]
}
