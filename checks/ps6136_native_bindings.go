package checks

import (
	"go/ast"
	"go/types"
	"slices"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/ssa"
)

func (context *ps6136ContractContext) nativeBindings(selection *ps6136Selection, proof *ps6136ConstructorProof, contract *config.OutputWorkspaceContract) bool {
	if selection == nil || proof == nil || contract == nil {
		return false
	}
	base := context.named(contract.RecorderWrapperType)
	native := context.named(contract.NativeRecorderType)
	buffer := context.named(contract.BufferWrapperType)
	projector := context.named(contract.ProjectorType)
	if base == nil || native == nil || buffer == nil || projector == nil {
		return false
	}
	nativeField := ps6136FieldVar(base, contract.RecorderNativeField)
	bufferField := ps6136FieldVar(buffer, contract.BufferNativeField)
	recorderField := ps6136FieldVar(selection.ops.Type(), contract.BackendRecorderField)
	if nativeField == nil || bufferField == nil || recorderField == nil || !types.Identical(nativeField.Type(), types.NewPointer(native)) || !selection.recorderBinding(context.pass, recorderField, contract.NativeRecorderFactory, base, nativeField) {
		return false
	}
	if len(proof.factory.flow.function.Params) != 1 {
		return false
	}
	backend := ps6136FactoryResult(proof.factory, proof.factory.reference(proof.factory.flow.function.Params[0]), selection.allocator, selection.slot)
	if backend == nil || !ps6136BoundBufferWrapper(proof.factory, backend, buffer, bufferField) {
		return false
	}
	if contract.NativeBufferReleaseMethod != "" {
		release := context.callable(contract.RetainedBufferReleaseMethod)
		if release == nil || release.Type().(*types.Signature).Recv() == nil {
			return false
		}
		object, _, _ := types.LookupFieldOrMethod(buffer, false, context.pass.Pkg, release.Name())
		nativeRelease, ok := object.(*types.Func)
		if !ok || nativeRelease.Type().(*types.Signature).Recv() == nil || ps6090FunctionID(nativeRelease) != contract.NativeBufferReleaseMethod || nativeRelease.Type().(*types.Signature).Params().Len() != 0 || nativeRelease.Type().(*types.Signature).Results().Len() != 0 {
			return false
		}
	}
	bridge := context.callable(contract.BufferBridge)
	if bridge == nil || !ps6136BufferBridge(context.pass, context.declarations[bridge], buffer, bufferField) {
		return false
	}
	record := context.callable(contract.ProjectorRecordMethod)
	projection := context.callable(contract.RecorderProjectionMethod)
	if record == nil || projection == nil || record.Type().(*types.Signature).Recv() == nil || projection.Type().(*types.Signature).Recv() == nil || !types.Identical(record.Type().(*types.Signature).Recv().Type(), projector) {
		return false
	}
	weight := ps6136FieldVar(projector, contract.ProjectorWeightField)
	inner := ps6136FieldVar(projector, contract.ProjectorInnerField)
	width := ps6136FieldVar(projector, contract.ProjectorWidthField)
	if weight == nil || inner == nil || width == nil || !types.Identical(inner.Type(), types.Typ[types.Int]) || !types.Identical(width.Type(), types.Typ[types.Int]) || selection.projectorWidths[projector] != width || !ps6136ProjectionLowering(context.pass, context.declarations[record], contract.RecorderProjectionMethod, weight, inner, width) {
		return false
	}
	object, _, _ := types.LookupFieldOrMethod(base, false, context.pass.Pkg, projection.Name())
	adapter, ok := object.(*types.Func)
	if !ok || !ps6136ForwardedLeaf(context.pass, context.declarations[adapter], contract.NativeProjectionMethod, bridge, []int{0, 1, 2}) {
		return false
	}
	adapters := []*types.Func{adapter}
	var buffers []int
	for index := range contract.Leaves {
		leaf := &contract.Leaves[index]
		method := context.callable(leaf.Method)
		if method == nil {
			return false
		}
		signature := method.Type().(*types.Signature)
		if signature.Recv() == nil {
			return false
		}
		capability, ok := signature.Recv().Type().Underlying().(*types.Interface)
		if !ok {
			return false
		}
		if leaf.Kind == "projection" {
			if len(leaf.Implementations) != 1 || leaf.Implementations[0] != contract.ProjectorRecordMethod || !types.Implements(projector, capability) {
				return false
			}
			continue
		}
		if leaf.Kind == "row-width" {
			object, _, _ := types.LookupFieldOrMethod(base, false, context.pass.Pkg, method.Name())
			adapter, ok := object.(*types.Func)
			if !ok || len(leaf.Implementations) != 1 {
				return false
			}
			buffers = buffers[:0]
			for index := 0; index < signature.Params().Len(); index++ {
				if types.Identical(signature.Params().At(index).Type(), selection.slot.Type()) {
					buffers = append(buffers, index)
				}
			}
			if !ps6136ForwardedLeaf(context.pass, context.declarations[adapter], leaf.Implementations[0], bridge, buffers) {
				return false
			}
			adapters = append(adapters, adapter)
			continue
		}
		if !types.Implements(buffer, capability) {
			// The exact selected factory type cannot enter this optional
			// capability branch. Its source/logical inventory remains required.
			if !leaf.SelectedCapabilityAbsenceAllowed {
				return false
			}
			continue
		}
		object, _, _ := types.LookupFieldOrMethod(buffer, false, context.pass.Pkg, method.Name())
		implementation, ok := object.(*types.Func)
		if !ok || len(leaf.Implementations) != 1 || ps6090FunctionID(implementation) != leaf.Implementations[0] {
			return false
		}
		// Promotion through the exact one-field allocator wrapper must reach
		// the native buffer, not a new wrapper-owned override.
		pointer, ok := bufferField.Type().(*types.Pointer)
		if !ok || !bufferField.Embedded() || implementation.Type().(*types.Signature).Recv() == nil || !types.Identical(implementation.Type().(*types.Signature).Recv().Type(), pointer) {
			return false
		}
	}
	return context.recorderStateClosed(selection, contract, recorderField, base, nativeField, adapters)
}

