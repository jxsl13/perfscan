package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Parameter.Object wraps its source *types.Var in an interface, including a
// typed nil for zero/incomplete parameters. Extent roots require a real source var.
func ps6136ParameterObject(parameter *ssa.Parameter) *types.Var {
	if parameter == nil {
		return nil
	}
	object, _ := parameter.Object().(*types.Var)
	return object
}
