package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6011 implements the source-auditable numerical-parity requirements from
// owner issue #761 for graph/compiler-fused matmul promotion evidence.
var PS6011 = register(&lint.Check{
	ID:       "PS6011",
	Category: "verify",
	Slug:     "graph-matmul-gate-misses-large-shape-parity",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a graph-fused matmul timing gate omits large-shape numerical parity",
		Text: `A graph compiler may lower a cast-matmul-cast boundary with different
accumulator or reduction semantics even when the visible input and output
dtypes match. The difference can be shape-dependent: a small matrix may be
bit-identical while a broad or large-K production matrix exceeds the accepted
error budget. Timing that route without same-shape output comparisons can
therefore promote a semantically different operation.

This check implements the static/perf-harness boundary from owner issue #761.
It finds real func BenchmarkX(*testing.B) and promotion-oriented
func TestX(*testing.T) functions whose source mentions a graph/compiler-fused
matmul route, an explicit conversion/reference/vendor route, and a timing or
promotion claim. It then looks for a keyed GraphMatmulParityGate,
FusedMatmulParity, GraphFusionEvidence, ReductionParityGate,
MatmulPromotionEvidence, or similar manifest. The best manifest must carry:

  - the explicit-conversion control route and graph/compiler candidate route;
  - a small shape and a broad/large-K production shape;
  - same-shape output comparators for both shapes;
  - a finite-output gate; and
  - a declared numerical error budget or tolerance.

Field names are structural evidence even when harness values are dynamic.
Constant comparator descriptions such as "none", "visible dtype only", or
"same dtype" are rejected, as is an explicitly disabled finite-output gate.
The diagnostic deliberately does not infer accumulator width from visible
dtypes or accept a final rounding cast as proof of equivalent reduction.

There is NO automatic fix. Choosing the production shape, reference route,
finite policy, and absolute/relative tolerance is empirical correctness work;
inventing any of them could legitimize an invalid speedup.`,
		Before: `func BenchmarkMPSGraphMatmulPromotion(b *testing.B) {
    benchmarkAndReportRatio(b, explicitCastMatmul, graphFusedMatmul)
}`,
		After: `gate := GraphMatmulParityGate{
    ExplicitConversionControlRoute: "cached-f16 + vendor matmul",
    GraphFusedCandidateRoute: "cached graph cast-matmul-cast",
    SmallShape: small,
    BroadProductionShape: production,
    SmallSameShapeComparator: "candidate vs control outputs",
    ProductionSameShapeComparator: "candidate vs control outputs",
    FiniteOutputGate: true,
    RelativeErrorBudget: declaredTolerance,
}
// Require the gate before accepting the timing ratio.`,
		MeasuredWin: `In the Apple-M2 experiment behind issue #761, cached graph
matmul appeared 1.152x to 1.381x faster on three broad shapes, but the
production-route maximum normalized absolute differences ranged from 9.882e-3
to 7.430e-1. One smaller Q6_K shape was exactly equal while a production-size
Q6_K shape reached 7.430e-1; adding a visible f16 result cast did not repair the
reduction mismatch. The candidate was reverted.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6011",
		Doc:  "graph-fused matmul promotion evidence lacks large-shape parity",
		Run:  runPS6011,
	},
})

type ps6011Field struct {
	stringVal string
	hasString bool
	boolVal   bool
	hasBool   bool
}

type ps6011Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6011Field
}

type ps6011Axis struct {
	name    string
	present func(map[string]ps6011Field) bool
}

var ps6011Axes = []ps6011Axis{
	{name: "explicit-conversion control route", present: func(f map[string]ps6011Field) bool {
		return ps6011HasName(f, ps6011ControlRouteField)
	}},
	{name: "graph/compiler candidate route", present: func(f map[string]ps6011Field) bool {
		return ps6011HasName(f, ps6011CandidateRouteField)
	}},
	{name: "small shape", present: func(f map[string]ps6011Field) bool {
		return ps6011HasName(f, func(n string) bool { return strings.Contains(n, "small") && strings.Contains(n, "shape") })
	}},
	{name: "broad/large-K production shape", present: func(f map[string]ps6011Field) bool {
		return ps6011HasName(f, ps6011ProductionShapeField)
	}},
	{name: "small same-shape output comparator", present: func(f map[string]ps6011Field) bool {
		return ps6011HasName(f, func(n string) bool { return strings.Contains(n, "small") && ps6011ComparatorField(n) })
	}},
	{name: "production same-shape output comparator", present: func(f map[string]ps6011Field) bool {
		return ps6011HasName(f, func(n string) bool { return ps6011ProductionField(n) && ps6011ComparatorField(n) })
	}},
	{name: "finite-output gate", present: func(f map[string]ps6011Field) bool {
		return ps6011HasName(f, func(n string) bool {
			return strings.Contains(n, "finite") && ps6007ContainsAny(n, "output", "result", "gate", "check")
		})
	}},
	{name: "declared numerical error budget", present: func(f map[string]ps6011Field) bool {
		return ps6011HasName(f, func(n string) bool {
			return strings.Contains(n, "error") && ps6007ContainsAny(n, "budget", "tolerance", "threshold") || strings.Contains(n, "tolerance")
		})
	}},
}

func runPS6011(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6011Harness(pass, fn) || !ps6011Context(pass, fn) {
				continue
			}
			manifest, found := ps6011BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "graph/compiler-fused matmul timing gate has no numerical-parity manifest; matching visible dtypes do not prove matching accumulator/reduction semantics; missing %s", strings.Join(ps6011Missing(nil), ", "))
				continue
			}
			if missing := ps6011Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "graph/compiler-fused matmul parity manifest is incomplete; matching visible dtypes do not prove matching accumulator/reduction semantics; missing %s", strings.Join(missing, ", "))
				continue
			}
			if violations := ps6011Violations(manifest.fields); len(violations) > 0 {
				pass.Reportf(manifest.lit.Pos(), "graph/compiler-fused matmul parity evidence is invalid: %s", strings.Join(violations, "; "))
			}
		}
	}
	return nil, nil
}

func ps6011Harness(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	if fn.Recv != nil {
		return false
	}
	wantType := ""
	switch {
	case strings.HasPrefix(fn.Name.Name, "Benchmark"):
		wantType = "B"
	case strings.HasPrefix(fn.Name.Name, "Test") && ps6007ContainsAny(strings.ToLower(fn.Name.Name), "promotion", "gate", "benchmark", "speed", "ratio"):
		wantType = "T"
	default:
		return false
	}
	obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if !ok {
		return false
	}
	sig, ok := obj.Type().(*types.Signature)
	if !ok || sig.Params().Len() != 1 || sig.Results().Len() != 0 || sig.Variadic() {
		return false
	}
	ptr, ok := types.Unalias(sig.Params().At(0).Type()).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(ptr.Elem()).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "testing" && named.Obj().Name() == wantType
}

func ps6011BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6011Manifest, bool) {
	var best ps6011Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6011ManifestType(lit.Type) {
			return true
		}
		manifest := ps6011Manifest{lit: lit, fields: ps6011Fields(pass, lit)}
		score := len(ps6011Axes) - len(ps6011Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6011ManifestType(expr ast.Expr) bool {
	var name string
	switch n := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = n.Name
	case *ast.SelectorExpr:
		name = n.Sel.Name
	case *ast.IndexExpr:
		return ps6011ManifestType(n.X)
	case *ast.IndexListExpr:
		return ps6011ManifestType(n.X)
	}
	n := ps6007NormalizeName(name)
	return strings.Contains(n, "graphmatmulparity") || strings.Contains(n, "fusedmatmulparity") ||
		strings.Contains(n, "graphfusionevidence") || strings.Contains(n, "reductionparitygate") ||
		strings.Contains(n, "matmulpromotionevidence") || strings.Contains(n, "graphmatmulbenchmark")
}

func ps6011Fields(pass *analysis.Pass, lit *ast.CompositeLit) map[string]ps6011Field {
	fields := make(map[string]ps6011Field, len(lit.Elts))
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := ps2110Unparen(kv.Key).(*ast.Ident)
		if !ok {
			continue
		}
		field := ps6011Field{}
		if tv, ok := pass.TypesInfo.Types[ps2110Unparen(kv.Value)]; ok && tv.Value != nil {
			switch tv.Value.Kind() {
			case constant.String:
				field.stringVal, field.hasString = constant.StringVal(tv.Value), true
			case constant.Bool:
				field.boolVal, field.hasBool = constant.BoolVal(tv.Value), true
			}
		}
		fields[ps6007NormalizeName(key.Name)] = field
	}
	return fields
}

func ps6011Missing(fields map[string]ps6011Field) []string {
	missing := make([]string, 0, len(ps6011Axes))
	for _, axis := range ps6011Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6011HasName(fields map[string]ps6011Field, predicate func(string) bool) bool {
	for name := range fields {
		if predicate(name) {
			return true
		}
	}
	return false
}

func ps6011ControlRouteField(name string) bool {
	return ps6007ContainsAny(name, "route", "path", "control", "reference", "baseline") &&
		ps6007ContainsAny(name, "explicit", "conversion", "cast", "vendor", "control", "reference")
}

func ps6011CandidateRouteField(name string) bool {
	return ps6007ContainsAny(name, "graph", "compiler", "fused") && ps6007ContainsAny(name, "route", "path", "candidate")
}

func ps6011ProductionField(name string) bool {
	return ps6007ContainsAny(name, "production", "broad", "largek", "largeshape")
}

func ps6011ProductionShapeField(name string) bool {
	return ps6011ProductionField(name) && strings.Contains(name, "shape")
}

func ps6011ComparatorField(name string) bool {
	return ps6007ContainsAny(name, "comparator", "comparison", "parity", "outputcheck")
}

func ps6011Violations(fields map[string]ps6011Field) []string {
	var violations []string
	for name, field := range fields {
		if ps6011ComparatorField(name) && field.hasString && ps6011WeakComparator(field.stringVal) {
			violations = append(violations, "same-shape comparator is disabled or checks visible dtype only")
			break
		}
	}
	for name, field := range fields {
		if strings.Contains(name, "finite") && field.hasBool && !field.boolVal {
			violations = append(violations, "finite-output gate is explicitly disabled")
			break
		}
	}
	return violations
}

func ps6011WeakComparator(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "" || ps6007ContainsAny(value, "none", "disabled", "dtype only", "visible dtype", "same dtype", "rounding cast only")
}

func ps6011Context(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	graph, matmul, control, claim := false, false, false, false
	classify := func(text string) {
		text = strings.ToLower(text)
		graph = graph || ps6007ContainsAny(text, "graph", "compiler", "fused")
		matmul = matmul || ps6007ContainsAny(text, "matmul", "gemm", "matrixmultiplication", "matrix multiplication")
		control = control || ps6007ContainsAny(text, "explicit", "conversion", "convert", "cast", "vendor", "reference", "control", "baseline", "mpsmatrix")
		claim = claim || ps6007ContainsAny(text, "promotion", "gate", "benchmark", "speedup", "ratio", "timing", "faster")
	}
	classify(fn.Name.Name)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if graph && matmul && control && claim {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch n := node.(type) {
		case *ast.Ident:
			classify(n.Name)
		case *ast.BasicLit:
			if n.Kind == token.STRING {
				classify(n.Value)
			}
		case *ast.CallExpr:
			if callee, _, ok := typedCallee(pass, n.Fun); ok {
				classify(callee.Name())
				if callee.Pkg() != nil {
					classify(callee.Pkg().Path())
				}
			}
		}
		return !(graph && matmul && control && claim)
	})
	return graph && matmul && control && claim
}
