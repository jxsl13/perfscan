package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS1006ConversionWrapperIdentity(t *testing.T) {
	t.Parallel()
	const source = `package sample
type word = int8
func tile(a []float64, t, c, channels int) {
	_ = a[int(word(t*channels))+c]
	{
		type word = int16
		_ = a[int(word(t*channels))+c+1]
	}
	_ = a[t*channels+c+2]
	_ = a[int(t*channels)+c+3]
}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "identity.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := (&types.Config{}).Check("sample", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	function := file.Decls[1].(*ast.FuncDecl)
	inner := info.Defs[function.Type.Params.List[1].Names[0]]
	outer := info.Defs[function.Type.Params.List[1].Names[1]]
	var keys []string
	ast.Inspect(function.Body, func(node ast.Node) bool {
		index, ok := node.(*ast.IndexExpr)
		if !ok {
			return true
		}
		key, _, ok := ps1006StrideAndOuterOffset(pass, index.Index, inner, outer, nil)
		if !ok {
			t.Fatalf("did not recognize index at %s", fset.Position(index.Pos()))
		}
		keys = append(keys, key)
		return true
	})
	if len(keys) != 4 {
		t.Fatalf("got %d stride keys, want 4: %v", len(keys), keys)
	}
	if keys[0] == keys[1] {
		t.Fatalf("shadowed int8/int16 conversion keys collapsed: %q", keys[0])
	}
	if keys[2] != keys[3] {
		t.Fatalf("no-op int conversion changed key: direct=%q converted=%q", keys[2], keys[3])
	}
}
