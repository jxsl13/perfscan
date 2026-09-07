package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

func TestPS6101ReferenceBearingTypecheckRepros(t *testing.T) {
	t.Parallel()
	const source = `package repro
	type Safe struct { Value float64; Nested struct { Count int } }
	type Slice struct { Values []float64 }
	type Nested struct { Inner struct { Values []float64 } }
	type Array struct { Values [2]*float64 }
	type Map struct { Values map[string]float64 }
	type Pointer struct { Value *float64 }
	type Interface struct { Value any }
	type Function struct { Value func() }
	type Channel struct { Value chan float64 }
	`
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "repro.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	configuration := types.Config{}
	info := &types.Info{Defs: make(map[*ast.Ident]types.Object)}
	pkg, err := configuration.Check("repro", fileSet, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Slice", "Nested", "Array", "Map", "Pointer", "Interface", "Function", "Channel"} {
		if object := pkg.Scope().Lookup(name); object == nil || !ps6101ContainsReference(object.Type()) {
			t.Errorf("%s must be classified as reference-bearing", name)
		}
	}
	if object := pkg.Scope().Lookup("Safe"); object == nil || ps6101ContainsReference(object.Type()) {
		t.Error("Safe must remain a value-only negative control")
	}
}
