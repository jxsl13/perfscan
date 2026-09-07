package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/cfg"
)

func TestPS6095(t *testing.T) {
	t.Parallel()
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS6095.Analyzer, "ps6095")
}

// TestPS6095GenericNameFixedTargetTypeChecks closes analysistest's compile
// gap for generated identifiers that collide with generic type parameters.
func TestPS6095GenericNameFixedTargetTypeChecks(t *testing.T) {
	t.Parallel()
	path := filepath.Join(analysistest.TestData(), "src", "ps6095", "generic_names.go.golden")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, path, source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	configuration := types.Config{}
	if _, err := configuration.Check("ps6095", files, []*ast.File{file}, nil); err != nil {
		t.Fatalf("fixed generic-name target does not type-check: %v", err)
	}
}

func TestPS6095ExactFloat64Reuse(t *testing.T) {
	t.Parallel()
	values := []float64{
		0,
		math.Copysign(0, -1),
		1,
		-1,
		3,
		10,
		math.SmallestNonzeroFloat64,
		math.MaxFloat64,
		math.Inf(1),
		math.Inf(-1),
		math.Float64frombits(0x7ff8_0000_0000_0042),
	}
	for _, numerator := range values {
		for _, denominator := range values {
			cached := numerator / denominator
			for range 8 {
				repeated := numerator / denominator
				if math.Float64bits(repeated) != math.Float64bits(cached) {
					t.Fatalf("%016x / %016x changed from %016x to %016x", math.Float64bits(numerator), math.Float64bits(denominator), math.Float64bits(repeated), math.Float64bits(cached))
				}
			}
		}
	}
}

func TestPS6095ExactFloat32Reuse(t *testing.T) {
	t.Parallel()
	values := []float32{
		0,
		float32(math.Copysign(0, -1)),
		1,
		-1,
		3,
		10,
		math.SmallestNonzeroFloat32,
		math.MaxFloat32,
		float32(math.Inf(1)),
		float32(math.Inf(-1)),
		math.Float32frombits(0x7fc0_0042),
	}
	for _, numerator := range values {
		for _, denominator := range values {
			cached := numerator / denominator
			for range 8 {
				repeated := numerator / denominator
				if math.Float32bits(repeated) != math.Float32bits(cached) {
					t.Fatalf("%08x / %08x changed from %08x to %08x", math.Float32bits(numerator), math.Float32bits(denominator), math.Float32bits(repeated), math.Float32bits(cached))
				}
			}
		}
	}
}

func TestPS6095ReciprocalMultiplyIsNotEquivalent(t *testing.T) {
	t.Parallel()
	direct := float64(3) / float64(10)
	reciprocalMultiply := float64(3) * (float64(1) / float64(10))
	if math.Float64bits(direct) == math.Float64bits(reciprocalMultiply) {
		t.Fatal("test witness no longer distinguishes direct division from reciprocal multiplication")
	}
}

func TestPS6095CyclicBlocks(t *testing.T) {
	t.Parallel()
	first := &cfg.Block{Live: true}
	second := &cfg.Block{Live: true}
	self := &cfg.Block{Live: true}
	tail := &cfg.Block{Live: true}
	first.Succs = []*cfg.Block{second}
	second.Succs = []*cfg.Block{first, tail}
	self.Succs = []*cfg.Block{self}

	cyclic := ps6095CyclicBlocks([]*cfg.Block{first, second, self, tail})
	if !cyclic[first] || !cyclic[second] || !cyclic[self] {
		t.Fatal("expected non-trivial and self-edge SCC blocks to be cyclic")
	}
	if cyclic[tail] {
		t.Fatal("acyclic tail was classified as cyclic")
	}
}

func TestPS6095CandidateLimit(t *testing.T) {
	t.Parallel()
	if ps6095MaxCandidatesPerLoop < 2 {
		t.Fatal("candidate bound must preserve multi-quotient fixes")
	}
	var candidates []ps6095Candidate
	for range ps6095MaxCandidatesPerLoop + 17 {
		ps6095AddCandidate(&candidates, ps6095Candidate{})
	}
	if len(candidates) != ps6095MaxCandidatesPerLoop {
		t.Fatalf("recorded %d candidates; want hard bound %d", len(candidates), ps6095MaxCandidatesPerLoop)
	}
}

