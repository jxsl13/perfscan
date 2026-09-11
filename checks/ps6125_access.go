package checks

import (
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/ssa"
)

// An access description records the exact SSA root, typed field declarations,
// and intervening load sites. It is not an invariant or a must-alias fact:
// repeated loads may observe different values, including across loop iterations.
// Allocation/lifetime reasoning must separately close those memory effects.
type ps6125Access struct {
	root   ssa.Value
	fields []*types.Var
	loads  []*ssa.UnOp
}

type ps6125AccessResult struct {
	access ps6125Access
	known  bool
}

type ps6125AccessPaths struct {
	flow   *ps6125SSAExtents
	cache  map[ssa.Value]ps6125AccessResult
	active map[ssa.Value]bool
}

// resolve follows only typed field/value operations and executable phi edges.
// Calls, indexes, conversions, missing flow and cyclic value origins remain
// unresolved rather than turning an arbitrary same-typed value into the owner.
// Memoization bounds shared-origin traversal by the source value graph.
func (paths *ps6125AccessPaths) resolve(value ssa.Value) ps6125AccessResult {
	if value == nil {
		return ps6125AccessResult{}
	}
	if paths.cache == nil {
		paths.cache = make(map[ssa.Value]ps6125AccessResult)
		paths.active = make(map[ssa.Value]bool)
	}
	if result, ok := paths.cache[value]; ok {
		return result
	}
	if paths.active[value] {
		return ps6125AccessResult{}
	}
	paths.active[value] = true
	result := paths.describe(value)
	delete(paths.active, value)
	paths.cache[value] = result
	return result
}

func (paths *ps6125AccessPaths) describe(value ssa.Value) ps6125AccessResult {
	switch instruction := value.(type) {
	case *ssa.Parameter, *ssa.FreeVar, *ssa.Alloc, *ssa.Global:
		return ps6125AccessResult{access: ps6125Access{root: value}, known: true}
	case *ssa.FieldAddr:
		pointer, ok := types.Unalias(instruction.X.Type()).Underlying().(*types.Pointer)
		if !ok {
			return ps6125AccessResult{}
		}
		return paths.field(instruction.X, pointer.Elem(), instruction.Field)
	case *ssa.Field:
		return paths.field(instruction.X, instruction.X.Type(), instruction.Field)
	case *ssa.UnOp:
		if instruction.Op != token.MUL {
			return ps6125AccessResult{}
		}
		result := paths.resolve(instruction.X)
		if result.known {
			result.access.loads = append(slices.Clone(result.access.loads), instruction)
		}
		return result
	case *ssa.Phi:
		if paths.flow == nil || instruction.Parent() != paths.flow.function || !paths.flow.blocks[instruction.Block()] {
			return ps6125AccessResult{}
		}
		var result ps6125AccessResult
		found := false
		for index, predecessor := range instruction.Block().Preds {
			if !paths.flow.edges[ps6125SSAEdge{from: predecessor, to: instruction.Block()}] {
				continue
			}
			incoming := paths.resolve(instruction.Edges[index])
			if !incoming.known || found && !ps6125SameAccess(result.access, incoming.access) {
				return ps6125AccessResult{}
			}
			result, found = incoming, true
		}
		return result
	}
	return ps6125AccessResult{}
}

func (paths *ps6125AccessPaths) field(base ssa.Value, typ types.Type, index int) ps6125AccessResult {
	structure, ok := types.Unalias(typ).Underlying().(*types.Struct)
	if !ok || index < 0 || index >= structure.NumFields() {
		return ps6125AccessResult{}
	}
	result := paths.resolve(base)
	if result.known {
		result.access.fields = append(slices.Clone(result.access.fields), structure.Field(index))
	}
	return result
}

// Equal descriptions must retain load sites: two source reads of d.slot.b are
// not interchangeable evidence merely because their typed field paths match.
func ps6125SameAccess(left, right ps6125Access) bool {
	return left.root == right.root && slices.Equal(left.fields, right.fields) && slices.Equal(left.loads, right.loads)
}
