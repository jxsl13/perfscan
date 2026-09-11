package checks

import (
	"go/types"
	"os"
	"testing"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// This opt-in source replay complements (and does not replace) the hermetic
// fixtures. The external project is not vendored. Supply an independently
// verified pinned checkout via PERFSCAN_PS6125_EXTERNAL_SOURCE.
// It checks projection row specialization only, not complete issue coverage.
func TestPS6125ExternalProjectionRows(t *testing.T) {
	t.Parallel()
	directory := os.Getenv("PERFSCAN_PS6125_EXTERNAL_SOURCE")
	if directory == "" {
		t.Skip("external pinned source replay requires PERFSCAN_PS6125_EXTERNAL_SOURCE")
	}
	loaded, err := packages.Load(&packages.Config{Mode: packages.LoadSyntax, Dir: directory, Env: append(os.Environ(), "CGO_ENABLED=0")}, "./llamagpu")
	if err != nil || len(loaded) != 1 {
		t.Fatalf("load pinned source: packages=%d error=%v", len(loaded), err)
	}
	if packages.PrintErrors(loaded) != 0 {
		t.Fatal("pinned source has load/type errors")
	}
	program, converted := ssautil.Packages(loaded, ssa.SanityCheckFunctions)
	converted[0].Build()
	ownerObject, ok := loaded[0].Types.Scope().Lookup("GPTDecoder").(*types.TypeName)
	if !ok {
		t.Fatal("pinned source lacks GPTDecoder")
	}
	owner := ownerObject.Type().(*types.Named)
	function := program.LookupMethod(types.NewPointer(owner), loaded[0].Types, "gptStepN")
	if function == nil || len(function.Params) != 4 {
		t.Fatal("pinned source changed gptStepN signature")
	}
	structure := owner.Underlying().(*types.Struct)
	head := -1
	for index := range structure.NumFields() {
		if structure.Field(index).Name() == "head" {
			head = index
		}
	}
	if head < 0 {
		t.Fatal("pinned source lacks output projection field")
	}
	var pool ps6125ExtentPool
	for _, lastOnly := range []bool{true, false} {
		entryName := "StepN"
		if lastOnly {
			entryName = "StepNLast"
		}
		entry := program.LookupMethod(types.NewPointer(owner), loaded[0].Types, entryName)
		if entry == nil || len(entry.Params) != 3 {
			t.Fatalf("pinned source changed %s signature", entryName)
		}
		rows := ps6125SymbolicExtent(pool.identity(entry.Params[1].Object(), nil, true))
		caller := ps6125AnalyzeSSAExtents(entry, nil, map[*ssa.Parameter]ps6125Extent{entry.Params[1]: rows})
		var flow *ps6125SSAExtents
		for _, block := range entry.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok {
					continue
				}
				callee, arguments, lengths := caller.callInputs(call)
				if callee != function {
					continue
				}
				if flow != nil {
					t.Fatal("multiple entry-to-helper calls require separate contexts")
				}
				flow = ps6125AnalyzeSSAExtents(callee, arguments, lengths)
			}
		}
		if flow == nil {
			t.Fatalf("%s has no source-proven helper call", entryName)
		}
		found := 0
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok || !flow.blocks[block] || !call.Call.IsInvoke() || call.Call.Method.Name() != "record" {
					continue
				}
				load, ok := call.Call.Value.(*ssa.UnOp)
				if !ok {
					continue
				}
				address, ok := load.X.(*ssa.FieldAddr)
				if !ok || address.Field != head || address.X != function.Params[0] {
					continue
				}
				if len(call.Call.Args) != 4 {
					t.Fatal("pinned source changed projection signature")
				}
				fact := flow.scalar(call.Call.Args[3])
				want := rows
				if lastOnly {
					want = ps6125ConstantExtent(1)
				}
				if fact.state != ps6125Integer || !ps6125SameExtent(fact.extent, want) {
					t.Fatalf("lastOnly=%v projection rows not proved: %+v", lastOnly, fact)
				}
				found++
			}
		}
		if found != 1 {
			t.Fatalf("lastOnly=%v found %d exact output projection calls", lastOnly, found)
		}
		t.Logf("%s -> gptStepN -> exact typed output projection row extent verified", entryName)
	}
}
