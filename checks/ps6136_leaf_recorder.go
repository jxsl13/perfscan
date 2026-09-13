package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// leafRecorderOrigin joins the actual output call's recorder to this same
// published owner's source factory field. NativeBindings independently closes
// both fields and their only permitted profile override/native implementations.
// A matching interface method/signature alone never supplies this provenance.
func (selection *ps6136Selection) leafRecorderOrigin(context *ps6125SSAContext, call *ssa.Call, kind string, owner ps6125SSAReference) bool {
	if selection == nil || context == nil || call == nil || selection.ops == nil || selection.primaryRecorder == nil {
		return false
	}
	receiver := call.Call.Value
	arguments := call.Call.Args
	if !call.Call.IsInvoke() {
		if len(arguments) == 0 {
			return false
		}
		receiver, arguments = arguments[0], arguments[1:]
	}
	if kind == "projection" {
		// The separately proved F32 lowering binds source parameter0 to the
		// configured recorder projection receiver, not another same-typed field.
		if len(arguments) == 0 {
			return false
		}
		receiver = arguments[0]
	}
	reference := context.reference(receiver)
	extraction, ok := reference.value.(*ssa.Extract)
	if !ok || extraction.Index != 0 {
		return false
	}
	factory, ok := extraction.Tuple.(*ssa.Call)
	if !ok || factory.Call.IsInvoke() {
		return false
	}
	signature := factory.Call.Signature()
	if signature.Variadic() || signature.TypeParams().Len() != 0 || signature.Params().Len() != 0 || len(factory.Call.Args) != 0 || signature.Results().Len() != 2 || !types.Identical(signature.Results().At(0).Type(), receiver.Type()) || !types.Identical(signature.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return false
	}
	remaining := 128
	// Keep the actual read/phi: callable normalization can replace the profile
	// field's source read with its known closure, losing the owner-field join.
	factoryReference := ps6125SSAReference{context: reference.context, value: factory.Call.Value}
	return selection.recorderFactoryOrigin(factoryReference, owner, make(map[ps6125SSAReference]bool), &remaining)
}

func (selection *ps6136Selection) recorderFactoryOrigin(reference ps6125SSAReference, owner ps6125SSAReference, active map[ps6125SSAReference]bool, remaining *int) bool {
	if reference.value == nil || reference.context == nil || *remaining <= 0 || active[reference] {
		return false
	}
	*remaining--
	active[reference] = true
	defer delete(active, reference)
	paths := ps6125AccessPaths{flow: reference.context.flow}
	path := paths.resolve(reference.value)
	if path.known && len(path.access.fields) == 2 && path.access.fields[0] == selection.ops && (path.access.fields[1] == selection.primaryRecorder || selection.secondaryRecorder != nil && path.access.fields[1] == selection.secondaryRecorder) {
		return ps6136OwnerRoot(reference.context, path.access.root, selection.ownerType) == owner
	}
	phi, ok := reference.value.(*ssa.Phi)
	if !ok || len(phi.Edges) != len(phi.Block().Preds) {
		return false
	}
	seen := false
	for index, edge := range phi.Edges {
		if !reference.context.flow.blocks[phi.Block().Preds[index]] {
			continue
		}
		if !selection.recorderFactoryOrigin(ps6125SSAReference{context: reference.context, value: edge}, owner, active, remaining) {
			return false
		}
		seen = true
	}
	return seen
}
