package checks

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// A constructor-to-collection metadata lifetime proof. It does not establish
// immutable native buffers, runtime flags or absence of other scratch uses.
type ps6140SelectedCollectionProof struct {
	constructor *ps6125SSAContext
	owner       ps6125SSAReference
	appended    ps6125SSAReference
	blocks      *types.Var
	projectors  map[*types.Var]*types.Named
	nilFields   map[*types.Var]bool
}

func ps6140SelectedCollection(pass *analysis.Pass, pkg *ssa.Package, constructor *ps6125SSAContext, ownerType, blockType *types.Named, blocks *types.Var, projectors map[*types.Var]*types.Named, budget int) *ps6140SelectedCollectionProof {
	return ps6140SelectedCollectionCheck(pass, pkg, constructor, ownerType, blockType, blocks, projectors, budget, nil)
}

func ps6140SelectedCollectionCheck(pass *analysis.Pass, pkg *ssa.Package, constructor *ps6125SSAContext, ownerType, blockType *types.Named, blocks *types.Var, projectors map[*types.Var]*types.Named, budget int, rejected func(string)) (result *ps6140SelectedCollectionProof) {
	stage := "input"
	defer func() {
		if result == nil && rejected != nil {
			rejected(stage)
		}
	}()
	if pass == nil || pkg == nil || constructor == nil || constructor.flow == nil || ownerType == nil || blockType == nil || blocks == nil || len(projectors) == 0 || budget <= 0 {
		return nil
	}
	stage = "inventory"
	inventory := ps6136ConstructorInventory(pkg, ownerType)
	if inventory == nil || inventory[constructor.flow.function].value == nil {
		return nil
	}
	var selectedOwner ps6125SSAReference
	for _, block := range constructor.flow.function.Blocks {
		if !constructor.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok || len(returned.Results) == 0 {
				continue
			}
			if constant, ok := returned.Results[0].(*ssa.Const); ok && constant.IsNil() {
				continue
			}
			root := ps6136OwnerRoot(constructor, returned.Results[0], ownerType)
			if _, fresh := root.value.(*ssa.Alloc); !fresh || selectedOwner.value != nil && selectedOwner != root {
				return nil
			}
			selectedOwner = root
		}
	}
	if selectedOwner.value == nil {
		return nil
	}
	graph := &ps6140CollectionGraph{ownerType: ownerType, block: blockType, blocks: blocks, projectors: projectors, remaining: budget, seen: make(map[ps6125SSAReference]bool)}
	initializers := make(map[token.Pos]bool)
	var appended ps6125SSAReference
	selectedStores := 0
	for _, function := range ps6136SourceFunctions(pkg) {
		functionName := function.String()
		stage = "source " + functionName
		context := ps6125NewSSAContext(function, nil, nil, 1024)
		if context == nil {
			continue
		}
		if function == constructor.flow.function {
			context = constructor
		}
		paths := ps6125AccessPaths{flow: context.flow}
		for _, block := range function.Blocks {
			if !context.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				if !graph.take() {
					return nil
				}
				if store, ok := instruction.(*ssa.Store); ok {
					stage = "whole receiver " + functionName
					// Whole receiver replacement can invalidate every metadata
					// field without an explicit owner.blocks selector.
					if types.Identical(store.Addr.Type(), types.NewPointer(ownerType)) && types.Identical(store.Val.Type(), ownerType) {
						return nil
					}
					path := paths.resolve(store.Addr)
					storeOwner := ps6136AccessOwnerRoot(context, path, ownerType)
					if !path.known || len(path.access.fields) != 1 || path.access.fields[0] != blocks {
						field, exact := store.Addr.(*ssa.FieldAddr)
						if !exact || !types.Identical(field.X.Type(), types.NewPointer(ownerType)) {
							continue
						}
						structure := ownerType.Underlying().(*types.Struct)
						if field.Field < 0 || field.Field >= structure.NumFields() || structure.Field(field.Field) != blocks {
							continue
						}
						storeOwner = ps6136OwnerRoot(context, field.X, ownerType)
					}
					call, ok := store.Val.(*ssa.Call)
					stage = "append " + functionName
					if !ok || !graph.freshAppend(context, call) || !store.Pos().IsValid() {
						return nil
					}
					payload := ps6140AppendedBlock(context, call, blockType)
					stage = "private payload " + functionName
					positions := ps6140PrivateConstructedBlock(context, payload, call.Pos(), graph)
					if positions == nil {
						stage = "private payload " + functionName + ": " + graph.last
						return nil
					}
					initializers[store.Pos()] = true
					for position := range positions {
						initializers[position] = true
					}
					if context == constructor {
						stage = "selected projector " + functionName
						if storeOwner != selectedOwner {
							stage = "selected projector " + functionName + ": different owner"
							return nil
						}
						for field, concrete := range projectors {
							if !ps6140BlockProjectionValue(payload, field, concrete, 256) {
								stage = "selected projector " + functionName + ": " + field.Name() + " is not " + concrete.String()
								return nil
							}
						}
						selectedStores++
						appended = payload
					}
				}
				if value, ok := instruction.(ssa.Value); ok && ps6140DirectCollectionRead(value, ownerType, blocks) {
					stage = "read graph " + functionName
					if !graph.readOnly(context, value) {
						stage = "read graph " + functionName + ": " + graph.last
						return nil
					}
				}
			}
		}
	}
	stage = "observation closure"
	if selectedStores != 1 || !ps6140CollectionObservationClosure(pass, pkg, ownerType, blocks, projectors, inventory, initializers, &stage) {
		return nil
	}
	nilFields := make(map[*types.Var]bool)
	structure := blockType.Underlying().(*types.Struct)
	for index := 0; index < structure.NumFields(); index++ {
		field := structure.Field(index)
		if _, iface := field.Type().Underlying().(*types.Interface); iface && ps6140BlockFieldNil(appended, field, 128) {
			nilFields[field] = true
		}
	}
	return &ps6140SelectedCollectionProof{constructor: constructor, owner: selectedOwner, appended: appended, blocks: blocks, projectors: projectors, nilFields: nilFields}
}

