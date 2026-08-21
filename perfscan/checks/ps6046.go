package checks

import (
	"fmt"
	"go/ast"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6046 implements owner issue #732: serial encoder persistence needs a
// repeated exact-boundary gate and cannot stand in for full graph execution.
var PS6046 = register(&lint.Check{
	ID:       "PS6046",
	Category: "verify",
	Slug:     "metal-serial-encoder-needs-full-graph-validation",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "serial Metal encoder persistence is overclaimed as executor leverage",
		Text: `Keeping one serial compute encoder open can reduce encoder
creation without adding dependency-aware concurrency, graph fusion, or graph
partitioning. Short screens may be positive while repeated tail behavior and
end-to-end leverage fail.

This check implements owner issue #732. It audits MetalExecutorEvidence,
SerialEncoderValidationReport, ExecutorArchitectureGate,
PersistentEncoderCampaign, or equivalent manifests. Evidence must record:

  - hardware/OS/Go version, candidate default-off status, raw same-binary
    control, and control/candidate architecture modes;
  - control/candidate command buffers, compute/blit/framework encoders,
    dispatches and barriers, plus explicit boundary causes;
  - mixed compute/blit/framework, physical Finish, and Commit/Wait parity;
  - screen depths, times, ratios, iteration count, and triage-only status;
  - validation depth, independent process count, iterations, alternating
    order, control/candidate distributions, medians, paired ratios, and late
    candidate-tail samples;
  - allocation bytes/counts, frozen gate/verdict, architecture scope/reference
    bundle, full-model promotion status, classification, and decision.

Constant evidence is checked for stale topology, screen, paired, median, and
gate arithmetic. A failed repeated gate forbids the full-model run. Serial
encoder persistence must not be classified as dependency-managed full-graph
execution. There is NO automatic fix because boundaries, parity, GPU hazards,
tail latency, and architecture are runtime/backend facts.`,
		Before: `if screenSpeedup > 1.10 { runFullModel(persistentSerialEncoder) }`,
		After: `evidence := SerialEncoderValidationReport{
	ScreenDepths: []float64{32, 340},
	ScreenRatios: []float64{1.154, 1.113},
	ScreenIsTriageOnly: true,
	IndependentProcessCount: 10,
	ValidationRatioOfMedians: 1.037,
	PromotionThreshold: 1.10, PromotionVerdict: "fail",
	FullModelPromotionAllowed: false,
	CandidateArchitectureScope: "serial-encoder-persistence-only",
	FinalDecision: "removed",
}`,
		MeasuredWin: `The Apple-M2-Pro screens behind issue #732 measured
1.154x at depth 32 and 1.113x at depth 340. Ten independent alternating
depth-340 pairs measured medians of 2,742,816.5 and 2,645,069 ns/op, only
1.037x, with late candidate samples at 3.184 and 3.230 ms/op. The frozen 1.10x
gate failed, so full-model promotion was forbidden and the candidate removed.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6046",
		Doc:  "persistent serial Metal encoder lacks full graph validation",
		Run:  runPS6046,
	},
})

type ps6046Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6046Axes = []ps6046Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046HardwareField) }},
	{name: "OS identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046OSField) }},
	{name: "Go version", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046GoField) }},
	{name: "candidate default-off status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046DefaultOffField) }},
	{name: "raw same-binary control", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046RawControlField) }},
	{name: "control architecture mode", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6046ArchitectureField(n, "control") })
	}},
	{name: "candidate architecture mode", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6046ArchitectureField(n, "candidate") })
	}},
	{name: "boundary causes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046BoundaryCausesField) }},
	{name: "mixed compute/blit/framework parity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046MixedParityField) }},
	{name: "physical Finish parity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046FinishField) }},
	{name: "Commit/Wait parity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046CommitWaitField) }},
	{name: "screen depths", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046ScreenDepthsField) }},
	{name: "screen control times", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6046ScreenTimesField(n, "control") })
	}},
	{name: "screen candidate times", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6046ScreenTimesField(n, "candidate") })
	}},
	{name: "screen ratios", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046ScreenRatiosField) }},
	{name: "screen iterations", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046ScreenIterationsField) }},
	{name: "screen triage-only status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046TriageField) }},
	{name: "validation depth", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046ValidationDepthField) }},
	{name: "independent process count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046ProcessCountField) }},
	{name: "validation iterations", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046ValidationIterationsField) }},
	{name: "alternating-order status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046AlternatingField) }},
	{name: "validation control distribution", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6046ValidationSamplesField(n, "control") })
	}},
	{name: "validation candidate distribution", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6046ValidationSamplesField(n, "candidate") })
	}},
	{name: "validation paired ratios", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046PairedField) }},
	{name: "validation control median", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6046MedianField(n, "control") })
	}},
	{name: "validation candidate median", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6046MedianField(n, "candidate") })
	}},
	{name: "validation ratio of medians", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046RatioField) }},
	{name: "candidate tail samples", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046TailField) }},
	{name: "promotion threshold", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046ThresholdField) }},
	{name: "promotion verdict", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046VerdictField) }},
	{name: "candidate architecture scope", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046ScopeField) }},
	{name: "reference architecture bundle", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046BundleField) }},
	{name: "full-model promotion status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046FullModelField) }},
	{name: "classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046ClassificationField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6046DecisionField) }},
}

func init() {
	for _, side := range []string{"control", "candidate"} {
		for _, metric := range []string{"commandbuffer", "computeencoder", "blitencoder", "frameworkencoder", "dispatch", "barrier"} {
			side, metric := side, metric
			ps6046Axes = append(ps6046Axes, ps6046Axis{
				name: side + " " + metric + " count",
				present: func(f map[string]ps6016Field) bool {
					return ps6016HasName(f, func(n string) bool { return ps6046TopologyField(n, side, metric) })
				},
			})
		}
	}
	for _, side := range []string{"control", "candidate"} {
		for _, detail := range []string{"byte", "count"} {
			side, detail := side, detail
			ps6046Axes = append(ps6046Axes, ps6046Axis{
				name: side + " allocation " + detail + "s",
				present: func(f map[string]ps6016Field) bool {
					return ps6016HasName(f, func(n string) bool { return ps6046AllocationField(n, side, detail) })
				},
			})
		}
	}
}

type ps6046Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6046(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6046Context(text) {
				continue
			}
			manifest, found := ps6046BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "persistent Metal encoder campaign has no full-graph validation manifest; missing %s", strings.Join(ps6046Missing(nil), ", "))
				continue
			}
			if missing := ps6046Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "Metal executor validation evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6046Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "Metal executor validation audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6046Context(text string) bool {
	text = ps6007NormalizeName(text)
	return strings.Contains(text, "metal") && ps6007ContainsAny(text, "executor", "encoder") &&
		ps6007ContainsAny(text, "serialencoder", "persistentencoder") && ps6007ContainsAny(text, "fullgraph", "architecturegate", "validation")
}

func ps6046BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6046Manifest, bool) {
	var best ps6046Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6046ManifestType(lit.Type) {
			return true
		}
		manifest := ps6046Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6046Axes) - len(ps6046Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6046ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6046ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6046ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "metalexecutorevidence", "serialencodervalidationreport", "executorarchitecturegate", "persistentencodercampaign", "metalexecutorvalidation")
}

func ps6046Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6046Axes))
	for _, axis := range ps6046Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6046HardwareField(n string) bool {
	return ps6007ContainsAny(n, "hardware", "deviceidentity", "gpuid")
}
func ps6046OSField(n string) bool { return ps6007ContainsAny(n, "osidentity", "osversion", "macos") }
func ps6046GoField(n string) bool { return strings.Contains(n, "go") && strings.Contains(n, "version") }
func ps6046DefaultOffField(n string) bool {
	return strings.Contains(n, "candidate") && strings.Contains(n, "defaultoff")
}
func ps6046RawControlField(n string) bool {
	return strings.Contains(n, "raw") && strings.Contains(n, "samebinary") && strings.Contains(n, "control")
}
func ps6046ArchitectureField(n, side string) bool {
	return strings.Contains(n, side) && strings.Contains(n, "architecture") && strings.Contains(n, "mode")
}
func ps6046TopologyField(n, side, metric string) bool {
	return strings.Contains(n, side) && strings.Contains(n, metric) && strings.Contains(n, "count")
}
func ps6046BoundaryCausesField(n string) bool {
	return strings.Contains(n, "boundary") && strings.Contains(n, "cause")
}
func ps6046MixedParityField(n string) bool {
	return strings.Contains(n, "mixed") && strings.Contains(n, "compute") && strings.Contains(n, "blit") && strings.Contains(n, "framework") && strings.Contains(n, "parity")
}
func ps6046FinishField(n string) bool {
	return strings.Contains(n, "physicalfinish") && strings.Contains(n, "parity")
}
func ps6046CommitWaitField(n string) bool {
	return strings.Contains(n, "commitwait") && strings.Contains(n, "parity")
}
func ps6046ScreenDepthsField(n string) bool {
	return strings.Contains(n, "screen") && strings.Contains(n, "depth")
}
func ps6046ScreenTimesField(n, side string) bool {
	return strings.Contains(n, "screen") && strings.Contains(n, side) && strings.Contains(n, "time")
}
func ps6046ScreenRatiosField(n string) bool {
	return strings.Contains(n, "screen") && strings.Contains(n, "ratio")
}
func ps6046ScreenIterationsField(n string) bool {
	return strings.Contains(n, "screen") && strings.Contains(n, "iteration")
}
func ps6046TriageField(n string) bool {
	return strings.Contains(n, "screen") && strings.Contains(n, "triageonly")
}
func ps6046ValidationDepthField(n string) bool {
	return strings.Contains(n, "validation") && strings.Contains(n, "depth")
}
func ps6046ProcessCountField(n string) bool {
	return strings.Contains(n, "independentprocess") && strings.Contains(n, "count")
}
func ps6046ValidationIterationsField(n string) bool {
	return strings.Contains(n, "validation") && strings.Contains(n, "iteration")
}
func ps6046AlternatingField(n string) bool { return strings.Contains(n, "alternatingorder") }
func ps6046ValidationSamplesField(n, side string) bool {
	return strings.Contains(n, "validation") && strings.Contains(n, side) && ps6007ContainsAny(n, "latencies", "distribution", "samples")
}
func ps6046PairedField(n string) bool {
	return strings.Contains(n, "validation") && strings.Contains(n, "paired") && strings.Contains(n, "ratio")
}
func ps6046MedianField(n, side string) bool {
	return strings.Contains(n, "validation") && strings.Contains(n, side) && strings.Contains(n, "median") && !strings.Contains(n, "ratio")
}
func ps6046RatioField(n string) bool {
	return strings.Contains(n, "validation") && strings.Contains(n, "ratioofmedians")
}
func ps6046TailField(n string) bool {
	return strings.Contains(n, "candidate") && strings.Contains(n, "tail") && strings.Contains(n, "sample")
}
func ps6046AllocationField(n, side, detail string) bool {
	return strings.Contains(n, side) && strings.Contains(n, "allocation") && strings.Contains(n, detail)
}
func ps6046ThresholdField(n string) bool {
	return strings.Contains(n, "promotion") && ps6007ContainsAny(n, "threshold", "gate", "minimum")
}
func ps6046VerdictField(n string) bool {
	return strings.Contains(n, "promotion") && ps6007ContainsAny(n, "verdict", "result")
}
func ps6046ScopeField(n string) bool {
	return strings.Contains(n, "candidatearchitecture") && strings.Contains(n, "scope")
}
func ps6046BundleField(n string) bool {
	return strings.Contains(n, "referencearchitecture") && strings.Contains(n, "bundle")
}
func ps6046FullModelField(n string) bool {
	return strings.Contains(n, "fullmodel") && strings.Contains(n, "promotion")
}
func ps6046ClassificationField(n string) bool {
	return ps6007ContainsAny(n, "classification", "resultclass")
}
func ps6046DecisionField(n string) bool {
	return strings.Contains(n, "final") && strings.Contains(n, "decision")
}

func ps6046Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 16)
	for _, status := range []struct {
		name string
		pred func(string) bool
	}{
		{"candidate default-off", ps6046DefaultOffField}, {"raw same-binary control", ps6046RawControlField},
		{"mixed compute/blit/framework parity", ps6046MixedParityField}, {"physical Finish parity", ps6046FinishField},
		{"Commit/Wait parity", ps6046CommitWaitField}, {"screen triage-only", ps6046TriageField},
		{"alternating-order validation", ps6046AlternatingField},
	} {
		if value, known := ps6026Bool(fields, status.pred); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	depths, depthsOK := ps6016Numbers(fields, ps6046ScreenDepthsField)
	controls, controlsOK := ps6016Numbers(fields, func(n string) bool { return ps6046ScreenTimesField(n, "control") })
	candidates, candidatesOK := ps6016Numbers(fields, func(n string) bool { return ps6046ScreenTimesField(n, "candidate") })
	ratios, ratiosOK := ps6016Numbers(fields, ps6046ScreenRatiosField)
	if depthsOK && controlsOK && candidatesOK && ratiosOK {
		if len(depths) != len(controls) || len(depths) != len(candidates) || len(depths) != len(ratios) {
			warnings = append(warnings, "screen depth/time/ratio vectors have different lengths")
		} else {
			for index := range depths {
				if candidates[index] > 0 && !ps6025Close(ratios[index], controls[index]/candidates[index]) {
					warnings = append(warnings, fmt.Sprintf("screen ratio at depth %.0f is %.6gx, want %.6gx", depths[index], ratios[index], controls[index]/candidates[index]))
					break
				}
			}
		}
	}
	controlSamples, controlOK := ps6016Numbers(fields, func(n string) bool { return ps6046ValidationSamplesField(n, "control") })
	candidateSamples, candidateOK := ps6016Numbers(fields, func(n string) bool { return ps6046ValidationSamplesField(n, "candidate") })
	paired, pairedOK := ps6016Numbers(fields, ps6046PairedField)
	processCount, processCountOK := ps6016Number(fields, ps6046ProcessCountField)
	if controlOK && candidateOK && len(controlSamples) != len(candidateSamples) {
		warnings = append(warnings, "validation control/candidate distributions have different lengths")
	}
	if processCountOK && (controlOK && int(processCount) != len(controlSamples) || candidateOK && int(processCount) != len(candidateSamples)) {
		warnings = append(warnings, "independent process count disagrees with validation distributions")
	}
	if pairedOK && controlOK && candidateOK && len(paired) == len(controlSamples) && len(paired) == len(candidateSamples) {
		for index := range paired {
			if candidateSamples[index] > 0 && !ps6025Close(paired[index], controlSamples[index]/candidateSamples[index]) {
				warnings = append(warnings, "validation paired ratio "+strconv.Itoa(index)+" is stale")
				break
			}
		}
	}
	validationRatio, validationRatioOK := 0.0, false
	if controlOK && candidateOK && len(controlSamples) > 0 && len(candidateSamples) > 0 {
		controlMedian := ps6016Median(controlSamples)
		candidateMedian := ps6016Median(candidateSamples)
		if recorded, ok := ps6016Number(fields, func(n string) bool { return ps6046MedianField(n, "control") }); ok && !ps6025Close(recorded, controlMedian) {
			warnings = append(warnings, fmt.Sprintf("validation control median %.6g disagrees with %.6g", recorded, controlMedian))
		}
		if recorded, ok := ps6016Number(fields, func(n string) bool { return ps6046MedianField(n, "candidate") }); ok && !ps6025Close(recorded, candidateMedian) {
			warnings = append(warnings, fmt.Sprintf("validation candidate median %.6g disagrees with %.6g", recorded, candidateMedian))
		}
		if candidateMedian > 0 {
			validationRatio, validationRatioOK = controlMedian/candidateMedian, true
			if recorded, ok := ps6016Number(fields, ps6046RatioField); ok && !ps6025Close(recorded, validationRatio) {
				warnings = append(warnings, fmt.Sprintf("validation ratio of medians %.6gx disagrees with %.6gx", recorded, validationRatio))
			}
		}
	}
	for _, detail := range []string{"byte", "count"} {
		control, controlFound := ps6016Number(fields, func(n string) bool { return ps6046AllocationField(n, "control", detail) })
		candidate, candidateFound := ps6016Number(fields, func(n string) bool { return ps6046AllocationField(n, "candidate", detail) })
		if controlFound && candidateFound && control != candidate {
			warnings = append(warnings, "control/candidate allocation "+detail+"s differ")
		}
	}
	threshold, thresholdOK := ps6016Number(fields, ps6046ThresholdField)
	if validationRatioOK && thresholdOK {
		passed := validationRatio >= threshold
		if verdict, ok := ps6027String(fields, ps6046VerdictField); ok {
			normalized := ps6030StatusName(verdict)
			verdictPass := ps6007ContainsAny(normalized, "pass", "promote", "accept") && !ps6007ContainsAny(normalized, "fail", "reject")
			verdictFail := ps6007ContainsAny(normalized, "fail", "reject")
			if passed && !verdictPass || !passed && !verdictFail {
				warnings = append(warnings, fmt.Sprintf("promotion verdict %q disagrees with %.6gx versus %.6gx gate", verdict, validationRatio, threshold))
			}
		}
		if allowed, ok := ps6026Bool(fields, ps6046FullModelField); ok && !passed && allowed {
			warnings = append(warnings, fmt.Sprintf("full-model promotion is allowed despite %.6gx missing %.6gx gate", validationRatio, threshold))
		}
		if !passed {
			if classification, ok := ps6027String(fields, ps6046ClassificationField); ok && ps6007ContainsAny(ps6030StatusName(classification), "fullgraph", "executorspeedup", "productionwin") {
				warnings = append(warnings, fmt.Sprintf("classification %q overgeneralizes serial encoder persistence into full-graph executor leverage", classification))
			}
			if decision, ok := ps6027String(fields, ps6046DecisionField); ok && ps6007ContainsAny(ps6030StatusName(decision), "retain", "promote", "ship", "selected") {
				warnings = append(warnings, fmt.Sprintf("final decision %q retains a candidate that failed repeated validation", decision))
			}
		}
	}
	return warnings
}
