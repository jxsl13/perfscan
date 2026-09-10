package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6100NecessaryLoopPreflight(t *testing.T) {
	t.Parallel()
	const source = `package p
func zero() {}
func one(xs []int) { for range xs {} }
func literalOnly(xs []int) { _ = func() { for range xs {}; for range xs {} } }
func noncandidateHelper(xs []int) { for range xs {} }
func outerAndScan(xs []int) { for range xs { for range xs {} } }
func canonicalPair(xs []int) { noncandidateHelper(xs); for range xs {}; for range xs {} }
`
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, "preflight.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	pkg, err := new(types.Config).Check("preflight", files, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: files, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	candidates := ps6100CandidateFunctions(pass)
	if len(candidates) != 2 || candidates[0].Name.Name != "outerAndScan" || candidates[1].Name.Name != "canonicalPair" {
		var names []string
		for _, candidate := range candidates {
			names = append(names, candidate.Name.Name)
		}
		t.Fatalf("necessary candidates=%v want [outerAndScan canonicalPair]", names)
	}
	helpers := ps6100LocalFunctions(pass)
	helper, _ := pkg.Scope().Lookup("noncandidateHelper").(*types.Func)
	if helper == nil || helpers[helper] == nil {
		t.Fatal("noncandidate helper omitted from complete helper universe")
	}
}
