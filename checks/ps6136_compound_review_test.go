package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
)

func TestPS6136RegisteredCompoundReviewGuards(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ owner, mutation string }{
		{"Decoder", "destination_index_escape"},
		{"Decoder", "unknown_bias_recorder"},
		{"GPTDecoder", "unknown_projection_recorder"},
		{"Decoder", "unknown_projection_recorder"},
		{"GPTDecoder", "no_actual_bulk_extent"},
		{"Decoder", "no_actual_bulk_extent"},
	} {
		t.Run(test.owner+"/"+test.mutation, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6136CompileOwnersRewrite(t, "before", true, true, func(fs *token.FileSet, files []*ast.File) []*ast.File {
				changed := 0
				for _, file := range files {
					for _, declaration := range file.Decls {
						function, ok := declaration.(*ast.FuncDecl)
						if !ok || function.Recv == nil {
							continue
						}
						pointer, ok := function.Recv.List[0].Type.(*ast.StarExpr)
						if !ok {
							continue
						}
						receiver, ok := pointer.X.(*ast.Ident)
						if !ok || receiver.Name != test.owner {
							continue
						}
						ast.Inspect(function.Body, func(node ast.Node) bool {
							call, ok := node.(*ast.CallExpr)
							if ok {
								selector, selected := call.Fun.(*ast.SelectorExpr)
								if selected && test.mutation == "unknown_bias_recorder" && function.Name.Name == "recordLogits" && selector.Sel.Name == "AddBias" {
									selector.X = &ast.CallExpr{Fun: &ast.Ident{Name: "unknownRecorder"}}
									changed++
								}
								projectionFunction := function.Name.Name == "recordLogits" || test.owner == "GPTDecoder" && function.Name.Name == "Step"
								var projector *ast.SelectorExpr
								if selected {
									projector, _ = selector.X.(*ast.SelectorExpr)
								}
								if selected && test.mutation == "unknown_projection_recorder" && projectionFunction && selector.Sel.Name == "record" && projector != nil && (projector.Sel.Name == "head" || projector.Sel.Name == "out") {
									call.Args[0] = &ast.CallExpr{Fun: &ast.Ident{Name: "unknownRecorder"}}
									changed++
								}
								if selected && test.mutation == "no_actual_bulk_extent" && function.Name.Name == "StepN" && len(call.Args) == 3 {
									call.Args[2] = &ast.Ident{Name: "true"}
									changed++
								}
							}
							if test.mutation == "destination_index_escape" && function.Name.Name == "Generate" {
								assignment, ok := node.(*ast.AssignStmt)
								if ok && len(assignment.Lhs) == 1 && len(assignment.Rhs) == 1 {
									conversion, ok := assignment.Rhs[0].(*ast.CallExpr)
									if ok && len(conversion.Args) == 1 {
										index, ok := conversion.Args[0].(*ast.IndexExpr)
										if ok {
											if source, ok := index.X.(*ast.Ident); ok && source.Name == "lf" {
												destination := assignment.Lhs[0].(*ast.IndexExpr)
												iterator := destination.Index.(*ast.Ident)
												destination.X = &ast.CallExpr{Fun: &ast.Ident{Name: "unknownDestination"}, Args: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: &ast.Ident{Name: iterator.Name}}}}
												changed++
											}
										}
									}
								}
							}
							return true
						})
					}
				}
				if changed != 1 {
					t.Fatalf("compound mutation changes %d want1", changed)
				}
				extra, err := parser.ParseFile(fs, "review-helper.go", `package llamagpu;var externalRecorder recorder;func unknownRecorder()recorder{return externalRecorder};func unknownDestination(p *int)[]float64{*p=1<<20;return make([]float64,(1<<20)+1)}`, parser.SkipObjectResolution)
				if err != nil {
					t.Fatal(err)
				}
				return append(files, extra)
			})
			if err != nil {
				t.Fatal(err)
			}
			c := ps6136CompleteOwnerContract(ps6136FixtureSSA(fixture), test.owner)
			var diagnostics []analysis.Diagnostic
			pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg, Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }}
			if _, err := runPS6136WithContracts(pass, []config.OutputWorkspaceContract{c}); err != nil {
				t.Fatal(err)
			}
			if len(diagnostics) != 0 {
				t.Fatalf("unproved compound observation produced %d findings", len(diagnostics))
			}
		})
	}
}
