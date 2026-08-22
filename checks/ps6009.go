package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6009 implements the source-auditable benchmark-grid requirements from
// owner issue #760 for tiled accelerator kernels with a reuse axis.
var PS6009 = register(&lint.Check{
	ID:       "PS6009",
	Category: "verify",
	Slug:     "reuse-axis-gate-misses-second-tile",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a tiled GPU promotion gate omits the second reuse-axis tile or production comparator",
		Text: `A tiled accelerator kernel can look competitive at its first complete
tile while losing sharply once setup repeats along a reuse axis. Per-tile
dequantization or staging grows with the number of threadgroups; a persistent
expanded baseline can amortize its materialization across the whole axis. A
single one-tile shape therefore hides the slope that controls routed shapes.

This check implements the static/perf-harness boundary from owner issue #760.
It finds real func BenchmarkX(*testing.B) functions with accelerator, tile, and
promotion/gate/comparison context, then looks for a keyed TileBoundaryGate,
ReuseAxisGate, TiledKernelBenchmark, ReuseAxisEvidence, or similar manifest.
The best manifest must explicitly carry:

  - reuse-axis name and tile extent;
  - sampled shape grid and largest routed production shape;
  - comparator;
  - candidate per-tile setup/staging/dequantization policy;
  - baseline cache/materialization policy; and
  - separately reported first-tile intercept and per-tile slope.

When tile extent, sample shapes, and largest routed shape are compile-time
integers, the grid must contain T, T+1, 2*T, and the largest routed shape. For
BM=32 that means at least M32, M33, M64, plus the largest production M. When
constant strings explicitly say that the candidate stages/dequantizes per tile,
the baseline owns persistent/cached materialization, and the comparator is an
uncached/fallback path, the diagnostic requires the actual production
comparator. Dynamic manifest values count as structural evidence but are not
invented or classified.

This distinguishes the first-tile intercept from the repeated per-tile slope.
Parity and a faithful port establish correctness; boundary samples and the
production comparator establish whether reuse survives real routing.

There is NO automatic fix. Shape selection, the largest routed extent,
comparator identity, and timing decomposition are benchmark evidence that
cannot be derived from syntax.`,
		Before: `gate := TileBoundaryGate{
    ReuseAxis: "M", TileExtent: 32,
    SampleShapes: []int{32},
    Comparator: "uncached f32 fallback",
}`,
		After: `gate := TileBoundaryGate{
    ReuseAxis: "M", TileExtent: 32,
    SampleShapes: []int{32, 33, 64, largestRoutedM},
    LargestRoutedShape: largestRoutedM,
    Comparator: "cached-f16 production incumbent",
    CandidatePerTileSetup: "dequantize/stage every tile",
    BaselineMaterialization: "persistent expanded f16",
    FirstTileInterceptUS: intercept,
    PerTileSlopeUS: slope,
}`,
		MeasuredWin: `In the Apple-M2 Q4_K tile investigation behind issue #760,
the faithful 32-row tile was approximately parity with cached-f16 production at
M32 (0.979x and 1.011x in fresh runs), but fell to 0.741x and 0.746x at M64.
Each extra tile restaged quantized weights while production reused persistent
dense expansion across M. Comparisons against the uncached fallback misleadingly
showed 2.12x at M32 and 1.70x at M64.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6009",
		Doc:  "tiled accelerator benchmark gate omits reuse-axis boundary evidence",
		Run:  runPS6009,
	},
})

type ps6009Field struct {
	stringVal string
	hasString bool
	intVal    int64
	hasInt    bool
	ints      []int64
	hasInts   bool
}

type ps6009Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6009Field
}

type ps6009Axis struct {
	name    string
	present func(map[string]ps6009Field) bool
}

var ps6009Axes = []ps6009Axis{
	{name: "reuse axis", present: func(f map[string]ps6009Field) bool {
		return ps6009HasName(f, func(n string) bool { return strings.Contains(n, "reuseaxis") || strings.Contains(n, "axisname") })
	}},
	{name: "tile extent", present: func(f map[string]ps6009Field) bool { return ps6009HasName(f, ps6009TileField) }},
	{name: "boundary sample shapes", present: func(f map[string]ps6009Field) bool { return ps6009HasName(f, ps6009SamplesField) }},
	{name: "largest routed production shape", present: func(f map[string]ps6009Field) bool { return ps6009HasName(f, ps6009LargestField) }},
	{name: "production comparator", present: func(f map[string]ps6009Field) bool {
		return ps6009HasName(f, func(n string) bool { return strings.Contains(n, "comparator") })
	}},
	{name: "candidate per-tile setup", present: func(f map[string]ps6009Field) bool { return ps6009HasName(f, ps6009CandidateSetupField) }},
	{name: "baseline materialization policy", present: func(f map[string]ps6009Field) bool { return ps6009HasName(f, ps6009BaselinePolicyField) }},
	{name: "first-tile intercept", present: func(f map[string]ps6009Field) bool {
		return ps6009HasName(f, func(n string) bool { return strings.Contains(n, "intercept") && ps6007ContainsAny(n, "first", "tile") })
	}},
	{name: "per-tile slope", present: func(f map[string]ps6009Field) bool {
		return ps6009HasName(f, func(n string) bool { return strings.Contains(n, "slope") && strings.Contains(n, "tile") })
	}},
}

func runPS6009(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6006Benchmark(pass, fn) || !ps6009GateContext(pass, fn) {
				continue
			}
			manifest, found := ps6009BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "tiled accelerator promotion benchmark has no reuse-axis gate manifest; missing %s", strings.Join(ps6009Missing(nil), ", "))
				continue
			}
			if missing := ps6009Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "reuse-axis gate manifest is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			violations := ps6009Violations(manifest.fields)
			if len(violations) > 0 {
				pass.Reportf(manifest.lit.Pos(), "reuse-axis gate evidence fails boundary audit: %s", strings.Join(violations, "; "))
			}
		}
	}
	return nil, nil
}

func ps6009BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6009Manifest, bool) {
	var best ps6009Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6009ManifestType(lit.Type) {
			return true
		}
		manifest := ps6009Manifest{lit: lit, fields: ps6009Fields(pass, lit)}
		score := len(ps6009Axes) - len(ps6009Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6009ManifestType(expr ast.Expr) bool {
	var name string
	switch n := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = n.Name
	case *ast.SelectorExpr:
		name = n.Sel.Name
	case *ast.IndexExpr:
		return ps6009ManifestType(n.X)
	case *ast.IndexListExpr:
		return ps6009ManifestType(n.X)
	}
	n := ps6007NormalizeName(name)
	return strings.Contains(n, "tileboundarygate") || strings.Contains(n, "reuseaxisgate") ||
		strings.Contains(n, "tiledkernelbenchmark") || strings.Contains(n, "reuseaxisevidence") ||
		strings.Contains(n, "tilegate")
}

func ps6009Fields(pass *analysis.Pass, lit *ast.CompositeLit) map[string]ps6009Field {
	fields := make(map[string]ps6009Field, len(lit.Elts))
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := ps2110Unparen(kv.Key).(*ast.Ident)
		if !ok {
			continue
		}
		field := ps6009Field{}
		valueExpr := ps2110Unparen(kv.Value)
		if tv, ok := pass.TypesInfo.Types[valueExpr]; ok && tv.Value != nil {
			switch tv.Value.Kind() {
			case constant.String:
				field.stringVal, field.hasString = constant.StringVal(tv.Value), true
			case constant.Int:
				field.intVal, field.hasInt = constant.Int64Val(tv.Value)
			}
		}
		if values, ok := ps6009ConstIntSlice(pass, valueExpr); ok {
			field.ints, field.hasInts = values, true
		}
		fields[ps6007NormalizeName(key.Name)] = field
	}
	return fields
}

func ps6009ConstIntSlice(pass *analysis.Pass, expr ast.Expr) ([]int64, bool) {
	lit, ok := ps2110Unparen(expr).(*ast.CompositeLit)
	if !ok {
		return nil, false
	}
	values := make([]int64, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		value := ast.Expr(elt)
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			value = kv.Value
		}
		tv, ok := pass.TypesInfo.Types[ps2110Unparen(value)]
		if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
			return nil, false
		}
		integer, exact := constant.Int64Val(tv.Value)
		if !exact {
			return nil, false
		}
		values = append(values, integer)
	}
	return values, true
}

func ps6009Missing(fields map[string]ps6009Field) []string {
	missing := make([]string, 0, len(ps6009Axes))
	for _, axis := range ps6009Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6009HasName(fields map[string]ps6009Field, predicate func(string) bool) bool {
	for name := range fields {
		if predicate(name) {
			return true
		}
	}
	return false
}

func ps6009TileField(name string) bool {
	return strings.Contains(name, "tile") && ps6007ContainsAny(name, "extent", "size", "rows")
}

func ps6009SamplesField(name string) bool {
	return strings.Contains(name, "shape") && ps6007ContainsAny(name, "sample", "grid", "points") || strings.Contains(name, "reuseaxissamples")
}

func ps6009LargestField(name string) bool {
	return strings.Contains(name, "largest") && ps6007ContainsAny(name, "routed", "production") && ps6007ContainsAny(name, "shape", "extent", "axis")
}

func ps6009CandidateSetupField(name string) bool {
	return strings.Contains(name, "candidate") && strings.Contains(name, "tile") && ps6007ContainsAny(name, "setup", "staging", "dequant")
}

func ps6009BaselinePolicyField(name string) bool {
	return strings.Contains(name, "baseline") && ps6007ContainsAny(name, "materialization", "cache", "storage")
}

func ps6009Violations(fields map[string]ps6009Field) []string {
	var violations []string
	if tile, tileOK := ps6009UniqueInt(fields, ps6009TileField); tileOK && tile > 0 {
		if samples, samplesOK := ps6009UniqueInts(fields, ps6009SamplesField); samplesOK {
			required := []int64{tile, tile + 1, 2 * tile}
			if largest, largestOK := ps6009UniqueInt(fields, ps6009LargestField); largestOK && largest > 0 {
				required = append(required, largest)
			}
			slices.Sort(required)
			required = slices.Compact(required)
			var absent []string
			for _, point := range required {
				if !slices.Contains(samples, point) {
					absent = append(absent, strconv.FormatInt(point, 10))
				}
			}
			if len(absent) > 0 {
				violations = append(violations, "shape grid omits required reuse-axis point(s) "+strings.Join(absent, ", "))
			}
		}
	}
	if ps6009WrongComparator(fields) {
		violations = append(violations, "candidate declares per-tile staging/dequantization against persistent baseline materialization but comparator is an uncached/fallback path, not production")
	}
	return violations
}

func ps6009UniqueInt(fields map[string]ps6009Field, predicate func(string) bool) (int64, bool) {
	var value int64
	found := false
	for name, field := range fields {
		if !predicate(name) || !field.hasInt {
			continue
		}
		if found && value != field.intVal {
			return 0, false
		}
		value, found = field.intVal, true
	}
	return value, found
}

func ps6009UniqueInts(fields map[string]ps6009Field, predicate func(string) bool) ([]int64, bool) {
	var values []int64
	found := false
	for name, field := range fields {
		if !predicate(name) || !field.hasInts {
			continue
		}
		if found {
			return nil, false
		}
		values, found = field.ints, true
	}
	return values, found
}

func ps6009WrongComparator(fields map[string]ps6009Field) bool {
	candidate, candidateOK := ps6009UniqueString(fields, ps6009CandidateSetupField)
	baseline, baselineOK := ps6009UniqueString(fields, ps6009BaselinePolicyField)
	comparator, comparatorOK := ps6009UniqueString(fields, func(name string) bool { return strings.Contains(name, "comparator") })
	if !candidateOK || !baselineOK || !comparatorOK {
		return false
	}
	candidate = strings.ToLower(candidate)
	baseline = strings.ToLower(baseline)
	comparator = strings.ToLower(comparator)
	perTile := ps6007ContainsAny(candidate, "per-tile", "per tile", "each tile", "restage", "re-stage") ||
		strings.Contains(candidate, "dequant") && strings.Contains(candidate, "tile")
	persistent := ps6007ContainsAny(baseline, "persistent", "cached", "expanded", "resident")
	fallback := ps6007ContainsAny(comparator, "uncached", "fallback", "f32 dequant", "direct dequant")
	return perTile && persistent && fallback
}

func ps6009UniqueString(fields map[string]ps6009Field, predicate func(string) bool) (string, bool) {
	value := ""
	found := false
	for name, field := range fields {
		if !predicate(name) || !field.hasString {
			continue
		}
		if found && value != field.stringVal {
			return "", false
		}
		value, found = field.stringVal, true
	}
	return value, found
}

func ps6009GateContext(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	accelerator, tiled, gate := false, false, false
	classify := func(text string) {
		text = strings.ToLower(text)
		accelerator = accelerator || ps6007ContainsAny(text, "gpu", "metal", "cuda", "mps", "accelerator")
		tiled = tiled || ps6007ContainsAny(text, "tile", "threadgroup", "simdgroup", "block")
		gate = gate || ps6007ContainsAny(text, "gate", "promotion", "candidate", "compare", "comparator", "speedup", "throughput")
	}
	classify(fn.Name.Name)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if accelerator && tiled && gate {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch n := node.(type) {
		case *ast.Ident:
			classify(n.Name)
		case *ast.BasicLit:
			if n.Kind == token.STRING {
				classify(n.Value)
			}
		case *ast.CallExpr:
			if callee, _, ok := typedCallee(pass, n.Fun); ok {
				classify(callee.Name())
				if callee.Pkg() != nil {
					classify(callee.Pkg().Path())
				}
			}
		}
		return !(accelerator && tiled && gate)
	})
	return accelerator && tiled && gate
}
