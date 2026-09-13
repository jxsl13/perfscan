package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6136StructFieldZero proves a nil function or false bool in a closed source
// value struct, including an omitted literal field. It never treats an unknown
// heap field, unavailable struct projection or opaque helper as zero. Subsequent
// owner-memory invariance remains an independent full-observation obligation.
func ps6136StructFieldZero(context *ps6125SSAContext, value ssa.Value, field int, budget int) bool {
	if context == nil || value == nil || budget <= 0 {
		return false
	}
	reference := context.reference(value)
	if reference.value == nil {
		return false
	}
	if reference.context != context || reference.value != value {
		return ps6136StructFieldZero(reference.context, reference.value, field, budget-1)
	}
	structure, ok := value.Type().Underlying().(*types.Struct)
	if !ok || field < 0 || field >= structure.NumFields() {
		return false
	}
	kind := structure.Field(field).Type().Underlying()
	_, function := kind.(*types.Signature)
	boolean := types.Identical(structure.Field(field).Type(), types.Typ[types.Bool])
	if !function && !boolean {
		return false
	}
	// SSA Const's nil Value represents the build-time zero of any type,
	// including an entire source value struct (not an unknown runtime load).
	if literal, ok := value.(*ssa.Const); ok && literal.Value == nil {
		return true
	}
	if projected := context.structField(value, field); projected.value != nil {
		literal, ok := projected.value.(*ssa.Const)
		if !ok {
			return false
		}
		if function {
			return literal.IsNil()
		}
		fact := projected.context.scalar(literal)
		return fact.state == ps6125Boolean && !fact.truth
	}
	load, ok := value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	return ps6136StructFieldZeroAt(context, load.X, field, load, budget-1)
}

func ps6136StructFieldZeroAt(context *ps6125SSAContext, address ssa.Value, field int, point ssa.Instruction, budget int) bool {
	if context == nil || address == nil || point == nil || budget <= 0 {
		return false
	}
	cell := context.reference(address)
	allocation, ok := cell.value.(*ssa.Alloc)
	if !ok {
		return false
	}
	pointer, ok := allocation.Type().Underlying().(*types.Pointer)
	if !ok {
		return false
	}
	structure, ok := pointer.Elem().Underlying().(*types.Struct)
	if !ok || field < 0 || field >= structure.NumFields() {
		return false
	}
	_, function := structure.Field(field).Type().Underlying().(*types.Signature)
	if !function && !types.Identical(structure.Field(field).Type(), types.Typ[types.Bool]) {
		return false
	}
	stores := cell.context.structCell(allocation)
	if stores == nil || field < 0 || field >= len(stores.fields) {
		return false
	}
	// Use the actual read/call edge, not lexical closure creation. A load of
	// another invocation's cell cannot inherit zero facts from its same type.
	current := context
	for current != cell.context && current.parent != nil {
		point = current.site
		current = current.parent
	}
	if current != cell.context || !cell.context.flow.instructionDominates(allocation, point) {
		return false
	}
	if stores.whole != nil {
		return cell.context.flow.instructionDominates(stores.whole, point) && ps6136StructFieldZero(cell.context, stores.whole.Val, field, budget-1)
	}
	if store := stores.fields[field]; store != nil {
		literal, ok := store.Val.(*ssa.Const)
		if !ok || !cell.context.flow.instructionDominates(store, point) {
			return false
		}
		if _, function := literal.Type().Underlying().(*types.Signature); function {
			return literal.IsNil()
		}
		fact := cell.context.scalar(literal)
		return types.Identical(literal.Type(), types.Typ[types.Bool]) && fact.state == ps6125Boolean && !fact.truth
	}
	return true
}