func TestPS6095NestedLoopTraversalScalesLinearly(t *testing.T) {
	t.Parallel()

	const depth = 128
	var source strings.Builder
	source.WriteString("package p\nfunc nested(output []float64, numerator, denominator float64) {\n")
	for level := range depth {
		index := "index" + strconv.Itoa(level)
		source.WriteString("for " + index + " := 0; " + index + " < len(output); " + index + "++ {\n")
		source.WriteString("output[" + index + "] = numerator / denominator\n")
	}
	for range depth {
		source.WriteString("}\n")
	}
	source.WriteString("}\n")

	file, err := parser.ParseFile(token.NewFileSet(), "nested.go", source.String(), 0)
	if err != nil {
		t.Fatal(err)
	}
	var loops []*ast.ForStmt
	totalNodes := 0
	ast.Inspect(file, func(node ast.Node) bool {
		if node != nil {
			totalNodes++
		}
		if loop, ok := node.(*ast.ForStmt); ok {
			loops = append(loops, loop)
		}
		return true
	})
	directVisits := 0
	for _, loop := range loops {
		ps6095InspectDirect(loop.Body, func(ast.Node) bool {
			directVisits++
			return true
		})
	}
	if len(loops) != depth {
		t.Fatalf("found %d loops; want %d", len(loops), depth)
	}
	if directVisits > totalNodes*2 {
		t.Fatalf("direct loop scans visited %d nodes for a %d-node AST; nested effects are being rescanned", directVisits, totalNodes)
	}
}

// TestPS6095InstrumentedTraversalsScaleLinearly measures the two explicitly
// instrumented traversal classes: disjoint function-context preparation and
// direct loop-local scans. It does not claim to measure CFG construction,
// dependency parent walks, or every constant-factor pass over prepared facts.
func TestPS6095InstrumentedTraversalsScaleLinearly(t *testing.T) {
	t.Parallel()

	const (
		sequentialLoops = 96
		literalDepth    = 64
	)
	var source strings.Builder
	source.WriteString("package scale\n")
	source.WriteString("func sequential(output []float64, numerator, denominator float64) {\n")
	for level := range sequentialLoops {
		index := "sequentialIndex" + strconv.Itoa(level)
		source.WriteString("for " + index + " := range output { output[" + index + "] = numerator / denominator }\n")
	}
	source.WriteString("}\n")
	source.WriteString("func literals(output []float64, numerator, denominator float64) {\n")
	for level := range literalDepth {
		index := "literalIndex" + strconv.Itoa(level)
		source.WriteString("_ = func() {\n")
		source.WriteString("for " + index + " := range output { output[" + index + "] = numerator / denominator }\n")
	}
	for range literalDepth {
		source.WriteString("}\n")
	}
	source.WriteString("}\n")

	parsed, err := parser.ParseFile(token.NewFileSet(), "scale.go", source.String(), 0)
	if err != nil {
		t.Fatal(err)
	}
	totalNodes := uint64(0)
	ast.Inspect(parsed, func(node ast.Node) bool {
		if node != nil {
			totalNodes++
		}
		return true
	})

	testdata := t.TempDir()
	packageDir := filepath.Join(testdata, "src", "scale")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "scale.go"), []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	stats := new(ps6095Stats)
	analyzer := *PS6095.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		pass.Report = func(analysis.Diagnostic) {}
		return ps6095Run(pass, stats)
	}
	analysistest.Run(t, testdata, &analyzer, "scale")

	prepared := stats.preparedNodes.Load()
	direct := stats.directNodes.Load()
	functions := stats.functions.Load()
	diagnostics := stats.diagnostics.Load()
	if functions != literalDepth+2 {
		t.Fatalf("prepared %d function contexts; want %d", functions, literalDepth+2)
	}
	if prepared > totalNodes {
		t.Fatalf("prepared %d nodes for a %d-node AST; nested function bodies were rescanned", prepared, totalNodes)
	}
	if diagnostics != sequentialLoops+literalDepth {
		t.Fatalf("analyzed %d candidate loops; want %d", diagnostics, sequentialLoops+literalDepth)
	}
	if direct > prepared*4 {
		t.Fatalf("instrumented direct loop scans visited %d nodes after %d instrumented preparation visits; expected a linear upper bound", direct, prepared)
	}
}
