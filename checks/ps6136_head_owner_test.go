package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6136AuthenticModelHeadWidth(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		for _, name := range []string{"newGPTDecoder", "newDecoder"} {
			t.Run(revision+"/"+name, func(t *testing.T) {
				t.Parallel()
				fixture, err := ps6136CompileOwners(t, revision, true, true)
				if err != nil {
					t.Fatal(err)
				}
				pkg := ps6136FixtureSSA(fixture)
				function := pkg.Func(name)
				entryName := "NewGPT"
				if name == "newDecoder" {
					entryName = "New"
				}
				entry := ps6125NewSSAContext(pkg.Func(entryName), nil, nil, 512)
				var context *ps6125SSAContext
				for _, call := range ps6125ContextCalls(entry) {
					if call.Call.StaticCallee() == function {
						context = entry.call(call)
					}
				}
				if context == nil {
					t.Fatal("selected public constructor source invocation unavailable")
				}
				modelType := function.Params[0].Type().Underlying().(*types.Pointer).Elem().(*types.Named)
				headName, projectorName := "Head", "head"
				ownerName := "GPTDecoder"
				if name == "newDecoder" {
					headName, projectorName, ownerName = "Out", "out", "Decoder"
				}
				head, _, _ := types.LookupFieldOrMethod(modelType, true, modelType.Obj().Pkg(), headName)
				tensorType := head.Type().Underlying().(*types.Pointer).Elem().(*types.Named)
				shape, _, _ := types.LookupFieldOrMethod(tensorType, true, tensorType.Obj().Pkg(), "Shape")
				ownerType := pkg.Pkg.Scope().Lookup(ownerName).Type().(*types.Named)
				projector, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, projectorName)
				projectors := make(map[*types.Named]*types.Var)
				for _, kind := range []string{"f32Linear"} {
					named := pkg.Pkg.Scope().Lookup(kind).Type().(*types.Named)
					width, _, _ := types.LookupFieldOrMethod(named, true, pkg.Pkg, "n")
					projectors[named] = width.(*types.Var)
				}
				paths := ps6125AccessPaths{flow: context.flow}
				count := 0
				for _, block := range function.Blocks {
					for _, instruction := range block.Instrs {
						store, ok := instruction.(*ssa.Store)
						if !ok {
							continue
						}
						path := paths.resolve(store.Addr)
						if !path.known || len(path.access.fields) != 1 || path.access.fields[0] != projector {
							continue
						}
						if !ps6136ProjectorHeadWidth(context, store.Val, projectors, context.reference(function.Params[0]), modelType, head.(*types.Var), ps6090FunctionID(shape.(*types.Func)), 1, 256) {
							t.Fatal("actual selected model head -> source linear factory -> projector output width not proved")
						}
						count++
					}
				}
				if count != 1 {
					t.Fatalf("projector assignments %d", count)
				}
			})
		}
	}
}
