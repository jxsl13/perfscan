package checks

import (
	"github.com/jxsl13/perfscan/config"
	"go/ast"
	"go/parser"
	"go/token"
	"golang.org/x/tools/go/analysis"
	"testing"
)

func TestPS6136RegisteredOwnerSourceAdversaries(t *testing.T) {
	t.Parallel()
	for _, owner := range []string{"GPTDecoder", "Decoder"} {
		for _, mutation := range []string{"release_early_return", "late_error_write", "late_allocation", "allocator_rebind", "constructor_workspace_release", "ledger_alias_release", "ledger_drop", "geometry_write", "whole_owner_escape", "full_logical_download", "wrong_recorder", "wrong_wrapper"} {
			t.Run(owner+"/"+mutation, func(t *testing.T) {
				t.Parallel()
				fixture, err := ps6136CompileOwnersRewrite(t, "before", true, true, func(fs *token.FileSet, files []*ast.File) []*ast.File {
					constructor := "newGPTDecoder"
					if owner == "Decoder" {
						constructor = "newDecoder"
					}
					parseStatements := func(body string) []ast.Stmt {
						file, err := parser.ParseFile(fs, "adversary-"+mutation+".go", "package llamagpu;func temporary(){"+body+"}", parser.SkipObjectResolution)
						if err != nil {
							t.Fatal(err)
						}
						return file.Decls[0].(*ast.FuncDecl).Body.List
					}
					changed := 0
					for _, file := range files {
						for _, declaration := range file.Decls {
							function, ok := declaration.(*ast.FuncDecl)
							if !ok {
								continue
							}
							if function.Name.Name == constructor {
								var body string
								switch mutation {
								case "late_error_write":
									body = "err=nil"
								case "late_allocation":
									body = "_=mk(nil)"
								case "allocator_rebind":
									body = "d.ops.newBuffer=nil"
								case "constructor_workspace_release":
									body = "d.logits.b.Release()"
								case "ledger_alias_release":
									body = "d.all[len(d.all)-1].Release()"
								case "whole_owner_escape":
									body = "_=func(){_=d}"
								}
								if body != "" {
									index := len(function.Body.List) - 1
									function.Body.List = append(function.Body.List[:index], append(parseStatements(body), function.Body.List[index:]...)...)
									changed++
								}
							}
							receiver := ""
							if function.Recv != nil {
								if pointer, ok := function.Recv.List[0].Type.(*ast.StarExpr); ok {
									if identifier, ok := pointer.X.(*ast.Ident); ok {
										receiver = identifier.Name
									}
								}
							}
							if receiver == owner && function.Name.Name == "Release" && mutation == "release_early_return" {
								function.Body.List = append(parseStatements("if d.v>0{return}"), function.Body.List...)
								changed++
							}
							if receiver == owner && function.Name.Name == "Step" {
								body := ""
								switch mutation {
								case "ledger_drop":
									body = "d.all=nil"
								case "geometry_write":
									body = "d.v++"
								}
								if body != "" {
									function.Body.List = append(parseStatements(body), function.Body.List...)
									changed++
								}
								if mutation == "full_logical_download" {
									ast.Inspect(function.Body, func(node ast.Node) bool {
										call, ok := node.(*ast.CallExpr)
										if !ok {
											return true
										}
										identifier, ok := call.Fun.(*ast.Ident)
										if !ok || identifier.Name != "make" || len(call.Args) != 2 {
											return true
										}
										selector, ok := call.Args[1].(*ast.SelectorExpr)
										if !ok || selector.Sel.Name != "v" {
											return true
										}
										call.Args[1] = &ast.BinaryExpr{X: &ast.SelectorExpr{X: &ast.Ident{Name: "d"}, Sel: &ast.Ident{Name: "maxLen"}}, Op: token.MUL, Y: selector}
										changed++
										return true
									})
								}
							}
							entry := "NewGPT"
							if owner == "Decoder" {
								entry = "New"
							}
							if function.Name.Name == entry && (mutation == "wrong_recorder" || mutation == "wrong_wrapper") {
								ast.Inspect(function.Body, func(node ast.Node) bool {
									pair, ok := node.(*ast.KeyValueExpr)
									if !ok {
										return true
									}
									key, ok := pair.Key.(*ast.Ident)
									if !ok {
										return true
									}
									if mutation == "wrong_recorder" && key.Name == "newRecorder" {
										literal := pair.Value.(*ast.FuncLit)
										call := literal.Body.List[0].(*ast.AssignStmt).Rhs[0].(*ast.CallExpr)
										call.Fun.(*ast.SelectorExpr).Sel.Name = "NewConcurrentRecorder"
										changed++
									}
									if mutation == "wrong_wrapper" && key.Name == "newBuffer" {
										literal := pair.Value.(*ast.FuncLit)
										value := literal.Body.List[2].(*ast.ReturnStmt).Results[0].(*ast.CompositeLit)
										value.Type = &ast.Ident{Name: "otherBufferWrapper"}
										changed++
									}
									return true
								})
							}
						}
					}
					if mutation == "wrong_wrapper" {
						extra, err := parser.ParseFile(fs, "other-wrapper.go", "package llamagpu;type otherBufferWrapper struct{mBuf}", parser.SkipObjectResolution)
						if err != nil {
							t.Fatal(err)
						}
						files = append(files, extra)
						// The nested original native value preserves the ABI while changing wrapper identity.
						for _, file := range files {
							ast.Inspect(file, func(node ast.Node) bool {
								value, ok := node.(*ast.CompositeLit)
								if ok {
									if identifier, ok := value.Type.(*ast.Ident); ok && identifier.Name == "otherBufferWrapper" {
										value.Elts = []ast.Expr{&ast.CompositeLit{Type: &ast.Ident{Name: "mBuf"}, Elts: value.Elts}}
									}
								}
								return true
							})
						}
					}
					if changed != 1 {
						t.Fatalf("owner adversary rewrites %d want 1", changed)
					}
					return files
				})
				if err != nil {
					t.Fatal(err)
				}
				pkg := ps6136FixtureSSA(fixture)
				c := ps6136CompleteOwnerContract(pkg, owner)
				var diagnostics []analysis.Diagnostic
				pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg, Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }}
				if _, err := runPS6136WithContracts(pass, []config.OutputWorkspaceContract{c}); err != nil {
					t.Fatal(err)
				}
				if len(diagnostics) != 0 {
					t.Fatalf("unsafe owner-source candidate %d", len(diagnostics))
				}
			})
		}
	}
}
