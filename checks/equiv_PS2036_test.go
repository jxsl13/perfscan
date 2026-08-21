package checks

// Runtime differential for PS2036: fmt.Append(buf, x) with a single
// UNNAMED predeclared integer/bool/float operand vs the strconv.Append*
// form the fix emits. The safety argument: fmt.Append appends
// fmt.Sprint(a...); with ONE operand Sprint's between-operand spacing
// rule never applies, and %v over a plain scalar prints exactly the
// strconv form — integers as base-10 digits with a leading '-' for
// negatives (MinInt64 and MaxUint64 included: AppendInt works in uint64
// magnitude space), bool as the literal true/false, floats as the
// shortest-'g' form with the operand's own bit size (NaN, the Infs, -0,
// denormals and the fixed/exponent switchover included). An unnamed
// predeclared type cannot carry methods, so it can never dispatch
// through fmt.Stringer/fmt.Formatter — closing the only divergence
// path. Both forms grow buf with the same builtin append, so contents,
// length AND aliasing behavior coincide. This suite pins that claim
// over:
//
//   - every replacement arm the fix emits (int with the int64 widening,
//     int64/uint64 bare, each narrower signed/unsigned width, uintptr,
//     bool, float64, float32 with the float64 widening) crossed with
//     adversarial values: 0, ±1, every digit-count boundary, MinInt64,
//     MaxUint64, and for floats -0, NaN, ±Inf, MaxFloat, the smallest
//     denormals and the %v fixed/exponent switchover values (1e21,
//     1e-5);
//   - adversarial destinations: nil, empty-non-nil, preloaded with
//     arbitrary bytes (invalid UTF-8 included), with and without spare
//     capacity;
//   - randomized bit patterns: uniform uint64 bits reinterpreted as
//     int64/uint64/float64/float32 — the float cases sweep denormals,
//     NaNs and every exponent regime;
//   - aliasing/growth pins: with spare capacity BOTH forms write into
//     the same backing array (fmt.Append's final append is the same
//     builtin append strconv.Append* uses), and a preloaded prefix is
//     never disturbed;
//   - the guards' load-bearing exclusions, pinned as divergence
//     WITNESSES: a named int with a String() method (fmt honors it,
//     strconv prints digits), a complex operand (the (re+imi) form),
//     nil ("<nil>"), a []byte operand (decimal slice representation),
//     and two operands (the spacing rule) — each asserted to actually
//     diverge, so the guard is load-bearing, not folklore;
//   - the perf premise: the strconv form never allocates when buf has
//     capacity, while fmt.Append's interface box does.

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"testing"
)

// The exact Before/After shapes per replacement arm: Before is the check's
// matched call, After is byte-for-byte the rewrite the fix emits.
func ps2036IntBefore(buf []byte, n int) []byte     { return fmt.Append(buf, n) }
func ps2036IntAfter(buf []byte, n int) []byte      { return strconv.AppendInt(buf, int64(n), 10) }
func ps2036I64Before(buf []byte, n int64) []byte   { return fmt.Append(buf, n) }
func ps2036I64After(buf []byte, n int64) []byte    { return strconv.AppendInt(buf, n, 10) }
func ps2036U64Before(buf []byte, u uint64) []byte  { return fmt.Append(buf, u) }
func ps2036U64After(buf []byte, u uint64) []byte   { return strconv.AppendUint(buf, u, 10) }
func ps2036BoolBefore(buf []byte, b bool) []byte   { return fmt.Append(buf, b) }
func ps2036BoolAfter(buf []byte, b bool) []byte    { return strconv.AppendBool(buf, b) }
func ps2036F64Before(buf []byte, f float64) []byte { return fmt.Append(buf, f) }
func ps2036F64After(buf []byte, f float64) []byte {
	return strconv.AppendFloat(buf, f, 'g', -1, 64)
}
func ps2036F32Before(buf []byte, f float32) []byte { return fmt.Append(buf, f) }
func ps2036F32After(buf []byte, f float32) []byte {
	return strconv.AppendFloat(buf, float64(f), 'g', -1, 32)
}

