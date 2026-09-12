package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"maps"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// The assembled selection proof keeps source identities separate from the
// contract's reviewed model/backend/workload facts. A selection alone is not
// reportable: complete observations and constructor/lifetime gates follow it.
type ps6136Selection struct {
	context                                                       *ps6125SSAContext
	owner                                                         ps6125SSAReference
	model                                                         ps6125SSAReference
	ownerType, modelType                                          *types.Named
	maximumRows, width, workspace, projector, ops, retained, slot *types.Var
	modelConfig, configRows, configWidth, modelHead               *types.Var
	allocator                                                     *types.Var
	primaryRecorder, secondaryRecorder                            *types.Var
	allocatorIndex                                                int
	allocatorIDs                                                  map[string]bool
	allocationFunction, releaseMethod, shapeMethod                string
	retainedReleaseMethod                                         string
	projectorWidths                                               map[*types.Named]*types.Var
}

func (selection *ps6136Selection) closedSource(pass *analysis.Pass, pkg *ssa.Package, consumers []*ps6136ConsumerProof) bool {
	if selection == nil || pass == nil || pkg == nil {
		return false
	}
	fields := map[*types.Var]bool{selection.workspace: true, selection.width: true, selection.maximumRows: true, selection.ops: true, selection.projector: true, selection.retained: true}
	writes := ps6136FreshConstructorWrites(pkg, selection.ownerType, fields)
	if writes == nil {
		return false
	}
	for _, field := range writes.readFields {
		if field == selection.workspace {
			return false
		}
	}
	initializingWrites := *writes
	initializingWrites.writers = maps.Clone(writes.writers)
	for writer := range initializingWrites.writers {
		if object, ok := writer.Object().(*types.Func); ok && ps6090FunctionID(object) == selection.releaseMethod {
			delete(initializingWrites.writers, writer)
		}
	}
	initializers := ps6136ConstructorWriteNodes(pass, pkg, selection.ownerType, fields, &initializingWrites)
	if initializers == nil {
		return false
	}
	list, ok := selection.retained.Type().Underlying().(*types.Slice)
	if !ok {
		return false
	}
	bufferRelease, _, _ := types.LookupFieldOrMethod(list.Elem(), true, pass.Pkg, "Release")
	release, releaseOK := bufferRelease.(*types.Func)
	if !releaseOK {
		return false
	}
	if selection.retainedReleaseMethod == "" || ps6090FunctionID(release) != selection.retainedReleaseMethod {
		return false
	}
	var listRelease map[ast.Node]bool
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 || len(function.Recv.List[0].Names) != 1 {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if !ok || ps6090FunctionID(object) != selection.releaseMethod {
				continue
			}
			listRelease = ps6136ReleaseList(pass, function, pass.TypesInfo.Defs[function.Recv.List[0].Names[0]], selection.retained, release)
		}
	}
	if listRelease == nil {
		return false
	}
	for node := range listRelease {
		initializers[node] = true
	}
	for field := range fields {
		if !ps6136FieldEffects(pass, field, initializers) {
			return false
		}
	}
	// Value-literal keys initialize new backendOps values, not a published
	// owner's callback cell. All subsequent direct callback writes/addresses
	// remain forbidden, independently of sibling recorder policy overrides.
	allocatorInitializers := make(map[ast.Node]bool)
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			if pair, ok := node.(*ast.KeyValueExpr); ok {
				if key, ok := pair.Key.(*ast.Ident); ok && pass.TypesInfo.Uses[key] == selection.allocator {
					allocatorInitializers[pair] = true
				}
			}
			return true
		})
	}
	if !ps6136FieldEffects(pass, selection.allocator, allocatorInitializers) {
		return false
	}
	if !ps6136FieldEffects(pass, selection.modelConfig, nil) || !ps6136FieldEffects(pass, selection.modelHead, nil) {
		return false
	}
	proved := make(map[ast.Node]bool)
	for node := range initializers {
		if pair, ok := node.(*ast.KeyValueExpr); ok {
			proved[pair.Key] = true
		} else {
			proved[node] = true
		}
	}
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || !initializers[assignment.Lhs[0]] {
				return true
			}
			target, ok := assignment.Lhs[0].(*ast.SelectorExpr)
			if !ok {
				return true
			}
			selectionInfo := pass.TypesInfo.Selections[target]
			owner, ok := target.X.(*ast.Ident)
			if !ok || selectionInfo == nil || selectionInfo.Obj() != selection.retained {
				return true
			}
			call, ok := assignment.Rhs[0].(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			builtin, ok := call.Fun.(*ast.Ident)
			if !ok || pass.TypesInfo.Uses[builtin] != types.Universe.Lookup("append") || !ps6136DirectField(pass, call.Args[0], pass.TypesInfo.Uses[owner], selection.retained) {
				return true
			}
			proved[call.Args[0]] = true
			return true
		})
	}
	// readFields and positions use sparse offsets across independent owner files.
	positions := make(map[token.Pos]bool)
	calls := make(map[*ssa.Call]bool, len(writes.calls))
	for position := range writes.reads {
		if writes.readFields[position] != selection.retained { //perfscan:ignore PS3003
			positions[position] = true //perfscan:ignore PS3003
		}
	}
	for call := range writes.calls {
		calls[call] = true
	}
	for _, consumer := range consumers {
		if consumer == nil {
			return false
		}
		for position := range consumer.reads {
			positions[position] = true
		}
		for call := range consumer.calls {
			calls[call] = true
		}
	}
	constructors := ps6136ConstructorInventory(pkg, selection.ownerType)
	if constructors == nil {
		return false
	}
	for _, file := range pass.Files {
		listClosed := true
		ast.Inspect(file, func(node ast.Node) bool {
			switch occurrence := node.(type) {
			case *ast.SelectorExpr:
				selectionInfo := pass.TypesInfo.Selections[occurrence]
				if selectionInfo != nil && selectionInfo.Obj() == selection.retained {
					if !proved[occurrence] {
						for position := range positions {
							if occurrence.Pos() <= position && position < occurrence.End() {
								proved[occurrence] = true
							}
						}
					}
					if !proved[occurrence] {
						listClosed = false
					}
				}
				if selectionInfo == nil || selectionInfo.Obj() != selection.workspace {
					break
				}
				for position := range positions {
					if occurrence.Pos() <= position && position < occurrence.End() {
						proved[occurrence] = true
					}
				}
			case *ast.CallExpr:
				function, _, typed := typedCallee(pass, occurrence.Fun)
				if !typed {
					break
				}
				approved := false
				for call := range calls {
					callee := call.Call.StaticCallee()
					if callee != nil && callee.Object() == function && occurrence.Pos() <= call.Pos() && call.Pos() < occurrence.End() {
						approved = true
					}
				}
				if !approved {
					break
				}
				if selector, ok := ps2110Unparen(occurrence.Fun).(*ast.SelectorExpr); ok {
					if identifier, ok := ps2110Unparen(selector.X).(*ast.Ident); ok && types.Identical(pass.TypesInfo.TypeOf(identifier), types.NewPointer(selection.ownerType)) {
						proved[identifier] = true
					}
				}
				for _, argument := range occurrence.Args {
					if identifier, ok := ps2110Unparen(argument).(*ast.Ident); ok && types.Identical(pass.TypesInfo.TypeOf(identifier), types.NewPointer(selection.ownerType)) {
						proved[identifier] = true
					}
				}
			case *ast.FuncDecl:
				object, ok := pass.TypesInfo.Defs[occurrence.Name].(*types.Func)
				if !ok {
					break
				}
				function := pkg.Prog.FuncValue(object)
				if constructors[function].value == nil {
					break
				}
				ast.Inspect(occurrence.Body, func(node ast.Node) bool {
					if _, nested := node.(*ast.FuncLit); nested {
						return false
					}
					if returned, ok := node.(*ast.ReturnStmt); ok && len(returned.Results) != 0 {
						if identifier, ok := ps2110Unparen(returned.Results[0]).(*ast.Ident); ok && types.Identical(pass.TypesInfo.TypeOf(identifier), types.NewPointer(selection.ownerType)) {
							proved[identifier] = true
						}
					}
					return true
				})
			}
			return true
		})
		if !listClosed {
			return false
		}
	}
	return ps6136ClosedObservations(pass, selection.ownerType, selection.workspace, proved)
}

