package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6062 implements owner issue #779. It guards fused floating-point graph
// rewrites with exact forward/VJP semantics and workload-level leverage.
var PS6062 = register(&lint.Check{
	ID:       "PS6062",
	Category: "verify",
	Slug:     "floating-fusion-needs-exact-vjp-workload-gate",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a fused floating graph lacks exact forward/VJP and workload leverage gates",
		Text: `Algebraically equivalent floating-point graphs need not have the
same machine contract. Fusion and reassociation can change intermediate
rounding, contraction, overflow class, signed zero, NaN behavior, and the
operation order of a reverse-mode tape. A closed-form derivative can therefore
differ by one ULP from the established VJP even when both are mathematically
correct.

This check implements owner issue #779 in two complementary ways.

First, a function explicitly named as fused/reassociated/combined floating
loss, gradient, backward, VJP, Abs, ReLU, or Smooth-L1 code is reported when it
contains a multi-operation floating chain. A same-type float conversion or an
intermediate assignment in that source is reported separately as a possible
rounding barrier: do not remove it as redundant until contraction-on/off
raw-bit tests prove that the established contract survives. An
//perfscan:exact-floating-fusion-validated annotation suppresses source that is
already guarded by the evidence below.

Second, Test/Benchmark promotion harnesses with fused floating forward, VJP or
backward, leaf, and workload context must provide an
ExactFloatingFusionGate, FusedForwardVJPEvidence,
FloatingGraphParityEvidence, FusionContractionValidation,
ElementwiseFusionPromotion, or equivalent manifest containing:

  - hardware and exact workload identity;
  - raw-bit forward and VJP/backward oracle results;
  - coverage of signed zero, infinities, quiet NaNs, signaling NaNs, and finite
    extremes, plus a representative large parallel shape;
  - explicit rounding-barrier/contraction validation;
  - same-binary and order-alternating campaign status;
  - leaf, complete-workload candidate, and unchanged-control ratios; and
  - a frozen workload promotion threshold.

The oracle must reproduce the old graph's operation order, not merely a
symbolic derivative: materialize each multiply, add fan-out contributions in
tape order, apply ReLU/Abs selects, and accumulate in the established order.
For a leaf speedup S and workload target G, the audit computes the effective
fraction required by Amdahl's law,

  p = (1 - 1/G) / (1 - 1/S),

and compares it with the fraction implied by the measured workload median. A
shortfall blocks a leaf-leverage claim and explicitly directs the next
experiment to the adjacent fusion boundary. Order-alternating same-binary
samples separate a real low-single-digit gain from process and order drift.

There is NO automatic fix. Operation order, required rounding barriers,
parallel tape semantics, promotion thresholds, and the next safe fusion
boundary are project and workload contracts, not syntax substitutions.`,
		Before: `// Algebraically equivalent, but not necessarily raw-bit equal:
grad := closedFormSmoothL1Derivative(pred, target)
if leafRatio >= gate { promote() }`,
		After: `gate := FusedForwardVJPEvidence{
	ForwardRawBitsPassed: true, VJPRawBitsPassed: true,
	RawBitCases: []string{"signed zero", "infinities", "quiet NaNs", "signaling NaNs", "finite extremes"},
	RoundingBarriersValidated: true,
	SameBinary: true, OrderAlternating: true,
	LeafRatios: leaf, WorkloadCandidateRatios: workload,
	UnchangedControlRatios: control, WorkloadPromotionThreshold: 1.03,
}
// Reproduce tape order exactly; use Amdahl shortfall to choose the next seam.`,
		MeasuredWin: `In the owner EAGLE Smooth-L1 campaign, an Abs-only leaf
change produced only about 1.02x-1.03x at the real workload, below the frozen
1.03 gate. Fusing the full chain reached 1.41x-1.92x forward and 3.02x-3.60x
forward+backward medians across three independent -benchtime=100x -count=7
campaigns. Exact same-binary oracles found one-ULP closed-form VJP mismatches;
reproducing tape order and retaining explicit float32/float64 rounding
barriers restored bit-for-bit parity on special values and 300K-element
parallel tensors.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6062",
		Doc:  "fused floating graph lacks exact forward/VJP and same-binary workload leverage gates",
		Run:  runPS6062,
	},
})

type ps6062SourceFinding struct {
	barrier ast.Node
	kind    string
}

type ps6062Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

type ps6062Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6062Axes = []ps6062Axis{
	{name: "hardware identity", present: func(fields map[string]ps6016Field) bool {
		return ps6016HasName(fields, func(name string) bool { return ps6007ContainsAny(name, "hardware", "device", "cpu", "accelerator") })
	}},
	{name: "workload identity", present: func(fields map[string]ps6016Field) bool {
		return ps6016HasName(fields, func(name string) bool { return ps6007ContainsAny(name, "workload", "model", "graph", "operation") })
	}},
	{name: "forward raw-bit oracle", present: func(fields map[string]ps6016Field) bool { return ps6016HasName(fields, ps6062ForwardField) }},
	{name: "VJP/backward raw-bit oracle", present: func(fields map[string]ps6016Field) bool { return ps6016HasName(fields, ps6062VJPField) }},
	{name: "signed-zero coverage", present: func(fields map[string]ps6016Field) bool {
		return ps6062HasText(fields, func(text string) bool {
			return strings.Contains(text, "signedzero") || strings.Contains(text, "bothzerosigns")
		})
	}},
	{name: "infinity coverage", present: func(fields map[string]ps6016Field) bool {
		return ps6062HasText(fields, func(text string) bool {
			return strings.Contains(text, "infinity") || strings.Contains(text, "infinities")
		})
	}},
	{name: "quiet-NaN coverage", present: func(fields map[string]ps6016Field) bool {
		return ps6062HasText(fields, func(text string) bool { return strings.Contains(text, "quietnan") || strings.Contains(text, "qnan") })
	}},
	{name: "signaling-NaN coverage", present: func(fields map[string]ps6016Field) bool {
		return ps6062HasText(fields, func(text string) bool {
			return strings.Contains(text, "signalingnan") || strings.Contains(text, "snan")
		})
	}},
	{name: "finite-extreme coverage", present: func(fields map[string]ps6016Field) bool {
		return ps6062HasText(fields, func(text string) bool {
			return strings.Contains(text, "finiteextreme") || strings.Contains(text, "maxfinite")
		})
	}},
	{name: "large parallel shape", present: func(fields map[string]ps6016Field) bool { return ps6016HasName(fields, ps6062LargeShapeField) }},
	{name: "rounding-barrier validation", present: func(fields map[string]ps6016Field) bool { return ps6016HasName(fields, ps6062RoundingField) }},
	{name: "same-binary status", present: func(fields map[string]ps6016Field) bool { return ps6016HasName(fields, ps6062SameBinaryField) }},
	{name: "order-alternating status", present: func(fields map[string]ps6016Field) bool { return ps6016HasName(fields, ps6062AlternatingField) }},
	{name: "leaf ratio distribution", present: func(fields map[string]ps6016Field) bool { return ps6016HasName(fields, ps6062LeafField) }},
	{name: "workload candidate distribution", present: func(fields map[string]ps6016Field) bool { return ps6016HasName(fields, ps6062CandidateField) }},
	{name: "unchanged-control distribution", present: func(fields map[string]ps6016Field) bool { return ps6016HasName(fields, ps6062ControlField) }},
	{name: "frozen workload promotion threshold", present: func(fields map[string]ps6016Field) bool { return ps6016HasName(fields, ps6062PromotionField) }},
}

func runPS6062(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			if finding, ok := ps6062FusedSource(pass, function); ok && !ps6062ValidatedAnnotation(file, function) {
				pass.Reportf(function.Name.Pos(), "fused/reassociated floating graph %s is mathematically plausible but not necessarily machine-equivalent: preserve established forward and tape-order VJP operation order, same-type conversion/store rounding barriers, signed zero, infinities, NaNs, and finite extremes; require raw-bit forward+VJP oracles on large parallel shapes and an order-alternating same-binary workload gate before claiming leaf leverage (advisory, no automatic fix)", function.Name.Name)
				if finding.barrier != nil {
					pass.Reportf(finding.barrier.Pos(), "%s is a possible required floating rounding barrier in fused graph %s; do not remove it as redundant until contraction-on/off raw-bit forward and tape-order VJP oracles prove parity", finding.kind, function.Name.Name)
				}
			}
			if !ps6021Harness(pass, function) || !ps6062EvidenceContext(pass, function) {
				continue
			}
			manifest, found := ps6062BestManifest(pass, function.Body)
			if !found {
				pass.Reportf(function.Name.Pos(), "fused floating forward/VJP workload campaign has no exact parity and leverage manifest; missing %s", strings.Join(ps6062Missing(nil), ", "))
				continue
			}
			if missing := ps6062Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "floating fusion parity/leverage manifest is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if summary, warnings := ps6062Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "floating fusion exactness/workload gate fails: %s; %s", summary, strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6062FusedSource(pass *analysis.Pass, function *ast.FuncDecl) (ps6062SourceFinding, bool) {
	name := ps6007NormalizeName(function.Name.Name)
	if !ps6007ContainsAny(name, "fused", "fusion", "reassociated", "reassociation", "combined", "merged") ||
		!ps6007ContainsAny(name, "loss", "smooth", "vjp", "gradient", "backward", "grad", "abs", "relu") {
		return ps6062SourceFinding{}, false
	}
	floatOperations := 0
	var conversion ast.Node
	var store ast.Node
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.BinaryExpr:
			if ps6062Arithmetic(value.Op) && ps6062FloatType(pass.TypesInfo.TypeOf(value)) {
				floatOperations++
			}
		case *ast.CallExpr:
			if conversion == nil && ps6062SameTypeFloatConversion(pass, value) {
				conversion = value
			}
		case *ast.AssignStmt:
			if store == nil && ps6062IntermediateFloatStore(pass, value) {
				store = value
			}
		}
		return true
	})
	if floatOperations < 2 {
		return ps6062SourceFinding{}, false
	}
	if conversion != nil {
		return ps6062SourceFinding{barrier: conversion, kind: "same-type float conversion"}, true
	}
	if store != nil {
		return ps6062SourceFinding{barrier: store, kind: "intermediate typed float store"}, true
	}
	return ps6062SourceFinding{}, true
}

func ps6062Arithmetic(operator token.Token) bool {
	return operator == token.ADD || operator == token.SUB || operator == token.MUL || operator == token.QUO
}

func ps6062FloatType(value types.Type) bool {
	if value == nil {
		return false
	}
	basic, ok := value.Underlying().(*types.Basic)
	return ok && (basic.Kind() == types.Float32 || basic.Kind() == types.Float64)
}

func ps6062SameTypeFloatConversion(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return false
	}
	typeValue, ok := pass.TypesInfo.Types[ps2110Unparen(call.Fun)]
	if !ok || !typeValue.IsType() || !ps6062FloatType(typeValue.Type) {
		return false
	}
	argumentType := pass.TypesInfo.TypeOf(call.Args[0])
	return argumentType != nil && types.Identical(types.Default(typeValue.Type), types.Default(argumentType))
}

func ps6062IntermediateFloatStore(pass *analysis.Pass, assignment *ast.AssignStmt) bool {
	if len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return false
	}
	identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	if !ok || identifier.Name == "_" || !ps6062FloatType(pass.TypesInfo.TypeOf(identifier)) {
		return false
	}
	_, arithmetic := ps2110Unparen(assignment.Rhs[0]).(*ast.BinaryExpr)
	return arithmetic && ps6062FloatType(pass.TypesInfo.TypeOf(assignment.Rhs[0]))
}

func ps6062ValidatedAnnotation(file *ast.File, function *ast.FuncDecl) bool {
	for _, group := range file.Comments {
		if group != function.Doc && !(group.End() <= function.Pos() && function.Pos()-group.End() <= 3) && !(group.Pos() >= function.Body.Pos() && group.End() <= function.Body.End()) {
			continue
		}
		for _, comment := range group.List {
			text := ps6058Compact(comment.Text)
			if strings.Contains(text, "perfscanexactfloatingfusionvalidated") || strings.Contains(text, "perfscanexactvjpvalidated") {
				return true
			}
		}
	}
	return false
}

func ps6062BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6062Manifest, bool) {
	var best ps6062Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		literal, ok := node.(*ast.CompositeLit)
		if !ok || !ps6062ManifestType(literal.Type) {
			return true
		}
		manifest := ps6062Manifest{lit: literal, fields: ps6016Fields(pass, literal)}
		score := len(ps6062Axes) - len(ps6062Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6062ManifestType(expression ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6062ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6062ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return strings.Contains(name, "exactfloatingfusion") ||
		strings.Contains(name, "fusedforwardvjp") ||
		strings.Contains(name, "floatinggraphparity") ||
		strings.Contains(name, "fusioncontractionvalidation") ||
		strings.Contains(name, "elementwisefusionpromotion")
}

func ps6062Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6062Axes))
	for _, axis := range ps6062Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6062HasText(fields map[string]ps6016Field, predicate func(string) bool) bool {
	for name, field := range fields {
		if predicate(name) {
			return true
		}
		if field.hasString && predicate(ps6058Compact(field.stringVal)) {
			return true
		}
		for _, value := range field.stringValues {
			if predicate(ps6058Compact(value)) {
				return true
			}
		}
	}
	return false
}

func ps6062ForwardField(name string) bool {
	return strings.Contains(name, "forward") && ps6007ContainsAny(name, "rawbit", "bitexact", "exactbits", "parity")
}

func ps6062VJPField(name string) bool {
	return ps6007ContainsAny(name, "vjp", "backward", "gradient") && ps6007ContainsAny(name, "rawbit", "bitexact", "exactbits", "parity")
}

func ps6062LargeShapeField(name string) bool {
	return ps6007ContainsAny(name, "largeparallel", "parallelshape", "largetensor") && ps6007ContainsAny(name, "passed", "covered", "elements", "size", "shape")
}

func ps6062RoundingField(name string) bool {
	return ps6007ContainsAny(name, "roundingbarrier", "contractionbarrier", "explicitrounding") && ps6007ContainsAny(name, "passed", "validated", "status", "covered")
}

func ps6062SameBinaryField(name string) bool {
	return strings.Contains(name, "samebinary") && ps6007ContainsAny(name, "passed", "status", "used", "enabled", "comparison", "gate")
}

func ps6062AlternatingField(name string) bool {
	return ps6007ContainsAny(name, "orderalternating", "alternatingorder", "pairedalternating") && ps6007ContainsAny(name, "passed", "status", "used", "enabled", "gate")
}

func ps6062LeafField(name string) bool {
	return strings.Contains(name, "leaf") && ps6007ContainsAny(name, "ratio", "speedup", "sample", "distribution")
}

func ps6062CandidateField(name string) bool {
	return ps6007ContainsAny(name, "workload", "forwardbackward", "completeoperation", "endtoend") && strings.Contains(name, "candidate") && ps6007ContainsAny(name, "ratio", "speedup", "sample", "distribution")
}

func ps6062ControlField(name string) bool {
	return strings.Contains(name, "control") && ps6007ContainsAny(name, "ratio", "sample", "distribution") && !ps6007ContainsAny(name, "maximum", "minimum", "limit", "threshold")
}

func ps6062PromotionField(name string) bool {
	return ps6007ContainsAny(name, "workload", "endtoend", "completeoperation") && ps6007ContainsAny(name, "promotion", "minimum", "required", "threshold", "gate")
}

func ps6062Audit(fields map[string]ps6016Field) (string, []string) {
	var warnings []string
	booleanGates := []struct {
		label     string
		predicate func(string) bool
	}{
		{label: "forward raw-bit oracle", predicate: ps6062ForwardField},
		{label: "VJP/backward raw-bit oracle", predicate: ps6062VJPField},
		{label: "rounding-barrier validation", predicate: ps6062RoundingField},
		{label: "same-binary gate", predicate: ps6062SameBinaryField},
		{label: "order-alternating gate", predicate: ps6062AlternatingField},
	}
	for _, gate := range booleanGates {
		if value, known := ps6062Bool(fields, gate.predicate); known && !value {
			warnings = append(warnings, gate.label+" explicitly fails")
		}
	}
	if elements, ok := ps6016Number(fields, ps6062LargeShapeField); ok && elements <= 0 {
		warnings = append(warnings, "large parallel shape must contain a positive element count")
	}

	leaf, leafOK := ps6017Ratios(fields, ps6062LeafField)
	candidates, candidatesOK := ps6017Ratios(fields, ps6062CandidateField)
	controls, controlsOK := ps6017Ratios(fields, ps6062ControlField)
	promotion, promotionOK := ps6016Number(fields, ps6062PromotionField)
	for _, campaign := range []struct {
		label   string
		values  []float64
		present bool
	}{
		{label: "leaf", values: leaf, present: leafOK},
		{label: "complete-workload candidate", values: candidates, present: candidatesOK},
		{label: "unchanged-control", values: controls, present: controlsOK},
	} {
		if campaign.present && len(campaign.values) < 3 {
			warnings = append(warnings, campaign.label+" campaign has fewer than three independent invocations")
		}
		if campaign.present && (len(campaign.values) == 0 || slices.Min(campaign.values) <= 0) {
			warnings = append(warnings, campaign.label+" ratios must be positive")
		}
	}
	if promotionOK && promotion <= 1 {
		warnings = append(warnings, "workload promotion threshold must be greater than 1x")
	}
	if candidatesOK && len(candidates) > 0 && promotionOK {
		if lowest := slices.Min(candidates); lowest < promotion {
			warnings = append(warnings, fmt.Sprintf("complete-workload invocation %.4gx misses frozen %.4gx promotion threshold", lowest, promotion))
		}
	}

	summary := "dynamic evidence; Amdahl effective-fraction shortfall cannot be computed statically"
	if leafOK && len(leaf) > 0 && slices.Min(leaf) > 1 && candidatesOK && len(candidates) > 0 && slices.Min(candidates) > 0 {
		leafMedian := ps6016Median(leaf)
		workloadMedian := ps6016Median(candidates)
		observed := ps6062EffectiveFraction(leafMedian, workloadMedian)
		summary = fmt.Sprintf("leaf median %.4gx and workload median %.4gx imply %.2f%% effective optimized fraction", leafMedian, workloadMedian, observed*100)
		if promotionOK && promotion > 1 {
			required := ps6062EffectiveFraction(leafMedian, promotion)
			switch {
			case required > 1:
				warnings = append(warnings, fmt.Sprintf("leaf speedup cannot reach the %.4gx workload gate even at 100%% coverage; prioritize a wider adjacent fusion boundary", promotion))
			case workloadMedian < promotion && required > observed:
				warnings = append(warnings, fmt.Sprintf("Amdahl effective-fraction shortfall is %.2f percentage points (%.2f%% observed versus %.2f%% required); prioritize the next adjacent fusion boundary instead of claiming leaf leverage", (required-observed)*100, observed*100, required*100))
			}
		}
	}
	if controlsOK && len(controls) > 0 && slices.Min(controls) > 0 {
		spread := slices.Max(controls)/slices.Min(controls) - 1
		summary += fmt.Sprintf("; unchanged-control spread %.2f%%", spread*100)
	}
	return summary, warnings
}

func ps6062Bool(fields map[string]ps6016Field, predicate func(string) bool) (bool, bool) {
	var result bool
	found := false
	for name, field := range fields {
		if !predicate(name) || !field.hasBool {
			continue
		}
		if found && result != field.boolVal {
			return false, false
		}
		result, found = field.boolVal, true
	}
	return result, found
}

func ps6062EffectiveFraction(leafSpeedup, workloadSpeedup float64) float64 {
	denominator := 1 - 1/leafSpeedup
	if denominator <= 0 || workloadSpeedup <= 0 {
		return 0
	}
	//perfscan:ignore PS5001 Amdahl evidence keeps the specified division; reciprocal multiplication is not bit-identical
	return (1 - 1/workloadSpeedup) / denominator
}

func ps6062EvidenceContext(pass *analysis.Pass, function *ast.FuncDecl) bool {
	fusion, floating, forward, reverse, leaf, workload := false, false, false, false, false, false
	classify := func(text string) {
		text = ps6007NormalizeName(text)
		fusion = fusion || ps6007ContainsAny(text, "fused", "fusion", "reassociated", "combined")
		floating = floating || ps6007ContainsAny(text, "f32", "float32", "f64", "float64", "floatingpoint")
		forward = forward || strings.Contains(text, "forward")
		reverse = reverse || ps6007ContainsAny(text, "vjp", "backward", "gradient")
		leaf = leaf || strings.Contains(text, "leaf")
		workload = workload || ps6007ContainsAny(text, "workload", "endtoend", "completeoperation", "forwardbackward")
	}
	classify(function.Name.Name)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if fusion && floating && forward && reverse && leaf && workload {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.Ident:
			classify(value.Name)
		case *ast.BasicLit:
			classify(value.Value)
		case *ast.CallExpr:
			if function, _, ok := typedCallee(pass, value.Fun); ok {
				classify(function.Name())
			}
		}
		return true
	})
	return fusion && floating && forward && reverse && leaf && workload
}
