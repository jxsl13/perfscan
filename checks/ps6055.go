package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6055 implements owner issue #723: fewer Metal compute encoders are not an
// optimization signal unless a priced pressure/traffic/synchronization axis and
// a complete one-command-buffer seam establish leverage.
var PS6055 = register(&lint.Check{
	ID:       "PS6055",
	Category: "verify",
	Slug:     "metal-fusion-ranking-needs-priced-seam",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a Metal fusion candidate is ranked from encoder count without priced leverage and a complete seam",
		Text: `Placing adjacent dispatches in one compute encoder reduces encoder
count, but that structural change may leave dispatch work, intermediate
traffic, dependency depth, and synchronization unchanged. Stage-boundary GPU
intervals can also double-count or price implicit boundaries that disappear
from the measurement rather than the workload.

This check implements owner issue #723. It audits MetalFusionRankingEvidence,
EncoderCountFusionGate, CommandFusionSeamReport,
EncoderGroupingCandidateEvidence, or equivalent manifests. Evidence must
record:

  - hardware/toolchain, exact seam shape, default-off and warm-resident status;
  - unchanged production pipelines, command buffers, compute encoders,
    dispatches, explicit barriers, and the encoder reduction ratio;
  - measured CPU encoding/submission pressure and times, or concrete
    intermediate-traffic/synchronization counts and reduction claims;
  - exclusion of inflated stage-boundary intervals;
  - a complete same-one-command-buffer seam, iteration count, grouped profiler
    labels, control/candidate time and ratio, allocations, and exact odd-tail
    parity;
  - frozen performance threshold/verdict, ranking status, early-removal status,
    classification, and final decision.

Constant evidence is checked for stale ratios, false traffic/synchronization
claims, allocation differences, incomplete seam controls, and any ranking or
retention driven by encoder count when the priced eligibility or performance
gate fails. A correctly measured negative candidate may record no priced
leverage and remain clean when it is unranked and removed.

There is NO automatic fix because Metal hazard semantics, host pressure,
resource traffic, and one-command-buffer timings are backend/runtime facts.`,
		Before: `score := controlComputeEncoders / candidateComputeEncoders
rank(candidate, score) // structural count or stage-boundary intervals only`,
		After: `eligible := measuredHostPressure || provenTrafficReduction || provenSyncReduction
result := benchmarkCompleteOneCommandBufferSeam(control, candidate)
promote := eligible && result.ExactParity && result.Speedup >= frozenThreshold`,
		MeasuredWin: `The Apple-M2-Pro issue #723 candidate grouped two production
Q4_K matvec dispatches and binary SwiGLU in one encoder with a buffer barrier.
The fixed 200x M1,K2048,N5632 seam measured 426,352 ns/op control versus
425,334 ns/op grouped—only 1.002x—with both at 8 B/op and 1 alloc/op. Odd-tail
parity and one grouped profiler label passed; the candidate was removed before
paired/model stages.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6055",
		Doc:  "Metal encoder-count fusion ranking lacks priced leverage and complete seam evidence",
		Run:  runPS6055,
	},
})

type ps6055Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6055Axes = []ps6055Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055HardwareField) }},
	{name: "toolchain identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055ToolchainField) }},
	{name: "seam shape identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055ShapeField) }},
	{name: "candidate default-off status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055DefaultOffField) }},
	{name: "warm-resident inputs", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055WarmField) }},
	{name: "unchanged production Q4_K pipeline", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055Q4KField) }},
	{name: "unchanged production SwiGLU pipeline", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055SwiGLUField) }},
	{name: "control command-buffer count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055CountField(n, "control", "commandbuffer") })
	}},
	{name: "candidate command-buffer count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055CountField(n, "candidate", "commandbuffer") })
	}},
	{name: "control compute-encoder count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055CountField(n, "control", "computeencoder") })
	}},
	{name: "candidate compute-encoder count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055CountField(n, "candidate", "computeencoder") })
	}},
	{name: "encoder reduction ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055EncoderRatioField) }},
	{name: "control dispatch count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055CountField(n, "control", "dispatch") })
	}},
	{name: "candidate dispatch count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055CountField(n, "candidate", "dispatch") })
	}},
	{name: "candidate explicit barrier count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055BarrierField) }},
	{name: "CPU encoding/submission pressure measurement", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055HostMeasuredField) }},
	{name: "control CPU encoding/submission time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055HostTimeField(n, "control") })
	}},
	{name: "candidate CPU encoding/submission time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055HostTimeField(n, "candidate") })
	}},
	{name: "traffic-reduction claim", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055TrafficClaimField) }},
	{name: "control intermediate traffic bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055TrafficField(n, "control") })
	}},
	{name: "candidate intermediate traffic bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055TrafficField(n, "candidate") })
	}},
	{name: "synchronization-reduction claim", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055SyncClaimField) }},
	{name: "control synchronization event count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055SyncCountField(n, "control") })
	}},
	{name: "candidate synchronization event count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055SyncCountField(n, "candidate") })
	}},
	{name: "stage-boundary interval inflation exclusion", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055BoundaryField) }},
	{name: "complete one-command-buffer seam", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055SeamField) }},
	{name: "fixed iteration count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055IterationsField) }},
	{name: "grouped profiler label count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055ProfilerField) }},
	{name: "control seam time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055SeamTimeField(n, "control") })
	}},
	{name: "candidate seam time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055SeamTimeField(n, "candidate") })
	}},
	{name: "seam control/candidate ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055SeamRatioField) }},
	{name: "control allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055AllocationField(n, "control", "byte") })
	}},
	{name: "candidate allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055AllocationField(n, "candidate", "byte") })
	}},
	{name: "control allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055AllocationField(n, "control", "count") })
	}},
	{name: "candidate allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6055AllocationField(n, "candidate", "count") })
	}},
	{name: "exact odd-tail parity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055ParityField) }},
	{name: "performance minimum", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055MinimumField) }},
	{name: "performance verdict", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055VerdictField) }},
	{name: "candidate ranking status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055RankedField) }},
	{name: "removed-before-paired/model status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055EarlyRemovalField) }},
	{name: "classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055ClassificationField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6055DecisionField) }},
}

type ps6055Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6055(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6055Context(text) {
				continue
			}
			manifest, found := ps6055BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "Metal encoder-count fusion ranking has no priced seam manifest; missing %s", strings.Join(ps6055Missing(nil), ", "))
				continue
			}
			if missing := ps6055Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "Metal fusion-ranking evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6055Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "Metal fusion-ranking audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6055Context(text string) bool {
	text = ps6007NormalizeName(text)
	return strings.Contains(text, "metal") && ps6007ContainsAny(text, "encodercountfusion", "encodergrouping", "commandfusion") && ps6007ContainsAny(text, "ranking", "pricedseam", "onecommandbufferseam")
}

func ps6055BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6055Manifest, bool) {
	var best ps6055Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6055ManifestType(lit.Type) {
			return true
		}
		manifest := ps6055Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6055Axes) - len(ps6055Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6055ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6055ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6055ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "metalfusionrankingevidence", "encodercountfusiongate", "commandfusionseamreport", "encodergroupingcandidateevidence", "pricedfusionseamevidence")
}

func ps6055Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6055Axes))
	for _, axis := range ps6055Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6055HardwareField(n string) bool {
	return ps6007ContainsAny(n, "hardware", "deviceidentity", "gpuid")
}
func ps6055ToolchainField(n string) bool {
	return ps6007ContainsAny(n, "toolchain", "compileridentity")
}
func ps6055ShapeField(n string) bool {
	return strings.Contains(n, "seam") && strings.Contains(n, "shape")
}
func ps6055DefaultOffField(n string) bool {
	return strings.Contains(n, "candidate") && strings.Contains(n, "defaultoff")
}
func ps6055WarmField(n string) bool {
	return strings.Contains(n, "warm") && strings.Contains(n, "resident")
}
func ps6055Q4KField(n string) bool {
	return strings.Contains(n, "productionq4k") && strings.Contains(n, "unchanged")
}
func ps6055SwiGLUField(n string) bool {
	return strings.Contains(n, "productionswiglu") && strings.Contains(n, "unchanged")
}
func ps6055CountField(n, side, detail string) bool {
	return strings.Contains(n, side) && strings.Contains(n, detail) && strings.Contains(n, "count") && !strings.Contains(n, "allocation")
}
func ps6055EncoderRatioField(n string) bool {
	return strings.Contains(n, "encoder") && strings.Contains(n, "reduction") && strings.Contains(n, "ratio")
}
func ps6055BarrierField(n string) bool {
	return strings.Contains(n, "candidate") && strings.Contains(n, "explicitbarrier") && strings.Contains(n, "count")
}
func ps6055HostMeasuredField(n string) bool {
	return strings.Contains(n, "cpu") && strings.Contains(n, "encodingsubmission") && strings.Contains(n, "pressure") && strings.Contains(n, "measured")
}
func ps6055HostTimeField(n, side string) bool {
	return strings.Contains(n, side) && strings.Contains(n, "cpu") && strings.Contains(n, "encodingsubmission") && ps6007ContainsAny(n, "ns", "time")
}
func ps6055TrafficClaimField(n string) bool {
	return strings.Contains(n, "trafficreduction") && ps6007ContainsAny(n, "documented", "claimed", "proven")
}
func ps6055TrafficField(n, side string) bool {
	return strings.Contains(n, side) && strings.Contains(n, "intermediatetraffic") && strings.Contains(n, "byte")
}
func ps6055SyncClaimField(n string) bool {
	return strings.Contains(n, "synchronizationreduction") && ps6007ContainsAny(n, "documented", "claimed", "proven")
}
func ps6055SyncCountField(n, side string) bool {
	return strings.Contains(n, side) && strings.Contains(n, "synchronizationevent") && strings.Contains(n, "count")
}
func ps6055BoundaryField(n string) bool {
	return strings.Contains(n, "stageboundary") && strings.Contains(n, "intervalinflation") && strings.Contains(n, "excluded")
}
func ps6055SeamField(n string) bool {
	return strings.Contains(n, "complete") && strings.Contains(n, "onecommandbuffer") && strings.Contains(n, "seam")
}
func ps6055IterationsField(n string) bool {
	return strings.Contains(n, "fixed") && strings.Contains(n, "iteration") && strings.Contains(n, "count")
}
func ps6055ProfilerField(n string) bool {
	return strings.Contains(n, "groupedprofilerlabel") && strings.Contains(n, "count")
}
func ps6055SeamTimeField(n, side string) bool {
	return strings.Contains(n, side) && strings.Contains(n, "seam") && ps6007ContainsAny(n, "ns", "time") && !strings.Contains(n, "ratio")
}
func ps6055SeamRatioField(n string) bool {
	return strings.Contains(n, "seam") && strings.Contains(n, "controlcandidate") && strings.Contains(n, "ratio")
}
func ps6055AllocationField(n, side, detail string) bool {
	return strings.Contains(n, side) && strings.Contains(n, "allocation") && strings.Contains(n, detail)
}
func ps6055ParityField(n string) bool {
	return strings.Contains(n, "exactoddtail") && strings.Contains(n, "parity")
}
func ps6055MinimumField(n string) bool {
	return strings.Contains(n, "performance") && ps6007ContainsAny(n, "minimum", "threshold", "gate")
}
func ps6055VerdictField(n string) bool {
	return strings.Contains(n, "performance") && strings.Contains(n, "verdict")
}
func ps6055RankedField(n string) bool {
	return strings.Contains(n, "candidate") && strings.Contains(n, "ranked")
}
func ps6055EarlyRemovalField(n string) bool {
	return strings.Contains(n, "removedbefore") && strings.Contains(n, "paired") && strings.Contains(n, "model")
}
func ps6055ClassificationField(n string) bool {
	return ps6007ContainsAny(n, "classification", "resultclass")
}
func ps6055DecisionField(n string) bool {
	return strings.Contains(n, "final") && strings.Contains(n, "decision")
}

func ps6055Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 18)
	for _, status := range []struct {
		name string
		pred func(string) bool
	}{
		{"candidate default-off", ps6055DefaultOffField},
		{"warm-resident inputs", ps6055WarmField},
		{"unchanged production Q4_K pipeline", ps6055Q4KField},
		{"unchanged production SwiGLU pipeline", ps6055SwiGLUField},
		{"stage-boundary interval inflation exclusion", ps6055BoundaryField},
		{"complete one-command-buffer seam", ps6055SeamField},
		{"exact odd-tail parity", ps6055ParityField},
	} {
		if value, known := ps6026Bool(fields, status.pred); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	controlCommandBuffers, controlCommandBuffersOK := ps6016Number(fields, func(n string) bool { return ps6055CountField(n, "control", "commandbuffer") })
	candidateCommandBuffers, candidateCommandBuffersOK := ps6016Number(fields, func(n string) bool { return ps6055CountField(n, "candidate", "commandbuffer") })
	if controlCommandBuffersOK && candidateCommandBuffersOK && (controlCommandBuffers != 1 || candidateCommandBuffers != 1) {
		warnings = append(warnings, "control and candidate are not both complete one-command-buffer seams")
	}
	controlEncoders, controlEncodersOK := ps6016Number(fields, func(n string) bool { return ps6055CountField(n, "control", "computeencoder") })
	candidateEncoders, candidateEncodersOK := ps6016Number(fields, func(n string) bool { return ps6055CountField(n, "candidate", "computeencoder") })
	if recorded, ok := ps6016Number(fields, ps6055EncoderRatioField); ok && controlEncodersOK && candidateEncodersOK && candidateEncoders > 0 && !ps6025Close(recorded, controlEncoders/candidateEncoders) {
		warnings = append(warnings, fmt.Sprintf("encoder reduction ratio %.6gx disagrees with control/candidate %.6gx", recorded, controlEncoders/candidateEncoders))
	}
	controlDispatches, controlDispatchesOK := ps6016Number(fields, func(n string) bool { return ps6055CountField(n, "control", "dispatch") })
	candidateDispatches, candidateDispatchesOK := ps6016Number(fields, func(n string) bool { return ps6055CountField(n, "candidate", "dispatch") })
	if controlDispatchesOK && candidateDispatchesOK && controlDispatches != candidateDispatches {
		warnings = append(warnings, "control/candidate dispatch counts differ")
	}

	hostMeasured, hostMeasuredOK := ps6026Bool(fields, ps6055HostMeasuredField)
	controlHost, controlHostOK := ps6016Number(fields, func(n string) bool { return ps6055HostTimeField(n, "control") })
	candidateHost, candidateHostOK := ps6016Number(fields, func(n string) bool { return ps6055HostTimeField(n, "candidate") })
	hostEligible := hostMeasuredOK && hostMeasured && controlHostOK && candidateHostOK && controlHost > 0 && candidateHost > 0
	if hostMeasuredOK && hostMeasured && !hostEligible {
		warnings = append(warnings, "CPU encoding/submission pressure is marked measured without positive control/candidate times")
	}
	trafficClaim, trafficClaimOK := ps6026Bool(fields, ps6055TrafficClaimField)
	controlTraffic, controlTrafficOK := ps6016Number(fields, func(n string) bool { return ps6055TrafficField(n, "control") })
	candidateTraffic, candidateTrafficOK := ps6016Number(fields, func(n string) bool { return ps6055TrafficField(n, "candidate") })
	trafficReduced := controlTrafficOK && candidateTrafficOK && candidateTraffic < controlTraffic
	if trafficClaimOK && trafficClaim != trafficReduced {
		warnings = append(warnings, fmt.Sprintf("traffic-reduction claim is %t but control/candidate bytes are %.6g/%.6g", trafficClaim, controlTraffic, candidateTraffic))
	}
	syncClaim, syncClaimOK := ps6026Bool(fields, ps6055SyncClaimField)
	controlSync, controlSyncOK := ps6016Number(fields, func(n string) bool { return ps6055SyncCountField(n, "control") })
	candidateSync, candidateSyncOK := ps6016Number(fields, func(n string) bool { return ps6055SyncCountField(n, "candidate") })
	syncReduced := controlSyncOK && candidateSyncOK && candidateSync < controlSync
	if syncClaimOK && syncClaim != syncReduced {
		warnings = append(warnings, fmt.Sprintf("synchronization-reduction claim is %t but control/candidate event counts are %.6g/%.6g", syncClaim, controlSync, candidateSync))
	}
	eligible := hostEligible || trafficClaimOK && trafficClaim && trafficReduced || syncClaimOK && syncClaim && syncReduced

	controlSeam, controlSeamOK := ps6016Number(fields, func(n string) bool { return ps6055SeamTimeField(n, "control") })
	candidateSeam, candidateSeamOK := ps6016Number(fields, func(n string) bool { return ps6055SeamTimeField(n, "candidate") })
	seamRatio, seamRatioOK := 0.0, controlSeamOK && candidateSeamOK && candidateSeam > 0
	if seamRatioOK {
		seamRatio = controlSeam / candidateSeam
		if recorded, ok := ps6016Number(fields, ps6055SeamRatioField); ok && !ps6025Close(recorded, seamRatio) {
			warnings = append(warnings, fmt.Sprintf("seam control/candidate ratio %.6gx disagrees with %.6gx", recorded, seamRatio))
		}
	}
	allocationsEqual := true
	for _, detail := range []string{"byte", "count"} {
		control, controlOK := ps6016Number(fields, func(n string) bool { return ps6055AllocationField(n, "control", detail) })
		candidate, candidateOK := ps6016Number(fields, func(n string) bool { return ps6055AllocationField(n, "candidate", detail) })
		if controlOK && candidateOK && control != candidate {
			allocationsEqual = false
			warnings = append(warnings, "control/candidate allocation "+detail+"s differ")
		}
	}
	seamComplete, _ := ps6026Bool(fields, ps6055SeamField)
	parity, _ := ps6026Bool(fields, ps6055ParityField)
	boundaryExcluded, _ := ps6026Bool(fields, ps6055BoundaryField)
	iterations, iterationsOK := ps6016Number(fields, ps6055IterationsField)
	labels, labelsOK := ps6016Number(fields, ps6055ProfilerField)
	validSeam := seamComplete && parity && boundaryExcluded && allocationsEqual && iterationsOK && iterations > 0 && labelsOK && labels >= 1 && controlCommandBuffersOK && candidateCommandBuffersOK && controlCommandBuffers == 1 && candidateCommandBuffers == 1
	minimum, minimumOK := ps6016Number(fields, ps6055MinimumField)
	performancePassed := validSeam && seamRatioOK && minimumOK && seamRatio >= minimum
	if verdict, ok := ps6027String(fields, ps6055VerdictField); ok {
		normalized := ps6030StatusName(verdict)
		claimsPass := ps6007ContainsAny(normalized, "pass", "accept", "promote") && !ps6007ContainsAny(normalized, "fail", "reject")
		claimsFail := ps6007ContainsAny(normalized, "fail", "reject", "belowthreshold")
		if performancePassed && !claimsPass || !performancePassed && !claimsFail {
			warnings = append(warnings, fmt.Sprintf("performance verdict %q disagrees with computed seam gate", verdict))
		}
	}
	if ranked, ok := ps6026Bool(fields, ps6055RankedField); ok && ranked && (!eligible || !performancePassed) {
		warnings = append(warnings, "fusion candidate is ranked from encoder count despite absent priced leverage or a failed complete-seam performance gate")
	}
	if decision, ok := ps6027String(fields, ps6055DecisionField); ok && (!eligible || !performancePassed) && ps6007ContainsAny(ps6030StatusName(decision), "retain", "promote", "ship", "selected") {
		warnings = append(warnings, fmt.Sprintf("final decision %q retains an encoder-count candidate without priced leverage and a passing seam", decision))
	}
	return warnings
}
