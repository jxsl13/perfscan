package checks

import "go/types"

// A private constructor return is not the end of a public constructor. Recheck
// the selected owner's tracked flags through the complete actual caller chain:
// a wrapper may change a flag or publish a callback after the private return.
// Only a genuine public owner-returning root qualifies, never a synthetic call
// inserted into an inference method. Different final flags require another
// class proof rather than inheriting the private constructor's snapshot.
func ps6140PublicConstructorScope(constructor *ps6125SSAContext, flags *ps6140ImmutableFlags, owner *types.Named, budget int) *ps6125SSAContext {
	if constructor == nil || flags == nil || flags.snapshot == nil || flags.snapshot.constructor != constructor || owner == nil || budget <= 0 {
		return nil
	}
	root := constructor
	for root.parent != nil {
		budget--
		if budget <= 0 || root.site == nil || root.parent.calls[root.site] != root {
			return nil
		}
		root = root.parent
	}
	object, ok := root.flow.function.Object().(*types.Func)
	if !ok || !object.Exported() || root.flow.function.Signature.Results().Len() == 0 || !types.Identical(root.flow.function.Signature.Results().At(0).Type(), types.NewPointer(owner)) {
		return nil
	}
	fields := make(map[*types.Var]bool, len(flags.snapshot.values))
	for field := range flags.snapshot.values {
		fields[field] = true
	}
	final := ps6140ConstructorFlags(root, flags.snapshot.owner, owner, fields, budget)
	if final == nil || len(final.values) != len(flags.snapshot.values) {
		return nil
	}
	for field, value := range flags.snapshot.values {
		if final.values[field] != value {
			return nil
		}
	}
	return root
}
