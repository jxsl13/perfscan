package checks

import (
	"go/types"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/ssa"
)

// ps6136LeafExtent binds a configured leaf to actual instance/buffer/argument
// flow. Semantic flags, concrete implementation forwarding, projector-width
// provenance, and complete observation coverage must be checked independently.
// Capacity transfers deliberately do not return a one-row physical extent.
func ps6136LeafExtent(context *ps6125SSAContext, call *ssa.Call, leaf *config.OutputWorkspaceLeaf, facts *ps6136Extents, workspace, slot, projector, width *types.Var) (ps6125Extent, bool) {
	if context == nil || call == nil || facts == nil || facts.ownerType == nil || !leaf.SemanticsReviewed || call.Call.Signature().Variadic() || call.Call.Signature().TypeParams().Len() != 0 {
		return ps6125Extent{}, false
	}
	function := call.Call.Method
	arguments := call.Call.Args
	receiver := call.Call.Value
	if !call.Call.IsInvoke() {
		callee := call.Call.StaticCallee()
		if callee == nil {
			return ps6125Extent{}, false
		}
		function, _ = callee.Object().(*types.Func)
		if function == nil {
			return ps6125Extent{}, false
		}
		signature, _ := function.Type().(*types.Signature)
		if signature == nil || signature.Recv() == nil || len(arguments) == 0 {
			return ps6125Extent{}, false
		}
		receiver, arguments = arguments[0], arguments[1:]
	}
	if function == nil || ps6090FunctionID(function) != leaf.Method {
		return ps6125Extent{}, false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 || signature.Params().Len() != len(arguments) {
		return ps6125Extent{}, false
	}
	for _, role := range leaf.BufferArguments {
		value := receiver
		if role >= 0 {
			if role >= len(arguments) {
				return ps6125Extent{}, false
			}
			value = arguments[role]
		} else if role != -1 {
			return ps6125Extent{}, false
		}
		if !ps6136BufferOrigin(context, value, facts.owner, facts.ownerType, workspace, slot, 128) {
			return ps6125Extent{}, false
		}
	}
	integer := func(index int) ps6125Extent {
		if index < 0 || index >= len(arguments) || !types.Identical(signature.Params().At(index).Type(), types.Typ[types.Int]) {
			return ps6125Extent{}
		}
		return facts.extent(context, arguments[index])
	}
	switch leaf.Kind {
	case "projection":
		if projector == nil {
			return ps6125Extent{}, false
		}
		reference := context.reference(receiver)
		if reference.value == nil {
			return ps6125Extent{}, false
		}
		paths := ps6125AccessPaths{flow: reference.context.flow}
		path := paths.resolve(reference.value)
		if !path.known || len(path.access.fields) != 1 || path.access.fields[0] != projector || ps6136AccessOwnerRoot(reference.context, path, facts.ownerType) != facts.owner {
			return ps6125Extent{}, false
		}
		return ps6125MultiplyExtents(integer(leaf.RowsArgument), facts.immutable[width]), true
	case "row-width":
		return ps6125MultiplyExtents(integer(leaf.RowsArgument), integer(leaf.WidthArgument)), true
	case "prefix":
		return integer(leaf.WidthArgument), true
	case "download":
		index := leaf.HostArgument
		if index < 0 || index >= len(arguments) || !types.Identical(signature.Params().At(index).Type(), types.NewSlice(types.Typ[types.Float32])) {
			return ps6125Extent{}, false
		}
		return facts.length(context, arguments[index]), true
	case "capacity-transfer":
		return ps6125Extent{}, true // Physical capacity-wide, separately prove logical prefix.
	}
	return ps6125Extent{}, false
}
