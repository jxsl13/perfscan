package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6136AuthenticCapacityTransferPrefix(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		t.Run(revision, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6136CompileOwners(t, revision, false, true)
			if err != nil {
				t.Fatal(err)
			}
			pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
			owner := fixture.pkg.Scope().Lookup("Decoder").Type().(*types.Named)
			width, _, _ := types.LookupFieldOrMethod(owner, true, fixture.pkg, "v")
			var tensorPackage *types.Package
			for _, imported := range fixture.pkg.Imports() {
				if imported.Path() == "github.com/jxsl13/goai/tensor" {
					tensorPackage = imported
				}
			}
			storage, _, _ := types.LookupFieldOrMethod(types.NewPointer(tensorPackage.Scope().Lookup("Tensor").Type()), true, fixture.pkg, "Storage")
			f32, _, _ := types.LookupFieldOrMethod(types.NewPointer(tensorPackage.Scope().Lookup("Storage").Type()), true, fixture.pkg, "F32")
			found := 0
			for _, file := range fixture.files {
				for _, declaration := range file.Decls {
					function, ok := declaration.(*ast.FuncDecl)
					if !ok || function.Recv == nil || function.Name.Name != "Generate" || !types.Identical(fixture.info.TypeOf(function.Recv.List[0].Type), types.NewPointer(owner)) {
						continue
					}
					ast.Inspect(function.Body, func(node ast.Node) bool {
						call, ok := node.(*ast.CallExpr)
						if !ok {
							return true
						}
						selector, ok := call.Fun.(*ast.SelectorExpr)
						if !ok || selector.Sel.Name != "ToHost" {
							return true
						}
						found++
						if !ps6136HostPrefix(pass, call, fixture.info.Defs[function.Recv.List[0].Names[0]], width.(*types.Var), storage.(*types.Func), f32.(*types.Func)) {
							t.Fatal("authentic physical-capacity transfer's first-vocab logical consumption unproved")
						}
						return true
					})
				}
			}
			if found != 1 {
				t.Fatalf("authentic capacity transfer count %d", found)
			}
		})
	}
}

func TestPS6136HostPrefixBoundaries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body string
		valid      bool
	}{
		{"prefix", `for i:=0;i<d.v;i++{buf[i]=float64(lf[i])}`, true},
		{"full_storage", `for i:=0;i<len(lf);i++{buf[i]=float64(lf[i])}`, false},
		{"wrong_width", `for i:=0;i<d.c;i++{buf[i]=float64(lf[i])}`, false},
		{"offset", `for i:=0;i<d.v;i++{buf[i]=float64(lf[i+1])}`, false},
		{"nonzero_start", `for i:=1;i<d.v;i++{buf[i]=float64(lf[i])}`, false},
		{"alias", `alias:=lf;_=alias;for i:=0;i<d.v;i++{buf[i]=float64(lf[i])}`, false},
		{"full_extra_consumer", `consume(lf);for i:=0;i<d.v;i++{buf[i]=float64(lf[i])}`, false},
		{"index_rebound", `for i:=0;i<d.v;i++{i++;buf[i]=float64(lf[i])}`, false},
		{"destination_mutates_index", `for i:=0;i<d.v;i++{dst(&i)[i]=float64(lf[i])}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fs := token.NewFileSet()
			file, err := parser.ParseFile(fs, "prefix.go", `package prefix;type owner struct{v,c int};type tensor struct{};type storage struct{};func(tensor)Storage()storage{return storage{}};func(storage)F32()[]float32{return nil};func transfer()(tensor,error){return tensor{},nil};func consume([]float32){};func dst(p *int)[]float64{*p=1023;return make([]float64,1024)};func(d *owner)read(){buf:=make([]float64,d.v);_=buf;l,err:=transfer();_=err;lf:=l.Storage().F32();`+test.body+`}`, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
			pkg, err := (&types.Config{}).Check("prefix", fs, []*ast.File{file}, info)
			if err != nil {
				t.Fatal(err)
			}
			owner := pkg.Scope().Lookup("owner").Type().(*types.Named)
			width, _, _ := types.LookupFieldOrMethod(owner, true, pkg, "v")
			storage, _, _ := types.LookupFieldOrMethod(pkg.Scope().Lookup("tensor").Type(), true, pkg, "Storage")
			f32, _, _ := types.LookupFieldOrMethod(pkg.Scope().Lookup("storage").Type(), true, pkg, "F32")
			var function *ast.FuncDecl
			var call *ast.CallExpr
			for _, declaration := range file.Decls {
				if candidate, ok := declaration.(*ast.FuncDecl); ok && candidate.Name.Name == "read" {
					function = candidate
					ast.Inspect(candidate.Body, func(node ast.Node) bool {
						if candidate, ok := node.(*ast.CallExpr); ok {
							if id, ok := candidate.Fun.(*ast.Ident); ok && info.Uses[id] == pkg.Scope().Lookup("transfer") {
								call = candidate
							}
						}
						return true
					})
				}
			}
			pass := &analysis.Pass{Fset: fs, Files: []*ast.File{file}, TypesInfo: info, Pkg: pkg}
			got := ps6136HostPrefix(pass, call, info.Defs[function.Recv.List[0].Names[0]], width.(*types.Var), storage.(*types.Func), f32.(*types.Func))
			if got != test.valid {
				t.Fatalf("closed logical prefix=%v want %v", got, test.valid)
			}
		})
	}
}
