package checks

import (
	"fmt"
	"go/ast"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6027 implements owner issue #751: a leaf-neutral/negative GPU candidate
// may escalate only through a predeclared, same-work exact-parent gate.
var PS6027 = register(&lint.Check{
	ID:       "PS6027",
	Category: "verify",
	Slug:     "leaf-negative-fusion-needs-exact-parent-gate",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a leaf-negative fusion is promoted without a predeclared same-work exact-parent gate",
		Text: `A fused accelerator kernel can be neutral or slower in an
isolated leaf benchmark yet help a production graph by removing encoder and
dependency edges. A leaf-only kill rule can therefore reject real graph wins.
The converse is equally important: a parent win must not rewrite negative leaf
history or be called an isolated-kernel speedup.

This check implements owner issue #751 and its corrected-parent follow-ups. It
audits HierarchicalCampaignEvidence, LeafParentPromotionEvidence,
ExactParentGate, LeafEscalationEvidence, or equivalent manifests. A
leaf-neutral/negative escalation must record:

  - the leaf operation sequence and exact production-parent workload;
  - leaf ratio/classification and retained negative evidence;
  - a predeclared parent hypothesis that predicts topology/edge leverage;
  - explicit approval to escalate the leaf-negative candidate;
  - a corrected-unfused active-extent control executing the same work;
  - identical output, dtype, shape, weights, and warm/cold boundaries;
  - leaf and parent fresh-process isolation;
  - control/candidate event counts and exact deleted-edge identity/count;
  - a small fresh-process parent kill ratio and frozen minimum;
  - an alternating full parent campaign;
  - marginal median, p90, paired ratios, and leaf/parent allocation deltas;
  - corrected-control and candidate timings plus their ratio;
  - exactness/parity; and
  - an explicit graph-parent claim scope.

Constant evidence is checked for false prerequisite statuses, sparse paired
campaigns, failed kill gates, topology arithmetic mismatches, stale timing
ratios, a candidate slower than the corrected same-work parent, and an
isolated-kernel claim attached to a negative leaf plus positive parent result.

There is NO automatic fix. Campaign independence, exact-parent identity,
topology, active work, and claim scope are measured/domain evidence.`,
		Before: `if leafRatio < 1 {
	reject(candidate)
}
// Or later: call a parent win an isolated-kernel speedup.`,
		After: `evidence := HierarchicalCampaignEvidence{
	LeafOperationSequence: "Q4_K gate + up + SwiGLU",
	ExactProductionParentWorkload: "TinyLlama rows=1 decode",
	LeafControlCandidateRatio: 0.974,
	NegativeLeafEvidenceRetained: true,
	ParentHypothesis: "delete command/dependency edges",
	ParentHypothesisPredeclared: true,
	GraphEdgeLeveragePredicted: true,
	LeafNegativeEscalationApproved: true,
	CorrectedUnfusedActiveExtentControl: true,
	// identity, topology, kill-gate, campaign, allocation and parity fields...
	ClaimScope: "graph-parent",
}`,
		MeasuredWin: `The cited Apple-M2-Pro leaf candidate was 2.6%
slower. An initially reported exact parent win of 2.837988x combined fusion,
edge deletion, and obsolete inactive-capacity traversal. Against current
main's corrected active-extent parent, 200-token control was 1,190,455,583 ns
and the fusion candidate 1,208,686,833 ns: 0.984916x control/candidate, or
1.532% slower. The candidate was removed; the negative leaf result remains
valid history.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6027",
		Doc:  "leaf-negative fusion escalation lacks a predeclared same-work exact-parent gate",
		Run:  runPS6027,
	},
})

type ps6027Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6027Axes = []ps6027Axis{
	{name: "leaf operation sequence", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027LeafSequenceField) }},
	{name: "exact production-parent workload", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027ParentWorkloadField) }},
	{name: "leaf ratio/classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027LeafRatioField) }},
	{name: "negative leaf evidence retention", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027LeafRetentionField) }},
	{name: "predeclared parent hypothesis text", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027HypothesisTextField) }},
	{name: "parent hypothesis predeclaration status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027HypothesisStatusField) }},
	{name: "graph-edge leverage prediction", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027EdgePredictionField) }},
	{name: "leaf-negative escalation approval", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027EscalationField) }},
	{name: "corrected-unfused active-extent control", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027CorrectedControlField) }},
	{name: "identical outputs", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6027IdentityField(n, "output") })
	}},
	{name: "identical dtype", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6027IdentityField(n, "dtype") })
	}},
	{name: "identical shape", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6027IdentityField(n, "shape") })
	}},
	{name: "identical weights", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6027IdentityField(n, "weight") })
	}},
	{name: "identical warm/cold boundary", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027BoundaryIdentityField) }},
	{name: "leaf process isolation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027LeafIsolationField) }},
	{name: "parent process isolation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027ParentIsolationField) }},
	{name: "control event count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027ControlEventsField) }},
	{name: "candidate event count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027CandidateEventsField) }},
	{name: "deleted-edge identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027DeletedEdgeIdentityField) }},
	{name: "deleted-edge count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027DeletedEdgeCountField) }},
	{name: "fresh-process parent kill ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027KillRatioField) }},
	{name: "frozen parent kill minimum", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027KillMinimumField) }},
	{name: "fresh-process kill status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027KillFreshField) }},
	{name: "alternating parent campaign status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027AlternatingField) }},
	{name: "marginal median ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027MedianField) }},
	{name: "p90 ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027P90Field) }},
	{name: "paired ratios", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027PairedField) }},
	{name: "leaf allocation delta", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027LeafAllocationField) }},
	{name: "parent allocation delta", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027ParentAllocationField) }},
	{name: "corrected-control timing", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027CorrectedTimeField) }},
	{name: "fusion-candidate timing", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027CandidateTimeField) }},
	{name: "corrected-control/candidate ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027CorrectedRatioField) }},
	{name: "exactness/parity status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6016ExactnessField) }},
	{name: "claim scope", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6027ClaimScopeField) }},
}

type ps6027Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6027(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6011Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6027Context(text) {
				continue
			}
			manifest, found := ps6027BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "leaf-to-parent accelerator campaign has no hierarchical promotion manifest; missing %s", strings.Join(ps6027Missing(nil), ", "))
				continue
			}
			if missing := ps6027Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "leaf-to-parent promotion evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6027Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "leaf-to-parent promotion audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6027Context(text string) bool {
	text = ps6007NormalizeName(text)
	accelerator := ps6007ContainsAny(text, "gpu", "metal", "mps", "cuda", "vulkan", "accelerator", "device")
	leaf := strings.Contains(text, "leaf")
	parent := ps6007ContainsAny(text, "parent", "productiongraph", "exactparent")
	fusion := ps6007ContainsAny(text, "fusion", "fused", "candidate")
	campaign := ps6007ContainsAny(text, "campaign", "promotion", "escalation", "gate")
	return accelerator && leaf && parent && fusion && campaign
}

func ps6027BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6027Manifest, bool) {
	var best ps6027Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6027ManifestType(lit.Type) {
			return true
		}
		manifest := ps6027Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6027Axes) - len(ps6027Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6027ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6027ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6027ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "hierarchicalcampaignevidence", "leafparentpromotion", "exactparentgate", "leafescalationevidence", "parentpromotionevidence")
}

func ps6027Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6027Axes))
	for _, axis := range ps6027Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6027LeafSequenceField(name string) bool {
	return strings.Contains(name, "leaf") && ps6007ContainsAny(name, "operationsequence", "opsequence", "sequence", "operations")
}

func ps6027ParentWorkloadField(name string) bool {
	return ps6007ContainsAny(name, "exactproductionparent", "exactparentworkload", "productionparentworkload", "parentworkload")
}

func ps6027LeafRatioField(name string) bool {
	return strings.Contains(name, "leaf") && ps6007ContainsAny(name, "ratio", "classification", "result") &&
		!ps6007ContainsAny(name, "retained", "retention", "operation", "sequence")
}

func ps6027LeafRetentionField(name string) bool {
	return strings.Contains(name, "leaf") &&
		((strings.Contains(name, "negative") && strings.Contains(name, "evidence") && strings.Contains(name, "retained")) ||
			ps6007ContainsAny(name, "historyretained", "resultretained"))
}

func ps6027HypothesisTextField(name string) bool {
	return strings.Contains(name, "parenthypothesis") && !ps6007ContainsAny(name, "predeclared", "status", "passed")
}

func ps6027HypothesisStatusField(name string) bool {
	return strings.Contains(name, "parenthypothesis") && ps6007ContainsAny(name, "predeclared", "status", "passed")
}

func ps6027EdgePredictionField(name string) bool {
	return ps6007ContainsAny(name, "graphedgeleverage", "topologyleverage", "edgedeletionpredicted", "dependencyedgepredicted")
}

func ps6027EscalationField(name string) bool {
	return strings.Contains(name, "escalation") && ps6007ContainsAny(name, "leafnegative", "approved", "allowed", "permitted")
}

func ps6027CorrectedControlField(name string) bool {
	return strings.Contains(name, "correctedunfused") && ps6007ContainsAny(name, "activeextent", "samework", "control")
}

func ps6027IdentityField(name, dimension string) bool {
	return strings.Contains(name, dimension) && ps6007ContainsAny(name, "identical", "same", "matched") && ps6007ContainsAny(name, "bothlevels", "leafparent", "levels")
}

func ps6027BoundaryIdentityField(name string) bool {
	return ps6007ContainsAny(name, "warmcold", "thermalboundary", "warmboundary", "coldboundary") &&
		ps6007ContainsAny(name, "identical", "same", "matched")
}

func ps6027LeafIsolationField(name string) bool {
	return strings.Contains(name, "leaf") && strings.Contains(name, "process") && ps6007ContainsAny(name, "isolated", "isolation", "fresh")
}

func ps6027ParentIsolationField(name string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, "process") && ps6007ContainsAny(name, "isolated", "isolation", "fresh") &&
		!strings.Contains(name, "kill")
}

func ps6027ControlEventsField(name string) bool {
	return ps6007ContainsAny(name, "control", "incumbent", "before") && ps6007ContainsAny(name, "eventcount", "commandcount", "encodercount")
}

func ps6027CandidateEventsField(name string) bool {
	return ps6007ContainsAny(name, "candidate", "fusion", "after") && ps6007ContainsAny(name, "eventcount", "commandcount", "encodercount")
}

func ps6027DeletedEdgeIdentityField(name string) bool {
	return strings.Contains(name, "deletededge") && ps6007ContainsAny(name, "id", "name", "identity", "list") && !strings.Contains(name, "count")
}

func ps6027DeletedEdgeCountField(name string) bool {
	return strings.Contains(name, "deletededge") && strings.Contains(name, "count")
}

func ps6027KillRatioField(name string) bool {
	return strings.Contains(name, "parentkill") && ps6007ContainsAny(name, "ratio", "result", "sample") && !ps6007ContainsAny(name, "minimum", "threshold")
}

func ps6027KillMinimumField(name string) bool {
	return strings.Contains(name, "parentkill") && ps6007ContainsAny(name, "minimum", "threshold", "required")
}

func ps6027KillFreshField(name string) bool {
	return strings.Contains(name, "parentkill") && strings.Contains(name, "freshprocess")
}

func ps6027AlternatingField(name string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, "alternating") && ps6007ContainsAny(name, "campaign", "status", "passed")
}

func ps6027MedianField(name string) bool {
	return strings.Contains(name, "marginal") && strings.Contains(name, "median") && ps6007ContainsAny(name, "ratio", "speedup")
}

func ps6027P90Field(name string) bool {
	return strings.Contains(name, "p90") && ps6007ContainsAny(name, "ratio", "speedup")
}

func ps6027PairedField(name string) bool {
	return strings.Contains(name, "paired") && ps6007ContainsAny(name, "ratio", "sample", "distribution")
}

func ps6027LeafAllocationField(name string) bool {
	return strings.Contains(name, "leaf") && strings.Contains(name, "allocation") && ps6007ContainsAny(name, "delta", "change")
}

func ps6027ParentAllocationField(name string) bool {
	return strings.Contains(name, "parent") && strings.Contains(name, "allocation") && ps6007ContainsAny(name, "delta", "change")
}

func ps6027CorrectedTimeField(name string) bool {
	return strings.Contains(name, "corrected") && ps6007ContainsAny(name, "unfused", "control") && ps6007ContainsAny(name, "ns", "time", "duration", "median")
}

func ps6027CandidateTimeField(name string) bool {
	return ps6007ContainsAny(name, "fusioncandidate", "candidateparent") && ps6007ContainsAny(name, "ns", "time", "duration", "median")
}

func ps6027CorrectedRatioField(name string) bool {
	return strings.Contains(name, "corrected") && strings.Contains(name, "candidate") && ps6007ContainsAny(name, "ratio", "speedup")
}

func ps6027ClaimScopeField(name string) bool {
	return strings.Contains(name, "claim") && ps6007ContainsAny(name, "scope", "level", "kind")
}

func ps6027Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 10)
	leaf, leafOK := ps6016Number(fields, ps6027LeafRatioField)
	median, medianOK := ps6016Number(fields, ps6027MedianField)
	paired, pairedOK := ps6016Numbers(fields, ps6027PairedField)
	kill, killOK := ps6016Number(fields, ps6027KillRatioField)
	killMinimum, killMinimumOK := ps6016Number(fields, ps6027KillMinimumField)
	controlEvents, controlEventsOK := ps6016Number(fields, ps6027ControlEventsField)
	candidateEvents, candidateEventsOK := ps6016Number(fields, ps6027CandidateEventsField)
	deletedEdges, deletedEdgesOK := ps6016Number(fields, ps6027DeletedEdgeCountField)
	correctedTime, correctedTimeOK := ps6016Number(fields, ps6027CorrectedTimeField)
	candidateTime, candidateTimeOK := ps6016Number(fields, ps6027CandidateTimeField)
	correctedRatio, correctedRatioOK := ps6016Number(fields, ps6027CorrectedRatioField)

	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"negative leaf evidence retention", ps6027LeafRetentionField},
		{"parent hypothesis predeclaration", ps6027HypothesisStatusField},
		{"graph-edge leverage prediction", ps6027EdgePredictionField},
		{"leaf-negative escalation approval", ps6027EscalationField},
		{"corrected-unfused active-extent control", ps6027CorrectedControlField},
		{"identical outputs", func(n string) bool { return ps6027IdentityField(n, "output") }},
		{"identical dtype", func(n string) bool { return ps6027IdentityField(n, "dtype") }},
		{"identical shape", func(n string) bool { return ps6027IdentityField(n, "shape") }},
		{"identical weights", func(n string) bool { return ps6027IdentityField(n, "weight") }},
		{"identical warm/cold boundary", ps6027BoundaryIdentityField},
		{"leaf process isolation", ps6027LeafIsolationField},
		{"parent process isolation", ps6027ParentIsolationField},
		{"fresh-process parent kill", ps6027KillFreshField},
		{"alternating parent campaign", ps6027AlternatingField},
		{"exactness/parity", ps6016ExactnessField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	if pairedOK && len(paired) < 3 {
		warnings = append(warnings, "paired parent campaign has fewer than three ratios")
	}
	if pairedOK && len(paired) > 0 && slices.Min(paired) <= 0 {
		warnings = append(warnings, "paired parent ratios must be positive")
	}
	if killOK && killMinimumOK && kill < killMinimum {
		warnings = append(warnings, fmt.Sprintf("fresh-process parent kill ratio %.6gx misses frozen %.6gx minimum", kill, killMinimum))
	}
	if controlEventsOK && candidateEventsOK && deletedEdgesOK && controlEvents-candidateEvents != deletedEdges {
		warnings = append(warnings, fmt.Sprintf("event-count reduction %.0f disagrees with declared deleted-edge count %.0f", controlEvents-candidateEvents, deletedEdges))
	}
	if correctedTimeOK && candidateTimeOK && correctedTime > 0 && candidateTime > 0 {
		calculated := correctedTime / candidateTime
		if correctedRatioOK && !ps6025Close(correctedRatio, calculated) {
			warnings = append(warnings, fmt.Sprintf("recorded corrected-control/candidate ratio %.6gx disagrees with timing ratio %.6gx", correctedRatio, calculated))
		}
		if calculated < 1 {
			warnings = append(warnings, fmt.Sprintf("fusion candidate is slower than corrected same-work parent (control/candidate %.6gx); promotion must fail", calculated))
		}
	}
	if leafOK && leaf <= 1 && medianOK && median > 1 {
		if claim, ok := ps6027String(fields, ps6027ClaimScopeField); ok && ps6007ContainsAny(strings.ToLower(claim), "isolated", "kernel", "leaf") {
			warnings = append(warnings, fmt.Sprintf("leaf ratio %.6gx is non-positive leverage while parent median is %.6gx; claim scope %q rewrites a graph-parent result as an isolated-kernel speedup", leaf, median, claim))
		}
	}
	return warnings
}

func ps6027String(fields map[string]ps6016Field, predicate func(string) bool) (string, bool) {
	for name, field := range fields {
		if predicate(name) && field.hasString {
			return field.stringVal, true
		}
	}
	return "", false
}
