package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6051 implements owner issue #727: replacing tiny cache-local metadata
// loads with cross-lane shuffles needs an explicit bandwidth/correctness model.
var PS6051 = register(&lint.Check{
	ID:       "PS6051",
	Category: "verify",
	Slug:     "gpu-cross-lane-shuffle-needs-bandwidth-model",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "cache-local GPU metadata loads are replaced by unpriced cross-lane shuffles",
		Text: `Removing redundant lane-local loads is not automatically a GPU
optimization when the shared payload is tiny, coalesced, and cache-resident.
Leader conditionals and shuffle instructions can cost more and may change
compiler contraction or scheduling.

This check implements owner issue #727. It audits CrossLaneShuffleEvidence,
MetadataBroadcastExperiment, GPUBandwidthShuffleGate,
SubgroupLoadReplacementReport, or equivalent manifests. Evidence must record:

  - hardware/toolchain, candidate default-off status, warmups/iterations;
  - unchanged launch geometry, quant/activation loads, accumulation, and output
    mapping;
  - subgroup width, leader fanouts, shared payload, bytes eliminated, shuffle
    count/lane, and an explicit bandwidth model;
  - source uniformity/coalescing, cache residency, reuse distance, divergence,
    and numerical-order/contraction sensitivity;
  - incident shapes, control/candidate times and ratios, geometric mean and
    regression count;
  - control/candidate allocation bytes/counts;
  - exact odd-tail requirement/result, mismatch count and ULP class;
  - performance/correctness gates, recommendation, classification, decision.

Constant evidence is checked for stale ratios, geomean/regression count,
allocation differences, false unchanged-work claims, and recommendations that
add many shuffles for tiny cache-local payloads or fail performance/correctness
gates. There is NO automatic fix because cache residency, lane scheduling,
contraction, and numerical identity are backend/runtime facts.`,
		Before: `metadata = load_lane_local(block);`,
		After: `if (lane_is_leader) metadata = load(block);
metadata = simd_shuffle(metadata, leader); // price every shuffle`,
		MeasuredWin: `The Apple-M2-Pro Q4_K experiment behind issue #727 added
six shuffles per lane. Seven incident shapes produced a 0.880764x geometric
mean; six regressed, including 0.767x at K2048,N32000. Odd-tail parity also
changed one output at one-ULP class. Performance and correctness gates rejected
the candidate.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6051",
		Doc:  "cross-lane shuffle replacement lacks bandwidth/correctness model",
		Run:  runPS6051,
	},
})

type ps6051Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6051Axes = []ps6051Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051HardwareField) }},
	{name: "toolchain identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051ToolchainField) }},
	{name: "candidate default-off status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051DefaultOffField) }},
	{name: "warmup count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051WarmupsField) }},
	{name: "iteration count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051IterationsField) }},
	{name: "unchanged launch geometry", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6051UnchangedField(n, "launchgeometry") })
	}},
	{name: "unchanged quant loads", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6051UnchangedField(n, "quantload") })
	}},
	{name: "unchanged activation loads", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6051UnchangedField(n, "activationload") })
	}},
	{name: "unchanged accumulation", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6051UnchangedField(n, "accumulation") })
	}},
	{name: "unchanged output mapping", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6051UnchangedField(n, "outputmapping") })
	}},
	{name: "subgroup width", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051SubgroupField) }},
	{name: "primary leader fanout", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6051FanoutField(n, "primary") })
	}},
	{name: "secondary leader fanout", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6051FanoutField(n, "secondary") })
	}},
	{name: "shared payload bytes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051PayloadField) }},
	{name: "bytes saved per subgroup iteration", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051BytesSavedField) }},
	{name: "shuffle count per lane", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051ShuffleField) }},
	{name: "bandwidth model", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051BandwidthField) }},
	{name: "source address uniformity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051UniformityField) }},
	{name: "original-load coalescing", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051CoalescedField) }},
	{name: "cache residency", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051CacheField) }},
	{name: "reuse distance", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051ReuseField) }},
	{name: "divergence added", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051DivergenceField) }},
	{name: "numerical-order/contraction sensitivity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051NumericalField) }},
	{name: "incident shapes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051ShapesField) }},
	{name: "control latencies", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6051LatenciesField(n, "control") })
	}},
	{name: "candidate latencies", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6051LatenciesField(n, "candidate") })
	}},
	{name: "incident speedups", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051SpeedupsField) }},
	{name: "geometric-mean speedup", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051GeomeanField) }},
	{name: "regression count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051RegressionCountField) }},
	{name: "control allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6051AllocationField(n, "control", "byte") })
	}},
	{name: "candidate allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6051AllocationField(n, "candidate", "byte") })
	}},
	{name: "control allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6051AllocationField(n, "control", "count") })
	}},
	{name: "candidate allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6051AllocationField(n, "candidate", "count") })
	}},
	{name: "exact odd-tail requirement", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051ExactRequiredField) }},
	{name: "odd-tail exact parity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051ParityField) }},
	{name: "odd-tail mismatch count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051MismatchField) }},
	{name: "odd-tail ULP class", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051ULPField) }},
	{name: "performance minimum", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051PerformanceMinimumField) }},
	{name: "performance verdict", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051PerformanceVerdictField) }},
	{name: "correctness verdict", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051CorrectnessVerdictField) }},
	{name: "transformation recommendation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051RecommendationField) }},
	{name: "classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051ClassificationField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6051DecisionField) }},
}

type ps6051Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6051(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6051Context(text) {
				continue
			}
			manifest, found := ps6051BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "GPU cross-lane metadata broadcast has no bandwidth manifest; missing %s", strings.Join(ps6051Missing(nil), ", "))
				continue
			}
			if missing := ps6051Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU shuffle bandwidth evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6051Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU shuffle bandwidth audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6051Context(text string) bool {
	text = ps6007NormalizeName(text)
	return ps6007ContainsAny(text, "gpu", "metal", "cuda", "vulkan") &&
		ps6007ContainsAny(text, "crosslane", "shuffle", "subgroup") && ps6007ContainsAny(text, "metadata", "bandwidth", "broadcast")
}

func ps6051BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6051Manifest, bool) {
	var best ps6051Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6051ManifestType(lit.Type) {
			return true
		}
		manifest := ps6051Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6051Axes) - len(ps6051Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6051ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6051ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6051ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "crosslaneshuffleevidence", "metadatabroadcastexperiment", "gpubandwidthshufflegate", "subgrouploadreplacementreport", "shufflebandwidthevidence")
}

func ps6051Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6051Axes))
	for _, axis := range ps6051Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6051HardwareField(n string) bool {
	return ps6007ContainsAny(n, "hardware", "deviceidentity", "gpuid")
}
func ps6051ToolchainField(n string) bool {
	return ps6007ContainsAny(n, "toolchain", "compileridentity")
}
func ps6051DefaultOffField(n string) bool {
	return strings.Contains(n, "candidate") && strings.Contains(n, "defaultoff")
}
func ps6051WarmupsField(n string) bool {
	return strings.Contains(n, "warmup") && strings.Contains(n, "count")
}
func ps6051IterationsField(n string) bool {
	return strings.Contains(n, "timed") && strings.Contains(n, "iteration")
}
func ps6051UnchangedField(n, detail string) bool {
	return strings.Contains(n, "unchanged") && strings.Contains(n, detail)
}
func ps6051SubgroupField(n string) bool {
	return strings.Contains(n, "subgroup") && strings.Contains(n, "width")
}
func ps6051FanoutField(n, kind string) bool {
	return strings.Contains(n, kind) && strings.Contains(n, "leader") && strings.Contains(n, "fanout")
}
func ps6051PayloadField(n string) bool {
	return strings.Contains(n, "sharedpayload") && strings.Contains(n, "byte")
}
func ps6051BytesSavedField(n string) bool {
	return strings.Contains(n, "bytessaved") && strings.Contains(n, "subgroupiteration")
}
func ps6051ShuffleField(n string) bool    { return strings.Contains(n, "shufflecountperlane") }
func ps6051BandwidthField(n string) bool  { return strings.Contains(n, "bandwidthmodel") }
func ps6051UniformityField(n string) bool { return strings.Contains(n, "sourceaddressuniformity") }
func ps6051CoalescedField(n string) bool {
	return strings.Contains(n, "originalload") && strings.Contains(n, "coalesced")
}
func ps6051CacheField(n string) bool {
	return strings.Contains(n, "estimatedcache") && strings.Contains(n, "resident")
}
func ps6051ReuseField(n string) bool {
	return strings.Contains(n, "reuse") && strings.Contains(n, "distance")
}
func ps6051DivergenceField(n string) bool {
	return strings.Contains(n, "divergence") && strings.Contains(n, "added")
}
func ps6051NumericalField(n string) bool {
	return strings.Contains(n, "numericalorder") && strings.Contains(n, "contraction")
}
func ps6051ShapesField(n string) bool {
	return strings.Contains(n, "incident") && strings.Contains(n, "shape")
}
func ps6051LatenciesField(n, side string) bool {
	return strings.Contains(n, "incident") && strings.Contains(n, side) && ps6007ContainsAny(n, "latencies", "times")
}
func ps6051SpeedupsField(n string) bool {
	return strings.Contains(n, "incident") && strings.Contains(n, "speedup")
}
func ps6051GeomeanField(n string) bool {
	return strings.Contains(n, "geometricmean") && strings.Contains(n, "speedup")
}
func ps6051RegressionCountField(n string) bool {
	return strings.Contains(n, "regression") && strings.Contains(n, "count")
}
func ps6051AllocationField(n, side, detail string) bool {
	return strings.Contains(n, side) && strings.Contains(n, "allocation") && strings.Contains(n, detail)
}
func ps6051ExactRequiredField(n string) bool {
	return strings.Contains(n, "exactoddtail") && strings.Contains(n, "required")
}
func ps6051ParityField(n string) bool {
	return strings.Contains(n, "oddtail") && strings.Contains(n, "exactparity")
}
func ps6051MismatchField(n string) bool {
	return strings.Contains(n, "oddtail") && strings.Contains(n, "mismatchcount")
}
func ps6051ULPField(n string) bool {
	return strings.Contains(n, "oddtail") && strings.Contains(n, "ulp")
}
func ps6051PerformanceMinimumField(n string) bool {
	return strings.Contains(n, "performance") && ps6007ContainsAny(n, "minimum", "threshold", "gate")
}
func ps6051PerformanceVerdictField(n string) bool {
	return strings.Contains(n, "performance") && strings.Contains(n, "verdict")
}
func ps6051CorrectnessVerdictField(n string) bool {
	return strings.Contains(n, "correctness") && strings.Contains(n, "verdict")
}
func ps6051RecommendationField(n string) bool {
	return strings.Contains(n, "transformation") && strings.Contains(n, "recommended")
}
func ps6051ClassificationField(n string) bool {
	return ps6007ContainsAny(n, "classification", "resultclass")
}
func ps6051DecisionField(n string) bool {
	return strings.Contains(n, "final") && strings.Contains(n, "decision")
}

func ps6051Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 16)
	for _, status := range []struct {
		name string
		pred func(string) bool
	}{
		{"candidate default-off", ps6051DefaultOffField},
		{"unchanged launch geometry", func(n string) bool { return ps6051UnchangedField(n, "launchgeometry") }},
		{"unchanged quant loads", func(n string) bool { return ps6051UnchangedField(n, "quantload") }},
		{"unchanged activation loads", func(n string) bool { return ps6051UnchangedField(n, "activationload") }},
		{"unchanged accumulation", func(n string) bool { return ps6051UnchangedField(n, "accumulation") }},
		{"unchanged output mapping", func(n string) bool { return ps6051UnchangedField(n, "outputmapping") }},
	} {
		if value, known := ps6026Bool(fields, status.pred); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	shapes, shapesOK := ps6047Strings(fields, ps6051ShapesField)
	controls, controlsOK := ps6016Numbers(fields, func(n string) bool { return ps6051LatenciesField(n, "control") })
	candidates, candidatesOK := ps6016Numbers(fields, func(n string) bool { return ps6051LatenciesField(n, "candidate") })
	ratios, ratiosOK := ps6016Numbers(fields, ps6051SpeedupsField)
	if shapesOK && controlsOK && candidatesOK && ratiosOK {
		if len(shapes) != len(controls) || len(shapes) != len(candidates) || len(shapes) != len(ratios) {
			warnings = append(warnings, "incident shape/time/speedup vectors have different lengths")
		} else {
			for index := range shapes {
				if candidates[index] > 0 && !ps6025Close(ratios[index], controls[index]/candidates[index]) {
					warnings = append(warnings, fmt.Sprintf("incident speedup for %q is %.6gx, want %.6gx", shapes[index], ratios[index], controls[index]/candidates[index]))
					break
				}
			}
		}
	}
	geomean, geomeanOK := 0.0, false
	regressions := 0
	if ratiosOK {
		geomean, geomeanOK = ps6050Geomean(ratios)
		for _, ratio := range ratios {
			if ratio < 1 {
				regressions++
			}
		}
		if recorded, ok := ps6016Number(fields, ps6051GeomeanField); ok && geomeanOK && !ps6025Close(recorded, geomean) {
			warnings = append(warnings, fmt.Sprintf("geometric-mean speedup %.6gx disagrees with %.6gx", recorded, geomean))
		}
		if recorded, ok := ps6016Number(fields, ps6051RegressionCountField); ok && recorded != float64(regressions) {
			warnings = append(warnings, fmt.Sprintf("regression count %.0f disagrees with %d ratios below one", recorded, regressions))
		}
	}
	for _, detail := range []string{"byte", "count"} {
		control, controlOK := ps6016Number(fields, func(n string) bool { return ps6051AllocationField(n, "control", detail) })
		candidate, candidateOK := ps6016Number(fields, func(n string) bool { return ps6051AllocationField(n, "candidate", detail) })
		if controlOK && candidateOK && control != candidate {
			warnings = append(warnings, "control/candidate allocation "+detail+"s differ")
		}
	}
	parity, parityOK := ps6026Bool(fields, ps6051ParityField)
	mismatches, mismatchesOK := ps6016Number(fields, ps6051MismatchField)
	if parityOK && mismatchesOK && parity != (mismatches == 0) {
		warnings = append(warnings, fmt.Sprintf("odd-tail exact parity is %t but mismatch count is %.0f", parity, mismatches))
	}
	minimum, minimumOK := ps6016Number(fields, ps6051PerformanceMinimumField)
	performancePassed := geomeanOK && minimumOK && geomean >= minimum
	correctnessPassed := parityOK && parity
	for _, verdict := range []struct {
		name   string
		pred   func(string) bool
		passed bool
	}{
		{"performance", ps6051PerformanceVerdictField, performancePassed},
		{"correctness", ps6051CorrectnessVerdictField, correctnessPassed},
	} {
		if value, ok := ps6027String(fields, verdict.pred); ok {
			normalized := ps6030StatusName(value)
			claimsPass := ps6007ContainsAny(normalized, "pass", "accept") && !ps6007ContainsAny(normalized, "fail", "reject")
			claimsFail := ps6007ContainsAny(normalized, "fail", "reject")
			if verdict.passed && !claimsPass || !verdict.passed && !claimsFail {
				warnings = append(warnings, fmt.Sprintf("%s verdict %q disagrees with computed gate", verdict.name, value))
			}
		}
	}
	if recommended, ok := ps6026Bool(fields, ps6051RecommendationField); ok && recommended {
		payload, payloadOK := ps6016Number(fields, ps6051PayloadField)
		shuffles, shufflesOK := ps6016Number(fields, ps6051ShuffleField)
		cacheResident, cacheOK := ps6026Bool(fields, ps6051CacheField)
		coalesced, coalescedOK := ps6026Bool(fields, ps6051CoalescedField)
		riskyModel := payloadOK && payload <= 64 && shufflesOK && shuffles > 0 && cacheOK && cacheResident && coalescedOK && coalesced
		if riskyModel || !performancePassed || !correctnessPassed {
			warnings = append(warnings, "cross-lane shuffle transformation is recommended despite cache-local payload or failed performance/correctness gates")
		}
	}
	if decision, ok := ps6027String(fields, ps6051DecisionField); ok && (!performancePassed || !correctnessPassed) && ps6007ContainsAny(ps6030StatusName(decision), "retain", "promote", "ship", "selected") {
		warnings = append(warnings, fmt.Sprintf("final decision %q retains a shuffle candidate that failed a gate", decision))
	}
	return warnings
}