func ps6140DirectCollectionRead(value ssa.Value, owner *types.Named, blocks *types.Var) bool {
	if field, ok := value.(*ssa.Field); ok {
		structure, ok := field.X.Type().Underlying().(*types.Struct)
		return ok && types.Identical(field.X.Type(), owner) && field.Field >= 0 && field.Field < structure.NumFields() && structure.Field(field.Field) == blocks
	}
	load, ok := value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	field, ok := load.X.(*ssa.FieldAddr)
	if !ok || !types.Identical(field.X.Type(), types.NewPointer(owner)) {
		return false
	}
	structure := owner.Underlying().(*types.Struct)
	return field.Field >= 0 && field.Field < structure.NumFields() && structure.Field(field.Field) == blocks
}

// Private constructor block storage may have conditional sibling/role writes
// for unselected families. Only direct owned storage before the copied append
// payload is allowed; this does not attest their final geometry or dispatch.
func ps6140PrivateConstructedBlock(context *ps6125SSAContext, payload ps6125SSAReference, copiedAt token.Pos, graph *ps6140CollectionGraph) map[token.Pos]bool {
	load, ok := payload.value.(*ssa.UnOp)
	if !ok || payload.context != context || load.Op != token.MUL {
		graph.last = "payload is not owned load"
		return nil
	}
	allocation, ok := load.X.(*ssa.Alloc)
	if !ok || allocation.Parent() != context.flow.function || !types.Identical(allocation.Type(), types.NewPointer(graph.block)) {
		return nil
	}
	positions := make(map[token.Pos]bool)
	pending := []ssa.Value{allocation}
	seen := make(map[ssa.Value]bool)
	for len(pending) > 0 {
		if !graph.take() {
			return nil
		}
		address := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[address] {
			continue
		}
		seen[address] = true
		users := address.Referrers()
		if users == nil {
			return nil
		}
		for _, instruction := range *users {
			instructionText := instruction.String()
			graph.last = instructionText
			if !graph.take() || instruction.Parent() != context.flow.function {
				return nil
			}
			switch user := instruction.(type) {
			case *ssa.DebugRef:
			case *ssa.FieldAddr:
				if user.X != address {
					return nil
				}
				pending = append(pending, user)
			case *ssa.Store:
				if user.Addr != address || user.Val == address || !user.Pos().IsValid() || !copiedAt.IsValid() || user.Pos() >= copiedAt {
					graph.last = instructionText + " [store pos " + strconv.Itoa(int(user.Pos())) + " load pos " + strconv.Itoa(int(load.Pos())) + "]"
					return nil
				}
				positions[user.Pos()] = true
				if address == allocation {
					// Address-taking sibling writes make SSA build the literal in
					// a second private cell, then copy its value into this cell.
					// Join only that exact owned load and its unique copy use.
					literal, ok := user.Val.(*ssa.UnOp)
					if !ok || literal.Op != token.MUL || literal.Referrers() == nil {
						return nil
					}
					for _, use := range *literal.Referrers() {
						if _, debug := use.(*ssa.DebugRef); !debug && use != user {
							return nil
						}
					}
					if !context.flow.instructionDominates(literal, user) {
						return nil
					}
					nested := ps6140PrivateConstructedBlock(context, ps6125SSAReference{context: context, value: literal}, copiedAt, graph)
					if nested == nil {
						return nil
					}
					for position := range nested {
						positions[position] = true
					}
				}
			case *ssa.UnOp:
				if user.X != address || user.Op != token.MUL {
					return nil
				}
				if address == allocation && user != load {
					return nil
				}
			default:
				return nil
			}
		}
	}
	return positions
}

