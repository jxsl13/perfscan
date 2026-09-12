package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6136ClosedObservations(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, extra string
		valid       bool
	}{
		{"proved_use", "", true},
		{"sibling_owner", `type Other struct{logits []float32};func extra(other *Other){_=other.logits}`, true},
		{"nil_comparison", `func extra(d *Owner)bool{return ((d))==nil || nil!=d}`, true},
		{"pointer_comparison", `func extra(d, other *Owner)bool{return d==other}`, false},
		{"unclassified_use", `func extra(d *Owner){_=d.logits}`, false},
		{"alias", `func extra(d *Owner){alias:=d;_=alias}`, false},
		{"capture", `func extra(d *Owner){fn:=func(){_=d};_=fn}`, false},
		{"interior_geometry_pointer", `func extra(d *Owner){_= &((d.width))}`, false},
		{"opaque_mutation", `func extra(d *Owner){opaque(d)};func opaque(*Owner){}`, false},
		{"whole_reset", `func extra(d *Owner){*d=Owner{}}`, false},
		{"keyed_alternate", `var other=&Owner{logits:[]float32{1}}`, false},
		{"proved_initializer_key", `func initOwner()*Owner{return &Owner{logits:[]float32{1}}}`, true},
		{"proved_initializer_key_not_rhs", `func initOwner(d *Owner)*Owner{return &Owner{logits:d.logits}}`, false},
		{"positional_alternate", `var other=&Owner{[]float32{1},1}`, false},
		{"hidden_method", `type Holder struct{D *Owner};func extra(h Holder){h.D.mutate()};func(d *Owner)mutate(){d.width++}`, false},
		{"hidden_opaque", `type Holder struct{D *Owner};func extra(h Holder){opaque(h.D)};func opaque(*Owner){}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fs := token.NewFileSet()
			file, err := parser.ParseFile(fs, "coverage.go", `package coverage;type Owner struct{logits []float32;width int};func observed(d *Owner){_=d.logits};`+test.extra, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
			pkg, err := (&types.Config{}).Check("coverage", fs, []*ast.File{file}, info)
			if err != nil {
				t.Fatal(err)
			}
			owner := pkg.Scope().Lookup("Owner").Type().(*types.Named)
			workspace := owner.Underlying().(*types.Struct).Field(0)
			proved := map[ast.Node]bool{}
			if test.name == "proved_initializer_key" || test.name == "proved_initializer_key_not_rhs" {
				ast.Inspect(file, func(node ast.Node) bool {
					if initializer, ok := node.(*ast.KeyValueExpr); ok {
						proved[initializer.Key] = true
					}
					return true
				})
			}
			for _, declaration := range file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Name.Name != "observed" {
					continue
				}
				ast.Inspect(function.Body, func(node ast.Node) bool {
					if selection, ok := node.(*ast.SelectorExpr); ok {
						proved[selection] = true
					}
					return true
				})
			}
			pass := &analysis.Pass{Fset: fs, Files: []*ast.File{file}, TypesInfo: info, Pkg: pkg}
			if got := ps6136ClosedObservations(pass, owner, workspace, proved); got != test.valid {
				t.Fatalf("closed observations=%v want %v", got, test.valid)
			}
		})
	}
}
