package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"math"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6014 implements owner issue #770: a structural GPU fusion win must not be
// promoted when its independent latency leverage gate fails.
var PS6014 = register(&lint.Check{
	ID:       "PS6014",
	Category: "verify",
	Slug:     "structural-fusion-misses-latency-floor",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a structural GPU fusion win misses its declared latency leverage floor",
		Text: `Halving launches, dispatches, encoders, graph nodes, or library
calls is a structural result, not automatically a latency result. When work
volume remains similar, one grouped operation can cost almost as much as the
two operations it replaces. Treating the count reduction itself as a speedup
turns a useful mechanism signal into a false performance proxy.

This check implements owner issue #770. It finds real
func BenchmarkX(*testing.B) and promotion-oriented func TestX(*testing.T)
harnesses with accelerator, fusion/grouping, structural-count, and
latency/leverage context. It then requires a keyed StructuralLatencyGate,
FusionLeverageGate, StructuralCountEvidence, FusionPromotionEvidence,
LocalFusionBenchmark, or equivalent manifest with separate evidence for:

  - hardware, production shape, dtype, and warm/cold state;
  - structural metric, before count, and after count;
  - benchmark sample count or samples;
  - measured median speedup ratio and required ratio/floor; and
  - exactness/parity status.

Field names are accepted as structural evidence when values are dynamic. When
counts and ratios are compile-time numbers, the check independently evaluates
both gates. A reduced structural count paired with medianRatio < requiredRatio
is reported as low leverage/false proxy even when exactness passes. Nonpositive
sample counts, failed exactness, invalid ratios, and a claimed structural win
whose count does not decrease are reported as invalid evidence.

The count gate and latency gate intentionally remain separate. Exact output and
a 2 -> 1 projection count prove semantic reachability and topology reduction;
they do not prove that the grouped leaf clears a 1.08x promotion floor.

There is NO automatic fix. Hardware, production shape, sample campaign,
promotion threshold, and the decision to revert or retain the candidate are
empirical evidence that syntax cannot invent.`,
		Before: `gate := StructuralLatencyGate{
	StructuralBefore: 2,
	StructuralAfter:  1,
	MedianRatio:      1.021,
}
// Count halved, so promote.`,
		After: `gate := StructuralLatencyGate{
	Hardware: "Apple M2 Pro", ProductionShape: shape,
	DType: "cached f16 / Q4_K", WarmColdState: "warm",
	StructuralMetric: "MPS projections",
	StructuralBefore: 2, StructuralAfter: 1,
	BenchmarkSamples: 10,
	MedianRatio: 1.021, RequiredRatio: 1.08,
	ExactnessPassed: true,
}
// Structural gate passes; latency gate fails, so revert.`,
		MeasuredWin: `In the Apple-M2 experiment behind issue #770, grouping two
cached-f16 MPS gate/up projections reduced the structural count from 2 to 1 and
preserved bit-exact full-shape output. Ten M=64, K=2048, N=5632 repetitions
measured 876.2 us separate versus 858.1 us grouped: only 1.021x, below the
predeclared 1.08x floor. The executable optimization was reverted.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6014",
		Doc:  "structural fusion count improves but median latency misses its floor",
		Run:  runPS6014,
	},
})

type ps6014Field struct {
	stringVal string
	hasString bool
	numberVal float64
	hasNumber bool
	intVal    int64
	hasInt    bool
	boolVal   bool
	hasBool   bool
}

type ps6014Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6014Field
}

type ps6014Axis struct {
	name    string
	present func(map[string]ps6014Field) bool
}

var ps6014Axes = []ps6014Axis{
	{name: "hardware", present: func(f map[string]ps6014Field) bool {
		return ps6014HasName(f, func(n string) bool { return ps6007ContainsAny(n, "hardware", "device", "accelerator") })
	}},
	{name: "production shape", present: func(f map[string]ps6014Field) bool {
		return ps6014HasName(f, func(n string) bool {
			return strings.Contains(n, "shape") && ps6007ContainsAny(n, "production", "full", "benchmark", "workload")
		})
	}},
	{name: "dtype", present: func(f map[string]ps6014Field) bool {
		return ps6014HasName(f, func(n string) bool { return ps6007ContainsAny(n, "dtype", "datatype", "precision") })
	}},
	{name: "warm/cold state", present: func(f map[string]ps6014Field) bool {
		return ps6014HasName(f, func(n string) bool {
			return ps6007ContainsAny(n, "warmcold", "cachestate", "timingstate", "memorystate")
		})
	}},
	{name: "structural metric", present: func(f map[string]ps6014Field) bool {
		return ps6014HasName(f, func(n string) bool {
			return strings.Contains(n, "structural") && ps6007ContainsAny(n, "metric", "kind", "unit", "counter")
		})
	}},
	{name: "structural before count", present: func(f map[string]ps6014Field) bool {
		return ps6014HasName(f, ps6014BeforeField)
	}},
	{name: "structural after count", present: func(f map[string]ps6014Field) bool {
		return ps6014HasName(f, ps6014AfterField)
	}},
	{name: "benchmark samples", present: func(f map[string]ps6014Field) bool {
		return ps6014HasName(f, ps6014SamplesField)
	}},
	{name: "median latency ratio", present: func(f map[string]ps6014Field) bool {
		return ps6014HasName(f, ps6014MedianField)
	}},
	{name: "required latency ratio", present: func(f map[string]ps6014Field) bool {
		return ps6014HasName(f, ps6014RequiredField)
	}},
	{name: "exactness status", present: func(f map[string]ps6014Field) bool {
		return ps6014HasName(f, ps6014ExactnessField)
	}},
}

func runPS6014(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6011Harness(pass, fn) || !ps6014Context(pass, fn) {
				continue
			}
			manifest, found := ps6014BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "structural accelerator-fusion benchmark has no separate structural/latency leverage manifest; missing %s", strings.Join(ps6014Missing(nil), ", "))
				continue
			}
			if missing := ps6014Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "structural/latency fusion manifest is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if violations := ps6014Violations(manifest.fields); len(violations) > 0 {
				pass.Reportf(manifest.lit.Pos(), "structural/latency fusion evidence fails independent gates: %s", strings.Join(violations, "; "))
			}
		}
	}
	return nil, nil
}

func ps6014BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6014Manifest, bool) {
	var best ps6014Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6014ManifestType(lit.Type) {
			return true
		}
		manifest := ps6014Manifest{lit: lit, fields: ps6014Fields(pass, lit)}
		score := len(ps6014Axes) - len(ps6014Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6014ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6014ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6014ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return strings.Contains(name, "structurallatency") ||
		strings.Contains(name, "fusionleverage") ||
		strings.Contains(name, "structuralcountevidence") ||
		strings.Contains(name, "fusionpromotionevidence") ||
		strings.Contains(name, "localfusionbenchmark") ||
		strings.Contains(name, "structuralfusiongate")
}

func ps6014Fields(pass *analysis.Pass, lit *ast.CompositeLit) map[string]ps6014Field {
	fields := make(map[string]ps6014Field, len(lit.Elts))
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := ps2110Unparen(kv.Key).(*ast.Ident)
		if !ok {
			continue
		}
		field := ps6014Field{}
		expr := ps2110Unparen(kv.Value)
		if tv, ok := pass.TypesInfo.Types[expr]; ok && tv.Value != nil {
			switch tv.Value.Kind() {
			case constant.String:
				field.stringVal, field.hasString = constant.StringVal(tv.Value), true
			case constant.Bool:
				field.boolVal, field.hasBool = constant.BoolVal(tv.Value), true
			case constant.Int:
				field.intVal, field.hasInt = constant.Int64Val(tv.Value)
				field.numberVal, field.hasNumber = ps6014Float(tv.Value)
			case constant.Float:
				field.numberVal, field.hasNumber = ps6014Float(tv.Value)
			}
		}
		if length, ok := ps6014SequenceLen(expr); ok {
			field.intVal, field.hasInt = int64(length), true
			field.numberVal, field.hasNumber = float64(length), true
		}
		fields[ps6007NormalizeName(key.Name)] = field
	}
	return fields
}

func ps6014Float(value constant.Value) (float64, bool) {
	result, _ := constant.Float64Val(constant.ToFloat(value))
	return result, !math.IsInf(result, 0) && !math.IsNaN(result)
}

func ps6014SequenceLen(expr ast.Expr) (int, bool) {
	lit, ok := ps2110Unparen(expr).(*ast.CompositeLit)
	if !ok {
		return 0, false
	}
	return len(lit.Elts), true
}

func ps6014Missing(fields map[string]ps6014Field) []string {
	missing := make([]string, 0, len(ps6014Axes))
	for _, axis := range ps6014Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6014HasName(fields map[string]ps6014Field, predicate func(string) bool) bool {
	for name := range fields {
		if predicate(name) {
			return true
		}
	}
	return false
}

func ps6014BeforeField(name string) bool {
	return strings.Contains(name, "structural") && strings.Contains(name, "before") ||
		ps6007ContainsAny(name, "beforecount", "controlcount", "separatecount", "baselinecount")
}

func ps6014AfterField(name string) bool {
	return strings.Contains(name, "structural") && strings.Contains(name, "after") ||
		ps6007ContainsAny(name, "aftercount", "candidatecount", "fusedcount", "groupedcount")
}

func ps6014SamplesField(name string) bool {
	return strings.Contains(name, "sample") || strings.Contains(name, "repetition")
}

func ps6014MedianField(name string) bool {
	return strings.Contains(name, "median") && ps6007ContainsAny(name, "ratio", "speedup", "latency")
}

func ps6014RequiredField(name string) bool {
	return ps6007ContainsAny(name, "required", "floor", "threshold", "minimum") &&
		ps6007ContainsAny(name, "ratio", "speedup", "latency", "leverage")
}

func ps6014ExactnessField(name string) bool {
	return ps6007ContainsAny(name, "exactness", "exact", "parity") &&
		ps6007ContainsAny(name, "status", "passed", "gate", "result")
}

func ps6014Violations(fields map[string]ps6014Field) []string {
	var violations []string
	if samples, ok := ps6014UniqueInt(fields, ps6014SamplesField); ok && samples <= 0 {
		violations = append(violations, "benchmark sample count is not positive")
	}
	if failed, known := ps6014ExactnessFailed(fields); known && failed {
		violations = append(violations, "exactness/parity gate explicitly fails")
	}
	before, beforeOK := ps6014UniqueNumber(fields, ps6014BeforeField)
	after, afterOK := ps6014UniqueNumber(fields, ps6014AfterField)
	median, medianOK := ps6014UniqueNumber(fields, ps6014MedianField)
	required, requiredOK := ps6014UniqueNumber(fields, ps6014RequiredField)
	if beforeOK && afterOK {
		switch {
		case before <= 0 || after < 0:
			violations = append(violations, "structural counts are outside their valid range")
		case after >= before:
			violations = append(violations, "claimed structural win does not reduce the count ("+ps6014Number(before)+" -> "+ps6014Number(after)+")")
		case medianOK && requiredOK && (median <= 0 || required <= 0):
			violations = append(violations, "median and required latency ratios must be positive")
		case medianOK && requiredOK && median < required:
			violations = append(violations, fmt.Sprintf("structural count improves %s -> %s but median ratio %.4gx misses the declared %.4gx floor; classify this as low leverage/false proxy, not a performance win", ps6014Number(before), ps6014Number(after), median, required))
		}
	}
	return violations
}

func ps6014UniqueInt(fields map[string]ps6014Field, predicate func(string) bool) (int64, bool) {
	var result int64
	found := false
	for name, field := range fields {
		if !predicate(name) || !field.hasInt {
			continue
		}
		if found && result != field.intVal {
			return 0, false
		}
		result, found = field.intVal, true
	}
	return result, found
}

func ps6014UniqueNumber(fields map[string]ps6014Field, predicate func(string) bool) (float64, bool) {
	var result float64
	found := false
	for name, field := range fields {
		if !predicate(name) || !field.hasNumber {
			continue
		}
		if found && result != field.numberVal {
			return 0, false
		}
		result, found = field.numberVal, true
	}
	return result, found
}

func ps6014ExactnessFailed(fields map[string]ps6014Field) (bool, bool) {
	known := false
	failed := false
	for name, field := range fields {
		if !ps6014ExactnessField(name) {
			continue
		}
		if field.hasBool {
			known = true
			failed = failed || !field.boolVal
			continue
		}
		if field.hasString {
			value := strings.ToLower(strings.TrimSpace(field.stringVal))
			if ps6007ContainsAny(value, "fail", "mismatch", "not exact", "nonexact", "false") {
				known, failed = true, true
				continue
			}
			if ps6007ContainsAny(value, "pass", "exact", "identical", "true") {
				known = true
			}
		}
	}
	return failed, known
}

func ps6014Number(value float64) string {
	if value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'g', 4, 64)
}

func ps6014Context(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	accelerator, fusion, structural, latency := false, false, false, false
	classify := func(text string) {
		text = strings.ToLower(text)
		accelerator = accelerator || ps6007ContainsAny(text, "gpu", "metal", "cuda", "mps", "vulkan", "accelerator")
		fusion = fusion || ps6007ContainsAny(text, "fusion", "fused", "grouped", "combine", "merged")
		structural = structural || ps6007ContainsAny(text, "structural", "launch", "dispatch", "encoder", "graphnode", "librarycall", "projectioncount")
		latency = latency || ps6007ContainsAny(text, "latency", "speedup", "ratio", "median", "leverage", "promotion", "floor", "threshold")
	}
	classify(fn.Name.Name)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if accelerator && fusion && structural && latency {
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
				if callee.Pkg() != nil {
					classify(callee.Pkg().Path())
				}
			}
		}
		return !(accelerator && fusion && structural && latency)
	})
	return accelerator && fusion && structural && latency
}
