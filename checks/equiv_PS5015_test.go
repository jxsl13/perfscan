package checks

// Runtime differential for PS5015: fmt.Appendf(buf, verb, x) — one bare
// scalar verb — vs the strconv.Append* rewrite the fix emits. The safety
// argument is PS2107's set of formatting identities carried to the Appendf
// destination: fmt implements the bare %d/%b/%o/%x, %t, %g and %q verbs
// via exactly the strconv formatters the fix calls (base-N digits with the
// leading '-', "true"/"false", shortest-'g' with the operand's own bit
// size, Quote/QuoteRune), the operand's unnamed predeclared type cannot
// dispatch through Stringer/Formatter, and both sides reduce to appending
// the same formatted bytes onto buf. Every matched verb emits at least one
// byte, so both sides of Appendf(nil, ...) return a non-nil slice (PS2136's
// observation) — nil-ness is asserted alongside the bytes throughout.
//
// The suite pins that claim over:
//
//   - integer extremes and every width: 0, ±1, digit-length boundaries,
//     MinInt64/MaxInt64/MaxUint64, all four bases, the exact int64/uint64
//     no-wrap spellings and the widening-wrapper spellings for the
//     narrower widths (uintptr included), plus randomized 64-bit values;
//   - floats: -0, NaN, ±Inf, MaxFloat64, the smallest subnormals, the
//     exponent-form switchover, float32 through the float64(f) widening
//     with bitSize 32, and randomized BIT PATTERNS of both widths (every
//     NaN payload and subnormal arises);
//   - strings: empty, quotes/backslashes, control bytes, all 256 single
//     bytes (invalid UTF-8 included), multi-byte runes, and randomized
//     byte strings;
//   - runes: negative, surrogate halves, MaxRune and beyond (U+FFFD
//     collapse on both sides), MinInt32/MaxInt32, and randomized int32s;
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

// ps5015Buffers returns the adversarial destination shapes; each check
// clones them so both sides see an identical, unaliased buf.
func ps5015Buffers() [][]byte {
	return [][]byte{
		nil,
		{},
		[]byte("pre:"),
		make([]byte, 0, 64),
		append(make([]byte, 0, 4), 'x'), // tight capacity: forces growth
	}
}

// ps5015Compare asserts byte equality AND nil-ness equality of one
// before/after pair.
func ps5015Compare(t *testing.T, name string, before, after []byte) {
	t.Helper()
	if !bytes.Equal(before, after) {
		t.Fatalf("%s diverges: fmt.Appendf=%q strconv=%q", name, before, after)
	}
	if (before == nil) != (after == nil) {
		t.Fatalf("%s nil-ness diverges: fmt.Appendf nil=%v strconv nil=%v", name, before == nil, after == nil)
	}
}

// The before/after shapes below are EXACTLY the check's rewrite pairs: the
// Appendf call with the literal verb, and the strconv.Append* call with the
// widening wrapper (or the bare operand for the exact int64/uint64/float64
// widths) the fix splices in.

func ps5015CheckInt64(t *testing.T, i int64) {
	t.Helper()
	for _, b := range ps5015Buffers() {
		ps5015Compare(t, "%d/int64", fmt.Appendf(bytes.Clone(b), "%d", i), strconv.AppendInt(bytes.Clone(b), i, 10))
		ps5015Compare(t, "%b/int64", fmt.Appendf(bytes.Clone(b), "%b", i), strconv.AppendInt(bytes.Clone(b), i, 2))
		ps5015Compare(t, "%o/int64", fmt.Appendf(bytes.Clone(b), "%o", i), strconv.AppendInt(bytes.Clone(b), i, 8))
		ps5015Compare(t, "%x/int64", fmt.Appendf(bytes.Clone(b), "%x", i), strconv.AppendInt(bytes.Clone(b), i, 16))
	}
	// The narrower signed widths go through the value-preserving int64(...)
	// wrapper the fix emits; int goes through it too.
	i8, i16, i32, ii := int8(i), int16(i), int32(i), int(i)
	ps5015Compare(t, "%d/int8", fmt.Appendf(nil, "%d", i8), strconv.AppendInt(nil, int64(i8), 10))
	ps5015Compare(t, "%b/int16", fmt.Appendf(nil, "%b", i16), strconv.AppendInt(nil, int64(i16), 2))
	ps5015Compare(t, "%o/int32", fmt.Appendf(nil, "%o", i32), strconv.AppendInt(nil, int64(i32), 8))
	ps5015Compare(t, "%x/int", fmt.Appendf(nil, "%x", ii), strconv.AppendInt(nil, int64(ii), 16))
}

