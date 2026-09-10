package checks

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

type ps6122Importer struct{ pkg *types.Package }

func (i ps6122Importer) Import(path string) (*types.Package, error) {
	if path == i.pkg.Path() {
		return i.pkg, nil
	}
	return nil, fmt.Errorf("unexpected import %q", path)
}

func TestPS6122(t *testing.T) {
	t.Parallel()
	analyzer := *PS6122.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) { return runPS6122Package(pass, "ps6122archsimd") }
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6122")
}

func TestPS6122CanonicalPackageIdentity(t *testing.T) {
	t.Parallel()
	archsimd := types.NewPackage("simd/archsimd", "archsimd")
	typeName := types.NewTypeName(token.NoPos, archsimd, "Int64x2", nil)
	vector := types.NewNamed(typeName, types.NewStruct(nil, nil), nil)
	archsimd.Scope().Insert(typeName)
	receiver := types.NewVar(token.NoPos, archsimd, "", vector)
	params := types.NewTuple(types.NewVar(token.NoPos, archsimd, "distance", types.Typ[types.Uint64]))
	results := types.NewTuple(types.NewVar(token.NoPos, archsimd, "", vector))
	method := types.NewFunc(token.NoPos, archsimd, "ShiftAllLeft", types.NewSignatureType(receiver, nil, nil, params, results, false))
	vector.AddMethod(method)
	archsimd.MarkComplete()

	files := token.NewFileSet()
	file, err := parser.ParseFile(files, "canonical.go", `package canonical
import "simd/archsimd"
func f(v archsimd.Int64x2) { for i := 0; i < 2; i++ { _ = v.ShiftAllLeft(52) } }
`, parser.AllErrors)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
	pkg, err := (&types.Config{Importer: ps6122Importer{pkg: archsimd}}).Check("example/canonical", files, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics []analysis.Diagnostic
	pass := &analysis.Pass{Analyzer: PS6122.Analyzer, Fset: files, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }}
	if _, err := runPS6122(pass); err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "left shift by 52") {
		t.Fatalf("canonical simd/archsimd diagnostics = %#v, want one constant shift finding", diagnostics)
	}
}

func TestPS6122Documentation(t *testing.T) {
	t.Parallel()
	for _, phrase := range []string{"does not prove emitted", "//perfscan:ignore PS6122", "every supported", "no automatic fix", "speedup claim"} {
		if !strings.Contains(strings.ToLower(PS6122.Doc.Text), strings.ToLower(phrase)) {
			t.Errorf("documentation does not contain %q", phrase)
		}
	}
}

func TestPS6122AllARM64VectorMethods(t *testing.T) {
	t.Parallel()
	archsimd := types.NewPackage("simd/archsimd", "archsimd")
	var source strings.Builder
	source.WriteString("package vectors\nimport \"simd/archsimd\"\n")
	for _, name := range []string{"Int8x16", "Uint8x16", "Int16x8", "Uint16x8", "Int32x4", "Uint32x4", "Int64x2", "Uint64x2"} {
		typeName := types.NewTypeName(token.NoPos, archsimd, name, nil)
		vector := types.NewNamed(typeName, types.NewStruct(nil, nil), nil)
		archsimd.Scope().Insert(typeName)
		for _, methodName := range []string{"ShiftAllLeft", "ShiftAllRight"} {
			receiver := types.NewVar(token.NoPos, archsimd, "", vector)
			params := types.NewTuple(types.NewVar(token.NoPos, archsimd, "distance", types.Typ[types.Uint64]))
			results := types.NewTuple(types.NewVar(token.NoPos, archsimd, "", vector))
			vector.AddMethod(types.NewFunc(token.NoPos, archsimd, methodName, types.NewSignatureType(receiver, nil, nil, params, results, false)))
		}
		fmt.Fprintf(&source, "func test%s(v archsimd.%s) { for i:=0;i<2;i++ { _=v.ShiftAllLeft(1); _=v.ShiftAllRight(1) } }\n", name, name)
	}
	archsimd.MarkComplete()
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, "vectors.go", source.String(), parser.AllErrors)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
	pkg, err := (&types.Config{Importer: ps6122Importer{pkg: archsimd}}).Check("example/vectors", files, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics []analysis.Diagnostic
	pass := &analysis.Pass{Analyzer: PS6122.Analyzer, Fset: files, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }}
	if _, err := runPS6122(pass); err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 16 {
		t.Fatalf("all ARM64 vector diagnostics = %d, want 16", len(diagnostics))
	}
}
