package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

type ps6140ReleaseSite struct {
	context *ps6125SSAContext
	call    *ssa.Call
}

// Require that selected-owner Release cannot precede a successful public
// owner return. Nil/error cleanup is not successful publication. This is a
// source-phase proof, conjoined with owner/callback/list isolation; it does not
// infer native residency, completion or disjointness from a method name.
func ps6140ConstructorReleasePhase(publication *ps6125SSAContext, owner ps6125SSAReference, ownerType *types.Named, release *types.Func, budget int) bool {
	releases, complete := ps6140SourceOwnerReleases(publication, owner, ownerType, release, &budget)
	if !complete {
		return false
	}
	publications := ps6140SameOwnerPublications(publication, owner, ownerType, &budget)
	if len(publications) == 0 {
		return false
	}
	for _, returned := range publications {
		for _, released := range releases {
			if !ps6140PublicationExcludesRelease(publication, returned, released, owner, ownerType, &budget) {
				return false
			}
		}
	}
	return budget > 0
}

func ps6140SourceOwnerReleases(publication *ps6125SSAContext, owner ps6125SSAReference, ownerType *types.Named, release *types.Func, budget *int) ([]ps6140ReleaseSite, bool) {
	if publication == nil || publication.flow == nil || owner.context == nil || owner.value == nil || ownerType == nil || release == nil || budget == nil || *budget <= 0 {
		return nil, false
	}
	allocation, ok := owner.value.(*ssa.Alloc)
	if !ok || !types.Identical(allocation.Type(), types.NewPointer(ownerType)) {
		return nil, false
	}
	signature, ok := release.Type().(*types.Signature)
	if !ok || signature.Recv() == nil || !types.Identical(signature.Recv().Type(), types.NewPointer(ownerType)) || signature.Params().Len() != 0 || signature.Results().Len() != 0 || signature.Variadic() {
		return nil, false
	}
	var releases []ps6140ReleaseSite
	seen := make(map[*ps6125SSAContext]bool)
	pending := []*ps6125SSAContext{publication}
	for len(pending) != 0 {
		context := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[context] {
			continue
		}
		seen[context] = true
		for _, block := range context.flow.function.Blocks {
			if !context.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				*budget--
				if *budget <= 0 {
					return nil, false
				}
				switch instruction := instruction.(type) {
				case *ssa.Go, *ssa.Defer:
					return nil, false // No asynchronous/deferred completion is inferred.
				case *ssa.Call:
					child := ps6136Call(context, instruction)
					if child == nil {
						if callee := instruction.Call.StaticCallee(); callee != nil && callee.Pkg == publication.flow.function.Pkg && len(callee.Blocks) != 0 {
							return nil, false
						}
						for _, argument := range instruction.Call.Args {
							if ps6140ContainsOwner(argument.Type(), ownerType, make(map[types.Type]bool), budget) {
								return nil, false
							}
						}
						continue
					}
					// Bound-method wrappers can carry the method's Object while
					// binding its receiver as a free variable. Walk the wrapper;
					// count only the actual method invocation with its receiver.
					if child.flow.function.Object() == release && child.flow.function.Signature.Recv() != nil {
						if len(instruction.Call.Args) != 1 || ps6136OwnerRoot(context, instruction.Call.Args[0], ownerType) != owner {
							return nil, false // A foreign/unknown receiver may alias the owner.
						}
						releases = append(releases, ps6140ReleaseSite{context, instruction})
					}
					pending = append(pending, child)
				}
			}
		}
	}
	return releases, seen[owner.context] && *budget > 0
}

func ps6140SameOwnerPublications(context *ps6125SSAContext, owner ps6125SSAReference, ownerType *types.Named, budget *int) []*ssa.Return {
	if context == nil || context.flow == nil || budget == nil || *budget <= 0 {
		return nil
	}
	var publications []*ssa.Return
	for _, block := range context.flow.function.Blocks {
		if !context.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			*budget--
			if *budget <= 0 {
				return nil
			}
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			if len(returned.Results) == 0 || !types.Identical(returned.Results[0].Type(), types.NewPointer(ownerType)) {
				return nil
			}
			if constant, ok := returned.Results[0].(*ssa.Const); ok && constant.IsNil() {
				continue
			}
			if ps6136OwnerRoot(context, returned.Results[0], ownerType) != owner {
				return nil
			}
			publications = append(publications, returned)
		}
	}
	return publications
}

func ps6140PublicationExcludesRelease(context *ps6125SSAContext, returned *ssa.Return, released ps6140ReleaseSite, owner ps6125SSAReference, ownerType *types.Named, budget *int) bool {
	if context == nil || context.flow == nil || returned == nil || returned.Parent() != context.flow.function || released.context == nil || released.call == nil || budget == nil || *budget <= 0 {
		return false
	}
	*budget--
	if context == released.context {
		return !ps6136InstructionCanPrecede(context, released.call, returned)
	}
	child := released.context
	for child != nil && child.parent != context {
		*budget--
		if *budget <= 0 || child.site == nil || child.parent == nil || child.parent.calls[child.site] != child {
			return false
		}
		child = child.parent
	}
	if child == nil || child.site == nil || context.calls[child.site] != child {
		return false
	}
	if !ps6136InstructionCanPrecede(context, child.site, returned) {
		return true
	}
	// Only exact owner-result forwarding can distinguish the child's failed
	// returns from the selected owner's successful publication. Effects in a
	// different helper call conservatively remain possibly before publication.
	forward, ok := returned.Results[0].(*ssa.Call)
	if extract, extracted := returned.Results[0].(*ssa.Extract); extracted {
		if extract.Index != 0 {
			return false
		}
		forward, ok = extract.Tuple.(*ssa.Call)
	}
	if !ok || forward != child.site || !context.flow.instructionDominates(forward, returned) {
		return false
	}
	publications := ps6140SameOwnerPublications(child, owner, ownerType, budget)
	if len(publications) == 0 {
		return false
	}
	for _, publication := range publications {
		if !ps6140PublicationExcludesRelease(child, publication, released, owner, ownerType, budget) {
			return false
		}
	}
	return *budget > 0
}
