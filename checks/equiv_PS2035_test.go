package checks

// Runtime differential for PS2035: fmt.Appendf(buf, "%v", x) — x an unnamed
// predeclared integer, bool or float — vs the strconv.Append* rewrite the
// fix emits. The safety argument is PS2137's identity carried to the Appendf
// destination: %v of a plain scalar prints exactly the strconv form (the %d
// decimal for integers, "true"/"false" for bool, the shortest-'g' form with
// the operand's own bit size for floats), the operand's unnamed predeclared
// type cannot dispatch through Stringer/Formatter, and both sides reduce to
// appending the same formatted bytes onto buf. Every matched operand emits
// at least one byte, so both sides of Appendf(nil, ...) return a non-nil
// slice (PS2136's observation) — nil-ness is asserted alongside the bytes
// throughout.
//
// The suite pins that claim over:
//
//   - integer extremes and every width: 0, ±1, digit-length boundaries,
//     MinInt64/MaxInt64/MaxUint64, the exact int64/uint64 no-wrap spellings
//     and the widening-wrapper spellings for the narrower widths (uintptr
//     included), plus randomized 64-bit values;
//   - bool: both values over every destination shape;
//   - floats: -0, NaN, ±Inf, MaxFloat64, the smallest subnormals, the
//     exponent-form switchovers (1e21 and 1e-5), float32 through the
//     float64(f) widening with bitSize 32, and randomized BIT PATTERNS of
//     both widths (every NaN payload and subnormal arises);
//   - destination shapes: nil, empty-but-allocated, a non-empty prefix,
//     spare capacity, and tight capacity that forces growth.

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"testing"
)

// ps2035Buffers returns the adversarial destination shapes; each check
// clones them so both sides see an identical, unaliased buf.
func ps2035Buffers() [][]byte {
	return [][]byte{
		nil,
		{},
		[]byte("pre:"),
		make([]byte, 0, 64),
		append(make([]byte, 0, 4), 'x'), // tight capacity: forces growth
	}
}

// ps2035Compare asserts byte equality AND nil-ness equality of one
// before/after pair.
func ps2035Compare(t *testing.T, name string, before, after []byte) {
	t.Helper()
	if !bytes.Equal(before, after) {
		t.Fatalf("%s diverges: fmt.Appendf=%q strconv=%q", name, before, after)
	}
	if (before == nil) != (after == nil) {
		t.Fatalf("%s nil-ness diverges: fmt.Appendf nil=%v strconv nil=%v", name, before == nil, after == nil)
	}
}

// The before/after shapes below are EXACTLY the check's rewrite pairs: the
// Appendf call with the literal "%v", and the strconv.Append* call with the
// widening wrapper (or the bare operand for the exact int64/uint64/float64
// widths) the fix splices in.

func ps2035CheckInt64(t *testing.T, i int64) {
	t.Helper()
	for _, b := range ps2035Buffers() {
		ps2035Compare(t, "%v/int64", fmt.Appendf(bytes.Clone(b), "%v", i), strconv.AppendInt(bytes.Clone(b), i, 10))
	}
	// The narrower signed widths go through the value-preserving int64(...)
	// wrapper the fix emits; int goes through it too.
	i8, i16, i32, ii := int8(i), int16(i), int32(i), int(i)
	ps2035Compare(t, "%v/int8", fmt.Appendf(nil, "%v", i8), strconv.AppendInt(nil, int64(i8), 10))
	ps2035Compare(t, "%v/int16", fmt.Appendf(nil, "%v", i16), strconv.AppendInt(nil, int64(i16), 10))
	ps2035Compare(t, "%v/int32", fmt.Appendf(nil, "%v", i32), strconv.AppendInt(nil, int64(i32), 10))
	ps2035Compare(t, "%v/int", fmt.Appendf(nil, "%v", ii), strconv.AppendInt(nil, int64(ii), 10))
}

func ps2035CheckUint64(t *testing.T, u uint64) {
	t.Helper()
	for _, b := range ps2035Buffers() {
		ps2035Compare(t, "%v/uint64", fmt.Appendf(bytes.Clone(b), "%v", u), strconv.AppendUint(bytes.Clone(b), u, 10))
	}
	u8, u16, u32, uu, up := uint8(u), uint16(u), uint32(u), uint(u), uintptr(u)
	ps2035Compare(t, "%v/uint8", fmt.Appendf(nil, "%v", u8), strconv.AppendUint(nil, uint64(u8), 10))
	ps2035Compare(t, "%v/uint16", fmt.Appendf(nil, "%v", u16), strconv.AppendUint(nil, uint64(u16), 10))
	ps2035Compare(t, "%v/uint32", fmt.Appendf(nil, "%v", u32), strconv.AppendUint(nil, uint64(u32), 10))
	ps2035Compare(t, "%v/uint", fmt.Appendf(nil, "%v", uu), strconv.AppendUint(nil, uint64(uu), 10))
	ps2035Compare(t, "%v/uintptr", fmt.Appendf(nil, "%v", up), strconv.AppendUint(nil, uint64(up), 10))
}

