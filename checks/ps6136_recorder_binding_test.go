package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6136AuthenticRecorderFactoryBoundaries(t *testing.T) {
	t.Parallel()
	for _, entry := range []string{"New", "NewGPT"} {
		for _, mutation := range []string{"original", "wrong_native", "reversed_error", "late_factory", "discard_native"} {
			t.Run(entry+"/"+mutation, func(t *testing.T) {
				t.Parallel()
				fixture, err := ps6136CompileOwners(t, "before", true, true)
				if err != nil {
					t.Fatal(err)
				}
				pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
				wrapper := fixture.pkg.Scope().Lookup("mRec").Type().(*types.Named)
				field := wrapper.Underlying().(*types.Struct).Field(0)
				var literal *ast.FuncLit
				for _, file := range fixture.files {
					for _, declaration := range file.Decls {
						function, ok := declaration.(*ast.FuncDecl)
						if !ok || function.Name.Name != entry {
							continue
						}
						ast.Inspect(function.Body, func(node ast.Node) bool {
							pair, ok := node.(*ast.KeyValueExpr)
							if !ok {
								return true
							}
							key, ok := pair.Key.(*ast.Ident)
							if ok && key.Name == "newRecorder" {
								literal, _ = pair.Value.(*ast.FuncLit)
							}
							return true
						})
					}
				}
				if literal == nil {
					t.Fatal("pinned selected constructor factory missing")
				}
				id := "github.com/jxsl13/goai/backend/metal.NewRecorder"
				switch mutation {
				case "wrong_native":
					id = "github.com/jxsl13/goai/backend/metal.NewConcurrentRecorder"
				case "reversed_error":
					literal.Body.List[1].(*ast.IfStmt).Cond.(*ast.BinaryExpr).Op = token.EQL
				case "late_factory":
					literal.Body.List = append(literal.Body.List[:2], &ast.ExprStmt{X: literal.Body.List[0].(*ast.AssignStmt).Rhs[0]}, literal.Body.List[2])
				case "discard_native":
					nilValue := &ast.Ident{Name: "nil"}
					fixture.info.Uses[nilValue] = types.Universe.Lookup("nil")
					literal.Body.List[2].(*ast.ReturnStmt).Results[0].(*ast.CompositeLit).Elts[0] = nilValue
				}
				if got := ps6136RecorderLiteral(pass, literal, id, wrapper, field); got != (mutation == "original") {
					t.Fatalf("exact factory binding %v", got)
				}
			})
		}
	}
}
