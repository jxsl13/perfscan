package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Join the allocator's proved exact native result to the concrete wrapper
// unwrapped by the configured bridge. Equal native-field types in different
// wrapper types cannot satisfy this identity check.
func ps6136BoundBufferWrapper(context *ps6125SSAContext, backend *ssa.Call, wrapper *types.Named, field *types.Var) bool {
	if context == nil || backend == nil || wrapper == nil || field == nil {
		return false
	}
	structure, ok := wrapper.Underlying().(*types.Struct)
	if !ok || structure.NumFields() != 1 || structure.Field(0) != field {
		return false
	}
	callback := context.call(backend)
	if callback == nil {
		return false
	}
	populated := 0
	for _, block := range callback.flow.function.Blocks {
		if !callback.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			if len(returned.Results) != 2 {
				return false
			}
			if constant, ok := returned.Results[0].(*ssa.Const); ok && constant.IsNil() {
				continue
			}
			conversion, ok := returned.Results[0].(*ssa.MakeInterface)
			if !ok || !types.Identical(conversion.X.Type(), wrapper) {
				return false
			}
			reference := callback.structField(conversion.X, 0)
			result, ok := reference.value.(*ssa.Extract)
			if !ok || reference.context != callback || result.Index != 0 {
				return false
			}
			native, ok := result.Tuple.(*ssa.Call)
			if !ok || native.Call.StaticCallee() == nil || native.Call.Signature().Results().Len() != 2 || !types.Identical(native.Call.Signature().Results().At(0).Type(), field.Type()) {
				return false
			}
			populated++
		}
	}
	return populated == 1
}

// ps6136BoundAllocator requires the actual closed backend callback to call an
// exact reviewed allocator with the same host descriptor and return that exact
// result, directly or inside a one-field value wrapper. An allocator name or
// a constructor's opaque backendOps formal does not establish this binding.
func ps6136BoundAllocator(context *ps6125SSAContext, backend *ssa.Call, input ps6125SSAReference, identities map[string]bool) bool {
	if context == nil || backend == nil || input.value == nil {
		return false
	}
	callback := context.call(backend)
	if callback == nil {
		return false
	}
	var native *ssa.Call
	for _, block := range callback.flow.function.Blocks {
		if !callback.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok {
				continue
			}
			callee := call.Call.StaticCallee()
			if callee == nil || callee.Object() == nil || !identities[ps6090FunctionID(callee.Object().(*types.Func))] {
				continue
			}
			if native != nil || call.Call.IsInvoke() || len(call.Call.Args) != 1 || callback.reference(call.Call.Args[0]) != input {
				return false
			}
			signature := callee.Signature
			if signature.Recv() != nil || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.Params().Len() != 1 || signature.Results().Len() != 2 ||
				!types.Identical(signature.Params().At(0).Type(), types.NewSlice(types.Typ[types.Float32])) ||
				!types.Identical(signature.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
				return false
			}
			native = call
		}
	}
	if native == nil {
		return false
	}
	populated, empty := 0, 0
	var nativeResult *ssa.Extract
	var wrapper ps6125SSAReference
	for _, block := range callback.flow.function.Blocks {
		if !callback.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			if len(returned.Results) != 2 {
				return false
			}
			if literal, ok := returned.Results[0].(*ssa.Const); ok && literal.IsNil() {
				errorValue, ok := callback.reference(returned.Results[1]).value.(*ssa.Extract)
				if !ok || errorValue.Tuple != native || errorValue.Index != 1 {
					return false
				}
				if !ps6136NativeErrorBranch(callback, returned, native, true) {
					return false
				}
				empty++
				continue
			}
			if errorValue, ok := returned.Results[1].(*ssa.Const); !ok || !errorValue.IsNil() {
				return false
			}
			if !ps6136NativeErrorBranch(callback, returned, native, false) {
				return false
			}
			value := returned.Results[0]
			if converted, ok := value.(*ssa.MakeInterface); ok {
				value = converted.X
			}
			reference := callback.reference(value)
			if structure, ok := value.Type().Underlying().(*types.Struct); ok {
				if structure.NumFields() != 1 {
					return false
				}
				reference = callback.structField(value, 0)
				wrapper = callback.reference(value)
			}
			extracted, ok := reference.value.(*ssa.Extract)
			if !ok || reference.context != callback || extracted.Tuple != native || extracted.Index != 0 {
				return false
			}
			nativeResult = extracted
			populated++
		}
	}
	if populated != 1 || empty != 1 || nativeResult == nil {
		return false
	}
	users := nativeResult.Referrers()
	if users == nil {
		return false
	}
	for _, user := range *users {
		if user.Parent() == callback.flow.function && !callback.flow.blocks[user.Block()] {
			continue
		}
		switch use := user.(type) {
		case *ssa.DebugRef:
		case *ssa.MakeInterface:
			if wrapper.value != nil {
				return false
			}
			convertedUsers := use.Referrers()
			if convertedUsers == nil {
				return false
			}
			for _, convertedUser := range *convertedUsers {
				if _, debug := convertedUser.(*ssa.DebugRef); debug {
					continue
				}
				returned, ok := convertedUser.(*ssa.Return)
				if !ok || returned.Parent() != callback.flow.function || len(returned.Results) != 2 || returned.Results[0] != use {
					return false
				}
			}
		case *ssa.Return:
			if wrapper.value != nil {
				return false
			}
		case *ssa.Store:
			address, ok := use.Addr.(*ssa.FieldAddr)
			if !ok || wrapper.value == nil {
				return false
			}
			paths := ps6125AccessPaths{flow: callback.flow}
			path := paths.resolve(address)
			if !path.known || len(path.access.fields) != 1 {
				return false
			}
			// structField already closes the fresh wrapper cell and dominates
			// the actual read; unrelated stores may not masquerade as its init.
			load, ok := wrapper.value.(*ssa.UnOp)
			if !ok || load.X != address.X {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func ps6136NativeErrorBranch(context *ps6125SSAContext, returned *ssa.Return, native *ssa.Call, wantError bool) bool {
	block := returned.Block()
	if len(block.Preds) != 1 {
		return false
	}
	predecessor := block.Preds[0]
	if len(predecessor.Instrs) == 0 {
		return false
	}
	branch, ok := predecessor.Instrs[len(predecessor.Instrs)-1].(*ssa.If)
	if !ok {
		return false
	}
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok || (comparison.Op != token.EQL && comparison.Op != token.NEQ) {
		return false
	}
	value, nilValue := comparison.X, comparison.Y
	if literal, ok := value.(*ssa.Const); ok && literal.IsNil() {
		value, nilValue = nilValue, value
	}
	literal, ok := nilValue.(*ssa.Const)
	if !ok || !literal.IsNil() {
		return false
	}
	extracted, ok := context.reference(value).value.(*ssa.Extract)
	if !ok || extracted.Tuple != native || extracted.Index != 1 {
		return false
	}
	trueBranch := comparison.Op == token.NEQ
	if !wantError {
		trueBranch = !trueBranch
	}
	index := 1
	if trueBranch {
		index = 0
	}
	return len(predecessor.Succs) == 2 && predecessor.Succs[index] == block
}
