package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6079(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6079.Analyzer, "ps6079")
}

func TestPS6079PackageHasBenchmark(t *testing.T) {
	t.Parallel()
	testingPackage := types.NewPackage("testing", "testing")
	benchmarkType := types.NewNamed(
		types.NewTypeName(token.NoPos, testingPackage, "B", nil),
		types.NewStruct(nil, nil),
		nil,
	)
	parameter := types.NewVar(token.NoPos, nil, "b", types.NewPointer(benchmarkType))
	signature := types.NewSignatureType(nil, nil, nil, types.NewTuple(parameter), nil, false)
	name := ast.NewIdent("BenchmarkRoute")
	function := &ast.FuncDecl{Name: name, Type: &ast.FuncType{}, Body: &ast.BlockStmt{}}
	pass := &analysis.Pass{TypesInfo: &types.Info{Defs: map[*ast.Ident]types.Object{
		name: types.NewFunc(token.NoPos, nil, name.Name, signature),
	}}}
	if !ps6079PackageHasBenchmark(pass, []*ast.FuncDecl{function}) {
		t.Fatal("exact BenchmarkX(*testing.B) function was not recognized")
	}
	function.Name.Name = "Route"
	if ps6079PackageHasBenchmark(pass, []*ast.FuncDecl{function}) {
		t.Fatal("non-benchmark function was recognized as a benchmark")
	}
}

func TestPS6079PredicateRelations(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		relation ps6079Relation
		ok       bool
	}{
		{name: "allNonNegative", relation: ps6079AtLeastZero, ok: true},
		{name: "positiveValues", relation: ps6079AboveZero, ok: true},
		{name: "isNonPositive", relation: ps6079AtMostZero, ok: true},
		{name: "negativeMambaDecay", relation: ps6079BelowZero, ok: true},
		{name: "validDomain"},
	} {
		relation, ok := ps6079PredicateRelation(test.name)
		if relation != test.relation || ok != test.ok {
			t.Fatalf("ps6079PredicateRelation(%q) = (%v, %v), want (%v, %v)", test.name, relation, ok, test.relation, test.ok)
		}
	}
}

func TestPS6079ComparisonOperators(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		expression string
		operation  token.Token
		relation   ps6079Relation
	}{
		{expression: "x >= 0", operation: token.GEQ, relation: ps6079AtLeastZero},
		{expression: "x > 0", operation: token.GTR, relation: ps6079AboveZero},
		{expression: "x <= 0", operation: token.LEQ, relation: ps6079AtMostZero},
		{expression: "x < 0", operation: token.LSS, relation: ps6079BelowZero},
		{expression: "x == 0", operation: token.EQL, relation: ps6079ExactlyZero},
	} {
		if _, err := parser.ParseExpr(test.expression); err != nil {
			t.Fatal(err)
		}
		relation, ok := ps6079ComparisonRelation(test.operation)
		if !ok || relation != test.relation {
			t.Fatalf("ps6079ComparisonRelation(%s) = (%v, %v), want (%v, true)", test.operation, relation, ok, test.relation)
		}
	}
}

func TestPS6079RouteEvidenceRequiresOperation(t *testing.T) {
	t.Parallel()
	route := ps6079Route{gate: &ps6079Gate{
		optimized: "ssmEncodeNEON",
		fallback:  "ssmEncodeScalar",
	}}
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "ssmEncodeFastPathHits", want: true},
		{name: "ssmDecodeFastPathHits"},
		{name: "encodeFallbackHits"},
		{name: "unexpectedFastPathHits"},
	} {
		if got := ps6079RouteEvidenceForRoute(test.name, route); got != test.want {
			t.Errorf("ps6079RouteEvidenceForRoute(%q) = %v, want %v", test.name, got, test.want)
		}
	}
}