// ps2036Dests builds the destination []byte shapes crossed with every
// operand: nil, empty-non-nil, preloaded (ASCII and invalid-UTF-8
// prefixes), each without spare capacity and with plenty.
func ps2036Dests() [][]byte {
	prefixes := []string{"", "seed", "\xff\x00\x80pre"}
	var out [][]byte
	for _, p := range prefixes {
		tight := []byte(p)                         // len == cap (or nil for "")
		roomy := make([]byte, len(p), len(p)+4096) // plenty of spare capacity
		copy(roomy, p)
		out = append(out, tight, roomy)
	}
	out = append(out, nil, []byte{}) // explicit nil and empty-non-nil
	return out
}

// ps2036Check runs one operand through a Before/After pair over every
// destination shape and fails on any byte divergence.
func ps2036Check(t *testing.T, arm string, before, after func([]byte) []byte) {
	t.Helper()
	for _, buf := range ps2036Dests() {
		b1 := append([]byte(nil), buf...)
		b2 := append([]byte(nil), buf...)
		got1 := before(b1)
		got2 := after(b2)
		if string(got1) != string(got2) {
			t.Fatalf("%s divergence on buf=%q: fmt.Append=%q strconv=%q", arm, buf, got1, got2)
		}
	}
}

// TestEquiv_PS2036_Integers crosses every signed/unsigned width — each with
// the exact widening the fix emits — with the digit-count and overflow
// boundaries, MinInt64 and MaxUint64 included.
func TestEquiv_PS2036_Integers(t *testing.T) {
	signed := []int64{0, 1, -1, 9, 10, -9, -10, 99, 100, 12345, -12345,
		math.MaxInt8, math.MinInt8, math.MaxInt16, math.MinInt16,
		math.MaxInt32, math.MinInt32, math.MaxInt64, math.MinInt64}
	for _, v := range signed {
		v := v
		ps2036Check(t, "int64", func(b []byte) []byte { return ps2036I64Before(b, v) },
			func(b []byte) []byte { return ps2036I64After(b, v) })
		// The narrower widths go through the exact int64(x) widening the fix
		// splices in; the truncating conversions here just generate in-range
		// values for each width.
		n := int(v)
		ps2036Check(t, "int", func(b []byte) []byte { return ps2036IntBefore(b, n) },
			func(b []byte) []byte { return ps2036IntAfter(b, n) })
		n32, n16, n8 := int32(v), int16(v), int8(v)
		ps2036Check(t, "int32", func(b []byte) []byte { return fmt.Append(b, n32) },
			func(b []byte) []byte { return strconv.AppendInt(b, int64(n32), 10) })
		ps2036Check(t, "int16", func(b []byte) []byte { return fmt.Append(b, n16) },
			func(b []byte) []byte { return strconv.AppendInt(b, int64(n16), 10) })
		ps2036Check(t, "int8", func(b []byte) []byte { return fmt.Append(b, n8) },
			func(b []byte) []byte { return strconv.AppendInt(b, int64(n8), 10) })
	}
	unsigned := []uint64{0, 1, 9, 10, 99, 100, 255, 256, 65535, 65536,
		math.MaxUint32, math.MaxInt64, math.MaxInt64 + 1, math.MaxUint64}
	for _, v := range unsigned {
		v := v
		ps2036Check(t, "uint64", func(b []byte) []byte { return ps2036U64Before(b, v) },
			func(b []byte) []byte { return ps2036U64After(b, v) })
		u, u32, u16, u8, up := uint(v), uint32(v), uint16(v), uint8(v), uintptr(v)
		ps2036Check(t, "uint", func(b []byte) []byte { return fmt.Append(b, u) },
			func(b []byte) []byte { return strconv.AppendUint(b, uint64(u), 10) })
		ps2036Check(t, "uint32", func(b []byte) []byte { return fmt.Append(b, u32) },
			func(b []byte) []byte { return strconv.AppendUint(b, uint64(u32), 10) })
		ps2036Check(t, "uint16", func(b []byte) []byte { return fmt.Append(b, u16) },
			func(b []byte) []byte { return strconv.AppendUint(b, uint64(u16), 10) })
		ps2036Check(t, "uint8", func(b []byte) []byte { return fmt.Append(b, u8) },
			func(b []byte) []byte { return strconv.AppendUint(b, uint64(u8), 10) })
		ps2036Check(t, "uintptr", func(b []byte) []byte { return fmt.Append(b, up) },
			func(b []byte) []byte { return strconv.AppendUint(b, uint64(up), 10) })
	}
}

