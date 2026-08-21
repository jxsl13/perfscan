package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6042 implements owner issue #736: GPU encoder collapse must preserve
// dispatch, barrier, and dependency-depth evidence plus end-to-end timing.
var PS6042 = register(&lint.Check{
	ID:       "PS6042",
	Category: "verify",
	Slug:     "gpu-encoder-collapse-needs-barrier-depth",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a GPU encoder-count reduction hides unchanged dependency depth",
		Text: `Encoder or region count is not a sufficient proxy for GPU
submission overhead. Concurrent compute regions may collapse hundreds of
encoders while explicit barriers and dispatches preserve nearly the same
critical path.

This check implements owner issue #736. It audits EncoderCollapseEvidence,
GPURegionBarrierReport, DependencyDepthEvidence, SubmissionTopologyReport, or
equivalent manifests. Evidence must record:

  - hardware, OS, model/workload, synchronization pattern, warm residency,
    and iteration count;
  - control/candidate command buffers per submission, compute regions,
    non-compute encoders, and total encoder events;
  - encoder reduction ratio;
  - control/candidate compute dispatches and candidate dispatches/region;
  - control/candidate explicit barriers and candidate barriers/dispatch;
  - control/candidate longest dependency depth and depth ratio;
  - control/candidate end-to-end time and their ratio;
  - promotion threshold/verdict, exact output, classification, and decision.

Constant evidence is checked for topology-total and ratio inconsistencies. A
large encoder collapse whose dispatch count and dependency depth remain near
one, with dense barriers, must be classified as structural-only unless end-to-
end timing clears the frozen promotion gate. There is NO automatic fix because
GPU hazards, regions, critical paths, and timings are backend/runtime facts.`,
		Before: `speedup := controlEncoderEvents / candidateEncoderEvents`,
		After: `evidence := GPURegionBarrierReport{
	ControlEncoderEvents: 340, CandidateEncoderEvents: 45,
	CandidateComputeRegions: 23, CandidateNonComputeEncoders: 22,
	CandidateDispatches: 296, CandidateExplicitBarriers: 221,
	CandidateBarriersPerDispatch: 221.0 / 296,
	ControlEndToEndNS: 7_376_051_833,
	CandidateEndToEndNS: 7_344_744_834,
	EndToEndControlCandidateRatio: 1.0043,
	Classification: "structural-only",
}`,
		MeasuredWin: `The Apple-M2-Pro experiment behind issue #736 reduced
340 encoder events to 23 compute regions plus 22 blit encoders (45 total,
7.56x fewer), but still executed 296 dispatches and 221 explicit barriers per
step. Warm 200-token decode changed from 7,376,051,833 ns to 7,344,744,834 ns,
only about 1.0043x, so the topology result did not clear promotion.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6042",
		Doc:  "GPU encoder collapse lacks barrier/dependency-depth evidence",
		Run:  runPS6042,
	},
})

type ps6042Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6042Axes = []ps6042Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042HardwareField) }},
	{name: "OS identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042OSField) }},
	{name: "model/workload identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042WorkloadField) }},
	{name: "synchronization pattern", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042SyncField) }},
	{name: "warm residency status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042WarmField) }},
	{name: "iteration count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042IterationsField) }},
	{name: "control command buffers/submission", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042SideMetric(n, "control", "commandbuffer") })
	}},
	{name: "candidate command buffers/submission", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042SideMetric(n, "candidate", "commandbuffer") })
	}},
	{name: "control compute regions", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042SideMetric(n, "control", "computeregion") })
	}},
	{name: "candidate compute regions", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042SideMetric(n, "candidate", "computeregion") })
	}},
	{name: "control non-compute encoders", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042SideMetric(n, "control", "noncomputeencoder") })
	}},
	{name: "candidate non-compute encoders", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042SideMetric(n, "candidate", "noncomputeencoder") })
	}},
	{name: "control encoder events", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042SideMetric(n, "control", "encoderevent") })
	}},
	{name: "candidate encoder events", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042SideMetric(n, "candidate", "encoderevent") })
	}},
	{name: "encoder reduction ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042EncoderRatioField) }},
	{name: "control dispatches", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042SideMetric(n, "control", "dispatch") })
	}},
	{name: "candidate dispatches", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042SideMetric(n, "candidate", "dispatch") })
	}},
	{name: "candidate dispatches/region", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042DispatchDensityField) }},
	{name: "control explicit barriers", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042SideMetric(n, "control", "explicitbarrier") })
	}},
	{name: "candidate explicit barriers", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042SideMetric(n, "candidate", "explicitbarrier") })
	}},
	{name: "candidate barriers/dispatch", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042BarrierDensityField) }},
	{name: "control dependency depth", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042SideMetric(n, "control", "dependencydepth") })
	}},
	{name: "candidate dependency depth", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042SideMetric(n, "candidate", "dependencydepth") })
	}},
	{name: "dependency-depth ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042DepthRatioField) }},
	{name: "control end-to-end time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042EndToEndField(n, "control") })
	}},
	{name: "candidate end-to-end time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6042EndToEndField(n, "candidate") })
	}},
	{name: "end-to-end ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042EndRatioField) }},
	{name: "promotion threshold", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042ThresholdField) }},
	{name: "promotion verdict", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042VerdictField) }},
	{name: "exact output", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042ExactField) }},
	{name: "classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042ClassificationField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6042DecisionField) }},
}

type ps6042Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6042(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6042Context(text) {
				continue
			}
			manifest, found := ps6042BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "GPU encoder-collapse campaign has no barrier-depth manifest; missing %s", strings.Join(ps6042Missing(nil), ", "))
				continue
			}
			if missing := ps6042Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU encoder-collapse evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6042Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU encoder-collapse audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6042Context(text string) bool {
	text = ps6007NormalizeName(text)
	gpu := ps6007ContainsAny(text, "gpu", "metal", "cuda", "vulkan", "accelerator")
	encoder := ps6007ContainsAny(text, "encodercollapse", "regioncollapse", "submissiontopology")
	barrier := ps6007ContainsAny(text, "barrierdepth", "dependencydepth", "regionbarrier")
	return gpu && encoder && barrier
}

func ps6042BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6042Manifest, bool) {
	var best ps6042Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6042ManifestType(lit.Type) {
			return true
		}
		manifest := ps6042Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6042Axes) - len(ps6042Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6042ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6042ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6042ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "encodercollapseevidence", "gpuregionbarrierreport", "dependencydepthevidence", "submissiontopologyreport", "encodercollapsereport")
}

func ps6042Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6042Axes))
	for _, axis := range ps6042Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6042HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "deviceidentity", "gpuid")
}
func ps6042OSField(name string) bool {
	return ps6007ContainsAny(name, "osidentity", "osversion", "macos")
}
func ps6042WorkloadField(name string) bool {
	return ps6007ContainsAny(name, "model", "workload") && ps6007ContainsAny(name, "identity", "name", "shape")
}
func ps6042SyncField(name string) bool {
	return strings.Contains(name, "synchronization") && ps6007ContainsAny(name, "pattern", "boundary", "mode")
}
func ps6042WarmField(name string) bool {
	return strings.Contains(name, "warm") && strings.Contains(name, "resident")
}
func ps6042IterationsField(name string) bool {
	return strings.Contains(name, "iteration") && strings.Contains(name, "count")
}
func ps6042SideMetric(name, side, metric string) bool {
	if strings.Contains(name, "ratio") || strings.Contains(name, "perregion") || strings.Contains(name, "perdispatch") {
		return false
	}
	return strings.Contains(name, side) && strings.Contains(name, metric)
}
func ps6042EncoderRatioField(name string) bool {
	return strings.Contains(name, "encoder") && strings.Contains(name, "reduction") && strings.Contains(name, "ratio")
}
func ps6042DispatchDensityField(name string) bool {
	return strings.Contains(name, "candidate") && strings.Contains(name, "dispatchesperregion")
}
func ps6042BarrierDensityField(name string) bool {
	return strings.Contains(name, "candidate") && strings.Contains(name, "barriersperdispatch")
}
func ps6042DepthRatioField(name string) bool {
	return strings.Contains(name, "dependencydepth") && strings.Contains(name, "ratio")
}
func ps6042EndToEndField(name, side string) bool {
	return strings.Contains(name, side) && strings.Contains(name, "endtoend") && ps6007ContainsAny(name, "ns", "time", "duration")
}
func ps6042EndRatioField(name string) bool {
	return strings.Contains(name, "endtoend") && strings.Contains(name, "controlcandidate") && ps6007ContainsAny(name, "ratio", "speedup")
}
func ps6042ThresholdField(name string) bool {
	return strings.Contains(name, "promotion") && ps6007ContainsAny(name, "threshold", "minimum", "gate")
}
func ps6042VerdictField(name string) bool {
	return strings.Contains(name, "promotion") && ps6007ContainsAny(name, "verdict", "result")
}
func ps6042ExactField(name string) bool {
	return strings.Contains(name, "exact") && ps6007ContainsAny(name, "output", "logit", "parity")
}
func ps6042ClassificationField(name string) bool {
	return ps6007ContainsAny(name, "classification", "resultclass")
}
func ps6042DecisionField(name string) bool {
	return strings.Contains(name, "final") && strings.Contains(name, "decision")
}

func ps6042Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 14)
	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"warm residency", ps6042WarmField},
		{"exact output", ps6042ExactField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	for _, count := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"iteration count", ps6042IterationsField},
		{"control command buffers/submission", func(n string) bool { return ps6042SideMetric(n, "control", "commandbuffer") }},
		{"candidate command buffers/submission", func(n string) bool { return ps6042SideMetric(n, "candidate", "commandbuffer") }},
	} {
		if value, ok := ps6016Number(fields, count.predicate); ok && value <= 0 {
			warnings = append(warnings, count.name+" must be positive")
		}
	}
	controlRegions, controlRegionsOK := ps6016Number(fields, func(n string) bool { return ps6042SideMetric(n, "control", "computeregion") })
	candidateRegions, candidateRegionsOK := ps6016Number(fields, func(n string) bool { return ps6042SideMetric(n, "candidate", "computeregion") })
	controlNonCompute, controlNonComputeOK := ps6016Number(fields, func(n string) bool { return ps6042SideMetric(n, "control", "noncomputeencoder") })
	candidateNonCompute, candidateNonComputeOK := ps6016Number(fields, func(n string) bool { return ps6042SideMetric(n, "candidate", "noncomputeencoder") })
	controlEvents, controlEventsOK := ps6016Number(fields, func(n string) bool { return ps6042SideMetric(n, "control", "encoderevent") })
	candidateEvents, candidateEventsOK := ps6016Number(fields, func(n string) bool { return ps6042SideMetric(n, "candidate", "encoderevent") })
	if controlRegionsOK && controlNonComputeOK && controlEventsOK && controlRegions+controlNonCompute != controlEvents {
		warnings = append(warnings, fmt.Sprintf("control regions+non-compute encoders %.6g disagree with %.6g encoder events", controlRegions+controlNonCompute, controlEvents))
	}
	if candidateRegionsOK && candidateNonComputeOK && candidateEventsOK && candidateRegions+candidateNonCompute != candidateEvents {
		warnings = append(warnings, fmt.Sprintf("candidate regions+non-compute encoders %.6g disagree with %.6g encoder events", candidateRegions+candidateNonCompute, candidateEvents))
	}
	encoderRatio, encoderRatioOK := ps6016Number(fields, ps6042EncoderRatioField)
	calculatedEncoderRatio, calculatedEncoderRatioOK := 0.0, controlEventsOK && candidateEventsOK && candidateEvents > 0
	if calculatedEncoderRatioOK {
		calculatedEncoderRatio = controlEvents / candidateEvents
		if encoderRatioOK && !ps6025Close(encoderRatio, calculatedEncoderRatio) {
			warnings = append(warnings, fmt.Sprintf("encoder reduction ratio %.6gx disagrees with control/candidate %.6gx", encoderRatio, calculatedEncoderRatio))
		}
	}
	controlDispatches, controlDispatchesOK := ps6016Number(fields, func(n string) bool { return ps6042SideMetric(n, "control", "dispatch") })
	candidateDispatches, candidateDispatchesOK := ps6016Number(fields, func(n string) bool { return ps6042SideMetric(n, "candidate", "dispatch") })
	if density, ok := ps6016Number(fields, ps6042DispatchDensityField); ok && candidateDispatchesOK && candidateRegionsOK && candidateRegions > 0 && !ps6025Close(density, candidateDispatches/candidateRegions) {
		warnings = append(warnings, fmt.Sprintf("candidate dispatches/region %.6g disagrees with %.6g", density, candidateDispatches/candidateRegions))
	}
	candidateBarriers, candidateBarriersOK := ps6016Number(fields, func(n string) bool { return ps6042SideMetric(n, "candidate", "explicitbarrier") })
	barrierDensity, barrierDensityOK := ps6016Number(fields, ps6042BarrierDensityField)
	if barrierDensityOK && candidateBarriersOK && candidateDispatchesOK && candidateDispatches > 0 && !ps6025Close(barrierDensity, candidateBarriers/candidateDispatches) {
		warnings = append(warnings, fmt.Sprintf("candidate barriers/dispatch %.6g disagrees with %.6g", barrierDensity, candidateBarriers/candidateDispatches))
	}
	controlDepth, controlDepthOK := ps6016Number(fields, func(n string) bool { return ps6042SideMetric(n, "control", "dependencydepth") })
	candidateDepth, candidateDepthOK := ps6016Number(fields, func(n string) bool { return ps6042SideMetric(n, "candidate", "dependencydepth") })
	depthRatio, depthRatioOK := ps6016Number(fields, ps6042DepthRatioField)
	if depthRatioOK && controlDepthOK && candidateDepthOK && candidateDepth > 0 && !ps6025Close(depthRatio, controlDepth/candidateDepth) {
		warnings = append(warnings, fmt.Sprintf("dependency-depth ratio %.6gx disagrees with control/candidate %.6gx", depthRatio, controlDepth/candidateDepth))
	}
	controlEnd, controlEndOK := ps6016Number(fields, func(n string) bool { return ps6042EndToEndField(n, "control") })
	candidateEnd, candidateEndOK := ps6016Number(fields, func(n string) bool { return ps6042EndToEndField(n, "candidate") })
	endRatio, endRatioOK := ps6016Number(fields, ps6042EndRatioField)
	calculatedEndRatio, calculatedEndRatioOK := 0.0, controlEndOK && candidateEndOK && candidateEnd > 0
	if calculatedEndRatioOK {
		calculatedEndRatio = controlEnd / candidateEnd
		if endRatioOK && !ps6025Close(endRatio, calculatedEndRatio) {
			warnings = append(warnings, fmt.Sprintf("end-to-end ratio %.6gx disagrees with control/candidate %.6gx", endRatio, calculatedEndRatio))
		}
	}
	threshold, thresholdOK := ps6016Number(fields, ps6042ThresholdField)
	if calculatedEndRatioOK && thresholdOK {
		passed := calculatedEndRatio >= threshold
		if verdict, ok := ps6027String(fields, ps6042VerdictField); ok {
			normalized := ps6030StatusName(verdict)
			verdictPass := ps6007ContainsAny(normalized, "pass", "promote", "accept") && !ps6007ContainsAny(normalized, "fail", "reject")
			verdictFail := ps6007ContainsAny(normalized, "fail", "reject", "belowthreshold")
			if passed && !verdictPass || !passed && !verdictFail {
				warnings = append(warnings, fmt.Sprintf("promotion verdict %q disagrees with %.6gx versus %.6gx threshold", verdict, calculatedEndRatio, threshold))
			}
		}
		nearDispatch := controlDispatchesOK && candidateDispatchesOK && controlDispatches > 0 && candidateDispatches/controlDispatches >= 0.9
		nearDepth := controlDepthOK && candidateDepthOK && controlDepth > 0 && candidateDepth/controlDepth >= 0.9
		denseBarriers := barrierDensityOK && barrierDensity >= 0.5
		largeCollapse := calculatedEncoderRatioOK && calculatedEncoderRatio >= 2
		if largeCollapse && nearDispatch && nearDepth && denseBarriers && !passed {
			if classification, ok := ps6027String(fields, ps6042ClassificationField); ok && ps6007ContainsAny(ps6030StatusName(classification), "performanceimprovement", "speedup", "endtoendwin") {
				warnings = append(warnings, fmt.Sprintf("classification %q turns a %.6gx structural collapse with near-unchanged dispatch/depth into a performance win", classification, calculatedEncoderRatio))
			}
			if decision, ok := ps6027String(fields, ps6042DecisionField); ok && ps6007ContainsAny(ps6030StatusName(decision), "retain", "promote", "ship", "selected") {
				warnings = append(warnings, fmt.Sprintf("final decision %q retains an encoder collapse below the end-to-end promotion gate", decision))
			}
		}
	}
	return warnings
}
