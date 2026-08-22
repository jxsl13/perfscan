package checks

import (
	"math"
	"strconv"
	"testing"
)

// TestEquivPS5051 proves the rewrite is byte-identical: for a FIXED base in
// [2, 36], strconv.FormatInt / FormatUint are injective, so the string
// comparison equals the integer comparison for == and != alike.
func TestEquivPS5051(t *testing.T) {
	bases := []int{2, 8, 10, 16, 36}
	ints := []int64{0, 1, -1, 9, -9, 10, -10, 15, 21, 100, -100, 255, -256, math.MinInt64, math.MaxInt64}
	uints := []uint64{0, 1, 9, 10, 15, 21, 100, 255, 65535, math.MaxUint32, math.MaxUint64}

	for _, base := range bases {
		for _, a := range ints {
			for _, b := range ints {
				str := strconv.FormatInt(a, base) == strconv.FormatInt(b, base)
				if str != (a == b) {
					t.Fatalf("FormatInt base %d: a=%d b=%d strEq=%v valEq=%v", base, a, b, str, a == b)
				}
				strNe := strconv.FormatInt(a, base) != strconv.FormatInt(b, base)
				if strNe != (a != b) {
					t.Fatalf("FormatInt !=, base %d: a=%d b=%d", base, a, b)
				}
			}
		}
		for _, a := range uints {
			for _, b := range uints {
				str := strconv.FormatUint(a, base) == strconv.FormatUint(b, base)
				if str != (a == b) {
					t.Fatalf("FormatUint base %d: a=%d b=%d strEq=%v valEq=%v", base, a, b, str, a == b)
				}
			}
		}
	}

	// The cross-base trap the check excludes: equal strings, unequal values.
	if strconv.FormatInt(21, 16) != strconv.FormatInt(15, 10) {
		t.Fatal("expected FormatInt(21,16) == FormatInt(15,10) as strings")
	}
	if 21 == 15 {
		t.Fatal("unreachable")
	}
}
