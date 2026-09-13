package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6136ClosedSourceOwnerTransfers(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, extra string
		want        bool
	}{
		{"direct-source", "", true},
		{"alias", `func bad(d *Owner){other:=d;read(other)}`, false},
		{"opaque", `func opaque(*Owner);func bad(d *Owner){opaque(d)}`, false},
		{"interface-escape", `func sink(any){};func bad(d *Owner){sink(any(d))}`, false},
		{"compound-operand", `func opaque(*Owner);func wrap(d *Owner)*Owner{opaque(d);return &Owner{}};func bad(d *Owner){read(wrap(d))}`, false},
		{"captured-method", `func bad(d *Owner){fn:=d.Step;fn()}`, false},
		{"interior-pointer", `func bad(d *Owner){_= &((d.width))}`, false},
		{"whole-reset", `func bad(d *Owner){*d=Owner{}}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fs := token.NewFileSet()
			file, err := parser.ParseFile(fs, "transfers.go", `package transfers;type Owner struct{workspace []float32;width int};func fresh()*Owner{d:=&Owner{width:1};return d};func(d *Owner)Step(){read(d)};func read(d *Owner){_=d.width};`+test.extra, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
			pkg, err := (&types.Config{}).Check("transfers", fs, []*ast.File{file}, info)
			if err != nil {
				t.Fatal(err)
			}
			fixture := &analysisOwnerFixture{fs, []*ast.File{file}, info, pkg, nil}
			ssaPkg := ps6136FixtureSSA(fixture)
			owner := pkg.Scope().Lookup("Owner").Type().(*types.Named)
			workspace := owner.Underlying().(*types.Struct).Field(0)
			pass := &analysis.Pass{Fset: fs, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
			transfers := ps6136OwnerTransfers(pass, ssaPkg, owner)
			got := transfers != nil && ps6136ClosedObservations(pass, owner, workspace, transfers)
			if got != test.want {
				t.Fatalf("closed source transfers %v, want %v", got, test.want)
			}
		})
	}
}
