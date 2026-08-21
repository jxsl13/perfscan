package checks

import (
	"math"
	"math/rand"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6063(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6063.Analyzer, "ps6063", "ps6063native")
}

func TestPS6063Float32RawBitContract(t *testing.T) {
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
	random := rand.New(rand.NewSource(6063))
	for range 300_000 {
		patterns = append(patterns, random.Uint32())
	}
	for _, bits := range patterns {
		got := math.Float32bits(-math.Float32frombits(bits))
		want := bits ^ uint32(0x80000000)
		if got != want {
			t.Fatalf("float32 bits %#08x: unary negation returned %#08x, sign XOR %#08x", bits, got, want)
		}
	}
}

func TestPS6063Float64RawBitContract(t *testing.T) {
	patterns := []uint64{
		0x0000000000000000, 0x8000000000000000,
		0x0000000000000001, 0x8000000000000001,
		0x3ff0000000000000, 0xbff0000000000000,
		0x7fefffffffffffff, 0xffefffffffffffff,
		0x7ff0000000000000, 0xfff0000000000000,
		0x7ff8000000000000, 0xfff8000000000000,
		0x7ff0000000000001, 0xfff0000000000001,
		0x7ff4123456789abc, 0xfff4123456789abc,
		0x7fffffffffffffff, 0xffffffffffffffff,
	}
	random := rand.New(rand.NewSource(6064))
	for range 300_000 {
		patterns = append(patterns, random.Uint64())
	}
	for _, bits := range patterns {
		got := math.Float64bits(-math.Float64frombits(bits))
		want := bits ^ uint64(0x8000000000000000)
		if got != want {
			t.Fatalf("float64 bits %#016x: unary negation returned %#016x, sign XOR %#016x", bits, got, want)
		}
	}
}