func TestEquiv_PS2035_Integers(t *testing.T) {
	signed := []int64{
		0, 1, -1, 7, -7, 8, -8, 9, 10, -10, 99, 100, 255, -255, 256,
		1<<20 - 1, 1 << 20, -(1 << 20), 1<<32 - 1, 1 << 32, -(1 << 32),
		math.MaxInt8, math.MinInt8, math.MaxInt16, math.MinInt16,
		math.MaxInt32, math.MinInt32, math.MaxInt64, math.MinInt64,
	}
	for _, i := range signed {
		ps2035CheckInt64(t, i)
	}
	unsigned := []uint64{
		0, 1, 7, 8, 9, 10, 99, 100, 255, 256, 1<<32 - 1, 1 << 32,
		math.MaxUint8, math.MaxUint16, math.MaxUint32, math.MaxUint64, math.MaxUint64 - 1,
	}
	for _, u := range unsigned {
		ps2035CheckUint64(t, u)
	}
	rng := rand.New(rand.NewSource(0x20350001))
	for range 20000 {
		ps2035CheckInt64(t, int64(rng.Uint64()))
		ps2035CheckUint64(t, rng.Uint64())
	}
}

func TestEquiv_PS2035_Bool(t *testing.T) {
	for _, v := range []bool{true, false} {
		for _, b := range ps2035Buffers() {
			ps2035Compare(t, "%v/bool", fmt.Appendf(bytes.Clone(b), "%v", v), strconv.AppendBool(bytes.Clone(b), v))
		}
	}
}

func TestEquiv_PS2035_Floats(t *testing.T) {
	f64s := []float64{
		0, math.Copysign(0, -1), 1, -1, 0.5, 1.0 / 3.0, math.Pi,
		1e-4, 1e-5, 1e-10, // 1e-5 is %v/%g's small-exponent switchover
		1e20, 1e21, -1e21, // 1e21 is the large-exponent switchover
		math.MaxFloat64, math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64,
		2.2250738585072014e-308, // smallest normal
		math.Inf(1), math.Inf(-1), math.NaN(),
	}
	for _, f := range f64s {
		for _, b := range ps2035Buffers() {
			ps2035Compare(t, "%v/float64", fmt.Appendf(bytes.Clone(b), "%v", f), strconv.AppendFloat(bytes.Clone(b), f, 'g', -1, 64))
		}
	}
	f32s := []float32{
		0, float32(math.Copysign(0, -1)), 1, -1, 1.0 / 3.0,
		math.MaxFloat32, math.SmallestNonzeroFloat32,
		float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN()),
	}
	for _, f := range f32s {
		for _, b := range ps2035Buffers() {
			// The fix's float32 spelling: float64(f) widening, bitSize 32.
			ps2035Compare(t, "%v/float32", fmt.Appendf(bytes.Clone(b), "%v", f), strconv.AppendFloat(bytes.Clone(b), float64(f), 'g', -1, 32))
		}
	}
	// Randomized BIT PATTERNS: every NaN payload, subnormal and exponent
	// arises — not just round numbers.
	rng := rand.New(rand.NewSource(0x20350002))
	for range 100000 {
		f := math.Float64frombits(rng.Uint64())
		ps2035Compare(t, "%v/float64/bits", fmt.Appendf(nil, "%v", f), strconv.AppendFloat(nil, f, 'g', -1, 64))
		g := math.Float32frombits(uint32(rng.Uint64()))
		ps2035Compare(t, "%v/float32/bits", fmt.Appendf(nil, "%v", g), strconv.AppendFloat(nil, float64(g), 'g', -1, 32))
	}
}

// TestEquiv_PS2035_NamedTypeDiverges pins WHY named types are skipped
// entirely: %v dispatches through a named type's String() method, so the
// strconv form genuinely diverges there — the check must never match one.
func TestEquiv_PS2035_NamedTypeDiverges(t *testing.T) {
	m := ps2035Stringer(7)
	before := fmt.Appendf(nil, "%v", m)
	after := strconv.AppendInt(nil, int64(m), 10)
	if bytes.Equal(before, after) {
		t.Fatalf("expected divergence for a named Stringer operand, got %q on both sides — the skip-named guard would be obsolete", before)
	}
	if string(before) != "custom" {
		t.Fatalf("%%v of the named Stringer printed %q, want the String() result", before)
	}
}

type ps2035Stringer int

func (ps2035Stringer) String() string { return "custom" }

// TestEquiv_PS2035_AfterSideZeroAlloc pins the perf premise: with a
// destination that has spare capacity, the strconv.Append* side performs
// ZERO allocations, while fmt.Appendf always pays at least the
// interface-boxing allocation — the exact overhead the rewrite removes.
func TestEquiv_PS2035_AfterSideZeroAlloc(t *testing.T) {
	buf := make([]byte, 0, 128)
	var sink []byte
	allocs := testing.AllocsPerRun(100, func() {
		sink = strconv.AppendInt(buf[:0], int64(123456), 10)
		sink = strconv.AppendUint(buf[:0], uint64(123456), 10)
		sink = strconv.AppendBool(buf[:0], true)
		sink = strconv.AppendFloat(buf[:0], 3.25, 'g', -1, 64)
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the strconv.Append* quartet allocated %v times per run into spare capacity; PS2035's zero-alloc premise no longer holds", allocs)
	}
}
