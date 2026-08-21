package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6026 implements owner issue #752: GPU evidence must distinguish allocated
// buffer capacity, logical active work, and per-arm dispatched work.
var PS6026 = register(&lint.Check{
	ID:       "PS6026",
	Category: "verify",
	Slug:     "gpu-dispatch-amplifies-inactive-buffer-capacity",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a GPU comparison attributes removal of inactive-capacity traversal to fusion",
		Text: `Accelerator scratch buffers are often sized for a maximum
batch or context while one invocation operates on a smaller logical shape.
Deriving a grid from buffer capacity can traverse inactive storage without
changing outputs or allocation metrics. A candidate that dispatches only the
logical extent can then look like a kernel breakthrough even when its real
change is work correction.

This check implements owner issue #752. It audits ActiveExtentEvidence,
DispatchExtentEvidence, BufferWorkCoverage, GPUWorkExtentManifest, or
equivalent benchmark/profiler manifests. Evidence must retain:

  - hardware, workload, and a common extent unit;
  - allocated buffer capacity and logical active extent;
  - incumbent/control, candidate, and corrected-unfused dispatched extents;
  - dispatch-amplification ratio;
  - operation-label-to-shape bindings;
  - exact-shape status and an explicit padding justification status;
  - JSON preservation of capacity and active-shape metadata;
  - corrected-unfused and candidate timings plus their comparison ratio; and
  - exactness/parity status.

Constant evidence is checked for impossible extents, amplification-ratio
consistency, over-dispatch in an exact-shape campaign without justified
padding, changed work coverage between comparison arms, and failure of the
corrected-unfused control to execute the candidate's dispatched extent. If the
candidate is slower than the corrected control, PS6026 reports that directly
and blocks attribution to fusion. Optional per-arm logical extents are also
compared when present.

There is NO automatic fix. The logical shape, padding semantics, dispatch
geometry, JSON evidence, and corrected control are runtime/domain facts that
source rewriting cannot infer safely.`,
		Before: `evidence := FusionEvidence{
	AllocatedElements: len(maxScratch),
	ControlNS: oldParent,
	CandidateNS: fusedParent,
	ExactParity: true,
}
// Candidate appears 2.838x faster, allocations unchanged.`,
		After: `evidence := ActiveExtentEvidence{
	Hardware: "Apple M2 Pro", Workload: "TinyLlama rows=1 decode",
	ExtentUnit: "elements",
	AllocatedElements: maxLen * hidden,
	LogicalActiveElements: hidden,
	ControlDispatchedElements: maxLen * hidden,
	CandidateDispatchedElements: hidden,
	CorrectedUnfusedDispatchedElements: hidden,
	DispatchAmplificationRatio: float64(maxLen),
	OperationShapeLabels: labelsWithShape,
	ExactShapeCampaign: true, PaddingJustified: false,
	JSONPreservesBufferCapacity: true, JSONPreservesActiveShape: true,
	CorrectedUnfusedNS: corrected, CandidateNS: fused,
	CorrectedControlCandidateRatio: corrected / fused,
	ExactParityPassed: true,
}`,
		MeasuredWin: `The original Apple-M2-Pro gate/up/SwiGLU campaign
reported an exact 2.837988x TinyLlama decode win with unchanged allocations.
The incumbent dispatched rows=1 work across maxLen*hidden scratch capacity.
After BinaryN corrected the unfused parent to dispatch only hidden elements,
200-token timing was 1,190,455,583 ns corrected control versus 1,208,686,833 ns
fusion candidate: 0.984916x control/candidate, so the candidate was slower and
removed. The original gain was inactive-capacity work removal, not a fusion
speedup.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6026",
		Doc:  "GPU evidence confuses allocated capacity, active extent, and dispatched work",
		Run:  runPS6026,
	},
})

type ps6026Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6026Axes = []ps6026Axis{
	{name: "hardware", present: func(f map[string]ps6016Field) bool { return ps6025Has(f, "hardware", "device") }},
	{name: "workload", present: func(f map[string]ps6016Field) bool { return ps6025Has(f, "workload", "model", "shape", "campaign") }},
	{name: "common extent unit", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6026UnitField) }},
	{name: "allocated buffer capacity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6026AllocatedField) }},
	{name: "logical active extent", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6026ActiveField) }},
	{name: "control dispatched extent", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6026ControlDispatchField) }},
	{name: "candidate dispatched extent", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6026CandidateDispatchField) }},
	{name: "corrected-unfused dispatched extent", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6026CorrectedDispatchField) }},
	{name: "dispatch-amplification ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6026AmplificationField) }},
	{name: "operation-label shape binding", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6026LabelShapeField) }},
	{name: "exact-shape campaign status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6026ExactShapeField) }},
	{name: "padding justification status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6026PaddingField) }},
	{name: "JSON buffer-capacity preservation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6026JSONCapacityField) }},
	{name: "JSON active-shape preservation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6026JSONActiveField) }},
	{name: "corrected-unfused control timing", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6026CorrectedTimeField) }},
	{name: "candidate timing", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6026CandidateTimeField) }},
	{name: "corrected-control/candidate ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6026ComparisonField) }},
	{name: "exactness/parity status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6016ExactnessField) }},
}

type ps6026Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6026(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6011Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6026Context(text) {
				continue
			}
			manifest, found := ps6026BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "GPU active-extent harness has no dispatch-work manifest; missing %s", strings.Join(ps6026Missing(nil), ", "))
				continue
			}
			if missing := ps6026Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU dispatch-work evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6026Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU active-extent audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6026Context(text string) bool {
	text = ps6007NormalizeName(text)
	accelerator := ps6007ContainsAny(text, "gpu", "metal", "mps", "cuda", "vulkan", "accelerator", "device")
	dispatch := ps6007ContainsAny(text, "dispatch", "grid", "thread")
	extent := ps6007ContainsAny(text, "activeextent", "logicalextent", "buffercapacity", "allocatedcapacity", "workcoverage", "amplification")
	comparison := ps6007ContainsAny(text, "candidate", "control", "fusion", "beforeafter", "correctedunfused")
	return accelerator && dispatch && extent && comparison
}

func ps6026BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6026Manifest, bool) {
	var best ps6026Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6026ManifestType(lit.Type) {
			return true
		}
		manifest := ps6026Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6026Axes) - len(ps6026Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6026ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6026ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6026ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "activeextentevidence", "dispatchextentevidence", "bufferworkcoverage", "gpuworkextent", "dispatchamplification", "logicalworkevidence")
}

func ps6026Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6026Axes))
	for _, axis := range ps6026Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6026UnitField(name string) bool {
	return strings.Contains(name, "unit") && ps6007ContainsAny(name, "extent", "dispatch", "element", "byte", "work")
}

func ps6026AllocatedField(name string) bool {
	return ps6007ContainsAny(name, "allocated", "capacity") && ps6007ContainsAny(name, "element", "byte", "extent") &&
		!ps6007ContainsAny(name, "json", "preserve", "control", "candidate", "corrected")
}

func ps6026ActiveField(name string) bool {
	return ps6007ContainsAny(name, "logicalactive", "activeextent", "logicalextent", "activeelement", "activebyte") &&
		!ps6007ContainsAny(name, "json", "preserve", "control", "candidate", "corrected")
}

func ps6026DispatchExtent(name string) bool {
	return ps6007ContainsAny(name, "dispatch", "thread", "grid") && ps6007ContainsAny(name, "element", "byte", "extent", "count")
}

func ps6026ControlDispatchField(name string) bool {
	return ps6026DispatchExtent(name) && ps6007ContainsAny(name, "control", "incumbent", "before") && !strings.Contains(name, "corrected")
}

func ps6026CandidateDispatchField(name string) bool {
	return ps6026DispatchExtent(name) && ps6007ContainsAny(name, "candidate", "fusion", "after")
}

func ps6026CorrectedDispatchField(name string) bool {
	return ps6026DispatchExtent(name) && strings.Contains(name, "corrected") && ps6007ContainsAny(name, "unfused", "control", "parent")
}

func ps6026AmplificationField(name string) bool {
	return strings.Contains(name, "amplification") && ps6007ContainsAny(name, "dispatch", "work", "ratio")
}

func ps6026LabelShapeField(name string) bool {
	return ps6007ContainsAny(name, "operationshape", "stageshape", "labelshape", "shapelabel") &&
		ps6007ContainsAny(name, "binding", "metadata", "label", "map")
}

func ps6026ExactShapeField(name string) bool {
	return strings.Contains(name, "exactshape") && ps6007ContainsAny(name, "campaign", "passed", "status", "gate")
}

func ps6026PaddingField(name string) bool {
	return strings.Contains(name, "padding") && ps6007ContainsAny(name, "justified", "required", "status", "reason")
}

func ps6026JSONCapacityField(name string) bool {
	return strings.Contains(name, "json") && ps6007ContainsAny(name, "capacity", "allocated") && ps6007ContainsAny(name, "preserve", "include", "emit")
}

func ps6026JSONActiveField(name string) bool {
	return strings.Contains(name, "json") && ps6007ContainsAny(name, "active", "logical", "shape") && ps6007ContainsAny(name, "preserve", "include", "emit")
}

func ps6026CorrectedTimeField(name string) bool {
	return strings.Contains(name, "corrected") && ps6007ContainsAny(name, "unfused", "control", "parent") &&
		ps6007ContainsAny(name, "time", "duration", "ns", "median")
}

func ps6026CandidateTimeField(name string) bool {
	return ps6007ContainsAny(name, "candidate", "fusion", "after") && ps6007ContainsAny(name, "time", "duration", "ns", "median") &&
		!ps6007ContainsAny(name, "ratio", "speedup")
}

func ps6026ComparisonField(name string) bool {
	return strings.Contains(name, "corrected") && ps6007ContainsAny(name, "candidate", "fusion") &&
		ps6007ContainsAny(name, "ratio", "speedup", "comparison")
}

func ps6026ControlActiveField(name string) bool {
	return ps6007ContainsAny(name, "control", "incumbent", "before") && ps6007ContainsAny(name, "logicalactive", "activeextent", "logicalextent")
}

func ps6026CandidateActiveField(name string) bool {
	return ps6007ContainsAny(name, "candidate", "fusion", "after") && ps6007ContainsAny(name, "logicalactive", "activeextent", "logicalextent")
}

func ps6026Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 8)
	allocated, allocatedOK := ps6016Number(fields, ps6026AllocatedField)
	active, activeOK := ps6016Number(fields, ps6026ActiveField)
	controlDispatch, controlDispatchOK := ps6016Number(fields, ps6026ControlDispatchField)
	candidateDispatch, candidateDispatchOK := ps6016Number(fields, ps6026CandidateDispatchField)
	correctedDispatch, correctedDispatchOK := ps6016Number(fields, ps6026CorrectedDispatchField)
	amplification, amplificationOK := ps6016Number(fields, ps6026AmplificationField)
	correctedTime, correctedTimeOK := ps6016Number(fields, ps6026CorrectedTimeField)
	candidateTime, candidateTimeOK := ps6016Number(fields, ps6026CandidateTimeField)
	comparison, comparisonOK := ps6016Number(fields, ps6026ComparisonField)
	controlActive, controlActiveOK := ps6016Number(fields, ps6026ControlActiveField)
	candidateActive, candidateActiveOK := ps6016Number(fields, ps6026CandidateActiveField)

	if allocatedOK && allocated <= 0 {
		warnings = append(warnings, "allocated buffer capacity is not positive")
	}
	if activeOK && active <= 0 {
		warnings = append(warnings, "logical active extent is not positive")
	}
	if allocatedOK && activeOK && allocated < active {
		warnings = append(warnings, fmt.Sprintf("allocated capacity %.4g is smaller than logical active extent %.4g", allocated, active))
	}
	if controlDispatchOK && activeOK && active > 0 && amplificationOK {
		calculated := controlDispatch / active
		if !ps6025Close(amplification, calculated) {
			warnings = append(warnings, fmt.Sprintf("recorded dispatch amplification %.4gx disagrees with control-dispatched/active ratio %.4gx", amplification, calculated))
		}
	}

	exactShape, exactKnown := ps6026Bool(fields, ps6026ExactShapeField)
	padding, paddingKnown := ps6026Bool(fields, ps6026PaddingField)
	if exactKnown && exactShape && paddingKnown && !padding && activeOK && active > 0 {
		for _, arm := range []struct {
			name  string
			value float64
			ok    bool
		}{
			{"control", controlDispatch, controlDispatchOK},
			{"candidate", candidateDispatch, candidateDispatchOK},
			{"corrected-unfused control", correctedDispatch, correctedDispatchOK},
		} {
			if arm.ok && arm.value > active {
				warnings = append(warnings, fmt.Sprintf("%s dispatch %.4g exceeds exact logical extent %.4g without justified padding", arm.name, arm.value, active))
			}
		}
	}
	if controlDispatchOK && candidateDispatchOK && controlDispatch != candidateDispatch {
		warnings = append(warnings, fmt.Sprintf("before/after arms execute different dispatched work (control %.4g, candidate %.4g); attribute no fusion win until using the corrected-unfused control", controlDispatch, candidateDispatch))
	}
	if correctedDispatchOK && candidateDispatchOK && correctedDispatch != candidateDispatch {
		warnings = append(warnings, fmt.Sprintf("corrected-unfused/candidate dispatched extents still differ (%.4g vs %.4g)", correctedDispatch, candidateDispatch))
	}
	if controlActiveOK && candidateActiveOK && controlActive != candidateActive {
		warnings = append(warnings, fmt.Sprintf("control/candidate logical active extents differ (%.4g vs %.4g)", controlActive, candidateActive))
	}
	if correctedTimeOK && candidateTimeOK && correctedTime > 0 && candidateTime > 0 {
		calculated := correctedTime / candidateTime
		if comparisonOK && !ps6025Close(comparison, calculated) {
			warnings = append(warnings, fmt.Sprintf("recorded corrected-control/candidate ratio %.6gx disagrees with timing ratio %.6gx", comparison, calculated))
		}
		if calculated < 1 {
			warnings = append(warnings, fmt.Sprintf("fusion candidate is slower than corrected same-work control (control/candidate %.6gx); block fusion attribution", calculated))
		}
	}
	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"JSON buffer-capacity preservation", ps6026JSONCapacityField},
		{"JSON active-shape preservation", ps6026JSONActiveField},
		{"exactness/parity status", ps6016ExactnessField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	return warnings
}

func ps6026Bool(fields map[string]ps6016Field, predicate func(string) bool) (bool, bool) {
	value, found := false, false
	for name, field := range fields {
		if !predicate(name) || !field.hasBool {
			continue
		}
		if found && value != field.boolVal {
			return false, false
		}
		value, found = field.boolVal, true
	}
	return value, found
}