type ps6136ConsumerProof struct {
	reads             map[token.Pos]bool
	calls             map[*ssa.Call]bool
	capacityTransfers int
	boundedLeaves     int
}

func (selection *ps6136Selection) consumerCalls(context *ps6125SSAContext, leaves []config.OutputWorkspaceLeaf, bulk bool, capacity func(*ps6125SSAContext, *ssa.Call, *config.OutputWorkspaceLeaf) bool) *ps6136ConsumerProof {
	if selection == nil || context == nil || len(context.flow.function.Params) == 0 {
		return nil
	}
	parameter := context.flow.function.Params[0]
	parameterObject := ps6136ParameterObject(parameter)
	if parameterObject == nil || !types.Identical(parameter.Type(), types.NewPointer(selection.ownerType)) {
		return nil
	}
	owner := context.reference(parameter)
	var pool ps6125ExtentPool
	width := ps6125SymbolicExtent(pool.identity(parameterObject, []*types.Var{selection.width}, false))
	var bulkExtent ps6125Extent
	if bulk {
		if len(context.flow.function.Params) < 2 {
			return nil
		}
		tokens := context.flow.function.Params[1]
		tokensObject := ps6136ParameterObject(tokens)
		if tokensObject == nil || !types.Identical(tokens.Type(), types.NewSlice(types.Typ[types.Int])) {
			return nil
		}
		length := ps6125SymbolicExtent(pool.identity(tokensObject, nil, true))
		// Recompute the contextual scalar/branch algebra after binding the
		// actual bulk descriptor, retaining the same invocation identity.
		context.flow = ps6125AnalyzeSSAExtents(context.flow.function, nil, map[*ssa.Parameter]ps6125Extent{tokens: length})
		bulkExtent = ps6125MultiplyExtents(width, length)
	}
	facts := &ps6136Extents{owner: owner, ownerType: selection.ownerType, immutable: map[*types.Var]ps6125Extent{selection.width: width}, remaining: 16384}
	proof := &ps6136ConsumerProof{reads: make(map[token.Pos]bool), calls: make(map[*ssa.Call]bool)}
	leafCalls := make(map[*ps6125SSAContext]map[*ssa.Call]bool)
	readValues := make(map[ps6125SSAReference]token.Pos)
	valid := true
	bulkWitness := false
	remaining := 16384
	if !ps6136WalkConsumerCalls(context, func(current *ps6125SSAContext, call *ssa.Call) bool {
		if !valid {
			return true
		}
		paths := ps6125AccessPaths{flow: current.flow}
		for _, block := range current.flow.function.Blocks {
			if !current.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				load, ok := instruction.(*ssa.UnOp)
				if !ok || load.Op != token.MUL {
					continue
				}
				path := paths.resolve(load.X)
				if !path.known || len(path.access.fields) != 1 || path.access.fields[0] != selection.workspace {
					continue
				}
				if ps6136AccessOwnerRoot(current, path, selection.ownerType) != owner {
					valid = false
					return true
				}
				readValues[ps6125SSAReference{context: current, value: load}] = load.Pos()
			}
		}
		for index := range leaves {
			leaf := &leaves[index]
			extent, ok := ps6136LeafExtent(current, call, leaf, facts, selection.workspace, selection.slot, selection.projector, selection.width)
			if !ok {
				continue
			}
			if (leaf.Kind == "projection" || leaf.Kind == "row-width") && !selection.leafRecorderOrigin(current, call, leaf.Kind, owner) {
				valid = false
				return true
			}
			if leaf.Kind == "capacity-transfer" {
				if capacity == nil || !capacity(current, call, leaf) {
					valid = false
					return true
				}
				proof.capacityTransfers++
			} else {
				if !ps6125SameExtent(extent, width) && (!bulk || !ps6125SameExtent(extent, bulkExtent)) {
					valid = false
					return true
				}
				proof.boundedLeaves++
				if bulk && ps6125SameExtent(extent, bulkExtent) {
					bulkWitness = true
				}
			}
			proof.calls[call] = true
			if leafCalls[current] == nil {
				leafCalls[current] = make(map[*ssa.Call]bool)
			}
			leafCalls[current][call] = true
			return true
		}
		child := ps6136Call(current, call)
		for _, argument := range call.Call.Args {
			if types.Identical(argument.Type(), types.NewPointer(selection.ownerType)) {
				if child == nil || len(child.flow.function.Blocks) == 0 || ps6136OwnerRoot(current, argument, selection.ownerType) != owner {
					valid = false
					return true
				}
				proof.calls[call] = true
			}
		}
		return false
	}, &remaining) || !valid || context.budget.remaining == 0 {
		return nil
	}
	for reference, position := range readValues {
		// Only separately proved leaves, not ordinary helper edges, may
		// terminate the workspace's buffer-alias use graph.
		if !position.IsValid() || !ps6136WorkspaceReadClosed(reference.value, selection.slot, leafCalls[reference.context], 512) {
			return nil
		}
		proof.reads[position] = true
	}
	if proof.boundedLeaves == 0 || bulk && !bulkWitness {
		return nil
	}
	return proof
}

