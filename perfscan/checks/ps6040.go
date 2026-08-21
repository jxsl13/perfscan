package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6040 implements owner issue #738: a positive GPU fusion result remains an
// absolute measurement even when it misses a project promotion threshold.
var PS6040 = register(&lint.Check{
	ID:       "PS6040",
	Category: "verify",
	Slug:     "gpu-fusion-report-separates-gain-from-promotion",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a rejected GPU fusion loses its positive absolute result",
		Text: `A repeatable optimization can improve latency and still miss a
project-specific promotion threshold. Reporting only the final gate verdict
turns useful positive evidence into an apparent no-op or regression.

This check implements owner issue #738. It audits FusionPromotionReport,
RejectedPositiveEvidence, AbsoluteGainReport, GPUFusionOutcomeEvidence, or
equivalent manifests. A report must keep these dimensions separate:

  - hardware and workload identity;
  - control/candidate latency, measured speedup, absolute saved latency, and
    relative latency reduction;
  - sample count and confidence/sample-quality status;
  - the frozen promotion threshold and its pass/fail verdict;
  - control/candidate dispatch counts and candidate-minus-control delta;
  - control/candidate estimated intermediate bytes and their delta;
  - exact-output status and control/candidate allocation bytes;
  - measured direction, outcome classification, and retained state.

Constant evidence is checked by recomputing every ratio and delta. A positive
result below the threshold must have a failing promotion verdict and must not
be retained, but its direction/classification must still describe a positive,
sub-threshold result. There is NO automatic fix because timings, confidence,
GPU topology, exactness, and project policy are measured facts.`,
		Before: `if speedup < promotionThreshold {
	report("no improvement")
}`,
		After: `report := FusionPromotionReport{
	ControlLatencyNS: 215615, CandidateLatencyNS: 200139,
	MeasuredSpeedup: 1.077328,
	PromotionThreshold: 1.10, PromotionVerdict: "fail",
	DispatchDelta: -1, EstimatedIntermediateBytesDelta: -16384,
	MeasuredDirection: "improvement",
	OutcomeClassification: "positive-below-threshold",
	Retained: false,
}`,
		MeasuredWin: `The Apple-M2-Pro rows=1 residual-add plus RMSNorm
fusion behind issue #738 improved from 215,615 ns/op to 200,139 ns/op:
1.077328x, or about 7.18% lower latency. It removed one dispatch and one
intermediate write/read, but correctly failed the predeclared 1.10x project
gate and was rolled back. The absolute positive result remains reportable.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6040",
		Doc:  "GPU fusion reporting conflates measured gain with promotion verdict",
		Run:  runPS6040,
	},
})

type ps6040Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6040Axes = []ps6040Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6040HardwareField) }},
	{name: "workload identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6040WorkloadField) }},
	{name: "control latency", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6040SideField(n, "control", "latency") })
	}},
	{name: "candidate latency", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6040SideField(n, "candidate", "latency") })
	}},
	{name: "measured speedup", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6040SpeedupField) }},
	{name: "absolute latency delta", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6040LatencyDeltaField) }},
	{name: "relative latency reduction", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6040ReductionField) }},
	{name: "sample count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6040SampleCountField) }},
	{name: "confidence/sample quality", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6040ConfidenceField) }},
	{name: "promotion threshold", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6040ThresholdField) }},
	{name: "promotion verdict", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6040VerdictField) }},
	{name: "control dispatch count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6040SideField(n, "control", "dispatch") })
	}},
	{name: "candidate dispatch count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6040SideField(n, "candidate", "dispatch") })
	}},
	{name: "dispatch delta", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6040DispatchDeltaField) }},
	{name: "control intermediate bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6040SideField(n, "control", "intermediatebyte") })
	}},
	{name: "candidate intermediate bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6040SideField(n, "candidate", "intermediatebyte") })
	}},
	{name: "intermediate-bytes delta", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6040BytesDeltaField) }},
	{name: "exact-output status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6040ExactField) }},
	{name: "control allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6040SideField(n, "control", "allocationbyte") })
	}},
	{name: "candidate allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6040SideField(n, "candidate", "allocationbyte") })
	}},
	{name: "measured direction", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6040DirectionField) }},
	{name: "outcome classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6040ClassificationField) }},
	{name: "retained state", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6040RetainedField) }},
}

type ps6040Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6040(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6040Context(text) {
				continue
			}
			manifest, found := ps6040BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "GPU fusion promotion report has no absolute-gain manifest; missing %s", strings.Join(ps6040Missing(nil), ", "))
				continue
			}
			if missing := ps6040Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU fusion outcome evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6040Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU fusion outcome audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6040Context(text string) bool {
	text = ps6007NormalizeName(text)
	gpu := ps6007ContainsAny(text, "gpu", "metal", "cuda", "vulkan", "accelerator")
	fusion := ps6007ContainsAny(text, "fusion", "fused")
	report := ps6007ContainsAny(text, "promotionreport", "absolutegain", "rejectedpositive", "outcome")
	return gpu && fusion && report
}

func ps6040BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6040Manifest, bool) {
	var best ps6040Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6040ManifestType(lit.Type) {
			return true
		}
		manifest := ps6040Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6040Axes) - len(ps6040Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6040ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6040ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6040ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "fusionpromotionreport", "rejectedpositiveevidence", "absolutegainreport", "gpufusionoutcomeevidence", "fusionoutcomereport")
}

func ps6040Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6040Axes))
	for _, axis := range ps6040Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6040HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "deviceidentity", "gpuid")
}
func ps6040WorkloadField(name string) bool {
	return strings.Contains(name, "workload") && ps6007ContainsAny(name, "identity", "shape", "name")
}
func ps6040SideField(name, side, detail string) bool {
	return strings.Contains(name, side) && strings.Contains(name, detail)
}
func ps6040SpeedupField(name string) bool {
	return strings.Contains(name, "measured") && strings.Contains(name, "speedup")
}
func ps6040LatencyDeltaField(name string) bool {
	return strings.Contains(name, "latency") && strings.Contains(name, "delta") && ps6007ContainsAny(name, "absolute", "saved")
}
func ps6040ReductionField(name string) bool {
	return strings.Contains(name, "relative") && strings.Contains(name, "latency") && ps6007ContainsAny(name, "reduction", "delta")
}
func ps6040SampleCountField(name string) bool {
	return strings.Contains(name, "sample") && strings.Contains(name, "count")
}
func ps6040ConfidenceField(name string) bool {
	return ps6007ContainsAny(name, "confidence", "samplequality") && ps6007ContainsAny(name, "passed", "status", "class", "quality")
}
func ps6040ThresholdField(name string) bool {
	return strings.Contains(name, "promotion") && ps6007ContainsAny(name, "threshold", "minimum", "gate")
}
func ps6040VerdictField(name string) bool {
	return strings.Contains(name, "promotion") && ps6007ContainsAny(name, "verdict", "result")
}
func ps6040DispatchDeltaField(name string) bool {
	return strings.Contains(name, "dispatch") && strings.Contains(name, "delta")
}
func ps6040BytesDeltaField(name string) bool {
	return strings.Contains(name, "intermediatebyte") && strings.Contains(name, "delta")
}
func ps6040ExactField(name string) bool {
	return strings.Contains(name, "exactoutput") && ps6007ContainsAny(name, "passed", "status", "parity")
}
func ps6040DirectionField(name string) bool {
	return strings.Contains(name, "measured") && strings.Contains(name, "direction")
}
func ps6040ClassificationField(name string) bool {
	return strings.Contains(name, "outcome") && strings.Contains(name, "classification")
}
func ps6040RetainedField(name string) bool {
	return ps6007ContainsAny(name, "retained", "candidatekept", "shipped")
}

func ps6040Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 12)
	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"confidence/sample quality", ps6040ConfidenceField},
		{"exact-output parity", ps6040ExactField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	if samples, ok := ps6016Number(fields, ps6040SampleCountField); ok && samples <= 0 {
		warnings = append(warnings, "sample count must be positive")
	}
	control, controlOK := ps6016Number(fields, func(n string) bool { return ps6040SideField(n, "control", "latency") })
	candidate, candidateOK := ps6016Number(fields, func(n string) bool { return ps6040SideField(n, "candidate", "latency") })
	speedup, speedupOK := ps6016Number(fields, ps6040SpeedupField)
	gateSpeedup, gateSpeedupOK := speedup, speedupOK
	threshold, thresholdOK := ps6016Number(fields, ps6040ThresholdField)
	if controlOK && candidateOK && control > 0 && candidate > 0 {
		calculatedSpeedup := control / candidate
		gateSpeedup, gateSpeedupOK = calculatedSpeedup, true
		if speedupOK && !ps6025Close(speedup, calculatedSpeedup) {
			warnings = append(warnings, fmt.Sprintf("measured speedup %.6gx disagrees with latency ratio %.6gx", speedup, calculatedSpeedup))
		}
		if delta, ok := ps6016Number(fields, ps6040LatencyDeltaField); ok && !ps6025Close(delta, control-candidate) {
			warnings = append(warnings, fmt.Sprintf("absolute latency delta %.6g ns disagrees with control-candidate %.6g ns", delta, control-candidate))
		}
		if reduction, ok := ps6016Number(fields, ps6040ReductionField); ok && !ps6025Close(reduction, (control-candidate)/control) {
			warnings = append(warnings, fmt.Sprintf("relative latency reduction %.6g disagrees with measured %.6g", reduction, (control-candidate)/control))
		}
	}
	for _, delta := range []struct {
		label     string
		detail    string
		predicate func(string) bool
	}{
		{"dispatch", "dispatch", ps6040DispatchDeltaField},
		{"intermediate-bytes", "intermediatebyte", ps6040BytesDeltaField},
	} {
		controlValue, controlValueOK := ps6016Number(fields, func(n string) bool { return ps6040SideField(n, "control", delta.detail) })
		candidateValue, candidateValueOK := ps6016Number(fields, func(n string) bool { return ps6040SideField(n, "candidate", delta.detail) })
		recorded, recordedOK := ps6016Number(fields, delta.predicate)
		if controlValueOK && candidateValueOK && recordedOK && !ps6025Close(recorded, candidateValue-controlValue) {
			warnings = append(warnings, fmt.Sprintf("%s delta %.6g disagrees with candidate-control %.6g", delta.label, recorded, candidateValue-controlValue))
		}
	}
	controlAlloc, controlAllocOK := ps6016Number(fields, func(n string) bool { return ps6040SideField(n, "control", "allocationbyte") })
	candidateAlloc, candidateAllocOK := ps6016Number(fields, func(n string) bool { return ps6040SideField(n, "candidate", "allocationbyte") })
	if controlAllocOK && candidateAllocOK && controlAlloc != candidateAlloc {
		warnings = append(warnings, fmt.Sprintf("control/candidate allocation bytes differ (%.6g vs %.6g)", controlAlloc, candidateAlloc))
	}
	if gateSpeedupOK && thresholdOK {
		passed := gateSpeedup >= threshold
		if verdict, ok := ps6027String(fields, ps6040VerdictField); ok {
			verdict = ps6030StatusName(verdict)
			verdictPass := ps6007ContainsAny(verdict, "pass", "promote", "accept") && !ps6007ContainsAny(verdict, "fail", "reject")
			verdictFail := ps6007ContainsAny(verdict, "fail", "reject", "belowthreshold")
			if passed && !verdictPass {
				warnings = append(warnings, fmt.Sprintf("promotion verdict %q is not pass for %.6gx meeting %.6gx threshold", verdict, gateSpeedup, threshold))
			}
			if !passed && !verdictFail {
				warnings = append(warnings, fmt.Sprintf("promotion verdict %q is not fail for %.6gx below %.6gx threshold", verdict, gateSpeedup, threshold))
			}
		}
		if retained, ok := ps6026Bool(fields, ps6040RetainedField); ok && !passed && retained {
			warnings = append(warnings, fmt.Sprintf("candidate is retained despite %.6gx missing %.6gx promotion threshold", gateSpeedup, threshold))
		}
		if direction, ok := ps6027String(fields, ps6040DirectionField); ok {
			direction = ps6030StatusName(direction)
			if gateSpeedup > 1 && ps6007ContainsAny(direction, "regression", "noop", "nochange", "negative") {
				warnings = append(warnings, fmt.Sprintf("measured direction %q erases a positive %.6gx result", direction, gateSpeedup))
			}
			if gateSpeedup < 1 && ps6007ContainsAny(direction, "improvement", "positive", "faster") {
				warnings = append(warnings, fmt.Sprintf("measured direction %q contradicts a %.6gx regression", direction, gateSpeedup))
			}
		}
		if classification, ok := ps6027String(fields, ps6040ClassificationField); ok && gateSpeedup > 1 && !passed {
			classification = ps6030StatusName(classification)
			if !ps6007ContainsAny(classification, "positive", "improvement", "gain", "subthreshold", "belowthreshold") {
				warnings = append(warnings, fmt.Sprintf("outcome classification %q does not preserve the positive sub-threshold result", classification))
			}
		}
	}
	return warnings
}
