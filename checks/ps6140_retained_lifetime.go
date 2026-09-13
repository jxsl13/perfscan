package checks

// Join source-level allocation/error/retention closure with source-level
// release ordering for this exact published owner. The existing constructor
// proof establishes cleanup on allocation-failure returns; the additional
// phase proof excludes cleanup before successful publication. Neither proves
// what the native allocator, recorder drain or element Release actually does.
func ps6140RetainedLifetimeSource(selection *ps6136Selection, retained *ps6140RetainedListProof, budget int) bool {
	if selection == nil || retained == nil || retained.public == nil || retained.release == nil || budget <= 0 {
		return false
	}
	public := retained.public
	if public.constructor != selection.context || public.owner != selection.owner || public.workspace != selection.workspace || public.allocation == nil || retained.retentions[public.allocation.factory] == nil {
		return false
	}
	return selection.constructorLifetime(public.allocation, budget) &&
		ps6140PublicFailureCleanup(public.publication, public.constructor, public.owner, selection.ownerType, retained.release, budget) &&
		ps6140ConstructorReleasePhase(public.publication, public.owner, selection.ownerType, retained.release, budget)
}
