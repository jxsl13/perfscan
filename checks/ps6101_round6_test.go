package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

func TestPS6101NumericTypeParameterConstraints(t *testing.T) {
	t.Parallel()
	const source = `package repro
	type Floats interface { ~float32 | ~float64 }
	type Mixed interface { ~float64 | string }
	func Numeric[T Floats]() {}
	func NonNumeric[T Mixed]() {}
	func Unrestricted[T any]() {}
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
	typeParameter := func(name string) types.Type {
		t.Helper()
		function, ok := pkg.Scope().Lookup(name).(*types.Func)
		if !ok {
			t.Fatalf("%s is not a function", name)
		}
		signature := function.Type().(*types.Signature)
		return signature.TypeParams().At(0)
	}
	if !ps6101NumericType(typeParameter("Numeric")) {
		t.Error("finite float type set must be numeric")
	}
	for _, name := range []string{"NonNumeric", "Unrestricted"} {
		if ps6101NumericType(typeParameter(name)) {
			t.Errorf("%s type set must not be classified as wholly numeric", name)
		}
	}
}
