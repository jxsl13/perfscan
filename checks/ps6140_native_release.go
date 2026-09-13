package checks

import "go/types"

// Bind the retained interface's Release dispatch to the exact native pointer
// returned by the selected allocator. Only direct method promotion through the
// proved single embedded wrapper field qualifies. An explicit wrapper method,
// even one with the same name, needs a separate body proof. Native deallocation,
// allocation uniqueness and synchronization semantics remain reviewed facts.
func ps6140NativeReleaseBinding(storage *ps6140FactoryStorageProof, retainedRelease *types.Func, nativeReleaseID string, budget int) *types.Func {
	if storage == nil || storage.wrapper == nil || storage.nativeField == nil || storage.native == nil || retainedRelease == nil || nativeReleaseID == "" || budget <= 0 {
		return nil
	}
	abstract, ok := retainedRelease.Type().(*types.Signature)
	if !ok || abstract.Recv() == nil || abstract.Variadic() || abstract.Params().Len() != 0 || abstract.Results().Len() != 0 {
		return nil
	}
	capability, ok := abstract.Recv().Type().Underlying().(*types.Interface)
	if !ok || !types.Implements(storage.wrapper, capability) {
		return nil
	}
	structure, ok := storage.wrapper.Underlying().(*types.Struct)
	if !ok || structure.NumFields() != 1 || structure.Field(0) != storage.nativeField || !storage.nativeField.Embedded() {
		return nil
	}
	pointer, ok := storage.nativeField.Type().(*types.Pointer)
	if !ok {
		return nil
	}
	if _, named := pointer.Elem().(*types.Named); !named {
		return nil
	}
	allocator := storage.native.Call.Signature()
	if allocator == nil || allocator.Results().Len() != 2 || !types.Identical(allocator.Results().At(0).Type(), pointer) || !types.Identical(allocator.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return nil
	}
	method := types.NewMethodSet(storage.wrapper).Lookup(retainedRelease.Pkg(), retainedRelease.Name())
	if method == nil || len(method.Index()) != 2 || method.Index()[0] != 0 {
		return nil
	}
	native, ok := method.Obj().(*types.Func)
	if !ok || ps6090FunctionID(native) != nativeReleaseID {
		return nil
	}
	signature, ok := native.Type().(*types.Signature)
	if !ok || signature.Recv() == nil || !types.Identical(signature.Recv().Type(), pointer) || signature.Variadic() || signature.Params().Len() != 0 || signature.Results().Len() != 0 {
		return nil
	}
	return native
}
