package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"maps"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

type ps6140RetainedListProof struct {
	public       *ps6140RetainedPublicProof
	input        *ps6140RetainedListInput
	release      *types.Func
	elementClose *types.Func
	retentions   map[*ps6125SSAContext]*ps6140FactoryRetentionProof
}

// Actual source receipts only. Callers supply the complete reviewed public
// context inventory, not fabricated ScenarioUses. List closure does not prove
// constructor callbacks, storage isolation, failure cleanup, release phase or
// native lifetime; those remain separately composed prerequisites.
type ps6140RetainedListInput struct {
	constructor, publication *ps6125SSAContext
	owner                    ps6125SSAReference
	storage                  *ps6140FactoryStorageProof
	publicContexts           []ps6140RetainedListContext
	requiredFactories        map[*ps6125SSAContext]ps6125SSAReference
}

type ps6140RetainedListContext struct {
	context *ps6125SSAContext
	owner   ps6125SSAReference
}

func ps6140RetainedListPublicInput(public *ps6140RetainedPublicProof) *ps6140RetainedListInput {
	if public == nil || public.allocation == nil {
		return nil
	}
	input := &ps6140RetainedListInput{constructor: public.constructor, publication: public.publication, owner: public.owner, storage: public.storage, requiredFactories: map[*ps6125SSAContext]ps6125SSAReference{public.allocation.factory: public.allocation.errorCell}}
	for _, uses := range public.entries {
		if uses == nil || uses.scenario == nil {
			return nil
		}
		for context := range uses.scenario.refined {
			input.publicContexts = append(input.publicContexts, ps6140RetainedListContext{context, uses.scenario.receiver})
		}
	}
	return input
}

// Source-level list isolation for the selected constructor class and its typed
// public entries. Every global field occurrence must be a fresh-constructor
// append or the exact release-loop/clear grammar. Actual selected invocations
// then recheck append execution and identity; constructor-only syntax cannot
// authorize the same helper when called later on a published owner.
// Native drain/completion/release and unsupported dynamic owner entrants remain
// separate obligations, as does failure cleanup beyond this list's source uses.
func ps6140RetainedListSource(pass *analysis.Pass, pkg *ssa.Package, selection *ps6136Selection, public *ps6140RetainedPublicProof, budget int) *ps6140RetainedListProof {
	return ps6140RetainedListSourceCheck(pass, pkg, selection, public, budget, nil)
}

func ps6140RetainedListSourceCheck(pass *analysis.Pass, pkg *ssa.Package, selection *ps6136Selection, public *ps6140RetainedPublicProof, budget int, rejected func(string)) *ps6140RetainedListProof {
	result := ps6140RetainedListReceiptSourceCheck(pass, pkg, selection, ps6140RetainedListPublicInput(public), budget, rejected)
	if result != nil {
		result.public = public
	}
	return result
}

func ps6140RetainedListReceiptSource(pass *analysis.Pass, pkg *ssa.Package, selection *ps6136Selection, input *ps6140RetainedListInput, budget int) *ps6140RetainedListProof {
	return ps6140RetainedListReceiptSourceCheck(pass, pkg, selection, input, budget, nil)
}

