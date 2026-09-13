package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Only the nil interface itself qualifies; an interface boxing a typed nil is
// non-nil. Missing projections are unknown until a fresh closed struct cell's
// zero initialization and exact read ordering have been established.
func ps6140BlockFieldNil(reference ps6125SSAReference, field *types.Var, budget int) bool {
	if reference.context == nil || reference.value == nil || field == nil || budget <= 0 {
		return false
	}
	if _, iface := field.Type().Underlying().(*types.Interface); !iface {
		return false
	}
	normalized := reference.context.reference(reference.value)
	if normalized.value == nil {
		return false
	}
	if normalized != reference {
		return ps6140BlockFieldNil(normalized, field, budget-1)
	}
	structure, ok := reference.value.Type().Underlying().(*types.Struct)
	if !ok {
		return false
	}
	index := -1
	for i := 0; i < structure.NumFields(); i++ {
		if structure.Field(i) == field {
			index = i
		}
	}
	if index < 0 {
		return false
	}
	if literal, ok := reference.value.(*ssa.Const); ok && literal.Value == nil {
		return true // The exact source value struct's zero value.
	}
	load, ok := reference.value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	cell := reference.context.reference(load.X)
	allocation, ok := cell.value.(*ssa.Alloc)
	if !ok || cell.context == nil {
		return false
	}
	stores := cell.context.structCell(allocation)
	if stores == nil || index >= len(stores.fields) {
		return false
	}
	var point ssa.Instruction = load
	current := reference.context
	for current != cell.context && current.parent != nil {
		point = current.site
		current = current.parent
	}
	if current != cell.context || !cell.context.flow.instructionDominates(allocation, point) {
		return false
	}
	if stores.whole != nil {
		return cell.context.flow.instructionDominates(stores.whole, point) && ps6140BlockFieldNil(cell.context.reference(stores.whole.Val), field, budget-1)
	}
	store := stores.fields[index]
	if store == nil {
		return true
	}
	literal, ok := store.Val.(*ssa.Const)
	return ok && literal.IsNil() && types.Identical(literal.Type(), field.Type()) && cell.context.flow.instructionDominates(store, point)
}
