package checks

import (
	"go/types"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/ssa"
)

func TestPS6136AuthenticLastAndBulkConsumerSpecialization(t *testing.T) {
	t.Parallel()
	fixture, err := ps6136CompileOwners(t, "before", true, true)
	if err != nil {
		t.Fatal(err)
	}
	pkg := ps6136FixtureSSA(fixture)
	for _, ownerName := range []string{"GPTDecoder", "Decoder"} {
		for _, methodName := range []string{"StepNLast", "StepN"} {
			t.Run(ownerName+"/"+methodName, func(t *testing.T) {
				t.Parallel()
				ownerType := pkg.Pkg.Scope().Lookup(ownerName).Type().(*types.Named)
				method, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, methodName)
				function := pkg.Prog.FuncValue(method.(*types.Func))
				field := func(name string) *types.Var {
					object, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, name)
					return object.(*types.Var)
				}
				projectorName := "head"
				if ownerName == "Decoder" {
					projectorName = "out"
				}
				workspace, width, projector := field("logits"), field("v"), field(projectorName)
				slot := pkg.Pkg.Scope().Lookup("bufSlot").Type().Underlying().(*types.Struct).Field(0)
				var pool ps6125ExtentPool
				widthExtent := ps6125SymbolicExtent(pool.identity(function.Params[0].Object(), []*types.Var{width}, false))
				tokenExtent := ps6125SymbolicExtent(pool.identity(function.Params[1].Object(), nil, true))
				bulkExtent := ps6125MultiplyExtents(widthExtent, tokenExtent)
				context := ps6125NewSSAContext(function, nil, map[*ssa.Parameter]ps6125Extent{function.Params[1]: tokenExtent}, 16384)
				facts := &ps6136Extents{owner: context.reference(function.Params[0]), ownerType: ownerType, immutable: map[*types.Var]ps6125Extent{width: widthExtent}, remaining: 8192}
				leaves, bulk := 0, 0
				remaining := 8192
				if !ps6136WalkConsumerCalls(context, func(current *ps6125SSAContext, call *ssa.Call) bool {
					if !call.Call.IsInvoke() {
						return false
					}
					leaf := config.OutputWorkspaceLeaf{Method: ps6090FunctionID(call.Call.Method), RowsArgument: -1, WidthArgument: -1, HostArgument: -1, SemanticsReviewed: true}
					switch call.Call.Method.Name() {
					case "record":
						leaf.Kind, leaf.BufferArguments, leaf.RowsArgument = "projection", []int{2}, 3
					case "DownloadF32":
						leaf.Kind, leaf.BufferArguments, leaf.HostArgument = "download", []int{-1}, 0
					default:
						return false
					}
					got, ok := ps6136LeafExtent(current, call, &leaf, facts, workspace, slot, projector, width)
					if !ok {
						return false // unrelated layer/projector
					}
					leaves++
					if ps6125SameExtent(got, bulkExtent) {
						bulk++
					} else if !ps6125SameExtent(got, widthExtent) {
						t.Fatal("unproved owner leaf extent")
					}
					return true
				}, &remaining) {
					t.Fatal("consumer traversal exhausted")
				}
				if leaves < 2 || methodName == "StepNLast" && bulk != 0 || methodName == "StepN" && bulk < 2 {
					t.Fatalf("source leaves=%d bulk=%d", leaves, bulk)
				}
			})
		}
	}
}
