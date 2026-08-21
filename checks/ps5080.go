package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5080 removes or collapses heterogeneous Replace/ReplaceAll chains whose
// replacement is provably a content no-op.
var PS5080 = register(&lint.Check{
	ID:       "PS5080",
	Category: "arith",
	Slug:     "noop-replacement-call-chain",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "Replace/ReplaceAll uses equal old/new values or a zero replacement count",
		Text: `strings.Replace and bytes.Replace cannot change content when old and
new are equal or when n is zero. ReplaceAll has the same property when old and
new are equal. Adjacent mixtures of these functions therefore repeat calls,
scans, and—in the bytes package—full-slice copies:

  strings.ReplaceAll(strings.Replace(s, "x", "y", 0), "z", "z") -> s
  bytes.ReplaceAll(bytes.Replace(b, []byte("x"), []byte("x"), -1),
                   []byte("y"), []byte("y"))
  -> bytes.ReplaceAll(b, []byte("y"), []byte("y"))

This check uses a typed function-family chain rather than requiring every
layer to name the same function or have the same arity. Every callee must
resolve through go/types to strings.Replace/ReplaceAll or
bytes.Replace/ReplaceAll through one concrete import binding. String old/new
arguments must be compile-time constants. Byte arguments must be nil, empty
[]byte literals, or []byte conversions of compile-time strings. Replace's n
must be a compile-time integer whenever equality is not the proof.

For strings, each matched call returns exactly its input value, so even a
single layer is removed and a maximal adjacent heterogeneous chain collapses
in one fix. For bytes, the API returns a copy even when content is unchanged;
slice nilness, length, capacity, and aliasing are observable. A single call is
therefore retained, and only chains of two or more layers are reported. The
outermost byte call is kept byte-for-byte while its input is spliced to the
chain base. Its content-dependent allocation path is consequently unchanged,
preserving the final slice-header contract while deleting intermediate copies.

The rewrite is BIT-IDENTICAL. Original input evaluation remains exactly once,
and every deleted companion is a constant/literal. Aliases work; dot imports,
shadowed helpers, methods, dynamic byte slices, dynamic counts, ellipsis,
cross-package chains, and a changed import binding do not match. Comments and
last-use local constants keep the finding advisory. Removing the final strings
qualifier also removes its import safely; cgo and commented imports remain
advisory. A constant string input in a switch case or composite-literal key
also stays advisory, because removing the runtime call could introduce an
illegal duplicate constant.`,
		Before: `clean := strings.ReplaceAll(
	strings.Replace(payload, "x", "y", 0),
	"z", "z",
)`,
		After: `clean := payload`,
		MeasuredWin: `On Apple M2 Pro, collapsing five byte replacement-copy
layers to the required outer copy on a 62,813-byte input measured 490314 ns/op,
327680 B/op, 5 allocs/op -> 140549 ns/op, 65536 B/op, 1 alloc/op (median of
five runs): 3.49x faster, 71.3% less time, and 80% fewer allocated bytes and
allocations.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5080",
		Doc:  "no-op strings/bytes replacement call or chain",
		Run:  runPS5080,
	},
})

type ps5080Match struct {
	outer   *ast.CallExpr
	base    ast.Expr
	calls   []*ast.CallExpr
	pkgPath string
	spans   []tokenSpan
}

func runPS5080(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		parents := ps6071Parents(file)
		covered := make(map[*ast.CallExpr]bool)
		ownedByTerminalReplacement := ps5118OwnedIndependentNoops(pass, file)
		ast.Inspect(file, func(node ast.Node) bool {
			outer, ok := node.(*ast.CallExpr)
			if !ok || covered[outer] || ownedByTerminalReplacement[outer] {
				return true
			}
			matched, ok := ps5080MatchChain(pass, outer)
			if !ok {
				return true
			}
			for _, call := range matched.calls {
				covered[call] = true
			}
			var message string
			if matched.pkgPath == "strings" {
				message = fmt.Sprintf("%d adjacent strings Replace/ReplaceAll call(s) are content no-ops; remove the replacement scaffolding", len(matched.calls))
			} else {
				message = fmt.Sprintf("%d adjacent bytes Replace/ReplaceAll calls preserve content but each copies the slice; keep only the outer copy contract", len(matched.calls))
			}
			diagnostic := analysis.Diagnostic{
				Pos:     outer.Pos(),
				End:     outer.End(),
				Message: message,
			}
			if fix, ok := ps5080Fix(pass, file, matched, parents); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5080Fix(pass *analysis.Pass, file *ast.File, matched ps5080Match, parents map[ast.Node]ast.Node) (analysis.SuggestedFix, bool) {
	if matched.pkgPath == "strings" {
		typed, constantBase := pass.TypesInfo.Types[ps2110Unparen(matched.base)]
		if constantBase && typed.Value != nil && replacementIntroducesConstantInUniqueContext(pass, matched.outer, parents) {
			return analysis.SuggestedFix{}, false
		}
	}
	return fixDeletedCallScaffolding(pass, file, matched.pkgPath, "collapse no-op replacement calls", matched.spans...)
}

func ps5080MatchChain(pass *analysis.Pass, outer *ast.CallExpr) (ps5080Match, bool) {
	fn, sig, ok := typedCallee(pass, outer.Fun)
	if !ok || sig.Recv() != nil || fn.Pkg() == nil || (fn.Pkg().Path() != "strings" && fn.Pkg().Path() != "bytes") {
		return ps5080Match{}, false
	}
	binding, ok := typedPackageBinding(pass, outer.Fun)
	if !ok {
		return ps5080Match{}, false
	}
	pkgPath := fn.Pkg().Path()
	current := outer
	var calls []*ast.CallExpr
	for ps5080NoopCall(pass, current, pkgPath, binding) {
		calls = append(calls, current)
		next, ok := ps2110Unparen(current.Args[0]).(*ast.CallExpr)
		if !ok || !ps5080NoopCall(pass, next, pkgPath, binding) {
			break
		}
		current = next
	}
	if len(calls) == 0 || pkgPath == "bytes" && len(calls) < 2 {
		return ps5080Match{}, false
	}
	base := calls[len(calls)-1].Args[0]
	var spans []tokenSpan
	if pkgPath == "strings" {
		spans = []tokenSpan{
			{start: outer.Pos(), end: base.Pos()},
			{start: base.End(), end: outer.End()},
		}
	} else {
		inner := outer.Args[0]
		spans = []tokenSpan{
			{start: inner.Pos(), end: base.Pos()},
			{start: base.End(), end: inner.End()},
		}
	}
	return ps5080Match{outer: outer, base: base, calls: calls, pkgPath: pkgPath, spans: spans}, true
}

func ps5080NoopCall(pass *analysis.Pass, call *ast.CallExpr, pkgPath string, binding *types.PkgName) bool {
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok || sig.Recv() != nil || fn.Pkg() == nil || fn.Pkg().Path() != pkgPath {
		return false
	}
	callBinding, ok := typedPackageBinding(pass, call.Fun)
	if !ok || callBinding != binding || call.Ellipsis.IsValid() {
		return false
	}
	var oldValue, newValue string
	switch fn.Name() {
	case "ReplaceAll":
		if len(call.Args) != 3 {
			return false
		}
	case "Replace":
		if len(call.Args) != 4 {
			return false
		}
	default:
		return false
	}
	if pkgPath == "strings" {
		oldValue, ok = ps5077Cutset(pass, call.Args[1])
		if !ok {
			return false
		}
		newValue, ok = ps5077Cutset(pass, call.Args[2])
	} else {
		oldValue, ok = ps5080ConstBytes(pass, call.Args[1])
		if !ok {
			return false
		}
		newValue, ok = ps5080ConstBytes(pass, call.Args[2])
	}
	if !ok {
		return false
	}
	if oldValue == newValue {
		return true
	}
	return fn.Name() == "Replace" && ps5080ConstZero(pass, call.Args[3])
}

func ps5080ConstZero(pass *analysis.Pass, expr ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[ps2110Unparen(expr)]
	return ok && tv.Value != nil && tv.Value.Kind() == constant.Int && constant.Sign(tv.Value) == 0
}

func ps5080ConstBytes(pass *analysis.Pass, expr ast.Expr) (string, bool) {
	expr = ps2110Unparen(expr)
	if id, ok := expr.(*ast.Ident); ok {
		if _, isNil := pass.TypesInfo.Uses[id].(*types.Nil); isNil {
			return "", true
		}
	}
	if !types.Identical(pass.TypesInfo.TypeOf(expr), types.NewSlice(types.Typ[types.Uint8])) {
		return "", false
	}
	switch value := expr.(type) {
	case *ast.CompositeLit:
		if _, plain := ps2110Unparen(value.Type).(*ast.ArrayType); plain && len(value.Elts) == 0 {
			return "", true
		}
	case *ast.CallExpr:
		if _, plain := ps2110Unparen(value.Fun).(*ast.ArrayType); !plain || len(value.Args) != 1 || value.Ellipsis.IsValid() {
			return "", false
		}
		if id, ok := ps2110Unparen(value.Args[0]).(*ast.Ident); ok {
			if _, isNil := pass.TypesInfo.Uses[id].(*types.Nil); isNil {
				return "", true
			}
		}
		return ps5077Cutset(pass, value.Args[0])
	}
	return "", false
}
