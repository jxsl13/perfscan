package checks

import (
	"go/types"
	"os"
	"testing"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// This opt-in replay follows the actual constructor's host input through its
// captured allocation factory. It does not infer the backend function stored in
// unknown ops, native allocation size, successful construction, or retention.
func TestPS6125ExternalConstructorFactory(t *testing.T) {
	t.Parallel()
	directory := os.Getenv("PERFSCAN_PS6125_EXTERNAL_SOURCE")
	if directory == "" {
		t.Skip("external pinned source replay requires PERFSCAN_PS6125_EXTERNAL_SOURCE")
	}
	loaded, err := packages.Load(&packages.Config{Mode: packages.LoadSyntax, Dir: directory, Env: append(os.Environ(), "CGO_ENABLED=0")}, "./llamagpu")
	if err != nil || len(loaded) != 1 || packages.PrintErrors(loaded) != 0 {
		t.Fatalf("load pinned source: packages=%d error=%v", len(loaded), err)
	}
	_, converted := ssautil.Packages(loaded, ssa.SanityCheckFunctions)
	converted[0].Build()
	constructor := converted[0].Func("newGPTDecoder")
	if constructor == nil {
		t.Fatal("pinned source lacks newGPTDecoder")
	}
	field := func(typeName, fieldName string) *types.Var {
		object := loaded[0].Types.Scope().Lookup(typeName)
		if object == nil {
			t.Fatalf("pinned source lacks %s", typeName)
		}
		structure, ok := object.Type().Underlying().(*types.Struct)
		if !ok {
			t.Fatalf("pinned source changed %s", typeName)
		}
		for index := range structure.NumFields() {
			if candidate := structure.Field(index); candidate.Name() == fieldName {
				return candidate
			}
		}
		t.Fatalf("pinned source lacks %s.%s", typeName, fieldName)
		return nil
	}
	logits := field("GPTDecoder", "logits")
	newBuffer := field("backendOps", "newBuffer")
	buffer := field("bufSlot", "b")
	root := ps6125NewSSAContext(constructor, nil, nil, 8)
	paths := ps6125AccessPaths{flow: root.flow}
	found := 0
	for _, block := range constructor.Blocks {
		for _, instruction := range block.Instrs {
			store, ok := instruction.(*ssa.Store)
			if !ok || !root.flow.blocks[block] {
				continue
			}
			access := paths.resolve(store.Addr)
			if !access.known || len(access.access.fields) != 1 || access.access.fields[0] != logits {
				continue
			}
			found++
			call, ok := store.Val.(*ssa.Call)
			if !ok || len(call.Call.Args) != 1 {
				t.Fatal("logits store no longer comes from the allocation factory")
			}
			allocation, ok := call.Call.Args[0].(*ssa.MakeSlice)
			if !ok {
				t.Fatal("factory input is not the source host allocation")
			}
			factory := root.call(call)
			if factory == nil || len(factory.flow.function.Params) != 1 || len(factory.flow.function.FreeVars) == 0 {
				t.Fatalf("captured allocation factory context was not proved: callable %T %v", call.Call.Value, call.Call.Value)
			}
			wantInput := root.reference(allocation)
			if factory.reference(factory.flow.function.Params[0]) != wantInput {
				t.Fatal("factory parameter lost the exact constructor allocation")
			}
			factoryPaths := ps6125AccessPaths{flow: factory.flow}
			var backend *ssa.Call
			for _, candidate := range ps6125ContextCalls(factory) {
				path := factoryPaths.resolve(candidate.Call.Value)
				if path.known && len(path.access.fields) == 1 && path.access.fields[0] == newBuffer {
					if backend != nil || len(candidate.Call.Args) != 1 || factory.reference(candidate.Call.Args[0]) != wantInput {
						t.Fatal("backend allocation input provenance is ambiguous")
					}
					backend = candidate
				}
			}
			if backend == nil || factory.call(backend) != nil {
				t.Fatal("missing backend call or invented a target from unknown ops memory")
			}
			stored, empty := 0, 0
			for _, factoryBlock := range factory.flow.function.Blocks {
				for _, factoryInstruction := range factoryBlock.Instrs {
					returned, ok := factoryInstruction.(*ssa.Return)
					if !ok || !factory.flow.blocks[factoryBlock] {
						continue
					}
					_, fields, known := factory.returnedFields(returned, 0)
					if !known {
						t.Fatal("fresh source-local returned wrapper was not proved")
					}
					reference := fields[buffer]
					if reference.value == nil {
						empty++
						continue
					}
					extracted, ok := reference.value.(*ssa.Extract)
					if !ok || reference.context != factory || extracted.Tuple != backend || extracted.Index != 0 {
						t.Fatal("returned buffer field is not the exact backend result source")
					}
					stored++
				}
			}
			if stored != 1 || empty != 2 {
				t.Fatalf("wrapper source paths %d populated/%d unknown, want 1/2", stored, empty)
			}
		}
	}
	if found != 1 {
		t.Fatalf("got %d typed logits stores, want 1", found)
	}
	t.Log("constructor input -> captured factory -> backend argument and returned wrapper source verified; backend target/lifetime remain unknown")
}