func ps5015CheckUint64(t *testing.T, u uint64) {
	t.Helper()
	for _, b := range ps5015Buffers() {
		ps5015Compare(t, "%d/uint64", fmt.Appendf(bytes.Clone(b), "%d", u), strconv.AppendUint(bytes.Clone(b), u, 10))
		ps5015Compare(t, "%b/uint64", fmt.Appendf(bytes.Clone(b), "%b", u), strconv.AppendUint(bytes.Clone(b), u, 2))
		ps5015Compare(t, "%o/uint64", fmt.Appendf(bytes.Clone(b), "%o", u), strconv.AppendUint(bytes.Clone(b), u, 8))
		ps5015Compare(t, "%x/uint64", fmt.Appendf(bytes.Clone(b), "%x", u), strconv.AppendUint(bytes.Clone(b), u, 16))
	}
	u8, u16, u32, uu, up := uint8(u), uint16(u), uint32(u), uint(u), uintptr(u)
	ps5015Compare(t, "%d/uint8", fmt.Appendf(nil, "%d", u8), strconv.AppendUint(nil, uint64(u8), 10))
	ps5015Compare(t, "%b/uint16", fmt.Appendf(nil, "%b", u16), strconv.AppendUint(nil, uint64(u16), 2))
	ps5015Compare(t, "%o/uint32", fmt.Appendf(nil, "%o", u32), strconv.AppendUint(nil, uint64(u32), 8))
	ps5015Compare(t, "%x/uint", fmt.Appendf(nil, "%x", uu), strconv.AppendUint(nil, uint64(uu), 16))
	ps5015Compare(t, "%d/uintptr", fmt.Appendf(nil, "%d", up), strconv.AppendUint(nil, uint64(up), 10))
}

func TestEquiv_PS5015_Integers(t *testing.T) {
	signed := []int64{
		0, 1, -1, 7, -7, 8, -8, 9, 10, -10, 99, 100, 255, -255, 256,
		1<<20 - 1, 1 << 20, -(1 << 20), 1<<32 - 1, 1 << 32, -(1 << 32),
		math.MaxInt8, math.MinInt8, math.MaxInt16, math.MinInt16,
		math.MaxInt32, math.MinInt32, math.MaxInt64, math.MinInt64,
	}
	for _, i := range signed {
		ps5015CheckInt64(t, i)
	}
	unsigned := []uint64{
		0, 1, 7, 8, 9, 10, 99, 100, 255, 256, 1<<32 - 1, 1 << 32,
		math.MaxUint8, math.MaxUint16, math.MaxUint32, math.MaxUint64, math.MaxUint64 - 1,
	}
	for _, u := range unsigned {
		ps5015CheckUint64(t, u)
	}
	rng := rand.New(rand.NewSource(0x50150001))
	for range 20000 {
		ps5015CheckInt64(t, int64(rng.Uint64()))
		ps5015CheckUint64(t, rng.Uint64())
	}
}

func TestEquiv_PS5015_Bool(t *testing.T) {
	for _, v := range []bool{true, false} {
		for _, b := range ps5015Buffers() {
			ps5015Compare(t, "%t/bool", fmt.Appendf(bytes.Clone(b), "%t", v), strconv.AppendBool(bytes.Clone(b), v))
		}
	}
}

