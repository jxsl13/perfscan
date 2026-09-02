package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6100(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6100.Analyzer, "ps6100")
}

func TestPS6100CandidateBounds(t *testing.T) {
	t.Parallel()
	if ps6100MaxScansPerIteration < 2 || ps6100MaxMutationsPerIteration < 2 || ps6100MaxPredicateBits < 2 || ps6100MaxCallableWork < ps6100MaxHelperDepth {
		t.Fatalf("invalid finite-state candidate bounds: scans=%d mutations=%d bits=%d callable-work=%d helper-depth=%d", ps6100MaxScansPerIteration, ps6100MaxMutationsPerIteration, ps6100MaxPredicateBits, ps6100MaxCallableWork, ps6100MaxHelperDepth)
	}
}

func TestPS6100StorageCycle(t *testing.T) {
	t.Parallel()
	typ := types.NewSlice(types.Typ[types.Float64])
	leftObject := types.NewVar(token.NoPos, nil, "left", typ)
	rightObject := types.NewVar(token.NoPos, nil, "right", typ)
	left := ast.NewIdent("left")
	right := ast.NewIdent("right")
	pass := &analysis.Pass{TypesInfo: &types.Info{Uses: map[*ast.Ident]types.Object{
		left: leftObject, right: rightObject,
	}}}
	environment := map[types.Object]ast.Expr{
		leftObject:  &ast.SliceExpr{X: right},
		rightObject: &ast.SliceExpr{X: left},
	}
	if storage, ok := ps6100StorageOf(pass, left, environment); ok {
		t.Fatalf("cyclic invocation environment resolved as concrete storage %q", storage.name)
	}
}
