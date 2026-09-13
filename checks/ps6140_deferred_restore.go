package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Prove absence of scratch observations in a straight-line deferred scalar/
// callable-field restoration. Every instruction is inspected; no call, branch,
// buffer/interface/slice value, owner copy or callback execution is allowed.
// This does NOT approve the restored fields' effects for native/flag analysis.
func ps6140DeferredFieldRestoration(context *ps6125SSAContext, deferred *ssa.Defer, owner *types.Named, workspace *types.Var) bool {
	if context == nil || deferred == nil || owner == nil || workspace == nil || deferred.Parent() != context.flow.function || len(deferred.Call.Args) != 0 {
		return false
	}
	origin := ps6136ReturnedCallable(context.reference(deferred.Call.Value), make(map[ps6125SSAReference]bool))
	function, captures := ps6125SSACallee(origin.value)
	if function == nil || origin.context == nil || function.Pkg != context.flow.function.Pkg || len(function.Blocks) != 1 || len(function.Params) != 0 || function.Signature.Results().Len() != 0 {
		return false
	}
	ownerPointer := types.NewPointer(owner)
	ownerCell := types.NewPointer(ownerPointer)
	allowed := make(map[ssa.Value]bool, len(function.FreeVars))
	ownerLoads := make(map[ssa.Value]bool)
	fieldPaths := make(map[ssa.Value]bool)
	scalarLoads := make(map[ssa.Value]bool)
	remaining := 4096
	for index, free := range function.FreeVars {
		if !types.Identical(free.Type(), captures[index].Type()) {
			return false
		}
		if !types.Identical(free.Type(), ownerCell) {
			pointer, ok := free.Type().Underlying().(*types.Pointer)
			if !ok || !ps6140RestoreScalar(pointer.Elem()) || ps6140ContainsOwner(pointer.Elem(), owner, make(map[types.Type]bool), &remaining) {
				return false
			}
		}
		allowed[free] = true
	}
	instructions := function.Blocks[0].Instrs
	if len(instructions) == 0 {
		return false
	}
	for index, instruction := range instructions {
		remaining--
		if remaining <= 0 {
			return false
		}
		for _, operand := range instruction.Operands(nil) {
			if operand == nil || *operand == nil || !allowed[*operand] {
				return false
			}
		}
		switch instruction := instruction.(type) {
		case *ssa.DebugRef:
		case *ssa.UnOp:
			if instruction.Op != token.MUL {
				return false
			}
			free, ok := instruction.X.(*ssa.FreeVar)
			if !ok {
				return false // No field/pointer chasing or whole-owner load.
			}
			if types.Identical(free.Type(), ownerCell) {
				ownerLoads[instruction] = true
			} else if ps6140RestoreScalar(instruction.Type()) {
				scalarLoads[instruction] = true
			} else {
				return false
			}
			allowed[instruction] = true
		case *ssa.FieldAddr:
			if !ownerLoads[instruction.X] && !fieldPaths[instruction.X] {
				return false
			}
			pointer, ok := instruction.X.Type().Underlying().(*types.Pointer)
			if !ok {
				return false
			}
			structure, ok := pointer.Elem().Underlying().(*types.Struct)
			if !ok || instruction.Field < 0 || instruction.Field >= structure.NumFields() {
				return false
			}
			field := structure.Field(instruction.Field)
			if field == workspace || ps6140ContainsOwner(field.Type(), owner, make(map[types.Type]bool), &remaining) {
				return false
			}
			fieldPaths[instruction], allowed[instruction] = true, true
		case *ssa.Store:
			if !fieldPaths[instruction.Addr] || !scalarLoads[instruction.Val] || !ps6140RestoreScalar(instruction.Val.Type()) {
				return false
			}
		case *ssa.Return:
			if len(instruction.Results) != 0 || index != len(instructions)-1 {
				return false
			}
		default:
			return false
		}
	}
	_, returns := instructions[len(instructions)-1].(*ssa.Return)
	return returns
}

func ps6140RestoreScalar(typ types.Type) bool {
	switch typ := typ.Underlying().(type) {
	case *types.Basic:
		return typ.Info()&(types.IsBoolean|types.IsNumeric|types.IsString) != 0
	case *types.Signature:
		return true
	}
	return false
}