func TestEquiv_PS5015_Floats(t *testing.T) {
	f64s := []float64{
		0, math.Copysign(0, -1), 1, -1, 0.5, 1.0 / 3.0, math.Pi,
		1e-10, 1e20, 1e21, -1e21, // 1e21 is %g's exponent-form switchover
		math.MaxFloat64, math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64,
		2.2250738585072014e-308, // smallest normal
		math.Inf(1), math.Inf(-1), math.NaN(),
	}
	for _, f := range f64s {
		for _, b := range ps5015Buffers() {
			ps5015Compare(t, "%g/float64", fmt.Appendf(bytes.Clone(b), "%g", f), strconv.AppendFloat(bytes.Clone(b), f, 'g', -1, 64))
		}
	}
	f32s := []float32{
		0, float32(math.Copysign(0, -1)), 1, -1, 1.0 / 3.0,
		math.MaxFloat32, math.SmallestNonzeroFloat32,
		float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN()),
	}
	for _, f := range f32s {
		for _, b := range ps5015Buffers() {
			// The fix's float32 spelling: float64(f) widening, bitSize 32.
			ps5015Compare(t, "%g/float32", fmt.Appendf(bytes.Clone(b), "%g", f), strconv.AppendFloat(bytes.Clone(b), float64(f), 'g', -1, 32))
		}
	}
	// Randomized BIT PATTERNS: every NaN payload, subnormal and exponent
	// arises — not just round numbers.
	rng := rand.New(rand.NewSource(0x50150002))
	for range 100000 {
		f := math.Float64frombits(rng.Uint64())
		ps5015Compare(t, "%g/float64/bits", fmt.Appendf(nil, "%g", f), strconv.AppendFloat(nil, f, 'g', -1, 64))
		g := math.Float32frombits(uint32(rng.Uint64()))
		ps5015Compare(t, "%g/float32/bits", fmt.Appendf(nil, "%g", g), strconv.AppendFloat(nil, float64(g), 'g', -1, 32))
	}
}

func TestEquiv_PS5015_QuoteString(t *testing.T) {
	strs := []string{
		"", "a", "hello world",
		`with "quotes" and \backslashes\`,
		"\x00\x01\x1f\x7f",     // control bytes
		"\xff\xfe not utf-8",   // invalid UTF-8
		"\xc3\x28",             // truncated multi-byte sequence
		"\xed\xa0\x80",         // an ENCODED surrogate half
		"é…€\U0010FFFF",        // multi-byte runes incl. MaxRune
		"tab\tnl\nret\rbell\a", // escaped control characters
	}
	for c := range 256 {
		strs = append(strs, string([]byte{byte(c)}))
	}
	rng := rand.New(rand.NewSource(0x50150003))
	for range 5000 {
		raw := make([]byte, rng.Intn(48))
		rng.Read(raw)
		strs = append(strs, string(raw))
	}
	for _, s := range strs {
		for _, b := range ps5015Buffers() {
			ps5015Compare(t, "%q/string", fmt.Appendf(bytes.Clone(b), "%q", s), strconv.AppendQuote(bytes.Clone(b), s))
		}
	}
}

func TestEquiv_PS5015_QuoteRune(t *testing.T) {
	runes := []rune{
		0, 'a', '\n', '\'', '\\', 0x7f, 0x80, 'é', '€',
		0xD7FF, 0xD800, 0xDBFF, 0xDC00, 0xDFFF, 0xE000, // around and inside the surrogate range
		0x10FFFF, 0x10FFFF + 1, // MaxRune and just beyond
		-1, math.MinInt32, math.MaxInt32,
	}
	rng := rand.New(rand.NewSource(0x50150004))
	for range 100000 {
		runes = append(runes, rune(rng.Uint64()))
	}
	for _, r := range runes {
		ps5015Compare(t, "%q/rune", fmt.Appendf(nil, "%q", r), strconv.AppendQuoteRune(nil, r))
	}
	for _, b := range ps5015Buffers() {
		ps5015Compare(t, "%q/rune/buf", fmt.Appendf(bytes.Clone(b), "%q", rune(0xD800)), strconv.AppendQuoteRune(bytes.Clone(b), 0xD800))
	}
}

// TestEquiv_PS5015_AfterSideZeroAlloc pins the perf premise: with a
// destination that has spare capacity, the strconv.Append* side performs
// ZERO allocations, while fmt.Appendf always pays at least the
// interface-boxing allocation — the exact overhead the rewrite removes.
func TestEquiv_PS5015_AfterSideZeroAlloc(t *testing.T) {
	buf := make([]byte, 0, 128)
	var sink []byte
	allocs := testing.AllocsPerRun(100, func() {
		sink = strconv.AppendInt(buf[:0], int64(123456), 10)
		sink = strconv.AppendUint(buf[:0], uint64(123456), 16)
		sink = strconv.AppendBool(buf[:0], true)
		sink = strconv.AppendFloat(buf[:0], 3.25, 'g', -1, 64)
		sink = strconv.AppendQuote(buf[:0], "short string")
		sink = strconv.AppendQuoteRune(buf[:0], 'é')
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the strconv.Append* sextet allocated %v times per run into spare capacity; PS5015's zero-alloc premise no longer holds", allocs)
	}
}
