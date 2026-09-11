package checks

import (
	"go/types"
	"os"
	"runtime"
	"testing"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// This source-only replay enables the actual Darwin/CGO adapter. No native
// kernel executes. It identifies the allocator declaration, not its capacity,
// native lifetime, or the success of construction.
func TestPS6125ExternalMetalAllocator(t *testing.T) {
	t.Parallel()
	directory := os.Getenv("PERFSCAN_PS6125_EXTERNAL_SOURCE")
	if directory == "" || os.Getenv("PERFSCAN_PS6125_EXTERNAL_CGO") != "1" || runtime.GOOS != "darwin" {
		t.Skip("Metal source replay requires a verified external checkout, Darwin and PERFSCAN_PS6125_EXTERNAL_CGO=1")
	}
	loaded, err := packages.Load(&packages.Config{Mode: packages.LoadSyntax, Dir: directory, Env: append(os.Environ(), "CGO_ENABLED=1")}, "./llamagpu", "./backend/metal")
	if err != nil || len(loaded) != 2 || packages.PrintErrors(loaded) != 0 {
		t.Fatalf("load actual Metal adapter: packages=%d error=%v", len(loaded), err)
	}
	_, converted := ssautil.Packages(loaded, ssa.SanityCheckFunctions)
	var adapter *packages.Package
	var source *ssa.Package
	for index, pkg := range loaded {
		converted[index].Build()
		if pkg.PkgPath == "github.com/jxsl13/goai/llamagpu" {
			adapter, source = pkg, converted[index]
		}
	}
	if source == nil || adapter == nil {
		t.Fatal("pinned adapter package missing")
	}
	entry := source.Func("NewGPT")
	constructor := source.Func("newGPTDecoder")
	if entry == nil || constructor == nil {
		t.Fatal("pinned Metal constructor entries missing")
	}
	root := ps6125NewSSAContext(entry, nil, nil, 12)
	var child *ps6125SSAContext
	for _, call := range ps6125ContextCalls(root) {
		if call.Call.StaticCallee() == constructor {
			if child != nil {
				t.Fatal("ambiguous constructor entry")
			}
			child = root.call(call)
		}
	}
	if child == nil {
		t.Fatal("source constructor context missing")
	}
	owner := adapter.Types.Scope().Lookup("GPTDecoder")
	if owner == nil {
		t.Fatal("pinned decoder type missing")
	}
	logits, _, _ := types.LookupFieldOrMethod(owner.Type(), true, adapter.Types, "logits")
	metal := adapter.Imports["github.com/jxsl13/goai/backend/metal"]
	if logits == nil || metal == nil || metal.Types == nil {
		t.Fatal("pinned logits field or Metal package missing")
	}
	native := metal.Types.Scope().Lookup("NewDeviceBufferF32")
	if native == nil {
		t.Fatal("pinned Metal allocator declaration missing")
	}
	paths := ps6125AccessPaths{flow: child.flow}
	found := 0
	for _, block := range constructor.Blocks {
		for _, instruction := range block.Instrs {
			store, ok := instruction.(*ssa.Store)
			if !ok || !child.flow.blocks[block] {
				continue
			}
			path := paths.resolve(store.Addr)
			if !path.known || len(path.access.fields) != 1 || path.access.fields[0] != logits {
				continue
			}
			call, ok := store.Val.(*ssa.Call)
			if !ok || len(call.Call.Args) != 1 {
				t.Fatal("changed logits factory invocation")
			}
			input := child.reference(call.Call.Args[0])
			if _, ok := input.value.(*ssa.MakeSlice); !ok {
				t.Fatal("changed logits host allocation")
			}
			factory := child.call(call)
			if factory == nil {
				t.Fatal("captured factory context missing")
			}
			for _, invoke := range ps6125ContextCalls(factory) {
				allocator := factory.call(invoke)
				if allocator == nil {
					continue
				}
				for _, boundary := range ps6125ContextCalls(allocator) {
					callee := boundary.Call.StaticCallee()
					if callee == nil || callee.Object() != native {
						continue
					}
					if len(boundary.Call.Args) != 1 || allocator.reference(boundary.Call.Args[0]) != input {
						t.Fatal("native allocator lost exact constructor input provenance")
					}
					implementation := allocator.call(boundary)
					if implementation == nil || implementation.flow.function != callee || implementation.reference(callee.Params[0]) != input {
						t.Fatal("cross-package allocator body lost the exact constructor input")
					}
					found++
				}
			}
		}
	}
	if found != 1 {
		t.Fatalf("found %d proved native allocator boundaries, want 1", found)
	}
	t.Log("actual Metal entry -> operations value -> captured factory -> cross-package allocator body input verified; native extent/lifetime remain unproved")
}
