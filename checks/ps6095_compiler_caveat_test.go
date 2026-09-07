package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6095CompilerOptimizationCaveat(t *testing.T) {
	t.Parallel()
	const source = `package caveat
func apply(output []float64, numerator, denominator float64) {
	for index := range output {
		output[index] = numerator / denominator
	}
}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "caveat.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Implicits:  make(map[ast.Node]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Scopes:     make(map[ast.Node]*types.Scope),
		Instances:  make(map[*ast.Ident]types.Instance),
	}
	config := types.Config{}
	pkg, err := config.Check("caveat", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics []analysis.Diagnostic
	pass := &analysis.Pass{
		Analyzer: PS6095.Analyzer, Fset: fset, Files: []*ast.File{file},
		Pkg: pkg, TypesInfo: info, TypesSizes: types.SizesFor("gc", "amd64"),
		Report: func(diagnostic analysis.Diagnostic) { diagnostics = append(diagnostics, diagnostic) },
	}
	if _, err := ps6095Run(pass, nil); err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || len(diagnostics[0].SuggestedFixes) != 1 {
		t.Fatalf("expected one deterministic scalar fix, got %#v", diagnostics)
	}
	if !strings.Contains(diagnostics[0].Message, "compiler may already hoist") {
		t.Fatalf("diagnostic omits the compiler optimization caveat: %s", diagnostics[0].Message)
	}
	if !strings.Contains(PS6095.Doc.Text, "compiler may already hoist") ||
		!strings.Contains(PS6095.Doc.MeasuredWin, "no additional speedup") {
		t.Fatal("documentation must distinguish the neutral scalar benchmark from the owner's production gain")
	}
}
