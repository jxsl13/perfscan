package checks

import (
	"go/token"
	"go/types"
	"math"
	"testing"
)

func TestPS6125ExtentTypedIdentity(t *testing.T) {
	t.Parallel()
	var pool ps6125ExtentPool
	owner := types.NewVar(token.NoPos, nil, "d", types.Typ[types.Int])
	sibling := types.NewVar(token.NoPos, nil, "d", types.Typ[types.Int])
	first := types.NewField(token.NoPos, nil, "first", types.Typ[types.Int], false)
	second := types.NewField(token.NoPos, nil, "second", types.Typ[types.Int], false)
	width := types.NewField(token.NoPos, nil, "width", types.Typ[types.Int], false)
	shadow := types.NewField(token.NoPos, nil, "width", types.Typ[types.Int], false)
	base := pool.identity(owner, []*types.Var{first, width}, false)
	if base != pool.identity(owner, []*types.Var{first, width}, false) {
		t.Fatal("identical typed path was not interned")
	}
	for _, other := range []*ps6125ExtentIdentity{
		pool.identity(sibling, []*types.Var{first, width}, false),
		pool.identity(owner, []*types.Var{second, width}, false),
		pool.identity(owner, []*types.Var{first, shadow}, false),
		pool.identity(owner, []*types.Var{first, width}, true),
	} {
		if ps6125SameExtent(ps6125SymbolicExtent(base), ps6125SymbolicExtent(other)) {
			t.Fatal("distinct instance, field path, field object or length was conflated")
		}
	}
	if pool.identity(nil, nil, false) != nil || pool.identity(owner, []*types.Var{nil}, false) != nil {
		t.Fatal("invalid typed identity accepted")
	}
	var otherPool ps6125ExtentPool
	foreign := otherPool.identity(owner, []*types.Var{first, width}, false)
	if ps6125MultiplyExtents(ps6125SymbolicExtent(base), ps6125SymbolicExtent(foreign)).known {
		t.Fatal("identities from different analysis pools were combined")
	}
}

func TestPS6125ExtentCompositionAndJoin(t *testing.T) {
	t.Parallel()
	var pool ps6125ExtentPool
	rows := ps6125SymbolicExtent(pool.identity(types.NewVar(0, nil, "rows", types.Typ[types.Int]), nil, false))
	width := ps6125SymbolicExtent(pool.identity(types.NewVar(0, nil, "width", types.Typ[types.Int]), nil, false))
	forward := ps6125MultiplyExtents(rows, width)
	reverse := ps6125MultiplyExtents(width, rows)
	if !ps6125SameExtent(forward, reverse) || !ps6125JoinExtents(forward, reverse).known {
		t.Fatal("equal products lost their must-equality fact")
	}
	for _, other := range []ps6125Extent{ps6125Extent{}, width, ps6125ConstantExtent(1)} {
		if ps6125JoinExtents(forward, other).known {
			t.Fatal("a differing or unresolved incoming extent survived the join")
		}
	}
	if ps6125MultiplyExtents(ps6125Extent{}, ps6125ConstantExtent(0)).known {
		t.Fatal("unknown arithmetic became a zero-row proof")
	}
	before := rows.powers[0].power
	square := ps6125MultiplyExtents(rows, rows)
	if len(square.powers) != 1 || square.powers[0].power != 2 || rows.powers[0].power != before {
		t.Fatal("multiplication did not combine powers immutably")
	}
	for range 32 {
		square = ps6125MultiplyExtents(square, square)
	}
	if square.known {
		t.Fatal("exponent overflow did not become unknown")
	}
}

func TestPS6125ExtentCheckedGeometry(t *testing.T) {
	t.Parallel()
	var pool ps6125ExtentPool
	context := pool.identity(types.NewVar(0, nil, "context", types.Typ[types.Int]), nil, false)
	width := pool.identity(types.NewVar(0, nil, "width", types.Typ[types.Int]), nil, false)
	resident := ps6125MultiplyExtents(ps6125ConstantExtent(4), ps6125MultiplyExtents(ps6125SymbolicExtent(context), ps6125SymbolicExtent(width)))
	common := ps6125MultiplyExtents(ps6125ConstantExtent(4), ps6125SymbolicExtent(width))
	dimensions := map[*ps6125ExtentIdentity]int64{context: 1024, width: 50257}
	residentBytes, residentOK := ps6125EvaluateExtent(resident, dimensions, math.MaxInt32)
	commonBytes, commonOK := ps6125EvaluateExtent(common, dimensions, math.MaxInt32)
	if !residentOK || !commonOK || residentBytes != 205852672 || commonBytes != 201028 || residentBytes-commonBytes != 205651644 {
		t.Fatalf("configured geometry: resident=%d/%v common=%d/%v", residentBytes, residentOK, commonBytes, commonOK)
	}
	for _, invalid := range []map[*ps6125ExtentIdentity]int64{
		{context: 1024}, {context: -1, width: 50257}, {context: math.MaxInt32, width: 50257},
	} {
		if _, ok := ps6125EvaluateExtent(resident, invalid, math.MaxInt32); ok {
			t.Fatal("missing, negative or overflowing geometry accepted")
		}
	}
	if ps6125MultiplyExtents(ps6125ConstantExtent(math.MaxInt64), ps6125ConstantExtent(2)).known || ps6125ConstantExtent(-1).known {
		t.Fatal("invalid constant arithmetic accepted")
	}
	squared := ps6125MultiplyExtents(ps6125SymbolicExtent(width), ps6125SymbolicExtent(width))
	if _, ok := ps6125EvaluateExtent(squared, dimensions, math.MaxInt32); ok {
		t.Fatal("target-width power overflow accepted")
	}
}
