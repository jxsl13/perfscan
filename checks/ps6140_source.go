package checks

import (
	"go/types"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// One source-partition certificate, not yet a reportable finding or an edit.
// Checked positive/native-width geometry, native physical semantics, external
// observations and other provider/build partitions remain independent gates.
// In particular profile dimensions never participate in these source proofs.
type ps6140SourceProof struct {
	selection     *ps6140SourceSelection
	collection    *ps6140SelectedCollectionProof
	flags         *ps6140ImmutableFlags
	retained      *ps6140RetainedListProof
	nativeRelease *types.Func
}

func ps6140UnusedProjectionSource(pass *analysis.Pass, pkg *ssa.Package, contract *config.UnusedProjectionScratchContract, budget int) *ps6140SourceProof {
	if pass == nil || pass.TypesInfo == nil || pass.Pkg == nil || pass.Fset == nil || pkg == nil || pass.Pkg != pkg.Pkg || !contract.Valid() || budget <= 0 {
		return nil
	}
	selected := ps6136ContractsContext(pass).unusedProjectionSelection(pkg, contract, budget)
	if selected == nil {
		return nil
	}
	selection := selected.allocation
	collection := ps6140SelectedCollection(pass, pkg, selection.context, selection.ownerType, selected.block, selected.blocks, selected.projectors, budget)
	if collection == nil || collection.owner != selection.owner {
		return nil
	}
	snapshot := ps6140ConstructorFlags(selection.context, selection.owner, selection.ownerType, selected.flags, budget)
	flags := ps6140ImmutableConstructorFlags(pass, pkg, snapshot, selection.ownerType, budget)
	if flags == nil {
		return nil
	}
	public := ps6140RetainedPublicSource(pkg, selection, collection, flags, contract.UnusedFormal, budget)
	if public == nil || public.publication != selected.entry {
		return nil
	}
	retained := ps6140RetainedListSource(pass, pkg, selection, public, budget)
	if retained == nil || !ps6140RetainedLifetimeSource(selection, retained, budget) {
		return nil
	}
	nativeRelease := ps6140NativeReleaseBinding(public.storage, retained.elementClose, contract.NativeReleaseMethod, budget)
	if nativeRelease == nil {
		return nil
	}
	return &ps6140SourceProof{selection: selected, collection: collection, flags: flags, retained: retained, nativeRelease: nativeRelease}
}
