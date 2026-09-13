package checks

import (
	"cmp"
	"go/types"
	"slices"

	"golang.org/x/tools/go/ssa"
)

type ps6140OwnerEntry struct {
	function  *ssa.Function
	parameter int
}

// Inventory explicit typed owner roles of exported source entries. Private
// helpers are analyzed through their actual public invocation, not independently
// with arbitrary architecture/collection arguments. Unsupported owner-carrying
// aggregates, value receivers and exported function-valued storage reject the
// inventory instead of silently omitting an alternate entry into private code.
// Dynamic interface/generic inputs without typed owner recovery, reflection,
// opaque observers and native aliases still require independent closure. This
// exact-role inventory alone is not a whole-program reachability proof.
func ps6140PublicOwnerEntries(pkg *ssa.Package, owner *types.Named, budget int) []ps6140OwnerEntry {
	if pkg == nil || owner == nil || owner.Obj().Pkg() != pkg.Pkg || budget <= 0 {
		return nil
	}
	for _, member := range pkg.Members {
		if global, ok := member.(*ssa.Global); ok && ps6140ContainsOwner(global.Type(), owner, make(map[types.Type]bool), &budget) {
			return nil
		}
	}
	var entries []ps6140OwnerEntry
	for _, function := range ps6136SourceFunctions(pkg) {
		budget--
		if budget <= 0 {
			return nil
		}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				budget--
				if budget <= 0 {
					return nil
				}
				switch instruction := instruction.(type) {
				case *ssa.TypeAssert:
					if ps6140ContainsOwner(instruction.AssertedType, owner, make(map[types.Type]bool), &budget) {
						return nil // Interface/generic owner input needs its own binding proof.
					}
				case *ssa.MakeInterface:
					if literal, ok := instruction.X.(*ssa.Const); ok && literal.IsNil() {
						continue // A method-set assertion on typed nil publishes no instance.
					}
					if ps6140ContainsOwner(instruction.X.Type(), owner, make(map[types.Type]bool), &budget) {
						return nil // Includes owner method expressions hidden in any.
					}
				}
			}
		}
		object, declared := function.Object().(*types.Func)
		if !declared || !object.Exported() {
			continue
		}
		// Use signatures rather than SSA Params to include bodyless entries.
		signature := function.Signature
		for index := 0; index < signature.Results().Len(); index++ {
			result := signature.Results().At(index).Type()
			if ps6140ContainsOwner(result, owner, make(map[types.Type]bool), &budget) && !types.Identical(result, types.NewPointer(owner)) {
				return nil // Returned callable/aggregate can expose a private entry.
			}
		}
		var parameters []types.Type
		if signature.Recv() != nil {
			parameters = append(parameters, signature.Recv().Type())
		}
		for index := 0; index < signature.Params().Len(); index++ {
			parameters = append(parameters, signature.Params().At(index).Type())
		}
		for index, parameter := range parameters {
			if !ps6140ContainsOwner(parameter, owner, make(map[types.Type]bool), &budget) {
				continue
			}
			if !types.Identical(parameter, types.NewPointer(owner)) || len(function.Blocks) == 0 || len(function.Params) != len(parameters) || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 {
				return nil
			}
			entries = append(entries, ps6140OwnerEntry{function: function, parameter: index})
		}
	}
	if budget <= 0 || len(entries) == 0 {
		return nil
	}
	slices.SortFunc(entries, func(a, b ps6140OwnerEntry) int {
		if a.function.Pos() == b.function.Pos() {
			return cmp.Compare(a.parameter, b.parameter)
		}
		return cmp.Compare(a.function.Pos(), b.function.Pos())
	})
	return entries
}

func ps6140ContainsOwner(typ types.Type, owner *types.Named, seen map[types.Type]bool, budget *int) bool {
	*budget--
	if *budget <= 0 || typ == nil {
		return true
	}
	if types.Identical(typ, owner) {
		return true
	}
	if seen[typ] {
		return false
	}
	seen[typ] = true
	switch typ := typ.Underlying().(type) {
	case *types.Pointer:
		return ps6140ContainsOwner(typ.Elem(), owner, seen, budget)
	case *types.Slice:
		return ps6140ContainsOwner(typ.Elem(), owner, seen, budget)
	case *types.Array:
		return ps6140ContainsOwner(typ.Elem(), owner, seen, budget)
	case *types.Chan:
		return ps6140ContainsOwner(typ.Elem(), owner, seen, budget)
	case *types.Map:
		return ps6140ContainsOwner(typ.Key(), owner, seen, budget) || ps6140ContainsOwner(typ.Elem(), owner, seen, budget)
	case *types.Struct:
		for index := 0; index < typ.NumFields(); index++ {
			if ps6140ContainsOwner(typ.Field(index).Type(), owner, seen, budget) {
				return true
			}
		}
	case *types.Signature:
		if typ.Recv() != nil && ps6140ContainsOwner(typ.Recv().Type(), owner, seen, budget) {
			return true
		}
		for _, tuple := range []*types.Tuple{typ.Params(), typ.Results()} {
			for index := 0; index < tuple.Len(); index++ {
				if ps6140ContainsOwner(tuple.At(index).Type(), owner, seen, budget) {
					return true
				}
			}
		}
	case *types.Interface:
		for index := 0; index < typ.NumMethods(); index++ {
			if ps6140ContainsOwner(typ.Method(index).Type(), owner, seen, budget) {
				return true
			}
		}
	}
	return false
}

// Source-use closure for all public owner input roles, including entries with
// no direct workspace use. This does not prove native backing-storage aliases,
// retained-list observations, external effects or allocation/error semantics.
func ps6140PublicWorkspaceUses(pkg *ssa.Package, collection *ps6140SelectedCollectionProof, flags *ps6140ImmutableFlags, owner *types.Named, workspace, slot *types.Var, formal, budget int) map[ps6140OwnerEntry]*ps6140ScenarioUses {
	if workspace == nil || workspace.Exported() || slot == nil || slot.Exported() {
		return nil
	}
	entries := ps6140PublicOwnerEntries(pkg, owner, budget)
	if len(entries) == 0 {
		return nil
	}
	result := make(map[ps6140OwnerEntry]*ps6140ScenarioUses, len(entries))
	leaves := 0
	for _, entry := range entries {
		scenario := ps6140NewConstructorScenarioEntry(collection, flags, owner, entry.function, entry.parameter, budget)
		uses := ps6140ScenarioWorkspaceClosure(scenario, workspace, slot, formal, budget)
		if uses == nil {
			return nil
		}
		result[entry] = uses
		leaves += uses.leaves
	}
	if leaves == 0 {
		return nil
	}
	return result
}
