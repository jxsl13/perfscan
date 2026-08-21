package checks

import (
	"fmt"
	"go/ast"
	"math"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6044 implements owner issue #734: a same-process fixed-order accelerator
// screen cannot promote without independent order-alternating validation.
var PS6044 = register(&lint.Check{
	ID:       "PS6044",
	Category: "verify",
	Slug:     "gpu-screen-needs-independent-alternating-promotion",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a same-process GPU screen is treated as promotion evidence",
		Text: `Fixed-order subbenchmarks in one process can retain order,
thermal, and device-state bias even after local warmups. A large screen win is
therefore a hypothesis, not promotion evidence.

This check implements owner issue #734. It audits IndependentPromotionEvidence,
AlternatingGPUPromotionReport, ScreenPromotionGate, GPUOrderBiasEvidence, or
equivalent manifests. Evidence must separate:

  - hardware/workload, same-process/fixed-order screen status, local warmup,
    screen iterations and control/grouped/candidate times;
  - screen ratio and control/candidate allocation bytes/counts;
  - independent process count, promotion iterations, fresh-process status,
    and alternating-order status;
  - control/candidate process distributions and paired ratios;
  - control/candidate medians, ratio of medians, median paired ratio, and p90;
  - screen/promotion divergence, exact output, promotion threshold/verdict,
    evidence classification, and final decision.

Constant evidence is checked by recomputing screen ratio, every paired ratio,
medians, p90, ratio of medians, and divergence. A large screen result that
disappears under independent alternating validation must be classified as
screen-refuted and cannot be retained. There is NO automatic fix because
process isolation, device state, output identity, and timings are runtime
facts.`,
		Before: `if sameProcessScreenRatio > gate { retain(candidate) }`,
		After: `evidence := IndependentPromotionEvidence{
	ScreenControlCandidateRatio: 1.395,
	IndependentProcessCount: 10, AlternatingOrderPassed: true,
	RatioOfMedians: 1.0014, MedianPairedRatio: 1.0011,
	ControlP90NS: 431198, CandidateP90NS: 434737,
	PromotionVerdict: "fail",
	EvidenceClassification: "screen-refuted",
	FinalDecision: "removed",
}`,
		MeasuredWin: `The Apple-M2-Pro screen behind issue #734 reported
428,713 ns/op for production and 307,358 ns/op for concurrent dispatch, an
apparent 1.395x win. Ten independent alternating pairs measured medians of
429,111 and 428,494 ns/op (1.0014x), median paired speedup 1.0011x, while the
candidate p90 regressed from 431,198 to 434,737 ns/op. The candidate was
removed.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6044",
		Doc:  "same-process GPU screen lacks independent alternating promotion evidence",
		Run:  runPS6044,
	},
})

type ps6044Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6044Axes = []ps6044Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044HardwareField) }},
	{name: "workload identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044WorkloadField) }},
	{name: "same-process screen status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044SameProcessField) }},
	{name: "fixed-order screen status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044FixedOrderField) }},
	{name: "local warmup status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044WarmupField) }},
	{name: "screen iteration count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044ScreenIterationsField) }},
	{name: "screen control time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6044ScreenTimeField(n, "control") })
	}},
	{name: "screen grouped time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6044ScreenTimeField(n, "grouped") })
	}},
	{name: "screen candidate time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6044ScreenTimeField(n, "candidate") })
	}},
	{name: "screen ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044ScreenRatioField) }},
	{name: "control allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6044AllocationField(n, "control", "byte") })
	}},
	{name: "candidate allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6044AllocationField(n, "candidate", "byte") })
	}},
	{name: "control allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6044AllocationField(n, "control", "count") })
	}},
	{name: "candidate allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6044AllocationField(n, "candidate", "count") })
	}},
	{name: "independent process count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044ProcessCountField) }},
	{name: "promotion iteration count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044PromotionIterationsField) }},
	{name: "fresh-process status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044FreshField) }},
	{name: "alternating-order status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044AlternatingField) }},
	{name: "control process distribution", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6044DistributionField(n, "control") })
	}},
	{name: "candidate process distribution", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6044DistributionField(n, "candidate") })
	}},
	{name: "paired ratios", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044PairedField) }},
	{name: "control median", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6044SummaryField(n, "control", "median") })
	}},
	{name: "candidate median", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6044SummaryField(n, "candidate", "median") })
	}},
	{name: "ratio of medians", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044RatioOfMediansField) }},
	{name: "median paired ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044MedianPairedField) }},
	{name: "control p90", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6044SummaryField(n, "control", "p90") })
	}},
	{name: "candidate p90", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6044SummaryField(n, "candidate", "p90") })
	}},
	{name: "screen/promotion divergence", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044DivergenceField) }},
	{name: "exact output", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044ExactField) }},
	{name: "promotion threshold", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044ThresholdField) }},
	{name: "promotion verdict", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044VerdictField) }},
	{name: "evidence classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044ClassificationField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6044DecisionField) }},
}

type ps6044Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6044(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6044Context(text) {
				continue
			}
			manifest, found := ps6044BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "same-process GPU screen has no independent alternating promotion manifest; missing %s", strings.Join(ps6044Missing(nil), ", "))
				continue
			}
			if missing := ps6044Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU screen/promotion evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6044Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU screen/promotion audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6044Context(text string) bool {
	text = ps6007NormalizeName(text)
	gpu := ps6007ContainsAny(text, "gpu", "metal", "cuda", "vulkan", "accelerator")
	screen := strings.Contains(text, "screen")
	promotion := ps6007ContainsAny(text, "independentpromotion", "alternatingpromotion", "orderbias", "screenpromotion")
	return gpu && screen && promotion
}

func ps6044BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6044Manifest, bool) {
	var best ps6044Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6044ManifestType(lit.Type) {
			return true
		}
		manifest := ps6044Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6044Axes) - len(ps6044Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6044ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6044ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6044ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "independentpromotionevidence", "alternatinggpupromotionreport", "screenpromotiongate", "gpuorderbiasevidence", "screenpromotionevidence")
}

func ps6044Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6044Axes))
	for _, axis := range ps6044Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6044HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "deviceidentity", "gpuid")
}
func ps6044WorkloadField(name string) bool {
	return strings.Contains(name, "workload") && ps6007ContainsAny(name, "identity", "shape", "name")
}
func ps6044SameProcessField(name string) bool {
	return strings.Contains(name, "screen") && strings.Contains(name, "sameprocess")
}
func ps6044FixedOrderField(name string) bool {
	return strings.Contains(name, "screen") && strings.Contains(name, "fixedorder")
}
func ps6044WarmupField(name string) bool { return strings.Contains(name, "localwarmup") }
func ps6044ScreenIterationsField(name string) bool {
	return strings.Contains(name, "screen") && strings.Contains(name, "iteration")
}
func ps6044ScreenTimeField(name, side string) bool {
	return strings.Contains(name, "screen") && strings.Contains(name, side) && ps6007ContainsAny(name, "ns", "time", "latency") && !strings.Contains(name, "ratio")
}
func ps6044ScreenRatioField(name string) bool {
	return strings.Contains(name, "screen") && strings.Contains(name, "controlcandidate") && ps6007ContainsAny(name, "ratio", "speedup")
}
func ps6044AllocationField(name, side, detail string) bool {
	return strings.Contains(name, side) && strings.Contains(name, "allocation") && strings.Contains(name, detail)
}
func ps6044ProcessCountField(name string) bool {
	return strings.Contains(name, "independentprocess") && strings.Contains(name, "count")
}
func ps6044PromotionIterationsField(name string) bool {
	return strings.Contains(name, "promotion") && strings.Contains(name, "iteration")
}
func ps6044FreshField(name string) bool       { return strings.Contains(name, "freshprocess") }
func ps6044AlternatingField(name string) bool { return strings.Contains(name, "alternatingorder") }
func ps6044DistributionField(name, side string) bool {
	return strings.Contains(name, side) && strings.Contains(name, "process") && ps6007ContainsAny(name, "latencies", "distribution", "samples")
}
func ps6044PairedField(name string) bool {
	return strings.Contains(name, "paired") && strings.Contains(name, "ratio") && !strings.Contains(name, "median")
}
func ps6044SummaryField(name, side, metric string) bool {
	return strings.Contains(name, side) && strings.Contains(name, metric) && !strings.Contains(name, "ratio")
}
func ps6044RatioOfMediansField(name string) bool { return strings.Contains(name, "ratioofmedians") }
func ps6044MedianPairedField(name string) bool {
	return strings.Contains(name, "median") && strings.Contains(name, "paired") && strings.Contains(name, "ratio")
}
func ps6044DivergenceField(name string) bool {
	return strings.Contains(name, "screenpromotion") && ps6007ContainsAny(name, "divergence", "ratio")
}
func ps6044ExactField(name string) bool {
	return strings.Contains(name, "exact") && ps6007ContainsAny(name, "output", "logit", "parity")
}
func ps6044ThresholdField(name string) bool {
	return strings.Contains(name, "promotion") && ps6007ContainsAny(name, "threshold", "minimum", "gate")
}
func ps6044VerdictField(name string) bool {
	return strings.Contains(name, "promotion") && ps6007ContainsAny(name, "verdict", "result")
}
func ps6044ClassificationField(name string) bool {
	return strings.Contains(name, "evidence") && strings.Contains(name, "classification")
}
func ps6044DecisionField(name string) bool {
	return strings.Contains(name, "final") && strings.Contains(name, "decision")
}

func ps6044Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 16)
	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"local warmup", ps6044WarmupField},
		{"fresh-process validation", ps6044FreshField},
		{"alternating-order validation", ps6044AlternatingField},
		{"exact output", ps6044ExactField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	screenControl, screenControlOK := ps6016Number(fields, func(n string) bool { return ps6044ScreenTimeField(n, "control") })
	screenCandidate, screenCandidateOK := ps6016Number(fields, func(n string) bool { return ps6044ScreenTimeField(n, "candidate") })
	screenRatio, screenRatioOK := ps6016Number(fields, ps6044ScreenRatioField)
	calculatedScreen, calculatedScreenOK := 0.0, screenControlOK && screenCandidateOK && screenCandidate > 0
	if calculatedScreenOK {
		calculatedScreen = screenControl / screenCandidate
		if screenRatioOK && !ps6025Close(screenRatio, calculatedScreen) {
			warnings = append(warnings, fmt.Sprintf("screen ratio %.6gx disagrees with control/candidate %.6gx", screenRatio, calculatedScreen))
		}
	}
	for _, detail := range []string{"byte", "count"} {
		control, controlOK := ps6016Number(fields, func(n string) bool { return ps6044AllocationField(n, "control", detail) })
		candidate, candidateOK := ps6016Number(fields, func(n string) bool { return ps6044AllocationField(n, "candidate", detail) })
		if controlOK && candidateOK && control != candidate {
			warnings = append(warnings, fmt.Sprintf("control/candidate allocation %ss differ (%.6g vs %.6g)", detail, control, candidate))
		}
	}
	controlSamples, controlSamplesOK := ps6016Numbers(fields, func(n string) bool { return ps6044DistributionField(n, "control") })
	candidateSamples, candidateSamplesOK := ps6016Numbers(fields, func(n string) bool { return ps6044DistributionField(n, "candidate") })
	paired, pairedOK := ps6016Numbers(fields, ps6044PairedField)
	processCount, processCountOK := ps6016Number(fields, ps6044ProcessCountField)
	if controlSamplesOK && candidateSamplesOK && len(controlSamples) != len(candidateSamples) {
		warnings = append(warnings, "control/candidate process distributions have different lengths")
	}
	if processCountOK && (controlSamplesOK && int(processCount) != len(controlSamples) || candidateSamplesOK && int(processCount) != len(candidateSamples)) {
		warnings = append(warnings, fmt.Sprintf("independent process count %.0f disagrees with distribution lengths", processCount))
	}
	if controlSamplesOK && len(controlSamples) < 3 {
		warnings = append(warnings, "independent promotion has fewer than three process pairs")
	}
	if pairedOK && controlSamplesOK && candidateSamplesOK && len(paired) == len(controlSamples) && len(paired) == len(candidateSamples) {
		for index := range paired {
			if candidateSamples[index] > 0 && !ps6025Close(paired[index], controlSamples[index]/candidateSamples[index]) {
				warnings = append(warnings, fmt.Sprintf("paired ratio %d %.6gx disagrees with process pair %.6gx", index, paired[index], controlSamples[index]/candidateSamples[index]))
				break
			}
		}
	} else if pairedOK && controlSamplesOK && len(paired) != len(controlSamples) {
		warnings = append(warnings, "paired-ratio count disagrees with process distribution")
	}
	promotionRatio, promotionRatioOK := 0.0, false
	if controlSamplesOK && candidateSamplesOK && len(controlSamples) > 0 && len(candidateSamples) > 0 {
		controlMedian := ps6016Median(controlSamples)
		candidateMedian := ps6016Median(candidateSamples)
		if recorded, ok := ps6016Number(fields, func(n string) bool { return ps6044SummaryField(n, "control", "median") }); ok && !ps6025Close(recorded, controlMedian) {
			warnings = append(warnings, fmt.Sprintf("control median %.6g disagrees with distribution median %.6g", recorded, controlMedian))
		}
		if recorded, ok := ps6016Number(fields, func(n string) bool { return ps6044SummaryField(n, "candidate", "median") }); ok && !ps6025Close(recorded, candidateMedian) {
			warnings = append(warnings, fmt.Sprintf("candidate median %.6g disagrees with distribution median %.6g", recorded, candidateMedian))
		}
		if candidateMedian > 0 {
			promotionRatio, promotionRatioOK = controlMedian/candidateMedian, true
			if recorded, ok := ps6016Number(fields, ps6044RatioOfMediansField); ok && !ps6025Close(recorded, promotionRatio) {
				warnings = append(warnings, fmt.Sprintf("ratio of medians %.6gx disagrees with %.6gx", recorded, promotionRatio))
			}
		}
		for _, summary := range []struct {
			side    string
			samples []float64
		}{
			{"control", controlSamples},
			{"candidate", candidateSamples},
		} {
			p90 := ps6044P90(summary.samples)
			if recorded, ok := ps6016Number(fields, func(n string) bool { return ps6044SummaryField(n, summary.side, "p90") }); ok && !ps6025Close(recorded, p90) {
				warnings = append(warnings, fmt.Sprintf("%s p90 %.6g disagrees with distribution p90 %.6g", summary.side, recorded, p90))
			}
		}
	}
	if pairedOK && len(paired) > 0 {
		median := ps6016Median(paired)
		if recorded, ok := ps6016Number(fields, ps6044MedianPairedField); ok && !ps6025Close(recorded, median) {
			warnings = append(warnings, fmt.Sprintf("median paired ratio %.6gx disagrees with paired distribution %.6gx", recorded, median))
		}
	}
	if calculatedScreenOK && promotionRatioOK && promotionRatio > 0 {
		divergence := calculatedScreen / promotionRatio
		if recorded, ok := ps6016Number(fields, ps6044DivergenceField); ok && !ps6025Close(recorded, divergence) {
			warnings = append(warnings, fmt.Sprintf("screen/promotion divergence %.6gx disagrees with %.6gx", recorded, divergence))
		}
		threshold, thresholdOK := ps6016Number(fields, ps6044ThresholdField)
		refuted := thresholdOK && calculatedScreen >= threshold && promotionRatio < threshold
		if thresholdOK {
			if verdict, ok := ps6027String(fields, ps6044VerdictField); ok {
				normalized := ps6030StatusName(verdict)
				passed := promotionRatio >= threshold
				verdictPass := ps6007ContainsAny(normalized, "pass", "promote", "accept") && !ps6007ContainsAny(normalized, "fail", "reject")
				verdictFail := ps6007ContainsAny(normalized, "fail", "reject")
				if passed && !verdictPass || !passed && !verdictFail {
					warnings = append(warnings, fmt.Sprintf("promotion verdict %q disagrees with independent %.6gx versus %.6gx threshold", verdict, promotionRatio, threshold))
				}
			}
		}
		if refuted {
			if classification, ok := ps6027String(fields, ps6044ClassificationField); ok && !ps6007ContainsAny(ps6030StatusName(classification), "screenrefuted", "orderbiased", "promotionneutral") {
				warnings = append(warnings, fmt.Sprintf("evidence classification %q does not mark the large screen win as refuted", classification))
			}
			if decision, ok := ps6027String(fields, ps6044DecisionField); ok && ps6007ContainsAny(ps6030StatusName(decision), "retain", "promote", "ship", "selected") {
				warnings = append(warnings, fmt.Sprintf("final decision %q retains a candidate whose same-process screen disappeared independently", decision))
			}
		}
	}
	return warnings
}

func ps6044P90(values []float64) float64 {
	ordered := slices.Clone(values)
	slices.Sort(ordered)
	index := int(math.Ceil(0.9*float64(len(ordered)))) - 1
	return ordered[max(index, 0)]
}
