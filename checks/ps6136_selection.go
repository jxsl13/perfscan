package checks

import (
	"go/types"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/ssa"
)

func (context *ps6136ContractContext) selection(pkg *ssa.Package, entry *types.Func, contract *config.OutputWorkspaceContract) *ps6136Selection {
	owner, model := context.named(contract.OwnerType), context.named(contract.ModelType)
	factory := context.callable(contract.FactoryFunction)
	if owner == nil || model == nil || entry == nil || factory == nil || owner.Obj().Pkg() != context.pass.Pkg || context.declarations[entry] == nil || context.declarations[factory] == nil {
		return nil
	}
	function := pkg.Prog.FuncValue(entry)
	constructor := pkg.Prog.FuncValue(factory)
	if function == nil || constructor == nil || function.Signature.Params().Len() == 0 || constructor.Signature.Params().Len() == 0 || !types.Identical(function.Signature.Params().At(0).Type(), types.NewPointer(model)) || !types.Identical(constructor.Signature.Params().At(0).Type(), types.NewPointer(model)) || constructor.Signature.Results().Len() != 2 || !types.Identical(constructor.Signature.Results().At(0).Type(), types.NewPointer(owner)) || !types.Identical(constructor.Signature.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return nil
	}
	root := ps6125NewSSAContext(function, nil, nil, 16384)
	var selected *ps6125SSAContext
	if function == constructor {
		selected = root
	} else {
		remaining := 16384
		valid := ps6136WalkConsumerCalls(root, func(current *ps6125SSAContext, call *ssa.Call) bool {
			if call.Call.StaticCallee() == constructor {
				if selected != nil {
					selected = nil
					remaining = 0
					return true
				}
				selected = ps6136Call(current, call)
				return true
			}
			return false
		}, &remaining)
		if !valid {
			return nil
		}
	}
	if selected == nil {
		return nil
	}
	selection := &ps6136Selection{context: selected, ownerType: owner, modelType: model, model: selected.reference(constructor.Params[0]), allocationFunction: contract.AllocationFunction, releaseMethod: contract.ReleaseMethod, shapeMethod: contract.HeadShapeMethod, allocatorIDs: make(map[string]bool)}
	selection.maximumRows, selection.width, selection.workspace = ps6136FieldVar(owner, contract.MaximumRowsField), ps6136FieldVar(owner, contract.WidthField), ps6136FieldVar(owner, contract.WorkspaceField)
	selection.ops, selection.retained, selection.projector = ps6136FieldVar(owner, contract.BackendOpsField), ps6136FieldVar(owner, contract.RetainedListField), ps6136FieldVar(owner, contract.ProjectorField)
	selection.modelConfig, selection.modelHead = ps6136FieldVar(model, contract.ModelConfigField), ps6136FieldVar(model, contract.ModelHeadField)
	if selection.maximumRows == nil || selection.width == nil || selection.workspace == nil || selection.ops == nil || selection.retained == nil || selection.projector == nil || selection.modelConfig == nil || selection.modelHead == nil || !types.Identical(selection.maximumRows.Type(), types.Typ[types.Int]) || !types.Identical(selection.width.Type(), types.Typ[types.Int]) {
		return nil
	}
	selection.configRows, selection.configWidth = ps6136FieldVar(selection.modelConfig.Type(), contract.ConfigMaximumRowsField), ps6136FieldVar(selection.modelConfig.Type(), contract.ConfigWidthField)
	selection.retainedReleaseMethod = contract.RetainedBufferReleaseMethod
	selection.slot = ps6136FieldVar(selection.workspace.Type(), contract.SlotBufferField)
	selection.allocator = ps6136FieldVar(selection.ops.Type(), contract.BackendAllocatorField)
	selection.primaryRecorder = ps6136FieldVar(selection.ops.Type(), contract.BackendRecorderField)
	if contract.SecondaryRecorderField != "" {
		selection.secondaryRecorder = ps6136FieldVar(selection.ops.Type(), contract.SecondaryRecorderField)
	}
	projector := context.named(contract.ProjectorType)
	if selection.configRows == nil || selection.configWidth == nil || selection.slot == nil || selection.allocator == nil || projector == nil || !types.Identical(selection.configRows.Type(), types.Typ[types.Int]) || !types.Identical(selection.configWidth.Type(), types.Typ[types.Int]) {
		return nil
	}
	projectorWidth := ps6136FieldVar(projector, contract.ProjectorWidthField)
	if projectorWidth == nil {
		return nil
	}
	selection.projectorWidths = map[*types.Named]*types.Var{projector: projectorWidth}
	structure, ok := selection.ops.Type().Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	selection.allocatorIndex = -1
	for index := 0; index < structure.NumFields(); index++ {
		if structure.Field(index) == selection.allocator {
			selection.allocatorIndex = index
		}
	}
	if selection.allocatorIndex < 0 {
		return nil
	}
	for _, identity := range contract.BackendAllocators {
		if context.callable(identity) == nil {
			return nil
		}
		selection.allocatorIDs[identity] = true
	}
	for _, block := range constructor.Blocks {
		if !selected.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok || len(returned.Results) == 0 {
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
	return selection
}
