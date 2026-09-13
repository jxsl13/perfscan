package checks

import (
	"go/ast"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6136AuthenticNativeLeafForwarding(t *testing.T) {
	t.Parallel()
	fixture, err := ps6136CompileOwners(t, "before", true, true)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
	bridge := fixture.pkg.Scope().Lookup("mb").(*types.Func)
	wrapper := fixture.pkg.Scope().Lookup("mBuf").Type().(*types.Named)
	field := wrapper.Underlying().(*types.Struct).Field(0)
	count := 0
	for _, file := range fixture.files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if function.Name.Name == "mb" {
				if !ps6136BufferBridge(pass, function, wrapper, field) {
					t.Fatal("actual Metal buffer adapter source assertion not proved")
				}
				count++
			}
			if object, ok := fixture.info.Defs[function.Name].(*types.Func); ok && ps6090FunctionID(object) == "github.com/jxsl13/goai/llamagpu.f32Linear.record" {
				projector := fixture.pkg.Scope().Lookup("f32Linear").Type()
				field := func(name string) *types.Var {
					object, _, _ := types.LookupFieldOrMethod(projector, true, fixture.pkg, name)
					return object.(*types.Var)
				}
				if !ps6136ProjectionLowering(pass, function, "github.com/jxsl13/goai/llamagpu.recorder.MatMul", field("w"), field("k"), field("n")) {
					t.Fatal("actual F32 projection buffer/rows/width lowering not proved")
				}
				count++
			}
			if function.Recv == nil || function.Name.Name != "MatMul" && function.Name.Name != "AddBias" {
				continue
			}
			id := "github.com/jxsl13/goai/backend/metal.Recorder." + function.Name.Name
			if !ps6136ForwardedLeaf(pass, function, id, bridge, []int{0, 1, 2}) {
				t.Fatal("actual Native Metal buffer + geometry role-preserving adapter not proved")
			}
			if ps6136ForwardedLeaf(pass, function, "other.Recorder."+function.Name.Name, bridge, []int{0, 1, 2}) {
				t.Fatal("wrong native identity accepted")
			}
			count++
		}
	}
	if count != 4 {
		t.Fatalf("actual native source bridges %d", count)
	}
}
