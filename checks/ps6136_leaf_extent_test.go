package checks

import (
	"go/types"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/ssa"
)

func TestPS6136AuthenticProjectionAndDownloadExtents(t *testing.T) {
	t.Parallel()
	fixture, err := ps6136CompileOwners(t, "before", true, true)
	if err != nil {
		t.Fatal(err)
	}
	pkg := ps6136FixtureSSA(fixture)
	for _, ownerName := range []string{"GPTDecoder", "Decoder"} {
		ownerType := pkg.Pkg.Scope().Lookup(ownerName).Type().(*types.Named)
		var step *ssa.Function
		method, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, "Step")
		step = pkg.Prog.FuncValue(method.(*types.Func))
		context := ps6125NewSSAContext(step, nil, nil, 256)
		owner := context.reference(step.Params[0])
		workspace, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, "logits")
		width, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, "v")
		projectorName := "head"
		if ownerName == "Decoder" {
			projectorName = "out"
		}
		projector, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, projectorName)
		slot := pkg.Pkg.Scope().Lookup("bufSlot").Type().Underlying().(*types.Struct).Field(0)
		var pool ps6125ExtentPool
		widthExtent := ps6125SymbolicExtent(pool.identity(step.Params[0].Object(), []*types.Var{width.(*types.Var)}, false))
		facts := &ps6136Extents{owner: owner, ownerType: ownerType, immutable: map[*types.Var]ps6125Extent{width.(*types.Var): widthExtent}, remaining: 2048}
		count, projections := 0, 0
		provedLeaves := make(map[*ssa.Call]bool)
		reads := make(map[ssa.Value]bool)
		remaining := 4096
		if !ps6136WalkConsumerCalls(context, func(current *ps6125SSAContext, call *ssa.Call) bool {
			for _, block := range current.flow.function.Blocks {
				if !current.flow.blocks[block] {
					continue
				}
				for _, instruction := range block.Instrs {
					load, ok := instruction.(*ssa.UnOp)
					if !ok {
						continue
					}
					address, ok := load.X.(*ssa.FieldAddr)
					if !ok {
						continue
					}
					pointer, ok := address.X.Type().Underlying().(*types.Pointer)
					if !ok || !types.Identical(pointer.Elem(), ownerType) {
						continue
					}
					if ownerType.Underlying().(*types.Struct).Field(address.Field) == workspace {
						reads[load] = true
					}
				}
			}
			if !call.Call.IsInvoke() {
				return false
			}
			leaf := config.OutputWorkspaceLeaf{Method: ps6090FunctionID(call.Call.Method), Kind: "download", BufferArguments: []int{-1}, RowsArgument: -1, WidthArgument: -1, HostArgument: 0, SemanticsReviewed: true}
			switch call.Call.Method.Name() {
			case "DownloadF32":
				count++
			case "record":
				leaf.Kind, leaf.BufferArguments, leaf.RowsArgument, leaf.HostArgument = "projection", []int{2}, 3, -1
				// Other layers also invoke record; only the exact source owner
				// head/out receiver may establish output-workspace extent.
				if _, ok := ps6136LeafExtent(current, call, &leaf, facts, workspace.(*types.Var), slot, projector.(*types.Var), width.(*types.Var)); !ok {
					return false
				}
				projections++
			case "AddBias":
				leaf.Kind, leaf.BufferArguments, leaf.RowsArgument, leaf.WidthArgument, leaf.HostArgument = "row-width", []int{0, 2}, 3, 4, -1
				if _, ok := ps6136LeafExtent(current, call, &leaf, facts, workspace.(*types.Var), slot, projector.(*types.Var), width.(*types.Var)); !ok {
					return false
				}
			default:
				return false
			}
			got, ok := ps6136LeafExtent(current, call, &leaf, facts, workspace.(*types.Var), slot, projector.(*types.Var), width.(*types.Var))
			if !ok || !ps6125SameExtent(got, widthExtent) {
				t.Fatal("actual one-row source leaf extent or workspace identity missing")
			}
			provedLeaves[call] = true
			return true
		}, &remaining) {
			t.Fatal("source consumer traversal exhausted")
		}
		if count != 1 || projections != 1 {
			t.Fatalf("%s source downloads %d projections %d", ownerName, count, projections)
		}
		expectedReads := 2
		if ownerName == "Decoder" {
			expectedReads = 4
		} // optional output-bias input/output
		if len(reads) != expectedReads {
			t.Fatalf("%s workspace slot reads %d", ownerName, len(reads))
		}
		for read := range reads {
			if !ps6136WorkspaceReadClosed(read, slot, provedLeaves, 128) {
				t.Fatalf("%s unclosed authentic read %s", ownerName, read)
			}
			if ps6136WorkspaceReadClosed(read, slot, nil, 128) {
				t.Fatal("unproved consumer accepted")
			}
		}
	}
}
