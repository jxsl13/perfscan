package checks

import (
	"math"
	"strconv"
	"testing"
)

// TestEquivPS5048 proves the rewrite is byte-identical: Itoa is injective, so
// strconv.Itoa(a) == strconv.Itoa(b) equals a == b (and != equals !=) for every
// pair of ints. Ordering is deliberately NOT rewritten and is not tested as
// equivalent.
func TestEquivPS5048(t *testing.T) {
	vals := []int{0, 1, -1, 9, 10, -10, 99, 100, -99, 123456, -123456, math.MaxInt, math.MinInt}
	for _, a := range vals {
		for _, b := range vals {
			if (strconv.Itoa(a) == strconv.Itoa(b)) != (a == b) {
				t.Fatalf("eq diverge a=%d b=%d", a, b)
			}
			if (strconv.Itoa(a) != strconv.Itoa(b)) != (a != b) {
				t.Fatalf("neq diverge a=%d b=%d", a, b)
			}
		}
	}
}