func (selection *ps6136Selection) configValue(context *ps6125SSAContext, value ssa.Value, wanted *types.Var, budget int) bool {
	if selection == nil || context == nil || value == nil || wanted == nil || budget <= 0 || !types.Identical(value.Type(), types.Typ[types.Int]) {
		return false
	}
	path := ps6136SelectionPath(context, value, budget)
	return path.known && len(path.fields) == 2 && path.fields[0] == selection.modelConfig && path.fields[1] == wanted && selection.model == path.root
}

type ps6136SelectionAccess struct {
	root   ps6125SSAReference
	fields []*types.Var
	known  bool
}

// Compose typed field paths through actual invocation bindings. Loading a
// config value is only provenance, not a claim its mutable model stayed stable.
func ps6136SelectionPath(context *ps6125SSAContext, value ssa.Value, budget int) ps6136SelectionAccess {
	if context == nil || value == nil || budget <= 0 {
		return ps6136SelectionAccess{}
	}
	reference := context.reference(value)
	if reference.value == nil {
		return ps6136SelectionAccess{}
	}
	if reference.context != context || reference.value != value {
		return ps6136SelectionPath(reference.context, reference.value, budget-1)
	}
	paths := ps6125AccessPaths{flow: context.flow}
	path := paths.resolve(value)
	if !path.known {
		return ps6136SelectionAccess{}
	}
	root := context.reference(path.access.root)
	if root.value == nil {
		return ps6136SelectionAccess{}
	}
	if allocation, ok := root.value.(*ssa.Alloc); ok {
		stores := root.context.structCell(allocation)
		point, ok := value.(ssa.Instruction)
		if stores == nil || stores.whole == nil || !ok || root.context != context || !context.flow.instructionDominates(stores.whole, point) {
			return ps6136SelectionAccess{}
		}
		prefix := ps6136SelectionPath(context, stores.whole.Val, budget-1)
		if !prefix.known {
			return prefix
		}
		prefix.fields = append(prefix.fields, path.access.fields...)
		return prefix
	}
	if root.context != context || root.value != path.access.root {
		prefix := ps6136SelectionPath(root.context, root.value, budget-1)
		if !prefix.known {
			return prefix
		}
		prefix.fields = append(prefix.fields, path.access.fields...)
		return prefix
	}
	return ps6136SelectionAccess{root: root, fields: path.access.fields, known: true}
}

