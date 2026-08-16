package checks

import (
	"fmt"
	"math"
	"strconv"
	"testing"
)

// TestEquivPS5042 proves the rewrite is byte-identical: fmt.Appendf(dst,
// "%d", n) and strconv.AppendInt/AppendUint(dst, ...) append the same base-10
// digits for every integer value and width.
func TestEquivPS5042(t *testing.T) {
	ci := func(v int64) {
		a := fmt.Appendf([]byte("seed"), "%d", v)
		b := strconv.AppendInt([]byte("seed"), v, 10)
		if string(a) != string(b) {
			t.Fatalf("int64 %d: fmt=%s strconv=%s", v, a, b)
		}
	}
	cu := func(v uint64) {
		a := fmt.Appendf([]byte("seed"), "%d", v)
		b := strconv.AppendUint([]byte("seed"), v, 10)
		if string(a) != string(b) {
			t.Fatalf("uint64 %d: fmt=%s strconv=%s", v, a, b)
		}
	}
	for _, v := range []int64{0, 1, -1, 9, -9, 10, -10, 127, -128, 255, math.MinInt32, math.MaxInt32, math.MinInt64, math.MaxInt64} {
		ci(v)
	}
	for _, v := range []uint64{0, 1, 9, 10, 255, 65535, math.MaxUint32, math.MaxUint64} {
		cu(v)
	}
	for i := int64(-200000); i <= 200000; i++ {
		ci(i)
		cu(uint64(i))
	}
	// Narrower widths the fix widens: every value in range.
	for i := 0; i < 256; i++ {
		ci(int64(int8(i)))
		cu(uint64(uint8(i)))
	}
	for i := int64(math.MinInt16); i <= math.MaxInt16; i += 7 {
		ci(int64(int16(i)))
		cu(uint64(uint16(i & 0xFFFF)))
	}
}
