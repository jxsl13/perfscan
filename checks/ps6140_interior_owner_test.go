package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6140AuthenticInteriorCellEscapes(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, statement, declarations string
		want                          bool
	}{
		{"original", "", "", true},
		{"opaque-forwarding", "escapeCell(overflow)", "func escapeCell(*growBuffer)", false},
		{"retained-cell", "cellSink=overflow", "var cellSink *growBuffer", false},
		{"retained-field-address", "scalarSink=&overflow.n", "var scalarSink *int", false},
		{"capture", "cellThunk=func(){overflow.release()}", "var cellThunk func()", false},
		{"deferred-forwarding", "defer overflow.release()", "", false},
		{"implicit-other-call-site", "", "func escapeCell(*growBuffer);func(d *Decoder)exposeCell(){escapeCell(&d.fullLogits)}", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6140CompileLoadedMetalRewrite(t, "before", func(fs *token.FileSet, files []*ast.File) []*ast.File {
				if test.statement == "" && test.declarations == "" {
					return files
				}
				injected, err := parser.ParseFile(fs, "injected.go", "package llamagpu;func injected(){"+test.statement+"};"+test.declarations, parser.ParseComments|parser.SkipObjectResolution)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, file := range files {
					for _, declaration := range file.Decls {
						fn, ok := declaration.(*ast.FuncDecl)
						if !ok || fn.Name.Name != "logitsForRows" {
							continue
						}
						found = true
						fn.Body.List = append(injected.Decls[0].(*ast.FuncDecl).Body.List, fn.Body.List...)
						file.Decls = append(file.Decls, injected.Decls[1:]...)
					}
				}
				if !found {
					t.Fatal("original logits helper missing")
				}
				return files // Compiler reparses the complete mutated sources.
			})
			if err != nil {
				t.Fatal(err)
			}
			pkg := ps6136FixtureSSA(fixture)
			owner := pkg.Pkg.Scope().Lookup("Decoder").Type().(*types.Named)
			cell := ps6136FieldVar(owner, "fullLogits")
			if cell == nil || !ps6136FlatCell(cell.Type()) || pkg.Func("logitsForRows") == nil {
				t.Fatal("typed original cell/helper prerequisite missing")
			}
			pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
			proved := ps6136InteriorOwnerCells(pass, pkg, owner, ps6136FieldVar(owner, "blocks"))
			found, accepted := false, false
			for _, file := range pass.Files {
				for _, declaration := range file.Decls {
					fn, ok := declaration.(*ast.FuncDecl)
					if !ok || fn.Name.Name != "stepN" {
						continue
					}
					ast.Inspect(fn.Body, func(node ast.Node) bool {
						address, ok := node.(*ast.UnaryExpr)
						if !ok || address.Op != token.AND {
							return true
						}
						selector, ok := address.X.(*ast.SelectorExpr)
						if !ok {
							return true
						}
						if selection := pass.TypesInfo.Selections[selector]; selection != nil && selection.Obj() == cell {
							found = true
							accepted = accepted || proved[address]
						}
						return true
					})
				}
			}
			if !found || accepted != test.want {
				t.Fatalf("actual stepN interior address found=%v accepted=%v want=%v", found, accepted, test.want)
			}
		})
	}
}
