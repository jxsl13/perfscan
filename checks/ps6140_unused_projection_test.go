package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6140AuthenticUnusedResidualInvocation(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		t.Run(revision, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6140CompileLoadedMetal(t, revision)
			if err != nil {
				t.Fatal(err)
			}
			pkg := ps6136FixtureSSA(fixture)
			recorderType := pkg.Pkg.Scope().Lookup("mRec").Type()
			nativeMethod, _, _ := types.LookupFieldOrMethod(recorderType, true, pkg.Pkg, "QMatMulResident")
			if nativeMethod == nil || len(pkg.Prog.FuncValue(nativeMethod.(*types.Func)).Blocks) == 0 {
				t.Fatal("authentic Metal type-switch method was not built")
			}
			ownerType := pkg.Pkg.Scope().Lookup("Decoder").Type().(*types.Named)
			blockType := pkg.Pkg.Scope().Lookup("block").Type().(*types.Named)
			concrete := pkg.Pkg.Scope().Lookup("f32Linear").Type().(*types.Named)
			quantized := pkg.Pkg.Scope().Lookup("quantLinear").Type().(*types.Named)
			blocks := ps6136FieldVar(ownerType, "blocks")
			slotType := pkg.Pkg.Scope().Lookup("bufSlot").Type().(*types.Named)
			slot := ps6136FieldVar(slotType, "b")
			entry := ps6125NewSSAContext(pkg.Func("New"), nil, nil, 16384)
			var constructor *ps6125SSAContext
			for _, call := range ps6125ContextCalls(entry) {
				if call.Call.StaticCallee() == pkg.Func("newDecoder") {
					constructor = ps6136Call(entry, call)
				}
			}
			if constructor == nil {
				t.Fatal("selected authentic constructor missing")
			}
			var appended ps6125SSAReference
			for _, call := range ps6125ContextCalls(constructor) {
				if candidate := ps6140AppendedBlock(constructor, call, blockType); candidate.value != nil {
					if appended.value != nil {
						t.Fatal("ambiguous append")
					}
					appended = candidate
				}
			}
			if appended.value == nil {
				t.Fatal("authentic block append missing")
			}
			object, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, "encodeStep")
			function := pkg.Prog.FuncValue(object.(*types.Func))
			root := ps6125NewSSAContext(function, nil, nil, 16384)
			owner := root.reference(function.Params[0])
			checked := make(map[string]int)
			remaining := 16384
			if !ps6136WalkConsumerCalls(root, func(current *ps6125SSAContext, call *ssa.Call) bool {
				if !call.Call.IsInvoke() || call.Call.Method.Name() != "recordAdd" {
					return false
				}
				var projection, workspace *types.Var
				switch current.flow.function.Name() {
				case "recordOProj":
					projection, workspace = ps6136FieldVar(blockType, "wo"), ps6136FieldVar(ownerType, "ao")
				case "recordDownProj":
					projection, workspace = ps6136FieldVar(blockType, "wD"), ps6136FieldVar(ownerType, "mo")
				default:
					return false
				}
				proof := ps6140UnusedProjectionFormal(current, call, appended, concrete, owner, ownerType, blockType, blocks, projection, workspace, slot, 2, 256)
				if proof == nil {
					t.Fatalf("authentic %s scratch formal not joined", current.flow.function.Name())
				}
				if proof.caller != call || proof.method.Object().(*types.Func).Name() != "recordAdd" {
					t.Fatal("wrong exact source invocation")
				}
				// The record operation remains a source effect obligation, not
				// silently dropped merely because one parameter is unused.
				effects := 0
				for _, block := range proof.method.Blocks {
					for _, instruction := range block.Instrs {
						if effect, ok := instruction.(*ssa.Call); ok {
							if !effect.Call.IsInvoke() || effect.Call.Method.Name() != "MatMulAcc" {
								t.Fatal("unexpected actual source effect")
							}
							effects++
						}
					}
				}
				if effects != 1 {
					t.Fatal("actual recorder effect disappeared")
				}
				if ps6140UnusedProjectionFormal(current, call, appended, quantized, owner, ownerType, blockType, blocks, projection, workspace, slot, 2, 256) != nil || ps6140UnusedProjectionFormal(current, call, appended, concrete, owner, ownerType, blockType, blocks, projection, workspace, slot, 1, 256) != nil || ps6140UnusedProjectionFormal(current, call, appended, concrete, ps6125SSAReference{}, ownerType, blockType, blocks, projection, workspace, slot, 2, 256) != nil {
					t.Fatal("quantized/used/unknown-owner boundary accepted")
				}
				if ps6140UnusedProjectionFormal(current, call, appended, concrete, owner, ownerType, blockType, blocks, projection, ps6136FieldVar(ownerType, "dx"), slot, 2, 256) != nil || ps6140UnusedProjectionFormal(current, call, appended, concrete, owner, ownerType, blockType, blocks, ps6136FieldVar(blockType, "wqkv"), workspace, slot, 2, 256) != nil || ps6140UnusedProjectionFormal(root, call, appended, concrete, owner, ownerType, blockType, blocks, projection, workspace, slot, 2, 256) != nil {
					t.Fatal("wrong workspace/projection/call-context boundary accepted")
				}
				checked[current.flow.function.Name()]++
				return true
			}, &remaining) {
				t.Fatal("actual source traversal exhausted")
			}
			// Unknown runtime architecture flags keep every source branch in
			// this component census, not just the selected dense hot path.
			if checked["recordOProj"] != 2 || checked["recordDownProj"] != 60 {
				t.Fatalf("genuine residual coverage: %v", checked)
			}
		})
	}
}