func (selection *ps6136Selection) initializationValues(remaining int) bool {
	if selection == nil || selection.context == nil || selection.owner.value == nil || selection.model.value == nil {
		return false
	}
	counts := make(map[*types.Var]int)
	pending := []*ps6125SSAContext{selection.context}
	for len(pending) != 0 {
		context := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		remaining--
		if remaining <= 0 {
			return false
		}
		paths := ps6125AccessPaths{flow: context.flow}
		for _, block := range context.flow.function.Blocks {
			if !context.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				if store, ok := instruction.(*ssa.Store); ok {
					path := paths.resolve(store.Addr)
					if path.known && len(path.access.fields) == 1 && ps6136AccessOwnerRoot(context, path, selection.ownerType) == selection.owner {
						field := path.access.fields[0]
						if field == selection.maximumRows || field == selection.width {
							wanted := selection.configWidth
							if field == selection.maximumRows {
								wanted = selection.configRows
							}
							if !selection.configValue(context, store.Val, wanted, 128) {
								return false
							}
							counts[field]++
						}
					}
				}
				if call, ok := instruction.(*ssa.Call); ok {
					if child := ps6136Call(context, call); child != nil && len(child.flow.function.Blocks) != 0 {
						pending = append(pending, child)
					}
				}
				if store, ok := instruction.(*ssa.UnOp); ok && store.Op == token.ARROW {
					return false
				}
			}
		}
	}
	return counts[selection.maximumRows] == 1 && counts[selection.width] == 1
}