func TestEquiv_PS2036_Bool(t *testing.T) {
	for _, v := range []bool{true, false} {
		v := v
		ps2036Check(t, "bool", func(b []byte) []byte { return ps2036BoolBefore(b, v) },
			func(b []byte) []byte { return ps2036BoolAfter(b, v) })
	}
}

// TestEquiv_PS2036_Floats sweeps the %v-vs-AppendFloat identity over the
// nasty float edges: -0, NaN, ±Inf, the extremes, the smallest denormals
// and the %g fixed/exponent switchover values — for float64 and, through
// the value-preserving float64(f) widening with bitSize 32, for float32.
func TestEquiv_PS2036_Floats(t *testing.T) {
	f64s := []float64{0, math.Copysign(0, -1), 1, -1, 0.1, 2.5, 1.0 / 3.0,
		1e20, 1e21, 1e22, 1e-4, 1e-5, 1e-6, 123456789.123456789,
		math.MaxFloat64, -math.MaxFloat64, math.SmallestNonzeroFloat64,
		-math.SmallestNonzeroFloat64, 5e-324, math.Pi, 1e100, 1e6, 1e7,
		1234567.0, math.Inf(1), math.Inf(-1), math.NaN(),
		math.MaxFloat32, math.SmallestNonzeroFloat32}
	for _, v := range f64s {
		v := v
		ps2036Check(t, "float64", func(b []byte) []byte { return ps2036F64Before(b, v) },
			func(b []byte) []byte { return ps2036F64After(b, v) })
		g := float32(v)
		ps2036Check(t, "float32", func(b []byte) []byte { return ps2036F32Before(b, g) },
			func(b []byte) []byte { return ps2036F32After(b, g) })
	}
}

// TestEquiv_PS2036_RandomizedBits reinterprets uniform random bit patterns
// as every operand kind — for floats this sweeps denormals, NaN payloads
// and every exponent regime.
func TestEquiv_PS2036_RandomizedBits(t *testing.T) {
	rng := rand.New(rand.NewSource(0x2036))
	for range 50000 {
		bits := rng.Uint64()
		if got1, got2 := string(fmt.Append(nil, int64(bits))), string(strconv.AppendInt(nil, int64(bits), 10)); got1 != got2 {
			t.Fatalf("int64 divergence on %#x: fmt=%q strconv=%q", bits, got1, got2)
		}
		if got1, got2 := string(fmt.Append(nil, bits)), string(strconv.AppendUint(nil, bits, 10)); got1 != got2 {
			t.Fatalf("uint64 divergence on %#x: fmt=%q strconv=%q", bits, got1, got2)
		}
		f := math.Float64frombits(bits)
		if got1, got2 := string(fmt.Append(nil, f)), string(strconv.AppendFloat(nil, f, 'g', -1, 64)); got1 != got2 {
			t.Fatalf("float64 divergence on %#x (%v): fmt=%q strconv=%q", bits, f, got1, got2)
		}
		g := math.Float32frombits(uint32(bits))
		if got1, got2 := string(fmt.Append(nil, g)), string(strconv.AppendFloat(nil, float64(g), 'g', -1, 32)); got1 != got2 {
			t.Fatalf("float32 divergence on %#x (%v): fmt=%q strconv=%q", uint32(bits), g, got1, got2)
		}
	}
}

