package checks

import (
	"math"
	"math/rand"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6064(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6064.Analyzer, "ps6064")
}

func TestPS6064SharedBaseFloat64RawBitContract(t *testing.T) {
	patterns := []uint64{
		0x0000000000000000, 0x8000000000000000,
		0x0000000000000001, 0x8000000000000001,
		0x3ff0000000000000, 0xbff0000000000000,
		0x7fefffffffffffff, 0xffefffffffffffff,
		0x7ff0000000000000, 0xfff0000000000000,
		0x7ff8000000000000, 0xfff8000000000000,
		0x7ff0000000000001, 0xfff0000000000001,
		0x7ff4123456789abc, 0xfff4123456789abc,
	}
	random := rand.New(rand.NewSource(6064))
	for range 20_000 {
		patterns = append(patterns, random.Uint64())
	}
	for _, bits := range patterns {
		value := math.Float64frombits(bits)
		wantPositive := ps6064Standalone64(value)
		wantNegative := ps6064Standalone64(-value)
		gotPositive, gotNegative := ps6064Shared64(value)
		if math.Float64bits(gotPositive) != math.Float64bits(wantPositive) || math.Float64bits(gotNegative) != math.Float64bits(wantNegative) {
			t.Fatalf("float64 bits %#016x: standalone=(%#016x,%#016x) shared=(%#016x,%#016x)", bits,
				math.Float64bits(wantPositive), math.Float64bits(wantNegative), math.Float64bits(gotPositive), math.Float64bits(gotNegative))
		}
	}
}

func TestPS6064SharedBaseFloat32RoundingContracts(t *testing.T) {
	patterns := []uint32{
		0x00000000, 0x80000000,
		0x00000001, 0x80000001,
		0x3f800000, 0xbf800000,
		0x7f7fffff, 0xff7fffff,
		0x7f800000, 0xff800000,
		0x7fc00000, 0xffc00000,
		0x7f800001, 0xff800001,
		0x7fa12345, 0xffa12345,
	}
	random := rand.New(rand.NewSource(60_640))
	for range 20_000 {
		patterns = append(patterns, random.Uint32())
	}
	for _, bits := range patterns {
		value := math.Float32frombits(bits)
		wantNativePositive := ps6064StandaloneNative32(value)
		wantNativeNegative := ps6064StandaloneNative32(-value)
		gotNativePositive, gotNativeNegative := ps6064SharedNative32(value)
		if math.Float32bits(gotNativePositive) != math.Float32bits(wantNativePositive) || math.Float32bits(gotNativeNegative) != math.Float32bits(wantNativeNegative) {
			t.Fatalf("native float32 bits %#08x: standalone=(%#08x,%#08x) shared=(%#08x,%#08x)", bits,
				math.Float32bits(wantNativePositive), math.Float32bits(wantNativeNegative), math.Float32bits(gotNativePositive), math.Float32bits(gotNativeNegative))
		}

		wantNarrowPositive := ps6064Standalone64Narrow32(value)
		wantNarrowNegative := ps6064Standalone64Narrow32(-value)
		gotNarrowPositive, gotNarrowNegative := ps6064Shared64Narrow32(value)
		if math.Float32bits(gotNarrowPositive) != math.Float32bits(wantNarrowPositive) || math.Float32bits(gotNarrowNegative) != math.Float32bits(wantNarrowNegative) {
			t.Fatalf("F64-to-F32 bits %#08x: standalone=(%#08x,%#08x) shared=(%#08x,%#08x)", bits,
				math.Float32bits(wantNarrowPositive), math.Float32bits(wantNarrowNegative), math.Float32bits(gotNarrowPositive), math.Float32bits(gotNarrowNegative))
		}
	}
}

func ps6064Standalone64(value float64) float64 {
	base := math.Log1p(math.Exp(-math.Abs(value)))
	return base + max(value, 0)
}

func ps6064Shared64(value float64) (float64, float64) {
	if math.IsNaN(value) {
		return ps6064Standalone64(value), ps6064Standalone64(-value)
	}
	base := math.Log1p(math.Exp(-math.Abs(value)))
	return base + max(value, 0), base + max(-value, 0)
}

func ps6064StandaloneNative32(value float32) float32 {
	abs := math.Float32frombits(math.Float32bits(value) & 0x7fffffff)
	base := float32(math.Log1p(math.Exp(float64(-abs))))
	return base + max(value, 0)
}

func ps6064SharedNative32(value float32) (float32, float32) {
	if math.IsNaN(float64(value)) {
		return ps6064StandaloneNative32(value), ps6064StandaloneNative32(-value)
	}
	abs := math.Float32frombits(math.Float32bits(value) & 0x7fffffff)
	base := float32(math.Log1p(math.Exp(float64(-abs))))
	return base + max(value, 0), base + max(-value, 0)
}

func ps6064Standalone64Narrow32(value float32) float32 {
	wide := float64(value)
	base := math.Log1p(math.Exp(-math.Abs(wide)))
	return float32(base + max(wide, 0))
}

func ps6064Shared64Narrow32(value float32) (float32, float32) {
	if math.IsNaN(float64(value)) {
		return ps6064Standalone64Narrow32(value), ps6064Standalone64Narrow32(-value)
	}
	wide := float64(value)
	base := math.Log1p(math.Exp(-math.Abs(wide)))
	return float32(base + max(wide, 0)), float32(base + max(-wide, 0))
}
