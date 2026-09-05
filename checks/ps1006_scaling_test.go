package checks

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// TestPS1006RegisterTileScaling runs the real analyzer over successively
// doubled packages of complete positive tiles. The deterministic counters
// cover both whole-AST passes (one index build and one analysis traversal) and
// the direct stability queries. A per-tile package/function scan would make at
// least one of these proof-work counts super-linear as the file grows.
func TestPS1006RegisterTileScaling(t *testing.T) {
	t.Parallel()
	type counts struct {
		build, analyze, queries int
	}
	var previous counts
	for _, tiles := range []int{100, 200, 400, 800} {
		pass, reports := ps1006ScalingPass(t, tiles)
		index, err := ps1006RunIndexed(pass)
		if err != nil {
			t.Fatalf("%d tiles: analyzer failed: %v", tiles, err)
		}
		if *reports != 0 {
			t.Fatalf("%d complete tiles: got %d diagnostics, want none", tiles, *reports)
		}
		current := counts{build: index.buildVisits, analyze: index.analysisVisits, queries: index.stabilityQueries}
		if current.build <= 0 || current.analyze <= 0 || current.queries < tiles {
			t.Fatalf("%d tiles: incomplete instrumentation: %+v", tiles, current)
		}
		if previous.build != 0 {
			ps1006AssertNearLinearCount(t, tiles, "index build visits", previous.build, current.build)
			ps1006AssertNearLinearCount(t, tiles, "analyzer visits", previous.analyze, current.analyze)
			ps1006AssertNearLinearCount(t, tiles, "stability queries", previous.queries, current.queries)
		}
		previous = current
	}
}

// TestPS1006NestedPureCallScaling runs the real analyzer over the exact
// deeply nested conversion shape from the review. purityVisits counts the
// argument AST nodes whose value-purity is actually evaluated; memoization
// and subtree ownership must keep that work proportional to nesting depth.
func TestPS1006NestedPureCallScaling(t *testing.T) {
	t.Parallel()
	previous := 0
	for _, depth := range []int{12, 24, 48} {
		pass, reports := ps1006DeepConversionPass(t, depth)
		index, err := ps1006RunIndexed(pass)
		if err != nil {
			t.Fatalf("depth %d: analyzer failed: %v", depth, err)
		}
		if *reports != 0 {
			t.Fatalf("depth %d complete tile: got %d diagnostics, want none", depth, *reports)
		}
		work := index.purityVisits
		if work < depth {
			t.Fatalf("depth %d: incomplete purity instrumentation: %d visits", depth, work)
		}
		if previous != 0 {
			ps1006AssertNearLinearCount(t, depth, "nested-call purity visits", previous, work)
		}
		previous = work
	}
}

// TestPS1006RegisterTileTailControlScaling keeps many tile/tail pairs in one
// function. The control-flow graph must be indexed once and each local proof
// must stay proportional to its own path rather than rescanning the complete
// function for every scalar remainder.
func TestPS1006RegisterTileTailControlScaling(t *testing.T) {
	t.Parallel()
	previousVisits := 0
	for _, pairs := range []int{25, 50, 100, 200} {
		pass, reports := ps1006TailControlScalingPass(t, pairs)
		index, err := ps1006RunIndexed(pass)
		if err != nil {
			t.Fatalf("%d pairs: analyzer failed: %v", pairs, err)
		}
		if *reports != 0 {
			t.Fatalf("%d complete tile/tail pairs: got %d diagnostics, want none", pairs, *reports)
		}
		if index.controlFlowBuilds != 1 || index.controlFlowVisits < pairs {
			t.Fatalf("%d pairs: incomplete/repeated CFG instrumentation: builds=%d visits=%d", pairs, index.controlFlowBuilds, index.controlFlowVisits)
		}
		if previousVisits != 0 {
			ps1006AssertNearLinearCount(t, pairs, "tile-tail control-flow visits", previousVisits, index.controlFlowVisits)
		}
		previousVisits = index.controlFlowVisits
	}
}

func ps1006AssertNearLinearCount(t *testing.T, inputSize int, name string, previous, current int) {
	t.Helper()
	// The input exactly doubles. A small fixed allowance covers the package
	// declaration; quadratic work would approach 4x at every step.
	if current > 2*previous+32 {
		t.Fatalf("input size %d: %s grew super-linearly: previous=%d current=%d", inputSize, name, previous, current)
	}
}

