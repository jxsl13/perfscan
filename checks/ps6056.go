package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6056 implements owner issue #722: fixed-order grouped GPU screens are
// nomination evidence, not promotion evidence.
var PS6056 = register(&lint.Check{
	ID:       "PS6056",
	Category: "verify",
	Slug:     "gpu-grouped-screen-cannot-promote",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a GPU fusion candidate is promoted from a grouped short screen",
		Text: `Running all control samples before all candidate samples can turn
thermal, power-state, allocator, compilation, and device-residency drift into
an apparent kernel win. Repeating that order in one process does not create
independent evidence, and a large three-sample grouped median is still a screen.

This check implements owner issue #722. It audits
GroupedFusionPromotionEvidence, GPUShortScreenGate,
AlternatingFusionValidationReport, ScreenNominationContract, or equivalent
manifests. Evidence must separate:

  - a short screen's process/iteration count, control/candidate time and ratio;
  - grouped raw control/candidate samples, execution order, medians and ratio;
  - whether either screen was used directly for promotion;
  - a predeclared validation contract and threshold, at least ten required and
    accepted independent fresh-process pairs, 500x-style fixed iterations,
    explicit warm-up disclosure, fixed synchronization boundaries, and actual
    pair order;
  - raw promotion samples, medians and ratio, allocation bytes/counts, exact
    output, verdict, selection, classification, and final decision.

Constant evidence is checked for stale screen/grouped/promotion medians and
ratios, blocked control-then-candidate order, vector/count mismatches, false
alternation, allocation differences, and promotion below the predeclared gate.
A blocked short screen may remain in a clean report when it only nominates a
candidate and independent validation rejects it.

There is NO automatic fix because fresh processes, GPU state, synchronization,
warm-up, and measured samples cannot be synthesized from source.`,
		Before: `run(control, control, control)
run(candidate, candidate, candidate)
if groupedMedianRatio > gate { promote(candidate) }`,
		After: `predeclare(gate, requiredPairs=10)
for pair := range independentProcesses {
    alternate(control, candidate, pair)
}
promoteOnlyIf(rawSamples, medians, parity, allocations, sync).Pass(gate)`,
		MeasuredWin: `The Apple-M2-Pro issue #722 Q6_K residual-accumulate fusion
looked 1.259x faster in its first 200x screen and 1.640x by grouped three-sample
medians. Ten predeclared independent alternating 500x process pairs measured
370,977.5 ns/op control versus 359,695 ns/op candidate—only 1.031x, below the
1.10x gate—with identical allocations. Q4_K was likewise about 1.03x, so the
candidate was rejected.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6056",
		Doc:  "GPU fusion claim promotes from grouped short-screen evidence",
		Run:  runPS6056,
	},
})

type ps6056Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6056Axes = []ps6056Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056HardwareField) }},
	{name: "workload identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056WorkloadField) }},
	{name: "candidate default-off status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056DefaultOffField) }},
	{name: "short-screen process count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056ScreenCountField(n, "process") })
	}},
	{name: "short-screen iteration count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056ScreenCountField(n, "iteration") })
	}},
	{name: "short-screen control time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056ScreenTimeField(n, "control") })
	}},
	{name: "short-screen candidate time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056ScreenTimeField(n, "candidate") })
	}},
	{name: "short-screen ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056ScreenRatioField) }},
	{name: "short-screen promotion status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056ScreenPromotionField) }},
	{name: "grouped process count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056GroupedCountField(n, "process") })
	}},
	{name: "grouped sample count per side", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056GroupedCountField(n, "sample") })
	}},
	{name: "grouped control raw samples", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056GroupedSamplesField(n, "control") })
	}},
	{name: "grouped candidate raw samples", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056GroupedSamplesField(n, "candidate") })
	}},
	{name: "grouped execution order", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056GroupedOrderField) }},
	{name: "grouped control median", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056GroupedMedianField(n, "control") })
	}},
	{name: "grouped candidate median", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056GroupedMedianField(n, "candidate") })
	}},
	{name: "grouped ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056GroupedRatioField) }},
	{name: "grouped-screen promotion status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056GroupedPromotionField) }},
	{name: "promotion contract predeclaration", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056ContractField) }},
	{name: "promotion threshold predeclaration", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056ThresholdPredeclaredField) }},
	{name: "required independent pair count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056RequiredPairsField) }},
	{name: "accepted independent process pair count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056AcceptedPairsField) }},
	{name: "fresh-process status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056FreshField) }},
	{name: "order-alternating status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056AlternatingField) }},
	{name: "promotion iterations per process", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056PromotionIterationsField) }},
	{name: "warm-up disclosure", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056WarmupField) }},
	{name: "fixed synchronization boundaries", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056SyncField) }},
	{name: "promotion pair order", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056PairOrderField) }},
	{name: "promotion control raw samples", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056PromotionSamplesField(n, "control") })
	}},
	{name: "promotion candidate raw samples", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056PromotionSamplesField(n, "candidate") })
	}},
	{name: "raw-sample publication status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056RawPublishedField) }},
	{name: "promotion control median", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056PromotionMedianField(n, "control") })
	}},
	{name: "promotion candidate median", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056PromotionMedianField(n, "candidate") })
	}},
	{name: "promotion ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056PromotionRatioField) }},
	{name: "control allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056AllocationField(n, "control", "byte") })
	}},
	{name: "candidate allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056AllocationField(n, "candidate", "byte") })
	}},
	{name: "control allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056AllocationField(n, "control", "count") })
	}},
	{name: "candidate allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6056AllocationField(n, "candidate", "count") })
	}},
	{name: "exact output", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056ExactField) }},
	{name: "promotion threshold", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056ThresholdField) }},
	{name: "promotion verdict", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056VerdictField) }},
	{name: "candidate selection status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056SelectedField) }},
	{name: "evidence classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056ClassificationField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6056DecisionField) }},
}

type ps6056Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6056(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6056Context(text) {
				continue
			}
			manifest, found := ps6056BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "grouped GPU fusion screen has no alternating promotion manifest; missing %s", strings.Join(ps6056Missing(nil), ", "))
				continue
			}
			if missing := ps6056Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "grouped GPU screen evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6056Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "grouped GPU screen audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6056Context(text string) bool {
	text = ps6007NormalizeName(text)
	return ps6007ContainsAny(text, "gpu", "metal", "cuda", "vulkan") && ps6007ContainsAny(text, "groupedshortscreen", "groupedfusion", "screennomination") && ps6007ContainsAny(text, "alternatingpromotion", "independentvalidation", "promotioncontract")
}

func ps6056BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6056Manifest, bool) {
	var best ps6056Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6056ManifestType(lit.Type) {
			return true
		}
		manifest := ps6056Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6056Axes) - len(ps6056Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6056ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6056ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6056ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "groupedfusionpromotionevidence", "gpushortscreengate", "alternatingfusionvalidationreport", "screennominationcontract", "groupedscreenpromotionevidence")
}

func ps6056Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6056Axes))
	for _, axis := range ps6056Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6056HardwareField(n string) bool {
	return ps6007ContainsAny(n, "hardware", "deviceidentity", "gpuid")
}
func ps6056WorkloadField(n string) bool {
	return strings.Contains(n, "workload") && ps6007ContainsAny(n, "identity", "shape", "name")
}
func ps6056DefaultOffField(n string) bool {
	return strings.Contains(n, "candidate") && strings.Contains(n, "defaultoff")
}
func ps6056ScreenCountField(n, detail string) bool {
	return strings.Contains(n, "shortscreen") && strings.Contains(n, detail) && strings.Contains(n, "count")
}
func ps6056ScreenTimeField(n, side string) bool {
	return strings.Contains(n, "shortscreen") && strings.Contains(n, side) && ps6007ContainsAny(n, "ns", "time") && !strings.Contains(n, "ratio")
}
func ps6056ScreenRatioField(n string) bool {
	return strings.Contains(n, "shortscreen") && strings.Contains(n, "controlcandidate") && strings.Contains(n, "ratio")
}
func ps6056ScreenPromotionField(n string) bool {
	return strings.Contains(n, "shortscreen") && strings.Contains(n, "usedforpromotion")
}
func ps6056GroupedCountField(n, detail string) bool {
	return strings.Contains(n, "grouped") && strings.Contains(n, detail) && strings.Contains(n, "count")
}
func ps6056GroupedSamplesField(n, side string) bool {
	return strings.Contains(n, "grouped") && strings.Contains(n, side) && strings.Contains(n, "rawsample")
}
func ps6056GroupedOrderField(n string) bool {
	return strings.Contains(n, "grouped") && strings.Contains(n, "executionorder")
}
func ps6056GroupedMedianField(n, side string) bool {
	return strings.Contains(n, "grouped") && strings.Contains(n, side) && strings.Contains(n, "median")
}
func ps6056GroupedRatioField(n string) bool {
	return strings.Contains(n, "grouped") && strings.Contains(n, "controlcandidate") && strings.Contains(n, "ratio")
}
func ps6056GroupedPromotionField(n string) bool {
	return strings.Contains(n, "grouped") && strings.Contains(n, "screen") && strings.Contains(n, "usedforpromotion")
}
func ps6056ContractField(n string) bool {
	return strings.Contains(n, "promotioncontract") && strings.Contains(n, "predeclared")
}
func ps6056ThresholdPredeclaredField(n string) bool {
	return strings.Contains(n, "promotionthreshold") && strings.Contains(n, "predeclared")
}
func ps6056RequiredPairsField(n string) bool {
	return strings.Contains(n, "required") && strings.Contains(n, "independent") && strings.Contains(n, "paircount")
}
func ps6056AcceptedPairsField(n string) bool {
	return strings.Contains(n, "accepted") && strings.Contains(n, "independentprocess") && strings.Contains(n, "paircount")
}
func ps6056FreshField(n string) bool {
	return strings.Contains(n, "freshprocess") && ps6007ContainsAny(n, "passed", "status", "verified")
}
func ps6056AlternatingField(n string) bool {
	return strings.Contains(n, "orderalternating") && ps6007ContainsAny(n, "passed", "status", "verified")
}
func ps6056PromotionIterationsField(n string) bool {
	return strings.Contains(n, "promotion") && strings.Contains(n, "iteration") && strings.Contains(n, "perprocess")
}
func ps6056WarmupField(n string) bool {
	return strings.Contains(n, "promotion") && strings.Contains(n, "warmup") && strings.Contains(n, "disclosure")
}
func ps6056SyncField(n string) bool {
	return strings.Contains(n, "fixed") && strings.Contains(n, "synchronization") && strings.Contains(n, "boundar")
}
func ps6056PairOrderField(n string) bool {
	return strings.Contains(n, "promotion") && strings.Contains(n, "pairorder")
}
func ps6056PromotionSamplesField(n, side string) bool {
	return strings.Contains(n, "promotion") && strings.Contains(n, side) && strings.Contains(n, "rawsample")
}
func ps6056RawPublishedField(n string) bool {
	return strings.Contains(n, "rawsample") && strings.Contains(n, "published")
}
func ps6056PromotionMedianField(n, side string) bool {
	return strings.Contains(n, "promotion") && strings.Contains(n, side) && strings.Contains(n, "median")
}
func ps6056PromotionRatioField(n string) bool {
	return strings.Contains(n, "promotion") && strings.Contains(n, "controlcandidate") && strings.Contains(n, "ratio")
}
func ps6056AllocationField(n, side, detail string) bool {
	return strings.Contains(n, side) && strings.Contains(n, "allocation") && strings.Contains(n, detail)
}
func ps6056ExactField(n string) bool {
	return strings.Contains(n, "exact") && ps6007ContainsAny(n, "output", "parity", "logit")
}
func ps6056ThresholdField(n string) bool {
	return strings.Contains(n, "promotionthreshold") && !strings.Contains(n, "predeclared")
}
func ps6056VerdictField(n string) bool {
	return strings.Contains(n, "promotion") && strings.Contains(n, "verdict")
}
func ps6056SelectedField(n string) bool {
	return strings.Contains(n, "candidate") && strings.Contains(n, "selected")
}
func ps6056ClassificationField(n string) bool {
	return strings.Contains(n, "evidence") && strings.Contains(n, "classification")
}
func ps6056DecisionField(n string) bool {
	return strings.Contains(n, "final") && strings.Contains(n, "decision")
}

func ps6056Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 20)
	for _, status := range []struct {
		name string
		pred func(string) bool
	}{
		{"candidate default-off", ps6056DefaultOffField},
		{"promotion contract predeclaration", ps6056ContractField},
		{"promotion threshold predeclaration", ps6056ThresholdPredeclaredField},
		{"fresh-process validation", ps6056FreshField},
		{"order-alternating validation", ps6056AlternatingField},
		{"fixed synchronization boundaries", ps6056SyncField},
		{"raw-sample publication", ps6056RawPublishedField},
		{"exact output", ps6056ExactField},
	} {
		if value, known := ps6026Bool(fields, status.pred); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}

	screenControl, screenControlOK := ps6016Number(fields, func(n string) bool { return ps6056ScreenTimeField(n, "control") })
	screenCandidate, screenCandidateOK := ps6016Number(fields, func(n string) bool { return ps6056ScreenTimeField(n, "candidate") })
	if recorded, ok := ps6016Number(fields, ps6056ScreenRatioField); ok && screenControlOK && screenCandidateOK && screenCandidate > 0 && !ps6025Close(recorded, screenControl/screenCandidate) {
		warnings = append(warnings, fmt.Sprintf("short-screen ratio %.6gx disagrees with control/candidate %.6gx", recorded, screenControl/screenCandidate))
	}

	groupedControl, groupedControlOK := ps6016Numbers(fields, func(n string) bool { return ps6056GroupedSamplesField(n, "control") })
	groupedCandidate, groupedCandidateOK := ps6016Numbers(fields, func(n string) bool { return ps6056GroupedSamplesField(n, "candidate") })
	groupedCount, groupedCountOK := ps6016Number(fields, func(n string) bool { return ps6056GroupedCountField(n, "sample") })
	groupedOrder, groupedOrderOK := ps6047Strings(fields, ps6056GroupedOrderField)
	if groupedControlOK && groupedCandidateOK && len(groupedControl) != len(groupedCandidate) {
		warnings = append(warnings, "grouped control/candidate raw-sample vectors have different lengths")
	}
	if groupedCountOK && (groupedControlOK && int(groupedCount) != len(groupedControl) || groupedCandidateOK && int(groupedCount) != len(groupedCandidate)) {
		warnings = append(warnings, fmt.Sprintf("grouped sample count %.0f disagrees with raw-sample lengths", groupedCount))
	}
	if groupedOrderOK && groupedControlOK && groupedCandidateOK && len(groupedOrder) != len(groupedControl)+len(groupedCandidate) {
		warnings = append(warnings, "grouped execution-order length disagrees with raw samples")
	}
	blockedGroupedOrder := groupedOrderOK && ps6056BlockedOrder(groupedOrder)
	groupedRatio, groupedRatioOK := ps6056AuditMedians(fields, "grouped", groupedControl, groupedControlOK, groupedCandidate, groupedCandidateOK, &warnings)
	_ = groupedRatio
	_ = groupedRatioOK

	requiredPairs, requiredPairsOK := ps6016Number(fields, ps6056RequiredPairsField)
	acceptedPairs, acceptedPairsOK := ps6016Number(fields, ps6056AcceptedPairsField)
	if requiredPairsOK && requiredPairs < 10 {
		warnings = append(warnings, fmt.Sprintf("required independent pair count %.0f is below 10", requiredPairs))
	}
	if requiredPairsOK && acceptedPairsOK && acceptedPairs < requiredPairs {
		warnings = append(warnings, fmt.Sprintf("accepted independent process pairs %.0f are below required %.0f", acceptedPairs, requiredPairs))
	}
	promotionControl, promotionControlOK := ps6016Numbers(fields, func(n string) bool { return ps6056PromotionSamplesField(n, "control") })
	promotionCandidate, promotionCandidateOK := ps6016Numbers(fields, func(n string) bool { return ps6056PromotionSamplesField(n, "candidate") })
	if promotionControlOK && promotionCandidateOK && len(promotionControl) != len(promotionCandidate) {
		warnings = append(warnings, "promotion control/candidate raw-sample vectors have different lengths")
	}
	if acceptedPairsOK && (promotionControlOK && int(acceptedPairs) != len(promotionControl) || promotionCandidateOK && int(acceptedPairs) != len(promotionCandidate)) {
		warnings = append(warnings, fmt.Sprintf("accepted independent process pair count %.0f disagrees with promotion sample lengths", acceptedPairs))
	}
	pairOrder, pairOrderOK := ps6047Strings(fields, ps6056PairOrderField)
	if pairOrderOK && acceptedPairsOK && len(pairOrder) != int(acceptedPairs) {
		warnings = append(warnings, "promotion pair-order length disagrees with accepted pair count")
	}
	actualAlternating := pairOrderOK && ps6056Alternates(pairOrder)
	if recorded, ok := ps6026Bool(fields, ps6056AlternatingField); ok && recorded != actualAlternating {
		warnings = append(warnings, fmt.Sprintf("order-alternating status is %t but pair order computes %t", recorded, actualAlternating))
	}
	promotionRatio, promotionRatioOK := ps6056AuditMedians(fields, "promotion", promotionControl, promotionControlOK, promotionCandidate, promotionCandidateOK, &warnings)

	allocationsEqual := true
	for _, detail := range []string{"byte", "count"} {
		control, controlOK := ps6016Number(fields, func(n string) bool { return ps6056AllocationField(n, "control", detail) })
		candidate, candidateOK := ps6016Number(fields, func(n string) bool { return ps6056AllocationField(n, "candidate", detail) })
		if controlOK && candidateOK && control != candidate {
			allocationsEqual = false
			warnings = append(warnings, "control/candidate allocation "+detail+"s differ")
		}
	}
	contract, _ := ps6026Bool(fields, ps6056ContractField)
	thresholdPredeclared, _ := ps6026Bool(fields, ps6056ThresholdPredeclaredField)
	fresh, _ := ps6026Bool(fields, ps6056FreshField)
	fixedSync, _ := ps6026Bool(fields, ps6056SyncField)
	rawPublished, _ := ps6026Bool(fields, ps6056RawPublishedField)
	exact, _ := ps6026Bool(fields, ps6056ExactField)
	iterations, iterationsOK := ps6016Number(fields, ps6056PromotionIterationsField)
	warmup, warmupOK := ps6027String(fields, ps6056WarmupField)
	minimum, minimumOK := ps6016Number(fields, ps6056ThresholdField)
	validPromotion := contract && thresholdPredeclared && fresh && actualAlternating && fixedSync && rawPublished && exact && allocationsEqual && requiredPairsOK && requiredPairs >= 10 && acceptedPairsOK && acceptedPairs >= requiredPairs && iterationsOK && iterations > 0 && warmupOK && strings.TrimSpace(warmup) != ""
	passed := validPromotion && promotionRatioOK && minimumOK && promotionRatio >= minimum
	if verdict, ok := ps6027String(fields, ps6056VerdictField); ok {
		normalized := ps6030StatusName(verdict)
		claimsPass := ps6007ContainsAny(normalized, "pass", "accept", "promote") && !ps6007ContainsAny(normalized, "fail", "reject")
		claimsFail := ps6007ContainsAny(normalized, "fail", "reject", "belowthreshold")
		if passed && !claimsPass || !passed && !claimsFail {
			warnings = append(warnings, fmt.Sprintf("promotion verdict %q disagrees with computed independent gate", verdict))
		}
	}
	shortProcesses, shortProcessesOK := ps6016Number(fields, func(n string) bool { return ps6056ScreenCountField(n, "process") })
	shortUsed, _ := ps6026Bool(fields, ps6056ScreenPromotionField)
	groupedProcesses, groupedProcessesOK := ps6016Number(fields, func(n string) bool { return ps6056GroupedCountField(n, "process") })
	groupedUsed, _ := ps6026Bool(fields, ps6056GroupedPromotionField)
	if shortUsed && shortProcessesOK && shortProcesses <= 1 {
		warnings = append(warnings, "single-process short screen is used directly for promotion")
	}
	if groupedUsed && (blockedGroupedOrder || groupedProcessesOK && groupedProcesses <= 1) {
		warnings = append(warnings, "blocked same-process grouped screen is used directly for promotion")
	}
	if selected, ok := ps6026Bool(fields, ps6056SelectedField); ok && selected && !passed {
		warnings = append(warnings, "fusion candidate is selected without a passing independent alternating promotion gate")
	}
	if decision, ok := ps6027String(fields, ps6056DecisionField); ok && !passed && ps6007ContainsAny(ps6030StatusName(decision), "retain", "promote", "ship", "selected") {
		warnings = append(warnings, fmt.Sprintf("final decision %q retains a candidate below the independent promotion gate", decision))
	}
	return warnings
}

func ps6056AuditMedians(fields map[string]ps6016Field, scope string, control []float64, controlOK bool, candidate []float64, candidateOK bool, warnings *[]string) (float64, bool) {
	if !controlOK || !candidateOK || len(control) == 0 || len(candidate) == 0 {
		return 0, false
	}
	controlMedian := ps6016Median(control)
	candidateMedian := ps6016Median(candidate)
	medianField := ps6056GroupedMedianField
	ratioField := ps6056GroupedRatioField
	if scope == "promotion" {
		medianField = ps6056PromotionMedianField
		ratioField = ps6056PromotionRatioField
	}
	if recorded, ok := ps6016Number(fields, func(n string) bool { return medianField(n, "control") }); ok && !ps6025Close(recorded, controlMedian) {
		*warnings = append(*warnings, fmt.Sprintf("%s control median %.6g disagrees with raw-sample median %.6g", scope, recorded, controlMedian))
	}
	if recorded, ok := ps6016Number(fields, func(n string) bool { return medianField(n, "candidate") }); ok && !ps6025Close(recorded, candidateMedian) {
		*warnings = append(*warnings, fmt.Sprintf("%s candidate median %.6g disagrees with raw-sample median %.6g", scope, recorded, candidateMedian))
	}
	if candidateMedian <= 0 {
		return 0, false
	}
	ratio := controlMedian / candidateMedian
	if recorded, ok := ps6016Number(fields, ratioField); ok && !ps6025Close(recorded, ratio) {
		*warnings = append(*warnings, fmt.Sprintf("%s control/candidate ratio %.6gx disagrees with %.6gx", scope, recorded, ratio))
	}
	return ratio, true
}

func ps6056BlockedOrder(order []string) bool {
	seenControl, seenCandidate := false, false
	for _, label := range order {
		normalized := ps6030StatusName(label)
		control := strings.Contains(normalized, "control")
		candidate := strings.Contains(normalized, "candidate")
		if control && seenCandidate || !control && !candidate {
			return false
		}
		seenControl = seenControl || control
		seenCandidate = seenCandidate || candidate
	}
	return seenControl && seenCandidate
}

func ps6056Alternates(order []string) bool {
	if len(order) < 2 {
		return false
	}
	previous := ""
	for _, label := range order {
		normalized := ps6030StatusName(label)
		current := ""
		switch {
		case strings.Contains(normalized, "controlcandidate"):
			current = "controlcandidate"
		case strings.Contains(normalized, "candidatecontrol"):
			current = "candidatecontrol"
		default:
			return false
		}
		if current == previous {
			return false
		}
		previous = current
	}
	return true
}
