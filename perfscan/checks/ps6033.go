package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6033 implements owner issue #745: command-construction leaf wins need a
// decomposed current-parent gate before they can claim application leverage.
var PS6033 = register(&lint.Check{
	ID:       "PS6033",
	Category: "verify",
	Slug:     "command-recording-leaf-win-needs-parent-leverage",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a cached command-construction leaf win is promoted without parent leverage",
		Text: `Caching a tiny immutable accelerator argument buffer can remove
microseconds of host command construction from every dispatch and substantially
improve a repeated cache-hot leaf. In a dependency-heavy graph, CPU recording
can overlap device execution and disappear from the application critical path.

This check implements owner issue #745. It audits CommandConstructionEvidence,
CachedArgumentBufferEvidence, HostBoundLeafGate, RecordingParentLeverage, or
equivalent manifests. The evidence must record:

  - hardware, optimization identity, immutable argument bytes, leaf workload,
    and dispatches per command;
  - leaf control/candidate recording, GPU, and wall time plus all three ratios;
  - an explicit host-bound-leaf classification;
  - exact parent workload, control/candidate time, marginal and paired ratios,
    and a frozen promotion threshold;
  - leaf and parent fresh-process pair counts and alternating order;
  - CPU/GPU overlap consideration;
  - control/candidate allocation bytes;
  - leaf and parent exact-digest status;
  - close-after-encoding/before-completion lifetime status; and
  - the final retain/remove decision.

Constant evidence is checked by recomputing every ratio. A cache-hot leaf is
misclassified if it claims application/end-to-end leverage while the exact
parent misses the frozen threshold. Retaining such a candidate is rejected.
Allocation, digest, lifetime, fresh-process, alternation, and overlap statuses
must also be explicit and successful.

There is NO automatic fix. Cached native resource lifetime and CPU/GPU overlap
depend on the backend and current parent critical path. Source rewriting cannot
prove that the cached object survives command completion or moves application
latency.`,
		Before: `BenchmarkCachedArgumentBuffer(b) // reports only 1.30x wall leaf`,
		After: `evidence := CommandConstructionEvidence{
	LeafControlRecordingNS: 4680, LeafCandidateRecordingNS: 520,
	LeafControlGPUNS: controlGPU, LeafCandidateGPUNS: candidateGPU,
	LeafControlWallNS: 20627, LeafCandidateWallNS: 15843,
	EvidenceClassification: "host-bound-leaf",
	ParentWorkload: "TinyLlama-1.1B Q4_K_M 200-token decode",
	ParentControlNSToken: 36_797_829.5,
	ParentCandidateNSToken: 36_764_181,
	ParentPromotionThreshold: 1.03,
	FinalDecision: "removed",
}`,
		MeasuredWin: `The Apple-M2 campaign behind issue #745 reduced recording
from roughly 4.5-4.7 us to 0.5 us per dispatch and improved cache-hot leaf wall
time by 1.10x-1.30x. Ten alternating fresh-process parent pairs measured only
1.000915x marginal and 1.000613x paired, with unchanged 147,760 B/op and 10
allocations plus an exact digest. The candidate was therefore removed.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6033",
		Doc:  "host-bound command-recording leaf evidence lacks exact parent leverage",
		Run:  runPS6033,
	},
})

type ps6033Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6033Axes = []ps6033Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6033HardwareField) }},
	{name: "optimization identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6033OptimizationField) }},
	{name: "immutable argument bytes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6033ArgumentBytesField) }},
	{name: "leaf workload", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6033LeafWorkloadField) }},
	{name: "dispatches per command", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6033DispatchesField) }},
	{name: "leaf control recording time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033LeafTimeField(n, "control", "recording") })
	}},
	{name: "leaf candidate recording time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033LeafTimeField(n, "candidate", "recording") })
	}},
	{name: "leaf recording ratio", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033LeafRatioField(n, "recording") })
	}},
	{name: "leaf control GPU time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033LeafTimeField(n, "control", "gpu") })
	}},
	{name: "leaf candidate GPU time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033LeafTimeField(n, "candidate", "gpu") })
	}},
	{name: "leaf GPU ratio", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033LeafRatioField(n, "gpu") })
	}},
	{name: "leaf control wall time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033LeafTimeField(n, "control", "wall") })
	}},
	{name: "leaf candidate wall time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033LeafTimeField(n, "candidate", "wall") })
	}},
	{name: "leaf wall ratio", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033LeafRatioField(n, "wall") })
	}},
	{name: "evidence classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6033ClassificationField) }},
	{name: "parent workload", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6033ParentWorkloadField) }},
	{name: "parent control time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033ParentTimeField(n, "control") })
	}},
	{name: "parent candidate time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033ParentTimeField(n, "candidate") })
	}},
	{name: "parent marginal ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6033ParentMarginalField) }},
	{name: "parent paired ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6033ParentPairedField) }},
	{name: "parent promotion threshold", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6033PromotionField) }},
	{name: "leaf fresh-process pairs", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033PairsField(n, "leaf") })
	}},
	{name: "parent fresh-process pairs", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033PairsField(n, "parent") })
	}},
	{name: "alternating-order status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6033AlternatingField) }},
	{name: "CPU/GPU overlap status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6033OverlapField) }},
	{name: "control allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033AllocationField(n, "control") })
	}},
	{name: "candidate allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033AllocationField(n, "candidate") })
	}},
	{name: "leaf exact-digest status", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033DigestField(n, "leaf") })
	}},
	{name: "parent exact-digest status", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6033DigestField(n, "parent") })
	}},
	{name: "close-before-completion lifetime status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6033LifetimeField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6033DecisionField) }},
}

type ps6033Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6033(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6033Context(text) {
				continue
			}
			manifest, found := ps6033BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "cached command-construction campaign has no host/parent leverage manifest; missing %s", strings.Join(ps6033Missing(nil), ", "))
				continue
			}
			if missing := ps6033Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "command-construction leverage evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6033Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "command-construction leverage audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6033Context(text string) bool {
	text = ps6007NormalizeName(text)
	accelerator := ps6007ContainsAny(text, "metal", "gpu", "accelerator", "device", "command")
	cache := ps6007ContainsAny(text, "cachedargument", "argumentbuffer", "commandconstruction", "recording")
	parent := ps6007ContainsAny(text, "parent", "leverage", "endtoend", "model")
	return accelerator && cache && parent
}

func ps6033BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6033Manifest, bool) {
	var best ps6033Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6033ManifestType(lit.Type) {
			return true
		}
		manifest := ps6033Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6033Axes) - len(ps6033Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6033ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6033ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6033ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "commandconstructionevidence", "cachedargumentbufferevidence", "hostboundleafgate", "recordingparentleverage", "argumentcachecampaign")
}

func ps6033Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6033Axes))
	for _, axis := range ps6033Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6033HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "deviceidentity", "gpuid")
}
func ps6033OptimizationField(name string) bool {
	return strings.Contains(name, "optimization") && ps6007ContainsAny(name, "identity", "name", "kind")
}
func ps6033ArgumentBytesField(name string) bool {
	return strings.Contains(name, "immutable") && strings.Contains(name, "argument") && strings.Contains(name, "byte")
}
func ps6033LeafWorkloadField(name string) bool {
	return strings.Contains(name, "leaf") && strings.Contains(name, "workload")
}
func ps6033DispatchesField(name string) bool {
	return strings.Contains(name, "dispatch") && strings.Contains(name, "percommand")
}
func ps6033LeafTimeField(name, side, component string) bool {
	return strings.Contains(name, "leaf") && strings.Contains(name, side) && strings.Contains(name, component) && ps6007ContainsAny(name, "ns", "time", "duration") && !strings.Contains(name, "ratio")
}
func ps6033LeafRatioField(name, component string) bool {
	return strings.Contains(name, "leaf") && strings.Contains(name, component) && ps6007ContainsAny(name, "ratio", "speedup")
}
func ps6033ClassificationField(name string) bool {
	return strings.Contains(name, "evidence") && ps6007ContainsAny(name, "classification", "class")
}
func ps6033ParentWorkloadField(name string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, "workload")
}
func ps6033ParentTimeField(name, side string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, side) && ps6007ContainsAny(name, "ns", "time", "duration") && !ps6007ContainsAny(name, "ratio", "threshold")
}
func ps6033ParentMarginalField(name string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, "marginal") && ps6007ContainsAny(name, "ratio", "speedup")
}
func ps6033ParentPairedField(name string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, "paired") && ps6007ContainsAny(name, "ratio", "speedup")
}
func ps6033PromotionField(name string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, "promotion") && ps6007ContainsAny(name, "threshold", "minimum", "ratio")
}
func ps6033PairsField(name, scope string) bool {
	return strings.Contains(name, scope) && strings.Contains(name, "freshprocess") && strings.Contains(name, "pair")
}
func ps6033AlternatingField(name string) bool {
	return strings.Contains(name, "alternating") && ps6007ContainsAny(name, "order", "status", "passed")
}
func ps6033OverlapField(name string) bool {
	return strings.Contains(name, "cpu") && strings.Contains(name, "gpu") && strings.Contains(name, "overlap")
}
func ps6033AllocationField(name, side string) bool {
	return strings.Contains(name, side) && strings.Contains(name, "allocation") && strings.Contains(name, "byte")
}
func ps6033DigestField(name, scope string) bool {
	return strings.Contains(name, scope) && strings.Contains(name, "digest") && ps6007ContainsAny(name, "exact", "passed", "matched")
}
func ps6033LifetimeField(name string) bool {
	return strings.Contains(name, "close") && strings.Contains(name, "beforecompletion") && ps6007ContainsAny(name, "lifetime", "passed", "status")
}
func ps6033DecisionField(name string) bool {
	return strings.Contains(name, "final") && strings.Contains(name, "decision")
}

func ps6033Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 10)
	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"alternating order", ps6033AlternatingField},
		{"CPU/GPU overlap consideration", ps6033OverlapField},
		{"leaf exact digest", func(n string) bool { return ps6033DigestField(n, "leaf") }},
		{"parent exact digest", func(n string) bool { return ps6033DigestField(n, "parent") }},
		{"close-before-completion lifetime", ps6033LifetimeField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	for _, ratio := range []struct {
		label     string
		control   func(string) bool
		candidate func(string) bool
		recorded  func(string) bool
	}{
		{"leaf recording", func(n string) bool { return ps6033LeafTimeField(n, "control", "recording") }, func(n string) bool { return ps6033LeafTimeField(n, "candidate", "recording") }, func(n string) bool { return ps6033LeafRatioField(n, "recording") }},
		{"leaf GPU", func(n string) bool { return ps6033LeafTimeField(n, "control", "gpu") }, func(n string) bool { return ps6033LeafTimeField(n, "candidate", "gpu") }, func(n string) bool { return ps6033LeafRatioField(n, "gpu") }},
		{"leaf wall", func(n string) bool { return ps6033LeafTimeField(n, "control", "wall") }, func(n string) bool { return ps6033LeafTimeField(n, "candidate", "wall") }, func(n string) bool { return ps6033LeafRatioField(n, "wall") }},
		{"parent marginal", func(n string) bool { return ps6033ParentTimeField(n, "control") }, func(n string) bool { return ps6033ParentTimeField(n, "candidate") }, ps6033ParentMarginalField},
	} {
		control, controlOK := ps6016Number(fields, ratio.control)
		candidate, candidateOK := ps6016Number(fields, ratio.candidate)
		recorded, recordedOK := ps6016Number(fields, ratio.recorded)
		if controlOK && candidateOK && recordedOK && control > 0 && candidate > 0 && !ps6025Close(recorded, control/candidate) {
			warnings = append(warnings, fmt.Sprintf("%s ratio %.6gx disagrees with control/candidate timing ratio %.6gx", ratio.label, recorded, control/candidate))
		}
	}
	parentMarginal, marginalOK := ps6016Number(fields, ps6033ParentMarginalField)
	parentPaired, pairedOK := ps6016Number(fields, ps6033ParentPairedField)
	promotion, promotionOK := ps6016Number(fields, ps6033PromotionField)
	classification, classificationOK := ps6027String(fields, ps6033ClassificationField)
	decision, decisionOK := ps6027String(fields, ps6033DecisionField)
	parentMisses := promotionOK && marginalOK && parentMarginal < promotion || promotionOK && pairedOK && parentPaired < promotion
	if parentMisses && classificationOK && ps6007ContainsAny(ps6030StatusName(classification), "applicationleverage", "endtoendwin", "parentwin") {
		warnings = append(warnings, fmt.Sprintf("evidence classification %q overstates a host-bound leaf while the parent misses %.6gx", classification, promotion))
	}
	if parentMisses && decisionOK && ps6007ContainsAny(ps6030StatusName(decision), "retain", "promote", "selected", "ship") {
		warnings = append(warnings, fmt.Sprintf("final decision %q retains a candidate whose parent marginal/paired ratio misses %.6gx", decision, promotion))
	}
	leafPairs, leafPairsOK := ps6016Number(fields, func(n string) bool { return ps6033PairsField(n, "leaf") })
	parentPairs, parentPairsOK := ps6016Number(fields, func(n string) bool { return ps6033PairsField(n, "parent") })
	if leafPairsOK && leafPairs < 1 || parentPairsOK && parentPairs < 1 {
		warnings = append(warnings, "leaf and parent fresh-process pair counts must be positive")
	}
	return warnings
}