func ps6136LiteralFieldInitializers(context *ps6136ContractContext, fields map[*types.Var]bool) map[ast.Node]bool {
	proved := make(map[ast.Node]bool)
	for _, file := range context.pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			if pair, ok := node.(*ast.KeyValueExpr); ok {
				if key, ok := pair.Key.(*ast.Ident); ok {
					if field, ok := context.pass.TypesInfo.Uses[key].(*types.Var); ok && fields[field] {
						proved[pair] = true
					}
				}
			}
			return true
		})
	}
	return proved
}

func (context *ps6136ContractContext) recorderStateClosed(selection *ps6136Selection, contract *config.OutputWorkspaceContract, primary *types.Var, base *types.Named, nativeField *types.Var, adapters []*types.Func) bool {
	fields := map[*types.Var]bool{primary: true}
	secondary := ps6136FieldVar(selection.ops.Type(), contract.SecondaryRecorderField)
	if contract.SecondaryRecorderField != "" {
		if secondary == nil {
			return false
		}
		fields[secondary] = true
		snapshot := ps6136InitialOwnerField(selection.owner, selection.ops)
		index := -1
		structure := selection.ops.Type().Underlying().(*types.Struct)
		for candidate := 0; candidate < structure.NumFields(); candidate++ {
			if structure.Field(candidate) == secondary {
				index = candidate
			}
		}
		if snapshot.value == nil || index < 0 || !ps6136StructFieldZero(snapshot.context, snapshot.value, index, 512) {
			return false
		}
	}
	proved := ps6136LiteralFieldInitializers(context, fields)
	for _, file := range context.pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			selectionInfo := context.pass.TypesInfo.Selections[selector]
			if selectionInfo == nil {
				return true
			}
			field, ok := selectionInfo.Obj().(*types.Var)
			if !ok || !fields[field] {
				return true
			}
			container, ok := selector.X.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			containerInfo := context.pass.TypesInfo.Selections[container]
			if containerInfo != nil && containerInfo.Kind() == types.FieldVal && containerInfo.Obj() != selection.ops && types.Identical(containerInfo.Type(), selection.ops.Type()) {
				// A distinct owner's by-value backendOps field is different
				// storage; it cannot mutate this selected owner's callback cell.
				proved[selector] = true
			}
			return true
		})
	}
	stableSelected := true
	remaining := 16384
	if !ps6136WalkConsumerCalls(selection.context, func(current *ps6125SSAContext, call *ssa.Call) bool {
		for _, current := range []*ps6125SSAContext{current, ps6136Call(current, call)} {
			if current == nil {
				continue
			}
			for _, block := range current.flow.function.Blocks {
				if !current.flow.blocks[block] {
					continue
				}
				for _, instruction := range block.Instrs {
					store, ok := instruction.(*ssa.Store)
					if !ok {
						continue
					}
					address, ok := store.Addr.(*ssa.FieldAddr)
					if !ok {
						continue
					}
					pointer, ok := address.X.Type().Underlying().(*types.Pointer)
					if !ok {
						continue
					}
					structure, ok := pointer.Elem().Underlying().(*types.Struct)
					if !ok || !fields[structure.Field(address.Field)] {
						continue
					}
					root := address.X
					for {
						parent, ok := root.(*ssa.FieldAddr)
						if !ok {
							break
						}
						root = parent.X
					}
					if types.Identical(root.Type(), types.NewPointer(selection.ownerType)) && ps6136OwnerRoot(current, root, selection.ownerType) == selection.owner {
						stableSelected = false
					}
				}
			}
		}
		return !stableSelected
	}, &remaining) || !stableSelected {
		return false
	}
	// Unselected constructors may specialize their own fresh instance, but
	// cannot grant an exemption to calls on a published selected owner.
	pkg := selection.context.flow.function.Pkg
	writes := ps6136FreshConstructorWrites(pkg, selection.ownerType, fields)
	if writes == nil {
		return false
	}
	for _, file := range context.pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			if selector, ok := node.(*ast.SelectorExpr); ok {
				if selection := context.pass.TypesInfo.Selections[selector]; selection != nil {
					if field, ok := selection.Obj().(*types.Var); ok && fields[field] {
						for position := range writes.positions {
							if selector.Pos() <= position && position < selector.End() {
								proved[selector] = true
							}
						}
					}
				}
			}
			return true
		})
	}
	profileTargets := make(map[ast.Node]bool)
	if contract.RecorderOverrideMethod != "" {
		method := context.callable(contract.RecorderOverrideMethod)
		wrapper := context.named(contract.RecorderOverrideWrapperType)
		if method == nil || wrapper == nil || secondary == nil || !ps6136SameAdapterMethods(wrapper, base, adapters) {
			return false
		}
		embedded := ps6136FieldVar(wrapper, contract.RecorderOverrideEmbedField)
		metadata := ps6136FieldVar(wrapper, contract.RecorderOverrideMetadataField)
		async := ps6136FieldVar(selection.ops.Type(), contract.AsyncRecorderField)
		frame := ps6136ProfileOverrideFrame(context.pass, context.declarations[method], selection.ops, primary, secondary, async, contract.OwnerType+".Step", contract.DropPendingMethod, func(literal *ast.FuncLit, capacity, profile types.Object) bool {
			return ps6136ProfileRecorderFactory(context.pass, literal, contract.NativeOverrideFactory, capacity, profile, wrapper, base, embedded, metadata, nativeField)
		})
		if frame == nil {
			return false
		}
		for node := range frame {
			proved[node] = true
			profileTargets[node] = true
		}
	}
	for _, identity := range append(slices.Clone(contract.CommonMethods), contract.BulkMethod) {
		method := context.callable(identity)
		if method == nil {
			return false
		}
		function := pkg.Prog.FuncValue(method)
		if function == nil || len(function.Params) == 0 || !types.Identical(function.Params[0].Type(), types.NewPointer(selection.ownerType)) {
			return false
		}
		root := ps6125NewSSAContext(function, nil, nil, 16384)
		allowed := make(map[ast.Node]bool)
		if identity == contract.RecorderOverrideMethod {
			allowed = profileTargets
		}
		if !ps6136InstanceFieldWritesClosed(root, root.reference(function.Params[0]), selection.ownerType, fields, allowed) {
			return false
		}
		if !ps6136InstanceFieldWritesClosed(root, root.reference(function.Params[0]), selection.ownerType, map[*types.Var]bool{selection.retained: true}, nil) {
			return false
		}
	}
	for field := range fields {
		if !ps6136FieldEffects(context.pass, field, proved) {
			return false
		}
	}
	return true
}

