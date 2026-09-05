package checks

import (
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6077(t *testing.T) {
	t.Parallel()
	const child = "PERFSCAN_PS6077_ARM64_TEST"
	if os.Getenv(child) == "1" {
		analysistest.Run(t, analysistest.TestData(), PS6077.Analyzer, "ps6077")
		return
	}
	// The diagnostic is anchored in the arm64 scalar partition. Pin the
	// analysistest package load in an isolated process so this test can still
	// run in parallel without mutating the parent process environment.
	command := exec.Command(os.Args[0], "-test.run=^TestPS6077$", "-test.parallel=1")
	command.Env = append(os.Environ(), "GOARCH=arm64", child+"=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("arm64 analysistest child failed: %v\n%s", err, output)
	}
}

func TestPS6077PartitionSatisfiability(t *testing.T) {
	t.Parallel()
	parse := func(text string) constraint.Expr {
		expression, err := constraint.Parse("//go:build " + text)
		if err != nil {
			t.Fatal(err)
		}
		return expression
	}
	simd := ps6077Source{constraint: parse("arm64 && goexperiment.simd")}
	scalar := ps6077Source{constraint: parse("arm64 && !goexperiment.simd")}
	if !ps6077MutuallyExclusive(simd, scalar) {
		t.Fatal("opposite experiment partitions must be satisfiable and exclusive")
	}
	overlap := ps6077Source{constraint: parse("arm64")}
	if ps6077MutuallyExclusive(simd, overlap) {
		t.Fatal("a broader arm64 partition overlaps the SIMD feature partition")
	}
	arches := ps6077SatisfiableArchitectures(simd)
	if len(arches) != 1 || !arches["arm64"] {
		t.Fatalf("unexpected satisfiable architecture set: %v", arches)
	}
}

func TestPS6077RelatedIdentityAndShape(t *testing.T) {
	t.Parallel()
	packageIdentity := ps6077PackageFunctionIdentity("example.test/p", "Apply")
	methodIdentity := ps6077MethodIdentity("example.test/p", "Processor", "Apply")
	if packageIdentity == methodIdentity {
		t.Fatal("same-named package function and method must have distinct identities")
	}

	parseFunction := func(importPath, alias string) ps6077Shape {
		source := "package p\nimport " + alias + " \"" + importPath + "\"\nfunc Apply(values []" + alias + ".Value) float64 { return 0 }\n"
		file, err := parser.ParseFile(token.NewFileSet(), "shape.go", source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		function := file.Decls[1].(*ast.FuncDecl)
		shapeSource := &ps6077Source{file: file, filename: importPath + ".go"}
		pass := &analysis.Pass{Pkg: types.NewPackage("example.test/p", "p")}
		return ps6077SliceResultShape(pass, shapeSource, function, ps6077Imports(file))
	}
	leftShape := parseFunction("example.test/left", "shared")
	rightShape := parseFunction("example.test/right", "shared")
	leftAgainShape := parseFunction("example.test/left", "renamed")
	if ps6077ShapesCompatible(leftShape, rightShape) {
		t.Fatal("same alias spelling must not erase imported named-type identity")
	}
	if !ps6077ShapesCompatible(leftShape, leftAgainShape) {
		t.Fatal("different aliases of the same imported named type must retain one identity")
	}
}

func TestPS6077InactiveCallIndexScaling(t *testing.T) {
	t.Parallel()
	const calls = 12_000
	source := "package p\nfunc Large() {\n" + strings.Repeat("f()\n", calls) + "}\n"
	file, err := parser.ParseFile(token.NewFileSet(), "large_amd64.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	variant := &ps6077Variant{
		source:   &ps6077Source{active: false},
		function: file.Decls[0].(*ast.FuncDecl),
	}
	pass := &analysis.Pass{Pkg: types.NewPackage("example.test/p", "p")}
	start := time.Now()
	direct := ps6077DirectCalls(pass, variant)
	elapsed := time.Since(start)
	if !direct[ps6077PackageFunctionIdentity(pass.Pkg.Path(), "f")] {
		t.Fatal("large inactive function lost its direct package call")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("inactive direct-call indexing must remain near-linear: %d calls took %s", calls, elapsed)
	}
}

func TestPS6077BindingIdentity(t *testing.T) {
	t.Parallel()
	const child = "PERFSCAN_PS6077_BINDING_IDENTITY_ARCH"
	if architecture := os.Getenv(child); architecture != "" {
		analysistest.Run(t, analysistest.TestData(), PS6077.Analyzer,
			"ps6077round2shape"+architecture,
			"ps6077round2alias"+architecture,
		)
		return
	}
	for _, architecture := range []string{"arm64", "amd64"} {
		architecture := architecture
		t.Run(architecture, func(t *testing.T) {
			t.Parallel()
			command := exec.Command(os.Args[0], "-test.run=^TestPS6077BindingIdentity$", "-test.parallel=1")
			command.Env = append(os.Environ(), "GOARCH="+architecture, child+"="+architecture)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("%s binding-identity analysistest child failed: %v\n%s", architecture, err, output)
			}
		})
	}
}
