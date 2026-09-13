package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// This is one argument-observation proof, not approval of a dead allocation.
// The returned source method's recorder/call effects still require inspection;
// complete collection invariance, flags and other argument aliasing are separate.
// The appended descriptor and runtime member are separate inputs: the caller
// must still join them to the same selected constructor lifetime/collection.
type ps6140UnusedProjectionProof struct {
	method *ssa.Function
	caller *ssa.Call
}

func ps6140UnusedProjectionFormal(context *ps6125SSAContext, call *ssa.Call, appended ps6125SSAReference, concrete *types.Named, owner ps6125SSAReference, ownerType, blockType *types.Named, blocks, projection, workspace, slot *types.Var, formal, budget int) *ps6140UnusedProjectionProof {
	if context == nil || context.flow == nil || call == nil || concrete == nil || call.Parent() != context.flow.function || !context.flow.blocks[call.Block()] || !call.Call.IsInvoke() || call.Call.Method == nil || formal < 0 || formal >= len(call.Call.Args) || budget <= 0 {
		return nil
	}
	if !ps6140BlockProjectionValue(appended, projection, concrete, budget) || !ps6140RuntimeProjectionMember(context, call.Call.Value, owner, ownerType, blockType, blocks, projection, budget) || !ps6136BufferOrigin(context, call.Call.Args[formal], owner, ownerType, workspace, slot, budget) {
		return nil
	}
	invokedInterface, ok := call.Call.Value.Type().Underlying().(*types.Interface)
	if !ok || !types.Implements(concrete, invokedInterface) {
		return nil
	}
	object, _, _ := types.LookupFieldOrMethod(concrete, false, call.Call.Method.Pkg(), call.Call.Method.Name())
	method, ok := object.(*types.Func)
	if !ok || method.Id() != call.Call.Method.Id() {
		return nil
	}
	signature, ok := method.Type().(*types.Signature)
	invocation := call.Call.Method.Type().(*types.Signature)
	if !ok || signature.Recv() == nil || !types.Identical(signature.Recv().Type(), concrete) || signature.Variadic() || signature.TypeParams().Len() != 0 || !types.Identical(signature.Params(), invocation.Params()) || !types.Identical(signature.Results(), invocation.Results()) {
		return nil
	}
	function := context.flow.function.Prog.FuncValue(method)
	if function == nil || len(function.Blocks) == 0 || len(function.Params) != len(call.Call.Args)+1 || ps6136ParameterObject(function.Params[formal+1]) == nil {
		return nil
	}
	users := function.Params[formal+1].Referrers()
	if users == nil {
		return nil
	}
	for _, user := range *users {
		if _, debug := user.(*ssa.DebugRef); !debug {
			return nil
		}
	}
	return &ps6140UnusedProjectionProof{method: function, caller: call}
}

func ps6140RuntimeProjectionMember(context *ps6125SSAContext, value ssa.Value, owner ps6125SSAReference, ownerType, blockType *types.Named, blocks, projection *types.Var, budget int) bool {
	if context == nil || value == nil || blockType == nil || projection == nil || budget <= 0 {
		return false
	}
	reference := context.reference(value)
	if reference.value == nil {
		return false
	}
	structure := blockType.Underlying().(*types.Struct)
	if field, ok := reference.value.(*ssa.Field); ok {
		return types.Identical(field.X.Type(), blockType) && field.Field >= 0 && field.Field < structure.NumFields() && structure.Field(field.Field) == projection && ps6140BlockMember(reference.context, field.X, owner, ownerType, blockType, blocks, budget-1)
	}
	load, ok := reference.value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	field, ok := load.X.(*ssa.FieldAddr)
	if !ok || !types.Identical(field.X.Type(), types.NewPointer(blockType)) || field.Field < 0 || field.Field >= structure.NumFields() || structure.Field(field.Field) != projection {
		return false
	}
	allocation, ok := field.X.(*ssa.Alloc)
	if !ok {
		return false
	}
	stores := reference.context.structCell(allocation)
	return stores != nil && stores.whole != nil && reference.context.flow.instructionDominates(stores.whole, load) && ps6140BlockMember(reference.context, stores.whole.Val, owner, ownerType, blockType, blocks, budget-1)
}
