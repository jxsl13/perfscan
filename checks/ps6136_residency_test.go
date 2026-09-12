package checks

import (
	"go/token"
	"go/types"
	"math"
	"testing"
)

func TestPS6136Residency(t *testing.T) {
	t.Parallel()
	var pool ps6125ExtentPool
	owner := types.NewVar(token.NoPos, nil, "decoder", types.Typ[types.Int])
	rows := pool.identity(owner, []*types.Var{types.NewField(token.NoPos, nil, "context", types.Typ[types.Int], false)}, false)
	width := pool.identity(owner, []*types.Var{types.NewField(token.NoPos, nil, "vocabulary", types.Typ[types.Int], false)}, false)
	r, valid := ps6136OutputResidency(rows, width)
	if !valid {
		t.Fatal("same-owner distinct geometry rejected")
	}
	retained, common, idle, amplification, known := r.evaluate(1024, 50257, math.MaxInt64)
	if !known || retained != 205852672 || common != 201028 || idle != 205651644 || amplification != 1024 {
		t.Fatalf("owner profile: %d %d %d %d %v", retained, common, idle, amplification, known)
	}
	for _, profile := range [][3]int64{{0, 50257, math.MaxInt64}, {1, 50257, math.MaxInt64}, {1024, 0, math.MaxInt64}, {-1, 50257, math.MaxInt64}, {1024, -1, math.MaxInt64}, {1024, 50257, 1000}, {math.MaxInt64, 50257, math.MaxInt64}, {1024, math.MaxInt64, math.MaxInt64}} {
		if _, _, _, _, known := r.evaluate(profile[0], profile[1], profile[2]); known {
			t.Fatalf("invalid/overflow profile accepted: %v", profile)
		}
	}
	var foreign ps6125ExtentPool
	other := foreign.identity(owner, nil, false)
	otherOwner := pool.identity(types.NewVar(token.NoPos, nil, "otherDecoder", types.Typ[types.Int]), nil, false)
	for _, pair := range [][2]*ps6125ExtentIdentity{{nil, width}, {rows, nil}, {rows, rows}, {rows, other}, {rows, otherOwner}} {
		if _, valid := ps6136OutputResidency(pair[0], pair[1]); valid {
			t.Fatal("unknown/contradictory geometry accepted")
		}
	}
}