type ps6136ConstructorProof struct {
	allocation *ssa.Store
	factory    *ps6125SSAContext
	errorCell  ps6125SSAReference
	residency  ps6136Residency
}

func (selection *ps6136Selection) constructorAllocation(remaining int) *ps6136ConstructorProof {
	if selection == nil || !selection.initializationValues(remaining) || selection.workspace == nil || selection.allocator == nil || selection.ops == nil || selection.slot == nil || selection.retained == nil || selection.projector == nil {
		return nil
	}
	model, ok := selection.model.value.(*ssa.Parameter)
	modelObject := ps6136ParameterObject(model)
	if !ok || modelObject == nil {
		return nil
	}
	var pool ps6125ExtentPool
	rows := pool.identity(modelObject, []*types.Var{selection.modelConfig, selection.configRows}, false)
	width := pool.identity(modelObject, []*types.Var{selection.modelConfig, selection.configWidth}, false)
	residency, ok := ps6136OutputResidency(rows, width)
	if !ok {
		return nil
	}
	facts := &ps6136Extents{owner: selection.owner, ownerType: selection.ownerType, immutable: map[*types.Var]ps6125Extent{selection.maximumRows: ps6125SymbolicExtent(rows), selection.width: ps6125SymbolicExtent(width)}, remaining: remaining}
	proof := &ps6136ConstructorProof{residency: residency}
	pending := []*ps6125SSAContext{selection.context}
	headStores := 0
	for len(pending) != 0 {
		context := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		remaining--
		if remaining <= 0 {
			return nil
		}
		paths := ps6125AccessPaths{flow: context.flow}
		for _, block := range context.flow.function.Blocks {
			if !context.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				store, ok := instruction.(*ssa.Store)
				if !ok {
					continue
				}
				path := paths.resolve(store.Addr)
				if !path.known || len(path.access.fields) != 1 || ps6136AccessOwnerRoot(context, path, selection.ownerType) != selection.owner {
					continue
				}
				switch path.access.fields[0] {
				case selection.projector:
					if !ps6136ProjectorHeadWidth(context, store.Val, selection.projectorWidths, selection.model, selection.modelType, selection.modelHead, selection.shapeMethod, 1, 512) {
						return nil
					}
					headStores++
				case selection.workspace:
					function, ok := context.flow.function.Object().(*types.Func)
					if proof.allocation != nil || !ok || ps6090FunctionID(function) != selection.allocationFunction {
						return nil
					}
					call, ok := store.Val.(*ssa.Call)
					if !ok || len(call.Call.Args) != 1 {
						return nil
					}
					input := context.reference(call.Call.Args[0])
					allocation, ok := input.value.(*ssa.MakeSlice)
					if !ok || !types.Identical(allocation.Type(), types.NewSlice(types.Typ[types.Float32])) || !ps6125SameExtent(facts.length(input.context, allocation), ps6125MultiplyExtents(ps6125SymbolicExtent(rows), ps6125SymbolicExtent(width))) {
						return nil
					}
					factory := ps6136Call(context, call)
					backend := ps6136FactoryResult(factory, input, selection.allocator, selection.slot)
					if backend == nil || !ps6136FactoryRetention(factory, backend, selection.owner, selection.ownerType, selection.retained, selection.slot) {
						return nil
					}
					if factory.call(backend) == nil {
						snapshot := ps6136InitialOwnerField(selection.owner, selection.ops)
						if snapshot.value == nil {
							return nil
						}
						origin := snapshot.context.structField(snapshot.value, selection.allocatorIndex)
						if ps6136CallOrigin(factory, backend, origin) == nil {
							return nil
						}
					}
					if !ps6136BoundAllocator(factory, backend, input, selection.allocatorIDs) {
						return nil
					}
					errorCell := ps6136FactoryErrorCell(factory, backend)
					if errorCell.value == nil || !ps6136ConstructorErrorBarrier(selection.context, selection.owner, selection.ownerType, errorCell, selection.releaseMethod) {
						return nil
					}
					proof.allocation, proof.factory, proof.errorCell = store, factory, errorCell
				}
			}
		}
		for _, block := range context.flow.function.Blocks {
			if !context.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				if call, ok := instruction.(*ssa.Call); ok {
					if child := ps6136Call(context, call); child != nil && len(child.flow.function.Blocks) != 0 {
						pending = append(pending, child)
					}
				}
			}
		}
	}
	if proof.allocation == nil || headStores != 1 {
		return nil
	}
	return proof
}

