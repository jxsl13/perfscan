package checks

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
)

// Pre-repair, all four fully reprinted/reparsed authentic controls emitted one
// registered diagnostic (checks 1.872s). Direct NoPos AST injection originally
// appeared to reject; real-source normalization exposed that vacuous result.
func TestPS6136RegisteredInitializationOrderReview(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"maxLen", "v"} {
		for _, conditional := range []bool{false, true} {
			t.Run(field+map[bool]string{false: "/late", true: "/conditional"}[conditional], func(t *testing.T) {
				t.Parallel()
				fixture, err := ps6136CompileOwnersRewrite(t, "before", true, true, func(fs *token.FileSet, files []*ast.File) []*ast.File {
					var value ast.Expr
					var constructor *ast.FuncDecl
					for _, file := range files {
						for _, decl := range file.Decls {
							fn, ok := decl.(*ast.FuncDecl)
							if !ok || fn.Name.Name != "newGPTDecoder" {
								continue
							}
							constructor = fn
							ast.Inspect(fn.Body, func(n ast.Node) bool {
								lit, ok := n.(*ast.CompositeLit)
								if !ok {
									return true
								}
								id, ok := lit.Type.(*ast.Ident)
								if !ok || id.Name != "GPTDecoder" {
									return true
								}
								for i, elt := range lit.Elts {
									kv, ok := elt.(*ast.KeyValueExpr)
									if ok && kv.Key.(*ast.Ident).Name == field {
										value = kv.Value
										lit.Elts = append(lit.Elts[:i], lit.Elts[i+1:]...)
										break
									}
								}
								return true
							})
						}
					}
					if constructor == nil || value == nil {
						t.Fatal("exact GPT initializer missing")
					}
					assignment := &ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent("d"), Sel: ast.NewIdent(field)}}, Tok: token.ASSIGN, Rhs: []ast.Expr{value}}
					var inserted ast.Stmt = assignment
					if conditional {
						inserted = &ast.IfStmt{Cond: &ast.BinaryExpr{X: &ast.SelectorExpr{X: ast.NewIdent("cfg"), Sel: ast.NewIdent("Ctx")}, Op: token.GTR, Y: &ast.BasicLit{Kind: token.INT, Value: "1"}}, Body: &ast.BlockStmt{List: []ast.Stmt{assignment}}}
					}
					count := 0
					for i, statement := range constructor.Body.List {
						assign, ok := statement.(*ast.AssignStmt)
						if !ok || len(assign.Lhs) != 1 {
							continue
						}
						selector, ok := assign.Lhs[0].(*ast.SelectorExpr)
						if !ok || selector.Sel.Name != "logits" {
							continue
						}
						constructor.Body.List = append(constructor.Body.List[:i+1], append([]ast.Stmt{inserted}, constructor.Body.List[i+1:]...)...)
						count++
						break
					}
					if count != 1 {
						t.Fatal("exact logits allocation missing")
					}
					for i, file := range files {
						for _, decl := range file.Decls {
							if decl != constructor {
								continue
							}
							var source bytes.Buffer
							if err := printer.Fprint(&source, fs, file); err != nil {
								t.Fatal(err)
							}
							parsed, err := parser.ParseFile(fs, "order-review-gpt.go", source.Bytes(), parser.SkipObjectResolution)
							if err != nil {
								t.Fatal(err)
							}
							files[i] = parsed
						}
					}
					return files
				})
				if err != nil {
					t.Fatal(err)
				}
				pkg := ps6136FixtureSSA(fixture)
				contract := ps6136CompleteOwnerContract(pkg, "GPTDecoder")
				var diagnostics []analysis.Diagnostic
				pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg, Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }}
				context := ps6136ContractsContext(pass)
				selection := context.selection(pkg, context.callable(contract.ConstructorEntries[0]), &contract)
				if selection == nil {
					t.Fatal("selected genuine constructor missing")
				}
				if selection.initializationValues(16384) {
					t.Fatal("late or conditional geometry accepted at initialization gate")
				}
				if _, err := runPS6136WithContracts(pass, []config.OutputWorkspaceContract{contract}); err != nil {
					t.Fatal(err)
				}
				if len(diagnostics) != 0 {
					t.Fatalf("late or conditional initialization produced %d findings", len(diagnostics))
				}
			})
		}
	}
}
