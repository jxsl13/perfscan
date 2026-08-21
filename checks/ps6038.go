package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6038 implements owner issue #740: a large host recorder leaf win must be
// priced against the exact parent wall boundary before promotion.
var PS6038 = register(&lint.Check{
	ID:       "PS6038",
	Category: "verify",
	Slug:     "host-recorder-win-needs-parent-leverage",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a host-only Metal recorder win has immaterial parent leverage",
		Text: `A command-buffer ownership or argument-binding change can remove
most host encoding time from a synthetic recorder benchmark yet leave a GPU-
duration or memory-bandwidth dominated application unchanged.

This check implements owner issue #740. It audits RecorderLeverageEvidence,
HostOnlyMetalEvidence, RecorderParentGate, CommandOwnershipEvidence, or
equivalent manifests. Evidence must record:

  - hardware, workload shape, warm/cold state, and control/candidate command-
    buffer ownership modes;
  - leaf workload, encoders per command, control/candidate host encode time,
    saved encode time, and leaf speedup;
  - control/candidate GPU command duration, synchronization time, and total
    wall time;
  - exact parent workload, control/candidate wall time and throughput ratio;
  - saved-leaf-time/parent-wall leverage fraction, materiality threshold, and
    parent promotion gate;
  - exact-output digest, control/candidate allocation bytes, mixed compute/
    blit lifetime, profiling-recorder compatibility, and parent-gate status;
  - host-only/application-leverage classification and final decision.

Constant evidence is checked by recomputing saved time, leaf speedup, parent
ratio, and leverage fraction. A large leaf whose maximum contribution is below
materiality must be classified host-only; a parent below its gate must not be
retained. There is NO automatic fix because ownership/lifetime and application
critical-path behavior are backend/runtime facts.`,
		Before: `BenchmarkRecorderConstruction(b) // 5.171x, promoted directly`,
		After: `evidence := RecorderLeverageEvidence{
	LeafControlEncodeNS: 106829, LeafCandidateEncodeNS: 20656,
	LeafSavedEncodeNS: 86173, LeafSpeedup: 5.171,
	ParentControlWallNS: 7_336_289_041,
	ParentCandidateWallNS: 7_352_295_125,
	ParentThroughputRatio: 0.997823,
	LeverageFraction: 86173.0 / 7_336_289_041,
	Classification: "host-only", FinalDecision: "removed",
}`,
		MeasuredWin: `Issue #740 reduced host construction of a 32-encoder
command buffer from 106,829 to 20,656 ns (5.171x), with unchanged 8 B/op and
one allocation. The 200-token parent measured 7,336,289,041 ns control versus
7,352,295,125 ns candidate, only 0.997823x throughput, so the host-only
candidate did not improve the application.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6038",
		Doc:  "host recorder leaf win is promoted without material exact-parent leverage",
		Run:  runPS6038,
	},
})

type ps6038Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6038Axes = []ps6038Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038HardwareField) }},
	{name: "workload shape", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038ShapeField) }},
	{name: "warm/cold state", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038StateField) }},
	{name: "control ownership mode", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6038OwnershipField(n, "control") })
	}},
	{name: "candidate ownership mode", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6038OwnershipField(n, "candidate") })
	}},
	{name: "leaf workload", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038LeafWorkloadField) }},
	{name: "encoders per command", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038EncodersField) }},
	{name: "leaf control encode time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6038LeafField(n, "control", "encode") })
	}},
	{name: "leaf candidate encode time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6038LeafField(n, "candidate", "encode") })
	}},
	{name: "leaf saved encode time", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038SavedField) }},
	{name: "leaf speedup", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038LeafSpeedupField) }},
	{name: "control GPU duration", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6038BoundaryField(n, "control", "gpu") })
	}},
	{name: "candidate GPU duration", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6038BoundaryField(n, "candidate", "gpu") })
	}},
	{name: "control synchronization time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6038BoundaryField(n, "control", "synchron") })
	}},
	{name: "candidate synchronization time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6038BoundaryField(n, "candidate", "synchron") })
	}},
	{name: "control total wall time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6038BoundaryField(n, "control", "totalwall") })
	}},
	{name: "candidate total wall time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6038BoundaryField(n, "candidate", "totalwall") })
	}},
	{name: "parent workload", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038ParentWorkloadField) }},
	{name: "parent control wall time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6038ParentTimeField(n, "control") })
	}},
	{name: "parent candidate wall time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6038ParentTimeField(n, "candidate") })
	}},
	{name: "parent throughput ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038ParentRatioField) }},
	{name: "leverage fraction", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038LeverageField) }},
	{name: "materiality threshold", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038MaterialityField) }},
	{name: "parent promotion gate", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038PromotionField) }},
	{name: "exact-output digest status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038DigestField) }},
	{name: "control allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6038AllocationField(n, "control") })
	}},
	{name: "candidate allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6038AllocationField(n, "candidate") })
	}},
	{name: "mixed compute/blit lifetime status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038LifetimeField) }},
	{name: "profiling-recorder compatibility", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038ProfilerField) }},
	{name: "parent-gate status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038ParentGateField) }},
	{name: "classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038ClassificationField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6038DecisionField) }},
}

type ps6038Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6038(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6038Context(text) {
				continue
			}
			manifest, found := ps6038BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "host-recorder/parent campaign has no leverage manifest; missing %s", strings.Join(ps6038Missing(nil), ", "))
				continue
			}
			if missing := ps6038Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "host-recorder leverage evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6038Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "host-recorder leverage audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6038Context(text string) bool {
	text = ps6007NormalizeName(text)
	metal := ps6007ContainsAny(text, "metal", "gpu")
	recorder := ps6007ContainsAny(text, "recorder", "commandbuffer", "commandownership")
	leverage := ps6007ContainsAny(text, "hostonly", "parentleverage", "recorderparent", "materiality")
	return metal && recorder && leverage
}

func ps6038BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6038Manifest, bool) {
	var best ps6038Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6038ManifestType(lit.Type) {
			return true
		}
		manifest := ps6038Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6038Axes) - len(ps6038Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6038ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6038ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6038ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "recorderleverageevidence", "hostonlymetalevidence", "recorderparentgate", "commandownershipevidence", "hostrecordercampaign")
}

func ps6038Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6038Axes))
	for _, axis := range ps6038Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6038HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "deviceidentity", "gpuid")
}
func ps6038ShapeField(name string) bool {
	return strings.Contains(name, "workload") && strings.Contains(name, "shape")
}
func ps6038StateField(name string) bool {
	return ps6007ContainsAny(name, "warmcoldstate", "powerstate", "thermalstate")
}
func ps6038OwnershipField(name, side string) bool {
	return strings.Contains(name, side) && strings.Contains(name, "commandbuffer") && strings.Contains(name, "ownership")
}
func ps6038LeafWorkloadField(name string) bool {
	return strings.Contains(name, "leaf") && strings.Contains(name, "workload")
}
func ps6038EncodersField(name string) bool {
	return strings.Contains(name, "encoder") && strings.Contains(name, "percommand")
}
func ps6038LeafField(name, side, component string) bool {
	return strings.Contains(name, "leaf") && strings.Contains(name, side) && strings.Contains(name, component) && ps6007ContainsAny(name, "ns", "time")
}
func ps6038SavedField(name string) bool {
	return strings.Contains(name, "leaf") && strings.Contains(name, "saved") && strings.Contains(name, "encode")
}
func ps6038LeafSpeedupField(name string) bool {
	return strings.Contains(name, "leaf") && strings.Contains(name, "speedup")
}
func ps6038BoundaryField(name, side, boundary string) bool {
	return strings.Contains(name, side) && strings.Contains(name, boundary) && ps6007ContainsAny(name, "ns", "time", "duration") && !strings.Contains(name, "parent")
}
func ps6038ParentWorkloadField(name string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, "workload")
}
func ps6038ParentTimeField(name, side string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, side) && strings.Contains(name, "wall") && ps6007ContainsAny(name, "ns", "time")
}
func ps6038ParentRatioField(name string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, "throughput") && ps6007ContainsAny(name, "ratio", "speedup")
}
func ps6038LeverageField(name string) bool {
	return strings.Contains(name, "leverage") && ps6007ContainsAny(name, "fraction", "ratio")
}
func ps6038MaterialityField(name string) bool {
	return strings.Contains(name, "materiality") && ps6007ContainsAny(name, "threshold", "minimum")
}
func ps6038PromotionField(name string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, "promotion") && strings.Contains(name, "gate")
}
func ps6038DigestField(name string) bool {
	return strings.Contains(name, "exactoutput") && strings.Contains(name, "digest")
}
func ps6038AllocationField(name, side string) bool {
	return strings.Contains(name, side) && strings.Contains(name, "allocation") && strings.Contains(name, "byte")
}
func ps6038LifetimeField(name string) bool {
	return strings.Contains(name, "mixed") && strings.Contains(name, "compute") && strings.Contains(name, "blit") && strings.Contains(name, "lifetime")
}
func ps6038ProfilerField(name string) bool {
	return strings.Contains(name, "profilingrecorder") && strings.Contains(name, "compatib")
}
func ps6038ParentGateField(name string) bool {
	return strings.Contains(name, "parentgate") && ps6007ContainsAny(name, "run", "passed", "status")
}
func ps6038ClassificationField(name string) bool {
	return ps6007ContainsAny(name, "classification", "evidenceclass")
}
func ps6038DecisionField(name string) bool {
	return strings.Contains(name, "final") && strings.Contains(name, "decision")
}

func ps6038Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 10)
	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"exact-output digest", ps6038DigestField},
		{"mixed compute/blit lifetime", ps6038LifetimeField},
		{"profiling-recorder compatibility", ps6038ProfilerField},
		{"parent-gate execution", ps6038ParentGateField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	controlLeaf, controlLeafOK := ps6016Number(fields, func(n string) bool { return ps6038LeafField(n, "control", "encode") })
	candidateLeaf, candidateLeafOK := ps6016Number(fields, func(n string) bool { return ps6038LeafField(n, "candidate", "encode") })
	saved, savedOK := ps6016Number(fields, ps6038SavedField)
	leafSpeedup, leafSpeedupOK := ps6016Number(fields, ps6038LeafSpeedupField)
	if controlLeafOK && candidateLeafOK {
		if savedOK && !ps6025Close(saved, controlLeaf-candidateLeaf) {
			warnings = append(warnings, fmt.Sprintf("saved leaf encode time %.6g ns disagrees with control-candidate %.6g ns", saved, controlLeaf-candidateLeaf))
		}
		if leafSpeedupOK && candidateLeaf > 0 && !ps6025Close(leafSpeedup, controlLeaf/candidateLeaf) {
			warnings = append(warnings, fmt.Sprintf("leaf speedup %.6gx disagrees with timing ratio %.6gx", leafSpeedup, controlLeaf/candidateLeaf))
		}
	}
	parentControl, parentControlOK := ps6016Number(fields, func(n string) bool { return ps6038ParentTimeField(n, "control") })
	parentCandidate, parentCandidateOK := ps6016Number(fields, func(n string) bool { return ps6038ParentTimeField(n, "candidate") })
	parentRatio, parentRatioOK := ps6016Number(fields, ps6038ParentRatioField)
	if parentControlOK && parentCandidateOK && parentRatioOK && parentCandidate > 0 && !ps6025Close(parentRatio, parentControl/parentCandidate) {
		warnings = append(warnings, fmt.Sprintf("parent throughput ratio %.6gx disagrees with control/candidate wall ratio %.6gx", parentRatio, parentControl/parentCandidate))
	}
	leverage, leverageOK := ps6016Number(fields, ps6038LeverageField)
	if savedOK && parentControlOK && parentControl > 0 && leverageOK && !ps6025Close(leverage, saved/parentControl) {
		warnings = append(warnings, fmt.Sprintf("leverage fraction %.6g disagrees with saved-leaf/parent-wall %.6g", leverage, saved/parentControl))
	}
	materiality, materialityOK := ps6016Number(fields, ps6038MaterialityField)
	promotion, promotionOK := ps6016Number(fields, ps6038PromotionField)
	classification, classificationOK := ps6027String(fields, ps6038ClassificationField)
	decision, decisionOK := ps6027String(fields, ps6038DecisionField)
	immaterial := leverageOK && materialityOK && leverage < materiality
	parentMisses := parentRatioOK && promotionOK && parentRatio < promotion
	if immaterial && classificationOK && !ps6007ContainsAny(ps6030StatusName(classification), "hostonly", "immaterial", "leafonly") {
		warnings = append(warnings, fmt.Sprintf("classification %q is not host-only despite %.6g leverage below %.6g materiality", classification, leverage, materiality))
	}
	if parentMisses && decisionOK && ps6007ContainsAny(ps6030StatusName(decision), "retain", "promote", "ship", "selected") {
		warnings = append(warnings, fmt.Sprintf("final decision %q retains a candidate below %.6gx parent gate", decision, promotion))
	}
	return warnings
}
