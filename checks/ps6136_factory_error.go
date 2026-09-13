package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6136FactoryErrorCell proves that the exact allocator error is propagated
// into one captured constructor error cell before its failed slot return. It
// does not establish the constructor's publication/cleanup barrier or native
// failure ownership. Those are separate mandatory lifetime gates.
func ps6136FactoryErrorCell(context *ps6125SSAContext, backend *ssa.Call) ps6125SSAReference {
	if context == nil || backend == nil || backend.Parent() != context.flow.function {
		return ps6125SSAReference{}
	}
	var write *ssa.Store
	var failed *ssa.Return
	for _, block := range context.flow.function.Blocks {
		if !context.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			if store, ok := instruction.(*ssa.Store); ok {
				extracted, ok := store.Val.(*ssa.Extract)
				if !ok || extracted.Tuple != backend || extracted.Index != 1 {
					continue
				}
				if write != nil || !types.Identical(store.Addr.Type(), types.NewPointer(types.Universe.Lookup("error").Type())) {
					return ps6125SSAReference{}
				}
				write = store
			}
			if ret, ok := instruction.(*ssa.Return); ok && ps6136NativeErrorBranch(context, ret, backend, true) {
				if failed != nil {
					return ps6125SSAReference{}
				}
				failed = ret
			}
		}
	}
	if write == nil || failed == nil || !context.flow.instructionDominates(write, failed) {
		return ps6125SSAReference{}
	}
	cell := ps6136ErrorCellRoot(context, write.Addr)
	allocation, ok := cell.value.(*ssa.Alloc)
	if !ok || allocation.Parent() != cell.context.flow.function || cell.context == context {
		return ps6125SSAReference{}
	}
	return cell
}

// Resolve only immutable pointer-to-error captures, not mutable error values.
func ps6136ErrorCellRoot(context *ps6125SSAContext, value ssa.Value) ps6125SSAReference {
	if context == nil || value == nil || !types.Identical(value.Type(), types.NewPointer(types.Universe.Lookup("error").Type())) {
		return ps6125SSAReference{}
	}
	reference := context.reference(value)
	seen := make(map[ps6125SSAReference]bool)
	for reference.value != nil && !seen[reference] {
		seen[reference] = true
		if _, ok := reference.value.(*ssa.Alloc); ok {
			return reference
		}
		load, ok := reference.value.(*ssa.UnOp)
		if !ok || load.Op != token.MUL {
			break
		}
		cell := reference.context.reference(load.X)
		allocation, ok := cell.value.(*ssa.Alloc)
		if !ok || !types.Identical(allocation.Type(), types.NewPointer(value.Type())) {
			break
		}
		store := ps6136ClosedOwnerCell(cell.context, allocation)
		if store == nil {
			break
		}
		var point ssa.Instruction = load
		current := reference.context
		for current != cell.context && current.parent != nil {
			point = current.site
			current = current.parent
		}
		if current != cell.context {
			point = ps6136ReturnedCapturePoint(reference.context, cell)
		}
		if point == nil || !cell.context.flow.instructionDominates(store, point) {
			break
		}
		reference = cell.context.reference(store.Val)
	}
	return ps6125SSAReference{}
}
