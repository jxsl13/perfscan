package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6136AuthenticSharedPublicAllocatorSnapshot(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		t.Run(revision, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6136CompileOwners(t, revision, true, true)
			if err != nil {
				t.Fatal(err)
			}
			pkg := ps6136FixtureSSA(fixture)
			entry := ps6125NewSSAContext(pkg.Func("New"), nil, nil, 128)
			var root *ps6125SSAContext
			for _, call := range ps6125ContextCalls(entry) {
				if call.Call.StaticCallee() == pkg.Func("newDecoder") {
					root = entry.call(call)
				}
			}
			ownerType := pkg.Pkg.Scope().Lookup("Decoder").Type().(*types.Named)
			workspace, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, "logits")
			ops, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, "ops")
			backendType := pkg.Pkg.Scope().Lookup("backendOps").Type().Underlying().(*types.Struct)
			allocator, indices, _ := types.LookupFieldOrMethod(pkg.Pkg.Scope().Lookup("backendOps").Type(), true, pkg.Pkg, "newBuffer")
			slot := pkg.Pkg.Scope().Lookup("bufSlot").Type().Underlying().(*types.Struct).Field(0)
			found := 0
			for _, call := range ps6125ContextCalls(root) {
				if call.Call.StaticCallee() == nil || call.Call.StaticCallee().Name() != "allocScratch" {
					continue
				}
				allocation := root.call(call)
				paths := ps6125AccessPaths{flow: allocation.flow}
				for _, block := range allocation.flow.function.Blocks {
					for _, instruction := range block.Instrs {
						write, ok := instruction.(*ssa.Store)
						if !ok {
							continue
						}
						path := paths.resolve(write.Addr)
						if !path.known || len(path.access.fields) != 1 || path.access.fields[0] != workspace {
							continue
						}
						factoryCall := write.Val.(*ssa.Call)
						factory := ps6136Call(allocation, factoryCall)
						input := allocation.reference(factoryCall.Call.Args[0])
						backend := ps6136FactoryResult(factory, input, allocator.(*types.Var), slot)
						if cell := ps6136FactoryErrorCell(factory, backend); cell.value == nil || cell.context != root {
							t.Fatal("returned factory allocator failure does not feed the same selected constructor error cell")
						}
						identity := ps6136AccessOwnerRoot(allocation, path, ownerType)
						method, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, "Release")
						if !ps6136ConstructorErrorBarrier(root, identity, ownerType, ps6136FactoryErrorCell(factory, backend), ps6090FunctionID(method.(*types.Func))) {
							t.Fatal("actual shared same-owner failure release/publication barrier unproved")
						}
						snapshot := ps6136InitialOwnerField(identity, ops.(*types.Var))
						if backend == nil || snapshot.value == nil || backendType.Field(indices[0]) != allocator {
							t.Fatal("actual public constructor ops initializer unavailable")
						}
						origin := snapshot.context.structField(snapshot.value, indices[0])
						if ps6136CallOrigin(factory, backend, origin) == nil || !ps6136BoundAllocator(factory, backend, input, map[string]bool{"github.com/jxsl13/goai/backend/metal.NewDeviceBufferF32": true}) {
							t.Fatal("actual shared public constructor -> initial ops -> captured factory -> exact native allocator binding unproved")
						}
						found++
					}
				}
			}
			if found != 1 {
				t.Fatalf("public shared output allocations %d", found)
			}
		})
	}
}
