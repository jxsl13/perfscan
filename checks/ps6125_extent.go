package checks

import (
	"go/types"
	"math"
	"slices"
)

// Extent identities use typed objects and the complete receiver/field path.
// Equal field spellings, sibling receiver instances, and nested fields of the
// same struct type must not become interchangeable geometry evidence.
type ps6125ExtentIdentity struct {
	index  int
	pool   *ps6125ExtentPool
	parent *ps6125ExtentIdentity
	object types.Object
	length bool
}

type ps6125ExtentKey struct {
	parent *ps6125ExtentIdentity
	object types.Object
	length bool
}

type ps6125ExtentPool struct {
	identities map[ps6125ExtentKey]*ps6125ExtentIdentity
}

func (pool *ps6125ExtentPool) identity(root types.Object, fields []*types.Var, length bool) *ps6125ExtentIdentity {
	if root == nil {
		return nil
	}
	if pool.identities == nil {
		pool.identities = make(map[ps6125ExtentKey]*ps6125ExtentIdentity)
	}
	intern := func(parent *ps6125ExtentIdentity, object types.Object, length bool) *ps6125ExtentIdentity {
		key := ps6125ExtentKey{parent: parent, object: object, length: length}
		if existing := pool.identities[key]; existing != nil {
			return existing
		}
		identity := &ps6125ExtentIdentity{index: len(pool.identities), pool: pool, parent: parent, object: object, length: length}
		pool.identities[key] = identity
		return identity
	}
	identity := intern(nil, root, false)
	for _, field := range fields {
		if field == nil {
			return nil
		}
		identity = intern(identity, field, false)
	}
	if length {
		identity = intern(identity, nil, true)
	}
	return identity
}

type ps6125ExtentPower struct {
	identity *ps6125ExtentIdentity
	power    uint32
}

// A known extent is a nonnegative integer coefficient times typed symbolic
// dimensions. This is geometry algebra, not an execution or no-overflow proof.
// Reporting numeric bytes requires evaluation against the target integer limit.
// Unsupported arithmetic is unknown; it must not become a zero-row proof.
type ps6125Extent struct {
	known       bool
	coefficient int64
	powers      []ps6125ExtentPower
}

func ps6125ConstantExtent(value int64) ps6125Extent {
	if value < 0 {
		return ps6125Extent{}
	}
	return ps6125Extent{known: true, coefficient: value}
}

func ps6125SymbolicExtent(identity *ps6125ExtentIdentity) ps6125Extent {
	if identity == nil {
		return ps6125Extent{}
	}
	return ps6125Extent{known: true, coefficient: 1, powers: []ps6125ExtentPower{{identity: identity, power: 1}}}
}

func ps6125MultiplyExtents(left, right ps6125Extent) ps6125Extent {
	if len(left.powers) != 0 && len(right.powers) != 0 && left.powers[0].identity.pool != right.powers[0].identity.pool {
		return ps6125Extent{}
	}
	if !left.known || !right.known || left.coefficient != 0 && right.coefficient > math.MaxInt64/left.coefficient {
		return ps6125Extent{}
	}
	coefficient := left.coefficient * right.coefficient
	if coefficient == 0 {
		return ps6125ConstantExtent(0)
	}
	powers := make([]ps6125ExtentPower, 0, len(left.powers)+len(right.powers))
	powers = append(powers, left.powers...)
	powers = append(powers, right.powers...)
	slices.SortFunc(powers, func(a, b ps6125ExtentPower) int { return a.identity.index - b.identity.index })
	merged := powers[:0]
	for _, term := range powers {
		if len(merged) > 0 && merged[len(merged)-1].identity == term.identity {
			previous := &merged[len(merged)-1]
			if term.power > math.MaxUint32-previous.power {
				return ps6125Extent{}
			}
			previous.power += term.power
		} else {
			merged = append(merged, term)
		}
	}
	return ps6125Extent{known: true, coefficient: coefficient, powers: merged}
}

func ps6125SameExtent(left, right ps6125Extent) bool {
	return left.known && right.known && left.coefficient == right.coefficient && slices.Equal(left.powers, right.powers)
}

// A join is a must-equality fact: one unresolved or differing incoming extent
// invalidates it. Branch reachability must be established by the caller first.
func ps6125JoinExtents(left, right ps6125Extent) ps6125Extent {
	if ps6125SameExtent(left, right) {
		return left
	}
	return ps6125Extent{}
}

func ps6125EvaluateExtent(extent ps6125Extent, dimensions map[*ps6125ExtentIdentity]int64, limit int64) (int64, bool) {
	if !extent.known || limit <= 0 || extent.coefficient < 0 || extent.coefficient > limit {
		return 0, false
	}
	result := extent.coefficient
	for _, term := range extent.powers {
		value, ok := dimensions[term.identity]
		if !ok || value < 0 || value > limit {
			return 0, false
		}
		power := term.power
		for power != 0 {
			if power&1 != 0 {
				if value != 0 && result > limit/value {
					return 0, false
				}
				result *= value
			}
			power >>= 1
			if power != 0 {
				if value != 0 && value > limit/value {
					return 0, false
				}
				value *= value
			}
		}
	}
	return result, true
}