func ps1006ScalingPass(t *testing.T, tiles int) (*analysis.Pass, *int) {
	t.Helper()
	var source strings.Builder
	source.WriteString("package scale\n")
	for index := 0; index < tiles; index++ {
		fmt.Fprintf(&source, `
func tile%d(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for tap := 0; tap < taps; tap++ {
			base := tap * channels
			s0 += a[base+c] * w[tap]
			s1 += a[base+c+1] * w[tap]
			s2 += a[base+c+2] * w[tap]
			s3 += a[base+c+3] * w[tap]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}
`, index)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "scale.go", source.String(), parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %d tiles: %v", tiles, err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := (&types.Config{}).Check("scale", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatalf("type-check %d tiles: %v", tiles, err)
	}
	reports := 0
	return &analysis.Pass{
		Analyzer:  PS1006.Analyzer,
		Fset:      fset,
		Files:     []*ast.File{file},
		Pkg:       pkg,
		TypesInfo: info,
		Report: func(analysis.Diagnostic) {
			reports++
		},
	}, &reports
}

func ps1006TailControlScalingPass(t *testing.T, pairs int) (*analysis.Pass, *int) {
	t.Helper()
	var source strings.Builder
	source.WriteString("package scale\nfunc tails(a, w, out []float64, taps, channels int, skip bool) {\n")
	for index := 0; index < pairs; index++ {
		fmt.Fprintf(&source, `
	for c%d := 0; c%d+3 < channels; c%d += 4 {
		var s0, s1, s2, s3 float64
		for tap%d := 0; tap%d < taps; tap%d++ {
			base%d := tap%d * channels
			s0 += a[base%d+c%d] * w[tap%d]
			s1 += a[base%d+c%d+1] * w[tap%d]
			s2 += a[base%d+c%d+2] * w[tap%d]
			s3 += a[base%d+c%d+3] * w[tap%d]
		}
		out[c%d], out[c%d+1], out[c%d+2], out[c%d+3] = s0, s1, s2, s3
		if skip { continue }
	}
	for c%d := channels - channels%%4; c%d < channels; c%d++ {
		s := 0.0
		for tap%d := 0; tap%d < taps; tap%d++ {
			base%d := tap%d * channels
			s += a[base%d+c%d] * w[tap%d]
		}
		out[c%d] = s
	}
`, index, index, index,
			index, index, index,
			index, index,
			index, index, index,
			index, index, index,
			index, index, index,
			index, index, index,
			index, index, index, index,
			index, index, index,
			index, index, index,
			index, index,
			index, index, index,
			index)
	}
	source.WriteString("}\n")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "tail_scale.go", source.String(), parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %d tile/tail pairs: %v", pairs, err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := (&types.Config{}).Check("scale", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatalf("type-check %d tile/tail pairs: %v", pairs, err)
	}
	reports := 0
	return &analysis.Pass{
		Analyzer:  PS1006.Analyzer,
		Fset:      fset,
		Files:     []*ast.File{file},
		Pkg:       pkg,
		TypesInfo: info,
		Report: func(analysis.Diagnostic) {
			reports++
		},
	}, &reports
}

func ps1006DeepConversionPass(t *testing.T, depth int) (*analysis.Pass, *int) {
	t.Helper()
	var expression strings.Builder
	for range depth {
		expression.WriteString("int(")
	}
	expression.WriteString("t * channels")
	for range depth {
		expression.WriteByte(')')
	}
	source := fmt.Sprintf(`package scale

func deep(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := %s
			s0 += a[base+c] * w[t]
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0
		out[c+1] = s1
		out[c+2] = s2
		out[c+3] = s3
	}
}
`, expression.String())
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "deep.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse depth %d: %v", depth, err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := (&types.Config{}).Check("scale", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatalf("type-check depth %d: %v", depth, err)
	}
	reports := 0
	return &analysis.Pass{
		Analyzer:  PS1006.Analyzer,
		Fset:      fset,
		Files:     []*ast.File{file},
		Pkg:       pkg,
		TypesInfo: info,
		Report: func(analysis.Diagnostic) {
			reports++
		},
	}, &reports
}
