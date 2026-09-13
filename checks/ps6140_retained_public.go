package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Source allocation and public-use conjunction for one constructor/backend
// class. Dynamic interface/generic owner entries,
// native storage disjointness, lifetime/release, retained-list observers,
// checked geometry and external observers remain independent before reporting.
type ps6140RetainedPublicProof struct {
	constructor *ps6125SSAContext
	publication *ps6125SSAContext
	owner       ps6125SSAReference
	workspace   *types.Var
	allocation  *ps6136ConstructorProof
	storage     *ps6140FactoryStorageProof
	entries     map[ps6140OwnerEntry]*ps6140ScenarioUses
}

func ps6140RetainedPublicSource(pkg *ssa.Package, selection *ps6136Selection, collection *ps6140SelectedCollectionProof, flags *ps6140ImmutableFlags, formal, budget int) *ps6140RetainedPublicProof {
	if pkg == nil || selection == nil || selection.context == nil || collection == nil || flags == nil || flags.snapshot == nil || budget <= 0 {
		return nil
	}
	if selection.context != collection.constructor || selection.context != flags.snapshot.constructor || selection.owner != collection.owner || selection.owner != flags.snapshot.owner || selection.context.flow.function.Pkg != pkg {
		return nil // Same types or constructor names are not the same instance.
	}
	publication := ps6140PublicConstructorScope(selection.context, flags, selection.ownerType, budget)
	if publication == nil {
		return nil
	}
	allocation := ps6140ConstructorAllocation(selection, budget)
	if allocation == nil {
		return nil // The after revision must be quiet for lack of eager storage.
	}
	if !ps6140ConstructorCallbacks(publication, selection.ownerType, selection.workspace, budget) {
		return nil
	}
	if !ps6140ConstructorStorage(publication, selection.owner, selection.ownerType, selection.workspace, allocation, budget) {
		return nil
	}
	storage := ps6140FactoryStorage(allocation.factory, selection.allocator, selection.slot, selection.allocatorIDs, budget)
	if storage == nil {
		return nil
	}
	entries := ps6140PublicWorkspaceUses(pkg, collection, flags, selection.ownerType, selection.workspace, selection.slot, formal, budget)
	if len(entries) == 0 {
		return nil
	}
	return &ps6140RetainedPublicProof{constructor: selection.context, publication: publication, owner: selection.owner, workspace: selection.workspace, allocation: allocation, storage: storage, entries: entries}
}
