package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6041 implements owner issue #737: dispatch-eliminating GPU fusions must
// price parent-kernel inflation against removed launch/dependency cost.
var PS6041 = register(&lint.Check{
	ID:       "PS6041",
	Category: "verify",
	Slug:     "gpu-fusion-needs-parent-kernel-inflation",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a GPU fusion counts removed dispatches but omits parent-kernel inflation",
		Text: `Removing a standalone GPU dispatch and dependency edge is a real
topology improvement, but fused special functions and reads can lengthen a
memory-bound parent kernel by more than the removed launch cost.

This check implements owner issue #737. It audits FusionInflationEvidence,
ParentKernelInflationReport, GPUSeamCostEvidence, DispatchFusionCostGate, or
equivalent manifests. Evidence must record:

  - hardware, seam/workload identity, warm residency, command-buffer count,
    and iteration count;
  - removed dispatch and dependency-edge counts;
  - control/fused parent-kernel time, absolute parent inflation, removed
    launch/dependency cost, and their comparison status;
  - added special-function count and added read bytes;
  - control/candidate end-to-end seam time and their ratio;
  - exact mixed-sign/odd-tail output and repeated-execution status;
  - control/candidate allocation bytes and counts; and
  - claim scope and final decision.

Constant evidence is checked by recomputing parent inflation, seam ratio, and
the inflation-versus-removed-cost status. A candidate whose parent inflation
equals or exceeds removed launch/dependency cost must not be retained or
called an end-to-end improvement merely because dispatches disappeared. There
is NO automatic fix because kernel duration, launch cost, memory traffic, and
GPU scheduling are runtime/backend facts.`,
		Before: `if dispatchesRemoved > 0 {
	retain(fusedKernel)
}`,
		After: `evidence := ParentKernelInflationReport{
	DispatchesRemoved: 1, DependencyEdgesRemoved: 1,
	ParentKernelControlNS: parentControl,
	ParentKernelFusedNS: parentFused,
	ParentKernelInflationNS: parentFused - parentControl,
	RemovedLaunchDependencyCostNS: removedCost,
	ControlSeamTotalNS: 432850, CandidateSeamTotalNS: 437181,
	SeamControlCandidateRatio: 0.9901,
	FinalDecision: "removed",
}`,
		MeasuredWin: `The Apple-M2-Pro Q4_K decode screen behind issue #737
removed one SwiGLU dispatch and one dependency edge, yet regressed from
432,850 ns/op to 437,181 ns/op (about 0.9901x control/candidate). The fused
parent added exp work and a gate read to a memory-bound matvec, so the
candidate was removed despite its genuine topology reduction.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6041",
		Doc:  "dispatch-eliminating GPU fusion omits parent-kernel inflation",
		Run:  runPS6041,
	},
})

type ps6041Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6041Axes = []ps6041Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041HardwareField) }},
	{name: "seam/workload identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041WorkloadField) }},
	{name: "warm residency status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041ResidencyField) }},
	{name: "command-buffer count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041CommandCountField) }},
	{name: "iteration count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041IterationField) }},
	{name: "dispatches removed", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041DispatchRemovedField) }},
	{name: "dependency edges removed", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041EdgeRemovedField) }},
	{name: "control parent-kernel time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6041ParentTimeField(n, "control") })
	}},
	{name: "fused parent-kernel time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6041ParentTimeField(n, "fused") })
	}},
	{name: "parent-kernel inflation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041InflationField) }},
	{name: "removed launch/dependency cost", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041RemovedCostField) }},
	{name: "inflation-below-removed-cost status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041InflationGateField) }},
	{name: "extra special functions", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041SpecialFunctionsField) }},
	{name: "extra read bytes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041ExtraReadsField) }},
	{name: "control seam total", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6041SeamField(n, "control") })
	}},
	{name: "candidate seam total", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6041SeamField(n, "candidate") })
	}},
	{name: "seam ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041SeamRatioField) }},
	{name: "mixed-sign odd-tail exactness", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041ExactField) }},
	{name: "repeated-execution parity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041RepeatedField) }},
	{name: "control allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6041AllocationField(n, "control", "byte") })
	}},
	{name: "candidate allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6041AllocationField(n, "candidate", "byte") })
	}},
	{name: "control allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6041AllocationField(n, "control", "count") })
	}},
	{name: "candidate allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6041AllocationField(n, "candidate", "count") })
	}},
	{name: "claim scope", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041ClaimField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6041DecisionField) }},
}

type ps6041Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6041(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6041Context(text) {
				continue
			}
			manifest, found := ps6041BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "dispatch-eliminating GPU fusion has no parent-inflation manifest; missing %s", strings.Join(ps6041Missing(nil), ", "))
				continue
			}
			if missing := ps6041Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU fusion inflation evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6041Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU fusion inflation audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6041Context(text string) bool {
	text = ps6007NormalizeName(text)
	gpu := ps6007ContainsAny(text, "gpu", "metal", "cuda", "vulkan", "accelerator")
	fusion := ps6007ContainsAny(text, "fusion", "fused")
	inflation := ps6007ContainsAny(text, "parentinflation", "kernelinflation", "seamcost", "dispatchfusioncost")
	return gpu && fusion && inflation
}

func ps6041BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6041Manifest, bool) {
	var best ps6041Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6041ManifestType(lit.Type) {
			return true
		}
		manifest := ps6041Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6041Axes) - len(ps6041Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6041ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6041ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6041ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "fusioninflationevidence", "parentkernelinflationreport", "gpuseamcostevidence", "dispatchfusioncostgate", "kernelinflationevidence")
}

func ps6041Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6041Axes))
	for _, axis := range ps6041Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6041HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "deviceidentity", "gpuid")
}
func ps6041WorkloadField(name string) bool {
	return ps6007ContainsAny(name, "seam", "workload") && ps6007ContainsAny(name, "identity", "shape", "name")
}
func ps6041ResidencyField(name string) bool {
	return strings.Contains(name, "warm") && strings.Contains(name, "resident")
}
func ps6041CommandCountField(name string) bool {
	return strings.Contains(name, "commandbuffer") && strings.Contains(name, "count")
}
func ps6041IterationField(name string) bool {
	return strings.Contains(name, "iteration") && strings.Contains(name, "count")
}
func ps6041DispatchRemovedField(name string) bool {
	return strings.Contains(name, "dispatch") && strings.Contains(name, "removed")
}
func ps6041EdgeRemovedField(name string) bool {
	return strings.Contains(name, "dependencyedge") && strings.Contains(name, "removed")
}
func ps6041ParentTimeField(name, side string) bool {
	return strings.Contains(name, "parentkernel") && strings.Contains(name, side) && ps6007ContainsAny(name, "ns", "time", "duration")
}
func ps6041InflationField(name string) bool {
	return strings.Contains(name, "parentkernel") && strings.Contains(name, "inflation") && !strings.Contains(name, "below")
}
func ps6041RemovedCostField(name string) bool {
	return strings.Contains(name, "removed") && strings.Contains(name, "cost") && ps6007ContainsAny(name, "launch", "dependency")
}
func ps6041InflationGateField(name string) bool {
	return strings.Contains(name, "inflation") && strings.Contains(name, "below") && strings.Contains(name, "removed")
}
func ps6041SpecialFunctionsField(name string) bool {
	return strings.Contains(name, "extra") && strings.Contains(name, "specialfunction")
}
func ps6041ExtraReadsField(name string) bool {
	return strings.Contains(name, "extra") && strings.Contains(name, "read") && strings.Contains(name, "byte")
}
func ps6041SeamField(name, side string) bool {
	return strings.Contains(name, side) && strings.Contains(name, "seam") && strings.Contains(name, "total")
}
func ps6041SeamRatioField(name string) bool {
	return strings.Contains(name, "seam") && strings.Contains(name, "controlcandidate") && ps6007ContainsAny(name, "ratio", "speedup")
}
func ps6041ExactField(name string) bool {
	return strings.Contains(name, "mixedsign") && strings.Contains(name, "oddtail") && ps6007ContainsAny(name, "exact", "parity")
}
func ps6041RepeatedField(name string) bool {
	return strings.Contains(name, "repeated") && strings.Contains(name, "execution") && ps6007ContainsAny(name, "identical", "parity", "exact")
}
func ps6041AllocationField(name, side, detail string) bool {
	return strings.Contains(name, side) && strings.Contains(name, "allocation") && strings.Contains(name, detail)
}
func ps6041ClaimField(name string) bool {
	return strings.Contains(name, "claim") && ps6007ContainsAny(name, "scope", "classification")
}
func ps6041DecisionField(name string) bool {
	return strings.Contains(name, "final") && strings.Contains(name, "decision")
}

func ps6041Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 12)
	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"warm residency", ps6041ResidencyField},
		{"mixed-sign odd-tail exactness", ps6041ExactField},
		{"repeated-execution parity", ps6041RepeatedField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	for _, count := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"command-buffer count", ps6041CommandCountField},
		{"iteration count", ps6041IterationField},
		{"dispatches removed", ps6041DispatchRemovedField},
		{"dependency edges removed", ps6041EdgeRemovedField},
	} {
		if value, ok := ps6016Number(fields, count.predicate); ok && value <= 0 {
			warnings = append(warnings, count.name+" must be positive")
		}
	}
	controlParent, controlParentOK := ps6016Number(fields, func(n string) bool { return ps6041ParentTimeField(n, "control") })
	fusedParent, fusedParentOK := ps6016Number(fields, func(n string) bool { return ps6041ParentTimeField(n, "fused") })
	inflation, inflationOK := ps6016Number(fields, ps6041InflationField)
	removedCost, removedCostOK := ps6016Number(fields, ps6041RemovedCostField)
	calculatedInflation, calculatedInflationOK := 0.0, controlParentOK && fusedParentOK
	if calculatedInflationOK {
		calculatedInflation = fusedParent - controlParent
		if inflationOK && !ps6025Close(inflation, calculatedInflation) {
			warnings = append(warnings, fmt.Sprintf("parent-kernel inflation %.6g ns disagrees with fused-control %.6g ns", inflation, calculatedInflation))
		}
	}
	if calculatedInflationOK && removedCostOK {
		below := calculatedInflation < removedCost
		if recorded, ok := ps6026Bool(fields, ps6041InflationGateField); ok && recorded != below {
			warnings = append(warnings, fmt.Sprintf("inflation-below-removed-cost status is %t but %.6g ns inflation versus %.6g ns removed cost computes %t", recorded, calculatedInflation, removedCost, below))
		}
	}
	controlSeam, controlSeamOK := ps6016Number(fields, func(n string) bool { return ps6041SeamField(n, "control") })
	candidateSeam, candidateSeamOK := ps6016Number(fields, func(n string) bool { return ps6041SeamField(n, "candidate") })
	seamRatio, seamRatioOK := ps6016Number(fields, ps6041SeamRatioField)
	calculatedSeamRatio, calculatedSeamRatioOK := 0.0, controlSeamOK && candidateSeamOK && controlSeam > 0 && candidateSeam > 0
	if calculatedSeamRatioOK {
		calculatedSeamRatio = controlSeam / candidateSeam
		if seamRatioOK && !ps6025Close(seamRatio, calculatedSeamRatio) {
			warnings = append(warnings, fmt.Sprintf("seam ratio %.6gx disagrees with control/candidate %.6gx", seamRatio, calculatedSeamRatio))
		}
	}
	for _, detail := range []string{"byte", "count"} {
		control, controlOK := ps6016Number(fields, func(n string) bool { return ps6041AllocationField(n, "control", detail) })
		candidate, candidateOK := ps6016Number(fields, func(n string) bool { return ps6041AllocationField(n, "candidate", detail) })
		if controlOK && candidateOK && control != candidate {
			warnings = append(warnings, fmt.Sprintf("control/candidate allocation %ss differ (%.6g vs %.6g)", detail, control, candidate))
		}
	}
	inflationDominates := calculatedInflationOK && removedCostOK && calculatedInflation >= removedCost
	seamRegresses := calculatedSeamRatioOK && calculatedSeamRatio < 1
	if inflationDominates || seamRegresses {
		if decision, ok := ps6027String(fields, ps6041DecisionField); ok && ps6007ContainsAny(ps6030StatusName(decision), "retain", "promote", "ship", "selected") {
			warnings = append(warnings, fmt.Sprintf("final decision %q retains a fusion whose parent inflation is not recovered by seam savings", decision))
		}
		if claim, ok := ps6027String(fields, ps6041ClaimField); ok && ps6007ContainsAny(ps6030StatusName(claim), "improvement", "speedup", "win", "faster") {
			warnings = append(warnings, fmt.Sprintf("claim scope %q reports an end-to-end win despite unrecovered parent inflation", claim))
		}
	}
	return warnings
}
