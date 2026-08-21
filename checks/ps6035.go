package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6035 implements owner issue #743: persistent accelerator encoder
// topologies must pass a cheap exact-parent kill screen before consuming a
// full campaign or profiler budget.
var PS6035 = register(&lint.Check{
	ID:       "PS6035",
	Category: "verify",
	Slug:     "persistent-encoder-needs-parent-kill-screen",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a persistent Metal encoder topology advances without an exact-parent kill screen",
		Text: `A persistent compute encoder can look attractive in synthetic
dispatch-depth benchmarks and stage-attributed counters while remaining
noise-equivalent in a production decoder. Cleaner internal topology is
permission to revalidate, not proof of end-to-end leverage.

This check implements owner issue #743. It audits PersistentEncoderEvidence,
EncoderTopologyParentGate, MetalEncoderKillScreen, ParentTopologyScreen, or
equivalent manifests. Evidence must record:

  - hardware, candidate topology, synthetic depth ratio, and exact parent
    workload;
  - profiler attribution as revalidation-only evidence;
  - predeclared screen-pair count, control/candidate parent time vectors,
    arithmetic-mean ratio, and frozen kill gate;
  - alternating fresh-process order;
  - exact digest, Compute-to-Blit-to-Compute, Compute-to-MPS-to-Compute, and
    buffer-barrier statuses before timing;
  - noise-equivalent classification near 1.00x;
  - whether the full ten-pair campaign or counter capture was launched;
  - whether the complete candidate runtime/API path was deleted; and
  - final retain/remove decision.

Constant evidence is checked for vector/pair-count mismatch, stale arithmetic-
mean ratios, failed semantic/topology/barrier prerequisites, and a candidate
that misses the kill gate yet is retained, sent to expensive measurement, or
left installed. There is NO automatic fix: deleting a backend API/runtime path
and proving mixed encoder boundaries require project-specific review.`,
		Before: `if syntheticDepthRatio > 1.03 {
	runTenPairAndProfilerCampaign(persistentEncoder)
}`,
		After: `evidence := EncoderTopologyParentGate{
	ParentControlTimesNS: []float64{7_364_441_583, 7_355_263_625},
	ParentCandidateTimesNS: []float64{7_347_287_458, 7_350_303_000},
	ArithmeticMeanRatio: 1.0015,
	ParentKillGate: 1.10,
	NoiseEquivalentClassified: true,
	FullCampaignLaunched: false, CounterCaptureLaunched: false,
	CandidatePathDeleted: true, FinalDecision: "removed",
}`,
		MeasuredWin: `In issue #743, two alternating Apple-M2 TinyLlama parent
pairs produced a 1.0015x arithmetic-mean candidate ratio, far below the
predeclared 1.10x gate, even though exact digest and mixed Compute/Blit/MPS
topology tests passed. The complete candidate path was removed before a ten-
pair campaign or counter capture. A synthetic 1.037x proxy was not promoted.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6035",
		Doc:  "persistent encoder topology lacks an exact production-parent kill screen",
		Run:  runPS6035,
	},
})

type ps6035Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6035Axes = []ps6035Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035HardwareField) }},
	{name: "candidate topology", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035TopologyField) }},
	{name: "synthetic depth ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035SyntheticField) }},
	{name: "production parent workload", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035ParentWorkloadField) }},
	{name: "profiler attribution scope", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035ProfilerScopeField) }},
	{name: "parent screen-pair count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035PairCountField) }},
	{name: "parent control times", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035ControlTimesField) }},
	{name: "parent candidate times", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035CandidateTimesField) }},
	{name: "arithmetic-mean ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035MeanRatioField) }},
	{name: "parent kill gate", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035KillGateField) }},
	{name: "alternating fresh-process order", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035AlternatingField) }},
	{name: "exact-digest status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035DigestField) }},
	{name: "Compute-Blit-Compute status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035BlitField) }},
	{name: "Compute-MPS-Compute status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035MPSField) }},
	{name: "buffer-barrier status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035BarrierField) }},
	{name: "noise-equivalent classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035NoiseField) }},
	{name: "full-campaign launch status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035CampaignField) }},
	{name: "counter-capture launch status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035CounterField) }},
	{name: "candidate-path deletion status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035DeletedField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6035DecisionField) }},
}

type ps6035Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6035(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6035Context(text) {
				continue
			}
			manifest, found := ps6035BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "persistent encoder campaign has no exact-parent kill-screen manifest; missing %s", strings.Join(ps6035Missing(nil), ", "))
				continue
			}
			if missing := ps6035Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "persistent encoder kill-screen evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6035Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "persistent encoder kill-screen audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6035Context(text string) bool {
	text = ps6007NormalizeName(text)
	metal := ps6007ContainsAny(text, "metal", "gpu")
	persistent := strings.Contains(text, "persistent") && strings.Contains(text, "encoder")
	parent := ps6007ContainsAny(text, "parentkillscreen", "exactparent", "parentgate", "topologyscreen")
	return metal && persistent && parent
}

func ps6035BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6035Manifest, bool) {
	var best ps6035Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6035ManifestType(lit.Type) {
			return true
		}
		manifest := ps6035Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6035Axes) - len(ps6035Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6035ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6035ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6035ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "persistentencoderevidence", "encodertopologyparentgate", "metalencoderkillscreen", "parenttopologyscreen", "persistentencoderparent")
}

func ps6035Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6035Axes))
	for _, axis := range ps6035Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6035HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "deviceidentity", "gpuid")
}
func ps6035TopologyField(name string) bool {
	return strings.Contains(name, "candidate") && strings.Contains(name, "topology")
}
func ps6035SyntheticField(name string) bool {
	return strings.Contains(name, "synthetic") && strings.Contains(name, "depth") && ps6007ContainsAny(name, "ratio", "speedup")
}
func ps6035ParentWorkloadField(name string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, "workload")
}
func ps6035ProfilerScopeField(name string) bool {
	return strings.Contains(name, "profiler") && strings.Contains(name, "attribution") && ps6007ContainsAny(name, "scope", "classification")
}
func ps6035PairCountField(name string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, "screen") && strings.Contains(name, "pair")
}
func ps6035ControlTimesField(name string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, "control") && strings.Contains(name, "times")
}
func ps6035CandidateTimesField(name string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, "candidate") && strings.Contains(name, "times")
}
func ps6035MeanRatioField(name string) bool {
	return strings.Contains(name, "arithmeticmean") && ps6007ContainsAny(name, "ratio", "speedup")
}
func ps6035KillGateField(name string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, "kill") && strings.Contains(name, "gate")
}
func ps6035AlternatingField(name string) bool {
	return strings.Contains(name, "alternating") && strings.Contains(name, "freshprocess")
}
func ps6035DigestField(name string) bool {
	return strings.Contains(name, "exact") && strings.Contains(name, "digest") && ps6007ContainsAny(name, "passed", "status", "matched")
}
func ps6035BlitField(name string) bool {
	return strings.Contains(name, "compute") && strings.Contains(name, "blit") && ps6007ContainsAny(name, "passed", "status", "tested")
}
func ps6035MPSField(name string) bool {
	return strings.Contains(name, "compute") && strings.Contains(name, "mps") && ps6007ContainsAny(name, "passed", "status", "tested")
}
func ps6035BarrierField(name string) bool {
	return strings.Contains(name, "buffer") && strings.Contains(name, "barrier") && ps6007ContainsAny(name, "passed", "inserted", "status")
}
func ps6035NoiseField(name string) bool {
	return strings.Contains(name, "noiseequivalent") && ps6007ContainsAny(name, "classified", "status")
}
func ps6035CampaignField(name string) bool {
	return strings.Contains(name, "fullcampaign") && strings.Contains(name, "launched")
}
func ps6035CounterField(name string) bool {
	return strings.Contains(name, "countercapture") && strings.Contains(name, "launched")
}
func ps6035DeletedField(name string) bool {
	return strings.Contains(name, "candidatepath") && strings.Contains(name, "deleted")
}
func ps6035DecisionField(name string) bool {
	return strings.Contains(name, "final") && strings.Contains(name, "decision")
}

func ps6035Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 10)
	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"alternating fresh-process order", ps6035AlternatingField},
		{"exact digest", ps6035DigestField},
		{"Compute-Blit-Compute topology", ps6035BlitField},
		{"Compute-MPS-Compute topology", ps6035MPSField},
		{"buffer barriers", ps6035BarrierField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	control, controlOK := ps6016Numbers(fields, ps6035ControlTimesField)
	candidate, candidateOK := ps6016Numbers(fields, ps6035CandidateTimesField)
	pairs, pairsOK := ps6016Number(fields, ps6035PairCountField)
	if controlOK && candidateOK && (len(control) != len(candidate) || pairsOK && float64(len(control)) != pairs) {
		warnings = append(warnings, "control/candidate time vectors disagree with each other or the parent screen-pair count")
	}
	recorded, recordedOK := ps6016Number(fields, ps6035MeanRatioField)
	calculated, calculatedOK := ps6035MeanRatio(control, candidate)
	if recordedOK && calculatedOK && !ps6025Close(recorded, calculated) {
		warnings = append(warnings, fmt.Sprintf("arithmetic-mean ratio %.6gx disagrees with control/candidate means %.6gx", recorded, calculated))
	}
	gate, gateOK := ps6016Number(fields, ps6035KillGateField)
	misses := gateOK && calculatedOK && calculated < gate
	if misses {
		if noise, known := ps6026Bool(fields, ps6035NoiseField); known && !noise {
			warnings = append(warnings, fmt.Sprintf("%.6gx parent result misses %.6gx gate but is not classified noise-equivalent", calculated, gate))
		}
		if launched, known := ps6026Bool(fields, ps6035CampaignField); known && launched {
			warnings = append(warnings, "full campaign is launched after the parent kill screen fails")
		}
		if launched, known := ps6026Bool(fields, ps6035CounterField); known && launched {
			warnings = append(warnings, "counter capture is launched after the parent kill screen fails")
		}
		if deleted, known := ps6026Bool(fields, ps6035DeletedField); known && !deleted {
			warnings = append(warnings, "candidate runtime/API path remains installed after the parent kill screen fails")
		}
		if decision, ok := ps6027String(fields, ps6035DecisionField); ok && ps6007ContainsAny(ps6030StatusName(decision), "retain", "promote", "ship", "selected") {
			warnings = append(warnings, fmt.Sprintf("final decision %q retains a candidate below the parent kill gate", decision))
		}
	}
	if scope, ok := ps6027String(fields, ps6035ProfilerScopeField); ok && !ps6007ContainsAny(ps6030StatusName(scope), "revalidationonly", "permissiontorevalidate", "diagnosticonly") {
		warnings = append(warnings, fmt.Sprintf("profiler attribution scope %q is not revalidation-only", scope))
	}
	return warnings
}

func ps6035MeanRatio(control, candidate []float64) (float64, bool) {
	if len(control) == 0 || len(control) != len(candidate) {
		return 0, false
	}
	var controlSum, candidateSum float64
	for i := range control {
		controlSum += control[i]
		candidateSum += candidate[i]
	}
	if controlSum <= 0 || candidateSum <= 0 {
		return 0, false
	}
	return controlSum / candidateSum, true
}
