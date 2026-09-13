package checks

import (
	"go/types"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/ssa"
)

// Resolve vocabulary against this loaded package. No configured type, flag,
// field or function name supplies a dispatch, geometry or ownership fact.
type ps6140SourceSelection struct {
	allocation *ps6136Selection
	entry      *ps6125SSAContext
	block      *types.Named
	blocks     *types.Var
	projectors map[*types.Var]*types.Named
	flags      map[*types.Var]bool
}

func (context *ps6136ContractContext) unusedProjectionSelection(pkg *ssa.Package, contract *config.UnusedProjectionScratchContract, budget int) *ps6140SourceSelection {
	if context == nil || context.pass == nil || pkg == nil || context.pass.Pkg != pkg.Pkg || !contract.Valid() || budget <= 0 {
		return nil
	}
	owner, model, block := context.named(contract.OwnerType), context.named(contract.ModelType), context.named(contract.BlockType)
	entry, constructor := context.callable(contract.ConstructorEntry), context.callable(contract.ConstructorFunction)
	if owner == nil || model == nil || block == nil || owner.Obj().Pkg() != pkg.Pkg || block.Obj().Pkg() != pkg.Pkg ||
		types.Identical(owner, model) || types.Identical(owner, block) || types.Identical(model, block) ||
		entry == nil || !entry.Exported() || constructor == nil || context.declarations[entry] == nil || context.declarations[constructor] == nil {
		return nil
	}
	function, factory := pkg.Prog.FuncValue(entry), pkg.Prog.FuncValue(constructor)
	if !ps6140ConstructorSignature(function, owner, model) || !ps6140ConstructorSignature(factory, owner, model) {
		return nil
	}
	root := ps6125NewSSAContext(function, nil, nil, budget)
	selected := ps6140SelectedConstructor(root, factory, budget)
	if selected == nil {
		return nil
	}
	selection := &ps6136Selection{
		context: selected, ownerType: owner, modelType: model, model: selected.reference(factory.Params[0]),
		allocationFunction: contract.AllocationFunction, releaseMethod: contract.ReleaseMethod,
		allocatorIDs: map[string]bool{contract.BackendAllocator: true},
		maximumRows:  ps6136FieldVar(owner, contract.MaximumRowsField), width: ps6136FieldVar(owner, contract.WidthField),
		workspace: ps6136FieldVar(owner, contract.WorkspaceField), ops: ps6136FieldVar(owner, contract.BackendOpsField),
		retained: ps6136FieldVar(owner, contract.RetainedListField), modelConfig: ps6136FieldVar(model, contract.ModelConfigField),
	}
	if selection.maximumRows == nil || selection.width == nil || selection.workspace == nil || selection.ops == nil || selection.retained == nil || selection.modelConfig == nil {
		return nil
	}
	selection.configRows = ps6136FieldVar(selection.modelConfig.Type(), contract.ConfigRowsField)
	selection.configWidth = ps6136FieldVar(selection.modelConfig.Type(), contract.ConfigWidthField)
	selection.slot = ps6136FieldVar(selection.workspace.Type(), contract.SlotBufferField)
	selection.allocator = ps6136FieldVar(selection.ops.Type(), contract.BackendAllocatorField)
	for _, field := range []*types.Var{selection.maximumRows, selection.width, selection.configRows, selection.configWidth} {
		if field == nil || !types.Identical(field.Type(), types.Typ[types.Int]) {
			return nil
		}
	}
	if selection.slot == nil || selection.allocator == nil {
		return nil
	}
	allocatorSignature, ok := selection.allocator.Type().Underlying().(*types.Signature)
	if !ok || allocatorSignature.Recv() != nil || allocatorSignature.Variadic() || allocatorSignature.Params().Len() != 1 ||
		!types.Identical(allocatorSignature.Params().At(0).Type(), types.NewSlice(types.Typ[types.Float32])) || allocatorSignature.Results().Len() != 2 ||
		!types.Identical(allocatorSignature.Results().At(0).Type(), selection.slot.Type()) || !types.Identical(allocatorSignature.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return nil
	}
	ops, ok := selection.ops.Type().Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	selection.allocatorIndex = -1
	for index := 0; index < ops.NumFields(); index++ {
		if ops.Field(index) == selection.allocator {
			selection.allocatorIndex = index
		}
	}
	list, ok := selection.retained.Type().Underlying().(*types.Slice)
	if !ok || !types.Identical(list.Elem(), selection.slot.Type()) || selection.allocatorIndex < 0 {
		return nil
	}
	capability, ok := selection.slot.Type().Underlying().(*types.Interface)
	if !ok || !capability.IsMethodSet() {
		return nil
	}
	closeObject, _, _ := types.LookupFieldOrMethod(selection.slot.Type(), true, pkg.Pkg, "Release")
	close, ok := closeObject.(*types.Func)
	if !ok {
		return nil
	}
	selection.retainedReleaseMethod = ps6090FunctionID(close)
	allocate, release := context.callable(contract.AllocationFunction), context.callable(contract.ReleaseMethod)
	if allocate == nil || release == nil || context.declarations[allocate] == nil || context.declarations[release] == nil ||
		context.callable(contract.BackendAllocator) == nil || context.callable(contract.NativeReleaseMethod) == nil {
		return nil
	}
	releaseSignature := release.Type().(*types.Signature)
	if releaseSignature.Recv() == nil || !types.Identical(releaseSignature.Recv().Type(), types.NewPointer(owner)) || releaseSignature.Variadic() || releaseSignature.Params().Len() != 0 || releaseSignature.Results().Len() != 0 {
		return nil
	}
	for _, block := range factory.Blocks {
		if !selected.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			budget--
			if budget <= 0 {
				return nil
			}
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			if value, ok := returned.Results[0].(*ssa.Const); ok && value.IsNil() {
				continue
			}
			identity := ps6136OwnerRoot(selected, returned.Results[0], owner)
			if _, fresh := identity.value.(*ssa.Alloc); !fresh || selection.owner.value != nil && selection.owner != identity {
				return nil
			}
			selection.owner = identity
		}
	}
	if selection.owner.value == nil {
		return nil
	}
	result := &ps6140SourceSelection{allocation: selection, entry: root, block: block, blocks: ps6136FieldVar(owner, contract.BlocksField), projectors: make(map[*types.Var]*types.Named), flags: make(map[*types.Var]bool)}
	if result.blocks == nil {
		return nil
	}
	collection, ok := result.blocks.Type().Underlying().(*types.Slice)
	if !ok || !types.Identical(collection.Elem(), block) {
		return nil
	}
	for _, binding := range contract.Projections {
		field, concrete := ps6136FieldVar(block, binding.Field), context.named(binding.ConcreteType)
		if field == nil || concrete == nil || concrete.Obj().Pkg() != pkg.Pkg {
			return nil
		}
		capability, ok := field.Type().Underlying().(*types.Interface)
		if !ok || !capability.IsMethodSet() || !types.Implements(concrete, capability) {
			return nil
		}
		result.projectors[field] = concrete
	}
	for _, name := range contract.ClassFlags {
		field := ps6136FieldVar(owner, name)
		if field == nil || field.Exported() || !types.Identical(field.Type(), types.Typ[types.Bool]) {
			return nil
		}
		result.flags[field] = true
	}
	return result
}

