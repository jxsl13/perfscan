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
	release      *types.Func
	elementClose *types.Func
	retentions   map[*ps6125SSAContext]*ps6140FactoryRetentionProof
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

func ps6140RetainedListSourceCheck(pass *analysis.Pass, pkg *ssa.Package, selection *ps6136Selection, public *ps6140RetainedPublicProof, budget int, rejected func(string)) (result *ps6140RetainedListProof) {
	stage := "input"
	defer func() {
		if result == nil && rejected != nil {
			rejected(stage)
		}
	}()
	if pass == nil || pkg == nil || pass.Pkg != pkg.Pkg || selection == nil || public == nil || selection.context != public.constructor || selection.owner != public.owner || public.publication == nil || public.storage == nil || budget <= 0 || selection.retained == nil || selection.retained.Exported() {
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
	proof := &ps6140RetainedListProof{public: public, release: release, elementClose: close, retentions: make(map[*ps6125SSAContext]*ps6140FactoryRetentionProof)}
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
					input := context.reference(context.flow.function.Params[0])
					backend := ps6136FactoryResult(context, input, selection.allocator, selection.slot)
					retention = ps6140FactoryRetention(context, backend, owner, selection.ownerType, list, selection.slot, budget)
					if retention == nil || ps6136FactoryErrorCell(context, backend) != public.allocation.errorCell {
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
	pending := []*ps6125SSAContext{public.publication}
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
	for _, uses := range public.entries {
		for context := range uses.scenario.refined {
			if !visit(context, uses.scenario.receiver, false) {
				return nil
			}
		}
	}
	if proof.retentions[public.allocation.factory] == nil {
		return nil // The selected allocation must participate, not just siblings.
	}
	return proof
}
