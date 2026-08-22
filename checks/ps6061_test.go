package checks

import (
	"math"
	"math/rand"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6061(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6061.Analyzer, "ps6061", "ps6061native")
}

func TestPS6061RawBitContract(t *testing.T) {
	patterns := []uint32{
		0x00000000, 0x80000000,
		0x00000001, 0x80000001,
		0x3f800000, 0xbf800000,
		0x7f7fffff, 0xff7fffff,
		0x7f800000, 0xff800000,
		0x7fc00000, 0xffc00000,
		0x7f800001, 0xff800001,
		0x7fa12345, 0xffa12345,
		0x7fffffff, 0xffffffff,
	}
	random := rand.New(rand.NewSource(6061))
	for range 300_000 {
		patterns = append(patterns, random.Uint32())
	}
	for _, bits := range patterns {
		before := math.Float32bits(float32(math.Abs(float64(math.Float32frombits(bits)))))
		magnitude := bits & 0x7fffffff
		preserved := magnitude
		quieted := magnitude
		if magnitude > 0x7f800000 {
			quieted |= 0x00400000
		}
		if before != preserved && before != quieted {
			t.Fatalf("bits %#08x: widened math.Abs returned %#08x, want payload-preserving sign-clear %#08x or quieted %#08x", bits, before, preserved, quieted)
		}
	}
	got := math.Float32bits(float32(math.Abs(float64(math.Float32frombits(0x7f800001)))))
	if got != 0x7f800001 && got != 0x7fc00001 {
		t.Fatalf("signaling-NaN sentinel returned unsupported encoding %#08x", got)
	}
	t.Logf("toolchain signaling-NaN contract: 0x7f800001 -> %#08x", got)
}