func ps6140CollectionObservationClosure(pass *analysis.Pass, pkg *ssa.Package, owner *types.Named, blocks *types.Var, projectors map[*types.Var]*types.Named, inventory map[*ssa.Function]ps6125SSAReference, initializers map[token.Pos]bool, stage *string) bool {
	proved := ps6136InteriorOwnerCells(pass, pkg, owner, blocks)
	fieldInitializers := make(map[ast.Node]bool)
	valid := true
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			if branch, ok := node.(*ast.BranchStmt); ok && branch.Tok == token.GOTO {
				valid = false
			}
			var field *types.Var
			switch node := node.(type) {
			case *ast.SelectorExpr:
				if selection := pass.TypesInfo.Selections[node]; selection != nil {
					field, _ = selection.Obj().(*types.Var)
				}
				if field == blocks {
					proved[node] = true
				} // complete SSA load/store graph above
			case *ast.KeyValueExpr:
				if key, ok := node.Key.(*ast.Ident); ok {
					field, _ = pass.TypesInfo.Uses[key].(*types.Var)
				}
			case *ast.UnaryExpr:
				if node.Op == token.AND {
					if index, ok := ps2110Unparen(node.X).(*ast.IndexExpr); ok {
						if selector, ok := ps2110Unparen(index.X).(*ast.SelectorExpr); ok {
							if selection := pass.TypesInfo.Selections[selector]; selection != nil && selection.Obj() == blocks {
								// The loaded collection's IndexAddr and every interior
								// pointer use were closed by the source SSA graph.
								fieldInitializers[selector] = true
								proved[node] = true
							}
						}
					}
				}
			case *ast.AssignStmt:
				if initializers[node.TokPos] {
					for _, target := range node.Lhs {
						if selector, ok := ps2110Unparen(target).(*ast.SelectorExpr); ok {
							if selection := pass.TypesInfo.Selections[selector]; selection != nil {
								field, _ := selection.Obj().(*types.Var)
								if field == blocks || projectors[field] != nil {
									fieldInitializers[selector] = true
								}
							}
						}
					}
				}
			case *ast.CallExpr:
				function, _, ok := typedCallee(pass, node.Fun)
				if !ok {
					break
				}
				callee := pkg.Prog.FuncValue(function)
				if callee == nil || len(callee.Blocks) == 0 {
					break
				}
				if selector, ok := ps2110Unparen(node.Fun).(*ast.SelectorExpr); ok && types.Identical(pass.TypesInfo.TypeOf(selector.X), types.NewPointer(owner)) {
					proved[selector.X] = true
				}
				for _, argument := range node.Args {
					if types.Identical(pass.TypesInfo.TypeOf(argument), types.NewPointer(owner)) || types.Identical(pass.TypesInfo.TypeOf(argument), owner) {
						proved[argument] = true
					}
				}
			case *ast.FuncDecl:
				object, _ := pass.TypesInfo.Defs[node.Name].(*types.Func)
				function := pkg.Prog.FuncValue(object)
				if inventory[function].value == nil {
					break
				}
				ast.Inspect(node.Body, func(child ast.Node) bool {
					if _, nested := child.(*ast.FuncLit); nested {
						return false
					}
					if returned, ok := child.(*ast.ReturnStmt); ok && len(returned.Results) > 0 && types.Identical(pass.TypesInfo.TypeOf(returned.Results[0]), types.NewPointer(owner)) {
						proved[returned.Results[0]] = true
					}
					return true
				})
			}
			if field == blocks || projectors[field] != nil {
				for position := range initializers {
					if node.Pos() <= position && position < node.End() {
						fieldInitializers[node] = true
					}
				}
			}
			return true
		})
	}
	if !valid || !ps6136FieldEffects(pass, blocks, fieldInitializers) {
		*stage = "collection field effects"
		for _, file := range pass.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				if assignment, ok := node.(*ast.AssignStmt); ok {
					for _, target := range assignment.Lhs {
						if selector, ok := target.(*ast.SelectorExpr); ok {
							if selection := pass.TypesInfo.Selections[selector]; selection != nil && selection.Obj() == blocks && !fieldInitializers[selector] {
								*stage += " " + pass.Fset.Position(selector.Pos()).String()
							}
						}
					}
				}
				return true
			})
		}
		return false
	}
	for field := range projectors {
		if !ps6136FieldEffects(pass, field, fieldInitializers) {
			*stage = "projector field effects " + field.Name()
			for _, file := range pass.Files {
				ast.Inspect(file, func(node ast.Node) bool {
					if pair, ok := node.(*ast.KeyValueExpr); ok {
						if key, ok := pair.Key.(*ast.Ident); ok && pass.TypesInfo.Uses[key] == field && !fieldInitializers[pair] {
							*stage += " literal " + pass.Fset.Position(pair.Pos()).String()
						}
					}
					if assignment, ok := node.(*ast.AssignStmt); ok {
						for _, target := range assignment.Lhs {
							if selector, ok := target.(*ast.SelectorExpr); ok {
								if selection := pass.TypesInfo.Selections[selector]; selection != nil && selection.Obj() == field && !fieldInitializers[selector] {
									*stage += " write " + pass.Fset.Position(selector.Pos()).String()
								}
							}
						}
					}
					return true
				})
			}
			return false
		}
	}
	*stage = "closed owner observations"
	return ps6136ClosedObservationsCheck(pass, owner, blocks, proved, func(node ast.Node, reason string) {
		*stage += ": " + reason + " " + pass.Fset.Position(node.Pos()).String()
		var source bytes.Buffer
		if format.Node(&source, pass.Fset, node) == nil {
			*stage += " expression " + source.String()
		}
		for _, file := range pass.Files {
			for _, declaration := range file.Decls {
				if function, ok := declaration.(*ast.FuncDecl); ok && function.Pos() <= node.Pos() && node.End() <= function.End() {
					*stage += " function " + function.Name.Name
				}
			}
		}
	})
}
