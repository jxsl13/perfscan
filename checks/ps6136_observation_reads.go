package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6136WorkspaceReadClosed closes the complete SSA use graph of a workspace
// slot load against independently validated leaf calls. It grants no leaf
// semantics itself. Storing a slot/buffer alias, returning it, observing other
// slot members or passing it to any unproved call rejects the read.
func ps6136WorkspaceReadClosed(value ssa.Value, slot *types.Var, proved map[*ssa.Call]bool, budget int) bool {
	if value == nil || slot == nil || budget <= 0 {
		return false
	}
	seen := make(map[ssa.Value]bool)
	var visit func(ssa.Value, bool) bool
	visit = func(current ssa.Value, buffer bool) bool {
		if seen[current] {
			return true
		}
		seen[current] = true
		budget--
		if budget < 0 || current.Referrers() == nil {
			return false
		}
		used := false
		for _, referrer := range *current.Referrers() {
			switch use := referrer.(type) {
			case *ssa.DebugRef:
				continue
			case *ssa.FieldAddr:
				if buffer || use.X != current {
					return false
				}
				pointer, ok := current.Type().Underlying().(*types.Pointer)
				if !ok {
					return false
				}
				structure, ok := pointer.Elem().Underlying().(*types.Struct)
				if !ok || use.Field >= structure.NumFields() || structure.Field(use.Field) != slot {
					return false
				}
				if use.Referrers() == nil || len(*use.Referrers()) == 0 {
					return false
				}
				for _, reference := range *use.Referrers() {
					if _, debug := reference.(*ssa.DebugRef); debug {
						continue
					}
					load, ok := reference.(*ssa.UnOp)
					if !ok || load.Op != token.MUL || load.X != use || !visit(load, true) {
						return false
					}
				}
			case *ssa.TypeAssert:
				if !buffer || use.X != current {
					return false
				}
				if use.CommaOk {
					if use.Referrers() == nil {
						return false
					}
					for _, reference := range *use.Referrers() {
						if _, debug := reference.(*ssa.DebugRef); debug {
							continue
						}
						extract, ok := reference.(*ssa.Extract)
						if !ok || extract.Tuple != use || extract.Index > 1 {
							return false
						}
						if extract.Index == 0 && !visit(extract, true) {
							return false
						}
						// The assertion's boolean is a capability predicate, not
						// an alias of its buffer. It cannot mutate buffer storage.
					}
				} else if !visit(use, true) {
					return false
				}
			case *ssa.ChangeInterface:
				if !buffer || use.X != current || !visit(use, true) {
					return false
				}
			case *ssa.Call:
				if !buffer || !proved[use] {
					return false
				}
			default:
				return false
			}
			used = true
		}
		return used
	}
	return visit(value, false)
}
