package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6136ConstructorInventory is unconditional over source-visible owner-returning
// functions. Every non-nil result must originate in a fresh, source-visible
// allocation; aliases, global/opaque returns or ambiguous instances reject the
// entire inventory. This does not establish selected-model width, allocation
// cleanup, provider identity or safe subsequent field writes.
func ps6136ConstructorInventory(pkg *ssa.Package, owner *types.Named) map[*ssa.Function]ps6125SSAReference {
	if pkg == nil || owner == nil {
		return nil
	}
	functions := ps6136SourceFunctions(pkg)
	result := make(map[*ssa.Function]ps6125SSAReference, len(functions))
	for _, function := range functions {
		for index := 1; index < function.Signature.Results().Len(); index++ {
			if types.Identical(function.Signature.Results().At(index).Type(), types.NewPointer(owner)) {
				return nil // Unsupported alternate owner-result roles are not omitted.
			}
		}
		if function.Signature.Results().Len() == 0 || !types.Identical(function.Signature.Results().At(0).Type(), types.NewPointer(owner)) {
			continue
		}
		if len(function.Blocks) == 0 || function.Signature.Variadic() || function.Signature.TypeParams().Len() != 0 {
			return nil
		}
		context := ps6125NewSSAContext(function, nil, nil, 512)
		var identity ps6125SSAReference
		for _, block := range function.Blocks {
			if !context.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				returned, ok := instruction.(*ssa.Return)
				if !ok || len(returned.Results) == 0 {
					continue
				}
				value := returned.Results[0]
				if constant, ok := value.(*ssa.Const); ok && constant.IsNil() {
					continue
				}
				root := ps6136OwnerRoot(context, value, owner)
				if _, fresh := root.value.(*ssa.Alloc); !fresh || identity.value != nil && identity != root {
					return nil
				}
				identity = root
			}
		}
		if identity.value == nil {
			return nil
		}
		result[function] = identity
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