func ps6136InstanceFieldWritesClosed(root *ps6125SSAContext, owner ps6125SSAReference, ownerType *types.Named, fields map[*types.Var]bool, allowed map[ast.Node]bool) bool {
	pending := []*ps6125SSAContext{root}
	remaining := 16384
	for len(pending) != 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		remaining--
		if remaining < 0 {
			return false
		}
		for _, block := range current.flow.function.Blocks {
			if !current.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				if call, ok := instruction.(*ssa.Call); ok {
					if child := ps6136Call(current, call); child != nil {
						pending = append(pending, child)
					}
				}
				store, ok := instruction.(*ssa.Store)
				if !ok {
					continue
				}
				address, ok := store.Addr.(*ssa.FieldAddr)
				if !ok {
					continue
				}
				pointer, ok := address.X.Type().Underlying().(*types.Pointer)
				if !ok {
					continue
				}
				structure, ok := pointer.Elem().Underlying().(*types.Struct)
				if !ok || !fields[structure.Field(address.Field)] {
					continue
				}
				base := address.X
				for {
					parent, ok := base.(*ssa.FieldAddr)
					if !ok {
						break
					}
					base = parent.X
				}
				if !types.Identical(base.Type(), types.NewPointer(ownerType)) || ps6136OwnerRoot(current, base, ownerType) != owner {
					continue
				}
				proved := false
				for target := range allowed {
					if target.Pos() <= store.Pos() && store.Pos() < target.End() {
						proved = true
					}
				}
				if !proved {
					return false
				}
			}
		}
	}
	return true
}