// TestEquiv_PS2036_AliasingAndGrowth pins that BOTH forms are the same
// builtin append over buf: with spare capacity each writes into buf's
// existing backing array (no reallocation), and without capacity each
// leaves the original backing array untouched.
func TestEquiv_PS2036_AliasingAndGrowth(t *testing.T) {
	const n int64 = -9876543210
	for _, form := range []struct {
		name string
		fn   func([]byte) []byte
	}{
		{"fmt.Append", func(b []byte) []byte { return ps2036I64Before(b, n) }},
		{"strconv.AppendInt", func(b []byte) []byte { return ps2036I64After(b, n) }},
	} {
		// Spare capacity: the result aliases buf's backing array.
		backing := make([]byte, 4, 64)
		copy(backing, "seed")
		got := form.fn(backing)
		if &got[0] != &backing[0] {
			t.Fatalf("%s with spare capacity reallocated; both forms must be the builtin append over buf", form.name)
		}
		if string(backing) != "seed" {
			t.Fatalf("%s disturbed the preloaded prefix: %q", form.name, backing)
		}
		if string(got) != "seed-9876543210" {
			t.Fatalf("%s appended wrong bytes: %q", form.name, got)
		}
		// No spare capacity: the original backing array stays untouched.
		tight := []byte("tight")
		tight = tight[:len(tight):len(tight)]
		got = form.fn(tight)
		if string(tight) != "tight" {
			t.Fatalf("%s modified the full source slice in place: %q", form.name, tight)
		}
		if string(got) != "tight-9876543210" {
			t.Fatalf("%s grew wrong: %q", form.name, got)
		}
	}
}

// ps2036NamedInt is the divergence witness for the unnamed-only guard: %v
// honors its String() method, strconv would print the digits.
type ps2036NamedInt int

func (ps2036NamedInt) String() string { return "STRINGER" }

// TestEquiv_PS2036_GuardWitnesses pins that every silent guard excludes a
// shape that ACTUALLY diverges — the guards are load-bearing.
func TestEquiv_PS2036_GuardWitnesses(t *testing.T) {
	// A NAMED int with String(): fmt honors the method, strconv the digits.
	var m ps2036NamedInt = 7
	if got := string(fmt.Append(nil, m)); got != "STRINGER" {
		t.Fatalf("fmt.Append over a Stringer-carrying named int = %q, want %q — re-audit the named-type guard", got, "STRINGER")
	}
	if got := string(strconv.AppendInt(nil, int64(m), 10)); got != "7" {
		t.Fatalf("strconv.AppendInt over the named int = %q, want %q", got, "7")
	}
	// A complex operand: the (re+imi) parenthesized form, nothing strconv
	// emits in one call.
	if got := string(fmt.Append(nil, complex(1, 2))); got != "(1+2i)" {
		t.Fatalf("fmt.Append over a complex128 = %q, want %q — the complex exclusion's premise moved", got, "(1+2i)")
	}
	// nil prints "<nil>".
	if got := string(fmt.Append(nil, nil)); got != "<nil>" {
		t.Fatalf("fmt.Append over nil = %q, want %q", got, "<nil>")
	}
	// A []byte operand: %v is the decimal slice representation.
	if got := string(fmt.Append(nil, []byte("hi"))); got != "[104 105]" {
		t.Fatalf("fmt.Append over a []byte = %q, want the decimal slice form %q", got, "[104 105]")
	}
	// Two non-string operands engage Sprint's spacing rule: two chained
	// strconv appends would drop the injected space.
	if got := string(fmt.Append(nil, 1, 2)); got != "1 2" {
		t.Fatalf("fmt.Append(nil, 1, 2) = %q, want %q — the multi-operand exclusion's premise moved", got, "1 2")
	}
}

// TestEquiv_PS2036_AfterSideZeroAlloc pins the perf premise: with spare
// capacity the strconv form never allocates, while fmt.Append pays the
// interface box for the operand on every call.
func TestEquiv_PS2036_AfterSideZeroAlloc(t *testing.T) {
	buf := make([]byte, 0, 64)
	n := 123456
	var sink []byte
	allocs := testing.AllocsPerRun(100, func() {
		sink = strconv.AppendInt(buf[:0], int64(n), 10)
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("strconv.AppendInt into spare capacity allocated %v times per run; PS2036's zero-alloc premise no longer holds", allocs)
	}
}
