package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6136TypedFieldEffects(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body, field string
		want              bool
	}{
		{"read", "_=d.width", "width", true},
		{"write", "d.width=0", "width", false},
		{"increment", "d.width++", "width", false},
		{"range-rebind", "for d.width=range 3{}", "width", false},
		{"parenthesized-address", "_= &((d.width))", "width", false},
		{"same-spelling-other-type", "o:=Other{};o.width=0;_=d.width", "width", true},
		{"whole-ops", "d.ops=Ops{}", "ops", false},
		{"sibling-recorder", "d.ops.recorder=nil", "ops", true},
		{"interior-ops-address", "_= &((d.ops).recorder)", "ops", false},
		{"allocator-write", "d.ops.allocator=nil", "allocator", false},
		{"recorder-not-allocator", "d.ops.recorder=nil", "allocator", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fs := token.NewFileSet()
			file, err := parser.ParseFile(fs, "effects.go", `package effects;type Ops struct{allocator,recorder func()};type Owner struct{width int;ops Ops};type Other struct{width int};func f(d *Owner){`+test.body+`}`, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
			pkg, err := (&types.Config{}).Check("effects", fs, []*ast.File{file}, info)
			if err != nil {
				t.Fatal(err)
			}
			owner := pkg.Scope().Lookup("Owner").Type()
			if test.field == "allocator" {
				owner = pkg.Scope().Lookup("Ops").Type()
			}
			field, _, _ := types.LookupFieldOrMethod(owner, true, pkg, test.field)
			pass := &analysis.Pass{Fset: fs, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
			if got := ps6136FieldEffects(pass, field.(*types.Var), nil); got != test.want {
				t.Fatalf("field effects %v, want %v", got, test.want)
			}
		})
	}
}
