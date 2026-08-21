package checks

import (
	"fmt"
	"go/ast"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6017 implements owner issue #768: leaf fusion speedups must demonstrate
// stable, threshold-clearing end-to-end graph leverage.
var PS6017 = register(&lint.Check{
	ID:       "PS6017",
	Category: "verify",
	Slug:     "leaf-fusion-overstates-graph-leverage",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a leaf fusion speedup overstates its supported end-to-end graph leverage",
		Text: `A production-shaped leaf benchmark can show a large fusion win
while the optimized stage occupies too little of the command graph to clear an
end-to-end promotion gate. Dispatch savings are not additive across a graph,
and scheduling can hide most of an isolated stage improvement.

This check implements owner issue #768. It finds accelerator fusion benchmark
or promotion harnesses with leaf/stage and graph/decode context, then requires
a LeafToGraphGate, FusionLeverageEvidence, EndToEndFusionEvidence,
EffectiveStageFractionGate, LeafGraphValidation, or equivalent manifest with:

  - hardware and workload identity;
  - a leaf speedup or leaf ratio distribution;
  - repeated end-to-end candidate ratios;
  - repeated unchanged-control ratios;
  - a frozen graph promotion threshold;
  - a maximum allowed unchanged-control spread; and
  - exactness/parity status.

For compile-time evidence, the analyzer reports the candidate range and uses
Amdahl inversion for every end-to-end sample:

  effectiveFraction = (1 - 1/graphSpeedup) / (1 - 1/leafSpeedup)

It warns when the best observed graph sample still misses the frozen promotion
threshold, when unchanged controls exceed their declared stability limit, when
candidate or control campaigns have fewer than three independent invocations,
when the leaf-versus-graph gain gap exceeds control spread, or when exactness
fails. A scalar leaf speedup is accepted because the repeated graph/control
campaign—not repeated microbenchmark syntax—is the promotion evidence.

There is NO automatic fix. Campaign independence, hardware state, graph
coverage, and whether to reject a kernel cannot be established by a syntax
rewrite. A narrower leaf-only optimization may still be useful, but it must not
be promoted as an end-to-end win without this evidence.`,
		Before: `gate := FusionLeverageEvidence{
	LeafSpeedup: 1.3815,
	CandidateRatios: []float64{1.0171, 1.0119, 1.0090},
	PromotionThreshold: 1.03,
}`,
		After: `gate := FusionLeverageEvidence{
	Hardware: "Apple M2", Workload: "TinyLlama Q4_K_M tg64",
	LeafSpeedup: 1.3815,
	EndToEndCandidateRatios: candidate,
	UnchangedControlRatios: controls,
	GraphPromotionThreshold: 1.03,
	MaximumControlSpread: 0.025,
	ExactnessPassed: true,
}
// Reject when the repeated graph campaign cannot clear the frozen gate.`,
		MeasuredWin: `In the issue #768 Apple-M2 Metal campaign, fusing three
heterogeneous QKV projections across ten production-shaped layers improved the
leaf stage from 0.815 ms to 0.590 ms (1.3815x). Three trained-model tg64
invocations improved only 1.0171x, 1.0119x, and 1.0090x. Amdahl inversion put
the effective optimized fraction at about 3.3%-6.1%, below the leverage needed
for the frozen 1.03x decode promotion gate, so the bit-exact candidate was
correctly rejected.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6017",
		Doc:  "leaf fusion evidence lacks stable threshold-clearing graph leverage",
		Run:  runPS6017,
	},
})

type ps6017Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6017Axes = []ps6017Axis{
	{name: "hardware", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6007ContainsAny(n, "hardware", "device", "accelerator") })
	}},
	{name: "workload", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6007ContainsAny(n, "workload", "model", "graph", "campaign") })
	}},
	{name: "leaf speedup", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6017LeafField)
	}},
	{name: "end-to-end candidate distribution", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6017CandidateField)
	}},
	{name: "unchanged-control distribution", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6017ControlField)
	}},
	{name: "graph promotion threshold", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6017PromotionField)
	}},
	{name: "maximum control spread", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6017ControlLimitField)
	}},
	{name: "exactness status", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6016ExactnessField)
	}},
}

type ps6017Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6017(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6011Harness(pass, fn) || !ps6017Context(pass, fn) {
				continue
			}
			manifest, found := ps6017BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "accelerator leaf-fusion campaign has no leaf-to-graph leverage manifest; missing %s", strings.Join(ps6017Missing(nil), ", "))
				continue
			}
			if missing := ps6017Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "leaf-to-graph leverage manifest is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			summary, warnings := ps6017Audit(manifest.fields)
			if len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "leaf-to-graph leverage gate fails: %s; %s", summary, strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6017BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6017Manifest, bool) {
	var best ps6017Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6017ManifestType(lit.Type) {
			return true
		}
		manifest := ps6017Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6017Axes) - len(ps6017Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6017HasManifest(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if found {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if lit, ok := node.(*ast.CompositeLit); ok && ps6017ManifestType(lit.Type) {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps6017ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6017ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6017ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return strings.Contains(name, "leaftographgate") ||
		strings.Contains(name, "fusionleverageevidence") ||
		strings.Contains(name, "endtoendfusionevidence") ||
		strings.Contains(name, "effectivestagefractiongate") ||
		strings.Contains(name, "leafgraphvalidation")
}

func ps6017Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6017Axes))
	for _, axis := range ps6017Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6017LeafField(name string) bool {
	return strings.Contains(name, "leaf") &&
		ps6007ContainsAny(name, "speedup", "ratio", "sample", "distribution") &&
		!ps6007ContainsAny(name, "minimum", "required", "threshold", "gate")
}

func ps6017CandidateField(name string) bool {
	return strings.Contains(name, "candidate") &&
		ps6007ContainsAny(name, "endtoend", "decode", "graph", "tg") &&
		ps6007ContainsAny(name, "ratio", "sample", "distribution")
}

func ps6017ControlField(name string) bool {
	return strings.Contains(name, "control") &&
		ps6007ContainsAny(name, "ratio", "sample", "distribution") &&
		!ps6007ContainsAny(name, "maximum", "minimum", "max", "min", "limit", "tolerance", "threshold", "allowed", "stability")
}

func ps6017PromotionField(name string) bool {
	return (strings.Contains(name, "promotion") || ps6007ContainsAny(name, "endtoend", "decode", "graph", "tg")) &&
		ps6007ContainsAny(name, "minimum", "required", "threshold", "gate")
}

func ps6017ControlLimitField(name string) bool {
	return strings.Contains(name, "control") &&
		ps6007ContainsAny(name, "spread", "stability") &&
		ps6007ContainsAny(name, "maximum", "max", "limit", "tolerance", "threshold", "allowed")
}

func ps6017Audit(fields map[string]ps6016Field) (string, []string) {
	leaf, leafOK := ps6017Ratios(fields, ps6017LeafField)
	candidates, candidatesOK := ps6017Ratios(fields, ps6017CandidateField)
	controls, controlsOK := ps6017Ratios(fields, ps6017ControlField)
	promotion, promotionOK := ps6016Number(fields, ps6017PromotionField)
	controlLimit, controlLimitOK := ps6017ControlLimit(fields)

	var warnings []string
	if failed, known := ps6016ExactnessFailed(fields); known && failed {
		warnings = append(warnings, "exactness/parity gate explicitly fails")
	}
	if candidatesOK && len(candidates) < 3 {
		warnings = append(warnings, "end-to-end candidate campaign has fewer than three independent invocations")
	}
	if controlsOK && len(controls) < 3 {
		warnings = append(warnings, "unchanged-control campaign has fewer than three independent invocations")
	}
	if leafOK && (len(leaf) == 0 || slices.Min(leaf) <= 1) {
		warnings = append(warnings, "leaf speedup must be greater than 1x for Amdahl inversion")
	}
	if candidatesOK && (len(candidates) == 0 || slices.Min(candidates) <= 0) {
		warnings = append(warnings, "end-to-end candidate ratios must be positive")
	}
	if controlsOK && (len(controls) == 0 || slices.Min(controls) <= 0) {
		warnings = append(warnings, "unchanged-control ratios must be positive")
	}
	if promotionOK && promotion <= 0 {
		warnings = append(warnings, "graph promotion threshold must be positive")
	}
	if controlLimitOK && controlLimit < 0 {
		warnings = append(warnings, "maximum control spread must be non-negative")
	}

	controlSpread, spreadOK := ps6017Spread(controls)
	if spreadOK && controlLimitOK && controlSpread > controlLimit {
		warnings = append(warnings, fmt.Sprintf("unchanged-control spread %.2f%% exceeds declared %.2f%% limit", controlSpread*100, controlLimit*100))
	}
	if candidatesOK && len(candidates) > 0 && promotionOK {
		best := slices.Max(candidates)
		if best < promotion {
			warnings = append(warnings, fmt.Sprintf("best end-to-end candidate %.4gx cannot clear frozen %.4gx promotion threshold", best, promotion))
		}
	}

	summary := "dynamic evidence; effective stage fraction cannot be computed statically"
	if leafOK && len(leaf) > 0 && slices.Min(leaf) > 1 &&
		candidatesOK && len(candidates) > 0 && slices.Min(candidates) > 0 {
		leafMedian := ps6016Median(leaf)
		candidateMedian := ps6016Median(candidates)
		fractions := ps6017EffectiveFractions(leafMedian, candidates)
		if len(fractions) > 0 {
			summary = fmt.Sprintf("leaf median %.4gx versus end-to-end %.4gx-%.4gx implies %.2f%%-%.2f%% effective stage fraction", leafMedian, slices.Min(candidates), slices.Max(candidates), slices.Min(fractions)*100, slices.Max(fractions)*100)
		}
		if spreadOK && leafMedian-1 > candidateMedian-1+controlSpread {
			warnings = append(warnings, fmt.Sprintf("leaf gain %.2f%% diverges from end-to-end median gain %.2f%% beyond %.2f%% control spread", (leafMedian-1)*100, (candidateMedian-1)*100, controlSpread*100))
		}
	}
	return summary, warnings
}

func ps6017Ratios(fields map[string]ps6016Field, predicate func(string) bool) ([]float64, bool) {
	if values, ok := ps6016Numbers(fields, predicate); ok {
		return values, true
	}
	if value, ok := ps6016Number(fields, predicate); ok {
		return []float64{value}, true
	}
	return nil, false
}

func ps6017ControlLimit(fields map[string]ps6016Field) (float64, bool) {
	var result float64
	found := false
	for name, field := range fields {
		if !ps6017ControlLimitField(name) || !field.hasNumber {
			continue
		}
		value := field.number
		switch {
		case strings.Contains(name, "percent") || strings.Contains(name, "pct"):
			value /= 100
		case strings.Contains(name, "ratio") && value >= 1:
			value--
		}
		if found && result != value {
			return 0, false
		}
		result, found = value, true
	}
	return result, found
}

func ps6017Spread(values []float64) (float64, bool) {
	if len(values) == 0 || slices.Min(values) <= 0 {
		return 0, false
	}
	return slices.Max(values)/slices.Min(values) - 1, true
}

func ps6017EffectiveFractions(leaf float64, candidates []float64) []float64 {
	denominator := 1 - 1/leaf
	if denominator <= 0 {
		return nil
	}
	result := make([]float64, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate <= 0 {
			continue
		}
		//perfscan:ignore PS5001 Amdahl evidence keeps the specified division; reciprocal multiplication is not bit-identical
		result = append(result, (1-1/candidate)/denominator)
	}
	return result
}

func ps6017Context(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	accelerator, fusion, leaf, graph := false, false, false, false
	classify := func(text string) {
		text = strings.ToLower(text)
		accelerator = accelerator || ps6007ContainsAny(text, "metal", "cuda", "vulkan", "gpu", "mps", "accelerator")
		fusion = fusion || ps6007ContainsAny(text, "fusion", "fused", "merge", "grouped")
		leaf = leaf || ps6007ContainsAny(text, "leaf", "stage", "microbenchmark", "microbench")
		graph = graph || ps6007ContainsAny(text, "endtoend", "end-to-end", "decode", "prefill", "graph", "tg")
	}
	classify(fn.Name.Name)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if accelerator && fusion && leaf && graph {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.Ident:
			classify(value.Name)
		case *ast.BasicLit:
			classify(value.Value)
		case *ast.CallExpr:
			if callee, _, ok := typedCallee(pass, value.Fun); ok {
				classify(callee.Name())
			}
		}
		return !(accelerator && fusion && leaf && graph)
	})
	return accelerator && fusion && leaf && graph
}