func (selection *ps6136Selection) constructorLifetime(proof *ps6136ConstructorProof, remaining int) bool {
	if selection == nil || proof == nil || proof.errorCell.value == nil {
		return false
	}
	pending := []*ps6125SSAContext{selection.context}
	phase := make(map[ssa.Instruction]bool)
	for len(pending) != 0 {
		context := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		remaining--
		if remaining <= 0 {
			return false
		}
		paths := ps6125AccessPaths{flow: context.flow}
		for _, block := range context.flow.function.Blocks {
			if !context.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				switch use := instruction.(type) {
				case *ssa.Defer, *ssa.Go:
					return false // deferred allocation/error effects can run after publication
				case *ssa.Store:
					if ps6136ErrorCellRoot(context, use.Addr) == proof.errorCell {
						errorValue, ok := use.Val.(*ssa.Extract)
						if !ok || errorValue.Index != 1 {
							return false
						}
						backend, ok := errorValue.Tuple.(*ssa.Call)
						if !ok || ps6136FactoryErrorCell(context, backend) != proof.errorCell {
							return false
						}
					}
					if ps6136ErrorCellRoot(context, use.Val) == proof.errorCell {
						allocation, ok := use.Addr.(*ssa.Alloc)
						if !ok || ps6136ClosedOwnerCell(context, allocation) != use {
							return false
						}
					}
					if _, function := use.Val.Type().Underlying().(*types.Signature); function {
						allocation, ok := use.Addr.(*ssa.Alloc)
						if !ok || context.functionCell(allocation) != use {
							return false
						}
					}
				case *ssa.Convert:
					if ps6136ErrorCellRoot(context, use.X) == proof.errorCell {
						return false
					}
				case *ssa.MakeInterface:
					if ps6136ErrorCellRoot(context, use.X) == proof.errorCell {
						return false
					}
				case *ssa.Call:
					path := paths.resolve(use.Call.Value)
					if path.known && len(path.access.fields) != 0 && path.access.fields[len(path.access.fields)-1] == selection.allocator {
						if len(use.Call.Args) != 1 {
							return false
						}
						input := context.reference(use.Call.Args[0])
						if ps6136FactoryResult(context, input, selection.allocator, selection.slot) != use || ps6136FactoryErrorCell(context, use) != proof.errorCell || !ps6136FactoryRetention(context, use, selection.owner, selection.ownerType, selection.retained, selection.slot) {
							return false
						}
						if context.call(use) == nil {
							snapshot := ps6136InitialOwnerField(selection.owner, selection.ops)
							if snapshot.value == nil {
								return false
							}
							if ps6136CallOrigin(context, use, snapshot.context.structField(snapshot.value, selection.allocatorIndex)) == nil {
								return false
							}
						}
						if !ps6136BoundAllocator(context, use, input, selection.allocatorIDs) {
							return false
						}
						var point ssa.Instruction = use
						ancestor := context
						for ancestor != selection.context && ancestor.parent != nil {
							point = ancestor.site
							ancestor = ancestor.parent
						}
						if ancestor != selection.context || point == nil {
							return false
						}
						phase[point] = true
					}
					child := ps6136Call(context, use)
					for _, argument := range use.Call.Args {
						_, function := argument.Type().Underlying().(*types.Signature)
						if function || ps6136ErrorCellRoot(context, argument) == proof.errorCell || types.Identical(argument.Type(), types.NewPointer(selection.ownerType)) {
							if child == nil || len(child.flow.function.Blocks) == 0 {
								return false
							}
						}
					}
					if child != nil && len(child.flow.function.Blocks) != 0 {
						ownerResult := false
						for index := 0; index < child.flow.function.Signature.Results().Len(); index++ {
							if types.Identical(child.flow.function.Signature.Results().At(index).Type(), types.NewPointer(selection.ownerType)) {
								ownerResult = true
							}
						}
						if !ownerResult {
							child = ps6136ZeroSourceFlow(child)
						}
						pending = append(pending, child)
					}
				}
			}
		}
	}
	if len(phase) == 0 {
		return false
	}
	for _, block := range selection.context.flow.function.Blocks {
		if !selection.context.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok || len(returned.Results) < 1 {
				continue
			}
			constant, nilOwner := returned.Results[0].(*ssa.Const)
			if !nilOwner || !constant.IsNil() {
				continue
			}
			afterAllocation := false
			for point := range phase {
				if ps6136InstructionCanPrecede(selection.context, point, returned) {
					afterAllocation = true
				}
			}
			if !afterAllocation {
				continue
			}
			cleanup := false
			for _, call := range ps6136Calls(selection.context) {
				callee := call.Call.StaticCallee()
				if callee == nil || callee.Object() == nil || ps6090FunctionID(callee.Object().(*types.Func)) != selection.releaseMethod || len(call.Call.Args) != 1 {
					continue
				}
				if ps6136OwnerRoot(selection.context, call.Call.Args[0], selection.ownerType) == selection.owner && selection.context.flow.instructionDominates(call, returned) {
					cleanup = true
				}
			}
			if !cleanup {
				return false
			}
		}
	}
	return true
}

func ps6136Calls(context *ps6125SSAContext) []*ssa.Call {
	var result []*ssa.Call
	for _, block := range context.flow.function.Blocks {
		if context.flow.blocks[block] {
			for _, instruction := range block.Instrs {
				if call, ok := instruction.(*ssa.Call); ok {
					result = append(result, call)
				}
			}
		}
	}
	return result
}

func ps6136InstructionCanPrecede(context *ps6125SSAContext, first, last ssa.Instruction) bool {
	if first.Block() == last.Block() {
		for _, instruction := range first.Block().Instrs {
			if instruction == first {
				return true
			}
			if instruction == last {
				return false
			}
		}
		return false
	}
	pending := []*ssa.BasicBlock{first.Block()}
	seen := make(map[*ssa.BasicBlock]bool)
	for len(pending) != 0 {
		block := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[block] {
			continue
		}
		seen[block] = true
		for _, next := range block.Succs {
			if !context.flow.edges[ps6125SSAEdge{from: block, to: next}] {
				continue
			}
			if next == last.Block() {
				return true
			}
			pending = append(pending, next)
		}
	}
	return false
}
