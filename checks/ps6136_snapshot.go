package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6136InitialOwnerField returns the one source initializer of a fresh owner
// field. This alone is NOT immutable memory proof: the selected constructor's
// full field-use/owner-alias inventory must independently establish that no
// escaped/captured/other-method write can change this field before its consumer.
func ps6136InitialOwnerField(owner ps6125SSAReference, field *types.Var) ps6125SSAReference {
	allocation, ok := owner.value.(*ssa.Alloc)
	if !ok || owner.context == nil || field == nil {
		return ps6125SSAReference{}
	}
	var source *ssa.Store
	for _, block := range owner.context.flow.function.Blocks {
		if !owner.context.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			address, ok := instruction.(*ssa.FieldAddr)
			if !ok || address.X != allocation {
				continue
			}
			pointer, ok := allocation.Type().Underlying().(*types.Pointer)
			if !ok {
				return ps6125SSAReference{}
			}
			structure, ok := pointer.Elem().Underlying().(*types.Struct)
			if !ok || structure.Field(address.Field) != field {
				continue
			}
			users := address.Referrers()
			if users == nil {
				return ps6125SSAReference{}
			}
			for _, user := range *users {
				if user.Parent() == owner.context.flow.function && !owner.context.flow.blocks[user.Block()] {
					continue
				}
				switch use := user.(type) {
				case *ssa.DebugRef:
				case *ssa.UnOp:
					if use.Op != token.MUL || use.X != address {
						return ps6125SSAReference{}
					}
				case *ssa.Store:
					if use.Addr != address || source != nil {
						return ps6125SSAReference{}
					}
					source = use
				default:
					return ps6125SSAReference{}
				}
			}
		}
	}
	if source == nil {
		return ps6125SSAReference{}
	}
	return owner.context.reference(source.Val)
}