func ps6140RetainedListReceiptSourceCheck(pass *analysis.Pass, pkg *ssa.Package, selection *ps6136Selection, input *ps6140RetainedListInput, budget int, rejected func(string)) (result *ps6140RetainedListProof) {
	stage := "input"
	defer func() {
		if result == nil && rejected != nil {
			rejected(stage)
		}
	}()
	if pass == nil || pkg == nil || pass.Pkg != pkg.Pkg || selection == nil || input == nil || selection.context != input.constructor || selection.owner != input.owner || input.publication == nil || input.storage == nil || budget <= 0 || selection.retained == nil || selection.retained.Exported() || len(input.requiredFactories) == 0 {
		return nil
	}
	if !ps6140RetainedListInputs(pkg, selection, input, &budget) {
		return nil
	}
	list := selection.retained
	shape, ok := list.Type().Underlying().(*types.Slice)
	if !ok || !types.Identical(shape.Elem(), selection.slot.Type()) {
		return nil
	}
	closeObject, _, _ := types.LookupFieldOrMethod(shape.Elem(), true, pass.Pkg, "Release")
	close, ok := closeObject.(*types.Func)
	if !ok || ps6090FunctionID(close) != selection.retainedReleaseMethod {
		return nil
	}
	var release *types.Func
	var releaseNodes map[ast.Node]bool
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
			signature := object.Type().(*types.Signature)
			if release != nil || !types.Identical(signature.Recv().Type(), types.NewPointer(selection.ownerType)) || signature.Params().Len() != 0 || signature.Results().Len() != 0 {
				return nil
			}
			release = object
			releaseNodes = ps6136ReleaseList(pass, function, pass.TypesInfo.Defs[function.Recv.List[0].Names[0]], list, close)
		}
	}
	stage = "exact release list grammar"
	if len(releaseNodes) != 2 {
		return nil
	}
	stage = "fresh constructor list occurrence census"
	writes := ps6136FreshConstructorWrites(pkg, selection.ownerType, map[*types.Var]bool{list: true})
	if writes == nil {
		return nil
	}
	initializing := *writes
	initializing.writers = maps.Clone(writes.writers)
	delete(initializing.writers, pkg.Prog.FuncValue(release))
	writeNodes := ps6136ConstructorWriteNodes(pass, pkg, selection.ownerType, map[*types.Var]bool{list: true}, &initializing)
	if writeNodes == nil {
		return nil
	}
	approved := maps.Clone(releaseNodes)
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || !writeNodes[assignment.Lhs[0]] {
				return true
			}
			target, ok := assignment.Lhs[0].(*ast.SelectorExpr)
			if !ok || pass.TypesInfo.Selections[target] == nil || pass.TypesInfo.Selections[target].Obj() != list {
				return true
			}
			receiver, ok := target.X.(*ast.Ident)
			if !ok {
				return true
			}
			appendCall, ok := assignment.Rhs[0].(*ast.CallExpr)
			if !ok || appendCall.Ellipsis.IsValid() || len(appendCall.Args) != 2 || !ps6136ObjectExpr(pass, appendCall.Fun, types.Universe.Lookup("append")) || !ps6136DirectField(pass, appendCall.Args[0], pass.TypesInfo.Uses[receiver], list) {
				return true
			}
			approved[target], approved[appendCall.Args[0]] = true, true
			return true
		})
	}
	stage = "package list and owner effects"
	if !ps6136FieldEffects(pass, list, approved) {
		return nil
	}
	ownerNodes := ps6136OwnerTransfers(pass, pkg, selection.ownerType)
	if ownerNodes == nil {
		return nil
	}
	for node := range ps6136InteriorOwnerCells(pass, pkg, selection.ownerType, list) {
		ownerNodes[node] = true
	}
	maps.Copy(ownerNodes, approved)
	if !ps6136ClosedObservationsCheck(pass, selection.ownerType, list, ownerNodes, func(node ast.Node, reason string) {
		stage += ": " + reason + " " + pass.Fset.Position(node.Pos()).String()
	}) {
		return nil
	}
	proof := &ps6140RetainedListProof{input: input, release: release, elementClose: close, retentions: make(map[*ps6125SSAContext]*ps6140FactoryRetentionProof)}
	visit := func(context *ps6125SSAContext, owner ps6125SSAReference, construction bool) bool {
		paths := ps6125AccessPaths{flow: context.flow}
		for _, block := range context.flow.function.Blocks {
			if !context.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				stage = context.flow.function.String() + ": " + instruction.String()
				budget--
				if budget <= 0 {
					return false
				}
				address, ok := instruction.(*ssa.FieldAddr)
				if !ok {
					continue
				}
				path := paths.resolve(address)
				if !path.known || len(path.access.fields) != 1 || path.access.fields[0] != list {
					continue
				}
				if ps6136AccessOwnerRoot(context, path, selection.ownerType) != owner {
					return false
				}
				if context.flow.function.Object() == release {
					matched := false
					for node := range releaseNodes {
						matched = matched || node.Pos() <= address.Pos() && address.Pos() < node.End()
					}
					if !matched {
						return false
					}
					continue
				}
				if !construction || len(context.flow.function.Params) != 1 {
					return false
				}
				retention := proof.retentions[context]
				if retention == nil {
					parameter := context.reference(context.flow.function.Params[0])
					backend := ps6136FactoryResult(context, parameter, selection.allocator, selection.slot)
					retention = ps6140FactoryRetention(context, backend, owner, selection.ownerType, list, selection.slot, budget)
					if retention == nil || !ps6140RetainedListErrorCell(input, ps6136FactoryErrorCell(context, backend), &budget) {
						return false
					}
					proof.retentions[context] = retention
				}
				if !retention.fields[address] {
					return false
				}
			}
		}
		return true
	}
	pending := []*ps6125SSAContext{input.publication}
	seen := make(map[*ps6125SSAContext]bool)
	for len(pending) != 0 {
		context := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[context] {
			continue
		}
		seen[context] = true
		if !visit(context, selection.owner, true) {
			return nil
		}
		for _, call := range ps6136Calls(context) {
			if child := ps6136Call(context, call); child != nil {
				pending = append(pending, child)
			}
		}
	}
	for _, public := range input.publicContexts {
		if !visit(public.context, public.owner, false) {
			return nil
		}
	}
	stage = "required allocation participation"
	for factory := range input.requiredFactories {
		if proof.retentions[factory] == nil {
			return nil // Every required allocation participates, not just siblings.
		}
	}
	return proof
}

