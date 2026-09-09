package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestPS6081SuppressionExactDeclarationDocLine(t *testing.T) {
	t.Parallel()
	source := `package p
//perfscan:shared-fanout-validated measured backend primitive
func accepted() {}
// xperfscan:shared-fanout-validated
func prefix() {}
//perfscan:shared-fanout-validated-suffix
func suffix() {}
// Perfscan:shared-fanout-validated
func capital() {}
// prose perfscan:shared-fanout-validated
func prose() {}
func body() { //perfscan:shared-fanout-validated
}
`
	file, err := parser.ParseFile(token.NewFileSet(), "suppression.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool)
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok {
			got[function.Name.Name] = ps6081Validated(function)
		}
	}
	if !got["accepted"] || got["prefix"] || got["suffix"] || got["capital"] || got["prose"] || got["body"] {
		t.Fatalf("suppression scope mismatch: %v", got)
	}
}
