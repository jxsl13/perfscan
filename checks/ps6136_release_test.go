package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6136AuthenticReleaseLists(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		t.Run(revision, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6136CompileOwners(t, revision, false, true)
			if err != nil {
				t.Fatal(err)
			}
			pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
			for _, ownerName := range []string{"GPTDecoder", "Decoder"} {
				owner := fixture.pkg.Scope().Lookup(ownerName).Type().(*types.Named)
				list, _, _ := types.LookupFieldOrMethod(owner, true, fixture.pkg, "all")
				element := list.Type().Underlying().(*types.Slice).Elem()
				release, _, _ := types.LookupFieldOrMethod(element, true, fixture.pkg, "Release")
				found := false
				for _, file := range fixture.files {
					for _, declaration := range file.Decls {
						function, ok := declaration.(*ast.FuncDecl)
						if !ok || function.Recv == nil || function.Name.Name != "Release" || !types.Identical(fixture.info.TypeOf(function.Recv.List[0].Type), types.NewPointer(owner)) {
							continue
						}
						found = true
						receiver := fixture.info.Defs[function.Recv.List[0].Names[0]]
						if ps6136ReleaseList(pass, function, receiver, list.(*types.Var), release.(*types.Func)) == nil {
							t.Fatalf("authentic %s retained-list release unproved", ownerName)
						}
					}
				}
				if !found {
					t.Fatalf("authentic %s release absent", ownerName)
				}
			}
		})
	}
}

func TestPS6136ReleaseListBoundaries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body string
		valid      bool
	}{
		{"canonical", `for _,b:=range d.all{if b!=nil{b.Release()}};d.all=nil`, true},
		{"early_return", `if flag{return};for _,b:=range d.all{if b!=nil{b.Release()}};d.all=nil`, false},
		{"goto_bypass", `goto done;for _,b:=range d.all{if b!=nil{b.Release()}};d.all=nil;done:return`, false},
		{"conditional_clear", `for _,b:=range d.all{if b!=nil{b.Release()}};if flag{d.all=nil}`, false},
		{"break", `for _,b:=range d.all{if b!=nil{b.Release();break}};d.all=nil`, false},
		{"double_release", `for _,b:=range d.all{if b!=nil{b.Release();b.Release()}};d.all=nil`, false},
		{"wrong_release", `for _,b:=range d.all{if b!=nil{b.Close()}};d.all=nil`, false},
		{"filtered", `for i,b:=range d.all{if i==0&&b!=nil{b.Release()}};d.all=nil`, false},
		{"wrong_list", `for _,b:=range d.other{if b!=nil{b.Release()}};d.all=nil`, false},
		{"no_clear", `for _,b:=range d.all{if b!=nil{b.Release()}}`, false},
		{"clear_wrong_list", `for _,b:=range d.all{if b!=nil{b.Release()}};d.other=nil`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fs := token.NewFileSet()
			file, err := parser.ParseFile(fs, "release.go", `package ownership;var flag bool;type buffer interface{Release();Close()};type owner struct{all,other []buffer};func(d *owner)release(){`+test.body+`}`, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
			pkg, err := (&types.Config{}).Check("ownership", fs, []*ast.File{file}, info)
			if err != nil {
				t.Fatal(err)
			}
			owner := pkg.Scope().Lookup("owner").Type().(*types.Named)
			list, _, _ := types.LookupFieldOrMethod(owner, true, pkg, "all")
			release, _, _ := types.LookupFieldOrMethod(pkg.Scope().Lookup("buffer").Type(), true, pkg, "Release")
			var declaration *ast.FuncDecl
			for _, node := range file.Decls {
				if function, ok := node.(*ast.FuncDecl); ok {
					declaration = function
				}
			}
			pass := &analysis.Pass{Fset: fs, Files: []*ast.File{file}, TypesInfo: info, Pkg: pkg}
			approved := ps6136ReleaseList(pass, declaration, info.Defs[declaration.Recv.List[0].Names[0]], list.(*types.Var), release.(*types.Func))
			if (approved != nil) != test.valid {
				t.Fatalf("closed retained-list release=%v want %v", approved != nil, test.valid)
			}
		})
	}
}