func ps6140RetainedListErrorCell(input *ps6140RetainedListInput, cell ps6125SSAReference, budget *int) bool {
	if cell.context == nil || cell.value == nil {
		return false
	}
	for _, required := range input.requiredFactories {
		*budget--
		if *budget <= 0 {
			return false
		}
		if cell == required {
			return true
		}
	}
	return false
}

func ps6140RetainedListInputs(pkg *ssa.Package, selection *ps6136Selection, input *ps6140RetainedListInput, budget *int) bool {
	if input.constructor == nil || input.constructor.flow == nil || input.constructor.flow.function.Pkg != pkg || input.publication.flow == nil || input.publication.flow.function.Pkg != pkg {
		return false
	}
	current := input.constructor
	for current != input.publication {
		*budget--
		if *budget <= 0 || current.parent == nil || current.site == nil || current.parent.calls[current.site] != current {
			return false
		}
		current = current.parent
	}
	storage := input.storage
	if _, required := input.requiredFactories[storage.factory]; !required || storage.factory == nil || storage.factory.flow == nil || len(storage.factory.flow.function.Params) != 1 || storage.backend == nil || storage.callback == nil || storage.callback.flow == nil || storage.native == nil || storage.wrapper == nil || storage.nativeField == nil {
		return false
	}
	if ps6136FactoryResult(storage.factory, storage.factory.reference(storage.factory.flow.function.Params[0]), selection.allocator, selection.slot) != storage.backend || storage.factory.call(storage.backend) != storage.callback {
		return false
	}
	native := storage.native.Call.StaticCallee()
	if storage.native.Parent() != storage.callback.flow.function || !storage.callback.flow.blocks[storage.native.Block()] || native == nil || native.Object() == nil || !selection.allocatorIDs[ps6090FunctionID(native.Object().(*types.Func))] || ps6136FieldVar(storage.wrapper, storage.nativeField.Name()) != storage.nativeField {
		return false
	}
	for factory, cell := range input.requiredFactories {
		*budget--
		if *budget <= 0 || factory == nil || factory.flow == nil || factory.flow.function.Pkg != pkg || len(factory.flow.function.Params) != 1 || cell.context == nil || cell.value == nil {
			return false
		}
		backend := ps6136FactoryResult(factory, factory.reference(factory.flow.function.Params[0]), selection.allocator, selection.slot)
		if backend == nil || ps6136FactoryErrorCell(factory, backend) != cell {
			return false
		}
	}
	for _, public := range input.publicContexts {
		*budget--
		if *budget <= 0 || public.context == nil || public.context.flow == nil || public.context.flow.function.Pkg != pkg || public.owner.context == nil || public.owner.value == nil || !types.Identical(public.owner.value.Type(), types.NewPointer(selection.ownerType)) {
			return false
		}
		root := public.context
		for root.parent != nil {
			*budget--
			if *budget <= 0 || root.site == nil || root.parent.calls[root.site] != root {
				return false
			}
			root = root.parent
		}
		if root.flow == nil || root.flow.function.Pkg != pkg {
			return false
		}
		formal := false
		for _, parameter := range root.flow.function.Params {
			formal = formal || root.reference(parameter) == public.owner
		}
		if !formal {
			return false
		}
	}
	return true
}
