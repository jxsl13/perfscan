package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Include methods on every local named type, not only the selected owner:
// wrapper/helper methods may introduce observations of that owner's fields.
func ps6136SourceFunctions(pkg *ssa.Package) []*ssa.Function {
	if pkg == nil {
		return nil
	}
	seen := make(map[*ssa.Function]bool)
	var result []*ssa.Function
	var add func(*ssa.Function)
	add = func(function *ssa.Function) {
		if function == nil || seen[function] {
			return
		}
		seen[function] = true
		result = append(result, function)
		for _, child := range function.AnonFuncs {
			add(child)
		}
	}
	for _, member := range pkg.Members {
		switch member := member.(type) {
		case *ssa.Function:
			add(member)
		case *ssa.Type:
			named, ok := types.Unalias(member.Type()).(*types.Named)
			if !ok || named.Obj().Pkg() != pkg.Pkg {
				continue
			}
			for index := 0; index < named.NumMethods(); index++ {
				add(pkg.Prog.FuncValue(named.Method(index)))
			}
		}
	}
	return result
}