func ps6140ConstructorSignature(function *ssa.Function, owner, model *types.Named) bool {
	if function == nil || function.Signature == nil || len(function.Blocks) == 0 {
		return false
	}
	signature := function.Signature
	return signature.Recv() == nil && signature.TypeParams().Len() == 0 && !signature.Variadic() && signature.Params().Len() > 0 &&
		types.Identical(signature.Params().At(0).Type(), types.NewPointer(model)) && signature.Results().Len() == 2 &&
		types.Identical(signature.Results().At(0).Type(), types.NewPointer(owner)) && types.Identical(signature.Results().At(1).Type(), types.Universe.Lookup("error").Type())
}

// Locate exactly one actual invocation, including forwarding helpers. The
// eventual publication proof must still show the public return is this owner.
func ps6140SelectedConstructor(root *ps6125SSAContext, factory *ssa.Function, budget int) *ps6125SSAContext {
	if root == nil || root.flow == nil || factory == nil || budget <= 0 {
		return nil
	}
	if root.flow.function == factory {
		return root
	}
	var selected *ps6125SSAContext
	count := 0
	valid := ps6136WalkConsumerCalls(root, func(current *ps6125SSAContext, call *ssa.Call) bool {
		child := ps6136Call(current, call)
		if child == nil || child.flow.function != factory {
			return false
		}
		count++
		selected = child
		return true
	}, &budget)
	if !valid || count != 1 {
		return nil
	}
	return selected
}
