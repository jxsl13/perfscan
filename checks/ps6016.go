package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6016 implements owner issue #769: Amdahl-aware validation for
// dispatch-eliminating accelerator fusions.
var PS6016 = register(&lint.Check{
	ID:       "PS6016",
	Category: "verify",
	Slug:     "fusion-gate-misses-amdahl-ceiling",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a dispatch-eliminating fusion gate ignores its Amdahl ceiling or end-to-end regressions",
		Text: `A bit-exact leaf fusion can remove every dispatch of a small stage
while producing only a noisy low-single-digit graph gain. Leaf ratios,
dispatch counts, and graph-level leverage answer different questions. A
promotion threshold near the stage's theoretical Amdahl ceiling leaves no room
for control noise, and decode-only evidence can hide prefill regressions.

This check implements owner issue #769. It finds benchmark/promotion harnesses
with accelerator, fusion, dispatch-elimination, and Amdahl/end-to-end context,
then requires an AmdahlFusionGate, FusionAmdahlEvidence,
DispatchFusionValidation, GraphLeverageGate, EndToEndFusionGate, or equivalent
manifest containing:

  - hardware and workload;
  - profiled stage share;
  - dispatch/encoder counts before and after;
  - repeated leaf ratios;
  - repeated end-to-end decode candidate and unchanged-control ratios;
  - a frozen decode promotion ratio;
  - repeated short-prefill ratios and a separate regression minimum;
  - repeated long-prefill ratios and a separate regression minimum; and
  - exactness/parity status.

For compile-time evidence the analyzer computes:

  - the full-removal graph ceiling: 1/(1-stageShare);
  - observed decode median and unchanged-control max/min spread;
  - implied removed-stage fraction: 1-1/observedSpeedup; and
  - leaf-versus-end-to-end transfer.

It warns when any invocation misses its frozen decode or prefill gate, when the
decode threshold's Amdahl headroom is no larger than control spread, when leaf
gain exceeds graph gain beyond that spread, when campaign arrays have fewer
than three independent invocations, or when dispatch and exactness evidence is
invalid. StageShare may be a fraction or an explicitly named percent field.

There is NO automatic fix. Stage attribution, campaign independence, frozen
thresholds, and whether to revert a kernel are measured policy decisions, not
syntax transformations.`,
		Before: `gate := AmdahlFusionGate{
	StageShare: 0.038,
	DispatchBefore: 44, DispatchAfter: 0,
	LeafRatios: []float64{1.023, 1.013},
	DecodeRatios: []float64{1.0235, 1.0206, 1.0140},
	DecodeRequired: 1.02,
}`,
		After: `gate := AmdahlFusionGate{
	Hardware: "...", Workload: "...",
	StageShare: 0.038,
	DispatchBefore: 44, DispatchAfter: 0,
	LeafRatios: leaf,
	DecodeCandidateRatios: decode,
	UnchangedControlRatios: controls,
	DecodeRequiredRatio: 1.02,
	ShortPrefillRatios: pp64, ShortPrefillMinimum: 0.99,
	LongPrefillRatios: pp512, LongPrefillMinimum: 0.99,
	ExactnessPassed: true,
}
// Reject if any independent gate fails or ceiling headroom is noise-sized.`,
		MeasuredWin: `In the issue #769 Metal experiment, removing all 44
binary.add encoders preserved 76/76 greedy tokens and 32,000 checked logits.
The stage represented only 3.8% of explicit event time, giving a roughly 1.040x
full-removal ceiling. Leaf gains were 1.013x-1.023x; three trained tg64
invocations measured 1.0235x, 1.0206x, and 1.0140x, while pp64 included a
0.9876x regression against a 0.99x floor. The candidate was rejected.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6016",
		Doc:  "dispatch fusion promotion evidence lacks Amdahl-aware graph gates",
		Run:  runPS6016,
	},
})

type ps6016Field struct {
	number          float64
	hasNumber       bool
	numbers         []float64
	hasNumbers      bool
	stringVal       string
	hasString       bool
	stringValues    []string
	hasStringValues bool
	boolVal         bool
	hasBool         bool
}

type ps6016Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

type ps6016Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6016Axes = []ps6016Axis{
	{name: "hardware", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6007ContainsAny(n, "hardware", "device", "accelerator") })
	}},
	{name: "workload", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6007ContainsAny(n, "workload", "model", "graph", "campaign") })
	}},
	{name: "profiled stage share", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6016StageShareField)
	}},
	{name: "dispatch count before", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6016DispatchBeforeField)
	}},
	{name: "dispatch count after", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6016DispatchAfterField)
	}},
	{name: "leaf ratio distribution", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6016LeafField)
	}},
	{name: "decode candidate distribution", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6016DecodeField)
	}},
	{name: "unchanged-control distribution", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6016ControlField)
	}},
	{name: "decode promotion ratio", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6016DecodeGateField)
	}},
	{name: "short-prefill distribution", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6016ShortPrefillField)
	}},
	{name: "short-prefill regression minimum", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6016ShortPrefillGateField)
	}},
	{name: "long-prefill distribution", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6016LongPrefillField)
	}},
	{name: "long-prefill regression minimum", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6016LongPrefillGateField)
	}},
	{name: "exactness status", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, ps6016ExactnessField)
	}},
}

func runPS6016(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6011Harness(pass, fn) || !ps6016Context(pass, fn) {
				continue
			}
			manifest, found := ps6016BestManifest(pass, fn.Body)
			if !found {
				if ps6017HasManifest(fn.Body) {
					continue
				}
				pass.Reportf(fn.Name.Pos(), "dispatch-eliminating accelerator fusion has no Amdahl-aware graph gate manifest; missing %s", strings.Join(ps6016Missing(nil), ", "))
				continue
			}
			if missing := ps6016Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "Amdahl fusion gate manifest is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			summary, warnings := ps6016Audit(manifest.fields)
			if len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "Amdahl fusion gate fails: %s; %s", summary, strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6016BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6016Manifest, bool) {
	var best ps6016Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6016ManifestType(lit.Type) {
			return true
		}
		manifest := ps6016Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6016Axes) - len(ps6016Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6016ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6016ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6016ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return strings.Contains(name, "amdahlfusion") ||
		strings.Contains(name, "fusionamdahlevidence") ||
		strings.Contains(name, "dispatchfusionvalidation") ||
		strings.Contains(name, "graphleveragegate") ||
		strings.Contains(name, "endtoendfusiongate")
}

func ps6016Fields(pass *analysis.Pass, lit *ast.CompositeLit) map[string]ps6016Field {
	fields := make(map[string]ps6016Field, len(lit.Elts))
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := ps2110Unparen(kv.Key).(*ast.Ident)
		if !ok {
			continue
		}
		field := ps6016Field{}
		expr := ps2110Unparen(kv.Value)
		if tv, ok := pass.TypesInfo.Types[expr]; ok && tv.Value != nil {
			switch tv.Value.Kind() {
			case constant.String:
				field.stringVal, field.hasString = constant.StringVal(tv.Value), true
			case constant.Bool:
				field.boolVal, field.hasBool = constant.BoolVal(tv.Value), true
			case constant.Int, constant.Float:
				field.number, field.hasNumber = ps6014Float(tv.Value)
			}
		}
		if values, ok := ps6016NumberSequence(pass, expr); ok {
			field.numbers, field.hasNumbers = values, true
		}
		if values, ok := ps6016StringSequence(pass, expr); ok {
			field.stringValues, field.hasStringValues = values, true
		}
		fields[ps6007NormalizeName(key.Name)] = field
	}
	return fields
}

func ps6016NumberSequence(pass *analysis.Pass, expr ast.Expr) ([]float64, bool) {
	lit, ok := ps2110Unparen(expr).(*ast.CompositeLit)
	if !ok {
		return nil, false
	}
	values := make([]float64, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		value := ast.Expr(elt)
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			value = kv.Value
		}
		tv, ok := pass.TypesInfo.Types[ps2110Unparen(value)]
		if !ok || tv.Value == nil || (tv.Value.Kind() != constant.Int && tv.Value.Kind() != constant.Float) {
			return nil, false
		}
		number, ok := ps6014Float(tv.Value)
		if !ok {
			return nil, false
		}
		values = append(values, number)
	}
	return values, true
}

func ps6016StringSequence(pass *analysis.Pass, expr ast.Expr) ([]string, bool) {
	lit, ok := ps2110Unparen(expr).(*ast.CompositeLit)
	if !ok {
		return nil, false
	}
	values := make([]string, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		value := ast.Expr(elt)
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			value = kv.Value
		}
		tv, ok := pass.TypesInfo.Types[ps2110Unparen(value)]
		if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
			return nil, false
		}
		values = append(values, constant.StringVal(tv.Value))
	}
	return values, true
}

func ps6016Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6016Axes))
	for _, axis := range ps6016Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6016HasName(fields map[string]ps6016Field, predicate func(string) bool) bool {
	for name := range fields {
		if predicate(name) {
			return true
		}
	}
	return false
}

func ps6016StageShareField(name string) bool {
	return strings.Contains(name, "share") && ps6007ContainsAny(name, "stage", "profiled", "event")
}

func ps6016DispatchBeforeField(name string) bool {
	return ps6007ContainsAny(name, "dispatch", "encoder", "launch") && ps6007ContainsAny(name, "before", "control", "split")
}

func ps6016DispatchAfterField(name string) bool {
	return ps6007ContainsAny(name, "dispatch", "encoder", "launch") && ps6007ContainsAny(name, "after", "candidate", "fused")
}

func ps6016LeafField(name string) bool {
	return strings.Contains(name, "leaf") && ps6007ContainsAny(name, "ratio", "sample", "distribution", "speedup")
}

func ps6016DecodeField(name string) bool {
	return ps6007ContainsAny(name, "decode", "tg") && ps6007ContainsAny(name, "candidate", "ratio", "sample", "distribution")
}

func ps6016ControlField(name string) bool {
	return strings.Contains(name, "control") && ps6007ContainsAny(name, "ratio", "sample", "distribution", "spread")
}

func ps6016DecodeGateField(name string) bool {
	return ps6007ContainsAny(name, "decode", "tg") && ps6007ContainsAny(name, "required", "minimum", "threshold", "gate")
}

func ps6016ShortPrefillField(name string) bool {
	return ps6007ContainsAny(name, "shortprefill", "prefillshort", "pp64") && ps6007ContainsAny(name, "ratio", "sample", "distribution")
}

func ps6016ShortPrefillGateField(name string) bool {
	return ps6007ContainsAny(name, "shortprefill", "prefillshort", "pp64") && ps6007ContainsAny(name, "minimum", "required", "threshold", "gate")
}

func ps6016LongPrefillField(name string) bool {
	return ps6007ContainsAny(name, "longprefill", "prefilllong", "pp512") && ps6007ContainsAny(name, "ratio", "sample", "distribution")
}

func ps6016LongPrefillGateField(name string) bool {
	return ps6007ContainsAny(name, "longprefill", "prefilllong", "pp512") && ps6007ContainsAny(name, "minimum", "required", "threshold", "gate")
}

func ps6016ExactnessField(name string) bool {
	return ps6007ContainsAny(name, "exactness", "parity", "bitexact") && ps6007ContainsAny(name, "passed", "status", "gate", "result")
}

func ps6016Audit(fields map[string]ps6016Field) (string, []string) {
	var warnings []string
	share, shareOK := ps6016StageShare(fields)
	before, beforeOK := ps6016Number(fields, ps6016DispatchBeforeField)
	after, afterOK := ps6016Number(fields, ps6016DispatchAfterField)
	leaf, leafOK := ps6016Numbers(fields, ps6016LeafField)
	decode, decodeOK := ps6016Numbers(fields, ps6016DecodeField)
	controls, controlsOK := ps6016Numbers(fields, ps6016ControlField)
	decodeGate, decodeGateOK := ps6016Number(fields, ps6016DecodeGateField)
	shortPrefill, shortOK := ps6016Numbers(fields, ps6016ShortPrefillField)
	shortGate, shortGateOK := ps6016Number(fields, ps6016ShortPrefillGateField)
	longPrefill, longOK := ps6016Numbers(fields, ps6016LongPrefillField)
	longGate, longGateOK := ps6016Number(fields, ps6016LongPrefillGateField)

	if beforeOK && afterOK && (before <= 0 || after < 0 || after >= before) {
		warnings = append(warnings, fmt.Sprintf("dispatch/encoder count does not validly decrease (%.4g -> %.4g)", before, after))
	}
	if failed, known := ps6016ExactnessFailed(fields); known && failed {
		warnings = append(warnings, "exactness/parity gate explicitly fails")
	}
	campaigns := []struct {
		label   string
		samples []float64
	}{
		{label: "leaf", samples: leaf},
		{label: "decode", samples: decode},
		{label: "unchanged-control", samples: controls},
		{label: "short-prefill", samples: shortPrefill},
		{label: "long-prefill", samples: longPrefill},
	}
	for _, campaign := range campaigns {
		if campaign.samples != nil && len(campaign.samples) < 3 {
			warnings = append(warnings, campaign.label+" campaign has fewer than three independent invocations")
		}
	}
	if decodeOK && decodeGateOK {
		if value, ok := ps6016Below(decode, decodeGate); ok {
			warnings = append(warnings, fmt.Sprintf("decode invocation %.4gx misses frozen %.4gx gate", value, decodeGate))
		}
	}
	if shortOK && shortGateOK {
		if value, ok := ps6016Below(shortPrefill, shortGate); ok {
			warnings = append(warnings, fmt.Sprintf("short-prefill invocation %.4gx misses %.4gx regression minimum", value, shortGate))
		}
	}
	if longOK && longGateOK {
		if value, ok := ps6016Below(longPrefill, longGate); ok {
			warnings = append(warnings, fmt.Sprintf("long-prefill invocation %.4gx misses %.4gx regression minimum", value, longGate))
		}
	}

	summary := "dynamic evidence; Amdahl quantities cannot be computed statically"
	if shareOK && share > 0 && share < 1 && decodeOK && len(decode) > 0 {
		ceiling := 1 / (1 - share)
		observed := ps6016Median(decode)
		implied := 1 - 1/observed
		spread := 0.0
		if controlsOK && len(controls) > 0 {
			minimum, maximum := slices.Min(controls), slices.Max(controls)
			if minimum > 0 {
				spread = maximum/minimum - 1
			}
		}
		summary = fmt.Sprintf("profiled share %.2f%% gives full-removal ceiling %.4gx; observed decode median %.4gx implies %.2f%% removed-stage fraction; unchanged-control spread %.2f%%", share*100, ceiling, observed, implied*100, spread*100)
		if decodeGateOK && ceiling-decodeGate <= spread {
			warnings = append(warnings, fmt.Sprintf("decode threshold %.4gx is within control spread of the %.4gx theoretical ceiling", decodeGate, ceiling))
		}
		if leafOK && len(leaf) > 0 {
			leafMedian := ps6016Median(leaf)
			if leafMedian > observed+spread {
				warnings = append(warnings, fmt.Sprintf("leaf median %.4gx does not transfer to end-to-end decode median %.4gx beyond control spread", leafMedian, observed))
			}
		}
		if implied+spread < share {
			warnings = append(warnings, fmt.Sprintf("observed ratio implies %.2f%% removed fraction versus %.2f%% profiled stage share", implied*100, share*100))
		}
	} else if shareOK && (share <= 0 || share >= 1) {
		warnings = append(warnings, "profiled stage share must be strictly between zero and one")
	}
	return summary, warnings
}

func ps6016StageShare(fields map[string]ps6016Field) (float64, bool) {
	var result float64
	found := false
	for name, field := range fields {
		if !ps6016StageShareField(name) || !field.hasNumber {
			continue
		}
		value := field.number
		if strings.Contains(name, "percent") || strings.Contains(name, "pct") {
			value /= 100
		}
		if found && result != value {
			return 0, false
		}
		result, found = value, true
	}
	return result, found
}

func ps6016Number(fields map[string]ps6016Field, predicate func(string) bool) (float64, bool) {
	var result float64
	found := false
	for name, field := range fields {
		if !predicate(name) || !field.hasNumber {
			continue
		}
		if found && result != field.number {
			return 0, false
		}
		result, found = field.number, true
	}
	return result, found
}

func ps6016Numbers(fields map[string]ps6016Field, predicate func(string) bool) ([]float64, bool) {
	var result []float64
	found := false
	for name, field := range fields {
		if !predicate(name) || !field.hasNumbers {
			continue
		}
		if found {
			return nil, false
		}
		result, found = field.numbers, true
	}
	return result, found
}

func ps6016ExactnessFailed(fields map[string]ps6016Field) (bool, bool) {
	known, failed := false, false
	for name, field := range fields {
		if !ps6016ExactnessField(name) {
			continue
		}
		if field.hasBool {
			known, failed = true, failed || !field.boolVal
		}
		if field.hasString {
			value := strings.ToLower(field.stringVal)
			if ps6007ContainsAny(value, "fail", "mismatch", "not exact", "false") {
				known, failed = true, true
			} else if ps6007ContainsAny(value, "pass", "exact", "identical", "true") {
				known = true
			}
		}
	}
	return failed, known
}

func ps6016Below(values []float64, threshold float64) (float64, bool) {
	for _, value := range values {
		if value < threshold {
			return value, true
		}
	}
	return 0, false
}

func ps6016Median(values []float64) float64 {
	copyOfValues := slices.Clone(values)
	slices.Sort(copyOfValues)
	middle := len(copyOfValues) / 2
	if len(copyOfValues)%2 == 1 {
		return copyOfValues[middle]
	}
	return (copyOfValues[middle-1] + copyOfValues[middle]) / 2
}

func ps6016Context(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	accelerator, fusion, dispatch, graph := false, false, false, false
	classify := func(text string) {
		text = strings.ToLower(text)
		accelerator = accelerator || ps6007ContainsAny(text, "metal", "cuda", "vulkan", "gpu", "mps", "accelerator")
		fusion = fusion || ps6007ContainsAny(text, "fusion", "fused", "merge", "grouped")
		dispatch = dispatch || ps6007ContainsAny(text, "dispatch", "encoder", "launch", "binaryadd")
		graph = graph || ps6007ContainsAny(text, "amdahl", "endtoend", "end-to-end", "decode", "prefill", "graphleverage")
	}
	classify(fn.Name.Name)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if accelerator && fusion && dispatch && graph {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.Ident:
			classify(value.Name)
		case *ast.BasicLit:
			if value.Kind == token.STRING {
				classify(value.Value)
			}
		case *ast.CallExpr:
			if callee, _, ok := typedCallee(pass, value.Fun); ok {
				classify(callee.Name())
			}
		}
		return !(accelerator && fusion && dispatch && graph)
	})
	return accelerator && fusion && dispatch && graph
}
