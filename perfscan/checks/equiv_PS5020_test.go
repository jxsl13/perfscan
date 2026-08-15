package checks

// Runtime differential for PS5020: append(dst, []byte(s)...) vs
// append(dst, s...). The fix's safety argument is that both forms
// append exactly the raw bytes of s — the []byte conversion copies
// those bytes into a throwaway slice append immediately copies out of,
// while the builtin string-append special form (spec: a second argument
// with core type bytestring) copies them straight from the string's
// immutable data — with NO UTF-8 interpretation in either form, so
// invalid UTF-8 rounds through verbatim. dst and s are each evaluated
// exactly once in both forms; the resulting length, capacity and
// nil-ness are identical because append growth depends only on
// len(dst), cap(dst) and the number of appended elements (len(s)), all
// unchanged; and the in-place/growth decision is likewise a pure
// function of those inputs, so the two forms agree byte-for-byte on
// the RESULT and on every byte of the destination's backing array,
// including whether the result aliases it. This suite pins that claim
// over:
//
//   - EXHAUSTIVE short inputs: every string of length <= 3 over an
//     adversarial alphabet (ASCII, NUL, the bytes of a multi-byte rune
//     so truncated and complete sequences both arise, and 0xFF),
//     crossed with destination states covering nil, non-nil empty,
//     exactly-full (growth forced), and prefixes with spare capacity
//     (in-place append into the same backing array);
//   - targeted seeds: empty string onto nil (nil stays nil in BOTH
//     forms), strings past the 32-byte stack-conversion buffer, 4 KiB
//     payloads, U+FFFD itself, raw invalid UTF-8, a NUL-ridden string,
//     and s derived from dst's own bytes (string(dst) — the conversion
//     that feeds append is a copy in both pipelines, so no aliasing
//     divergence is possible);
//   - randomized full-byte-range strings and destination shapes with a
//     fixed seed;
//   - the named-string-operand shape, compiled as its own pair, since
//     the check rewrites named string types too.
//
// It also pins the perf premise on the current toolchain (see
// TestEquiv_PS5020_AllocParity): go1.22+ escape analysis turns the
// Before's non-escaping, never-mutated conversion into a zero-copy
// alias of the string, so on gc today the pair is allocation-identical
// parity — the win the Doc claims lives on toolchains without that
// optimization and in refactors that hoist the conversion, which is
// exactly what the benchmark pair in benchmarks/ps5020_test.go
// documents.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// ps5020Before is the exact Before-shape of the check.
func ps5020Before(dst []byte, s string) []byte {
	return append(dst, []byte(s)...)
}

// ps5020After is the exact After-shape of the check.
func ps5020After(dst []byte, s string) []byte {
	return append(dst, s...)
}

type ps5020Str string

// ps5020BeforeNamed / ps5020AfterNamed are the named-string-operand
// shapes: the check rewrites those too, and the pair proves the After
// compiles and agrees for a named string type.
func ps5020BeforeNamed(dst []byte, s ps5020Str) []byte {
	return append(dst, []byte(s)...)
}

func ps5020AfterNamed(dst []byte, s ps5020Str) []byte {
	return append(dst, s...)
}

// ps5020Dst describes one destination state: a backing array of backing
// bytes with the slice header [:length:capacity] over it. capacity == 0
// with backing nil yields a nil dst.
type ps5020Dst struct {
	name    string
	backing []byte // prototype backing array content; len(backing) == cap
	length  int
}

// ps5020Make materializes an INDEPENDENT destination from the
// prototype: append may write into spare capacity, so the Before and
// After runs must each get their own backing array.
func (d ps5020Dst) ps5020Make() (dst, base []byte) {
	if d.backing == nil {
		return nil, nil
	}
	base = make([]byte, len(d.backing))
	copy(base, d.backing)
	return base[:d.length], base
}

func ps5020Check(t *testing.T, d ps5020Dst, s string) {
	t.Helper()
	dstB, baseB := d.ps5020Make()
	dstA, baseA := d.ps5020Make()
	before := ps5020Before(dstB, s)
	after := ps5020After(dstA, s)
	ps5020Compare(t, d, s, before, after, baseB, baseA)

	// The named-string pair must agree with itself the same way.
	dstB, baseB = d.ps5020Make()
	dstA, baseA = d.ps5020Make()
	before = ps5020BeforeNamed(dstB, ps5020Str(s))
	after = ps5020AfterNamed(dstA, ps5020Str(s))
	ps5020Compare(t, d, s, before, after, baseB, baseA)
}

func ps5020Compare(t *testing.T, d ps5020Dst, s string, before, after, baseB, baseA []byte) {
	t.Helper()
	if !bytes.Equal(before, after) {
		t.Fatalf("content divergence on dst=%s s=%q:\n before=%q\n after=%q", d.name, s, before, after)
	}
	if len(before) != len(after) || cap(before) != cap(after) {
		t.Fatalf("header divergence on dst=%s s=%q: before len=%d cap=%d, after len=%d cap=%d",
			d.name, s, len(before), cap(before), len(after), cap(after))
	}
	if (before == nil) != (after == nil) {
		t.Fatalf("nil-ness divergence on dst=%s s=%q: before==nil is %v, after==nil is %v",
			d.name, s, before == nil, after == nil)
	}
	// Every byte of the original backing array must end up identical:
	// an in-place append writes into spare capacity, and both forms
	// must do exactly the same writes (or, on growth, no writes at all).
	if !bytes.Equal(baseB, baseA) {
		t.Fatalf("backing-array divergence on dst=%s s=%q:\n before base=%q\n after base=%q", d.name, s, baseB, baseA)
	}
	// The in-place/growth decision must match: the result aliases its
	// input backing array in the Before iff it does in the After.
	aliasB := len(before) > 0 && len(baseB) > 0 && &before[0] == &baseB[0]
	aliasA := len(after) > 0 && len(baseA) > 0 && &after[0] == &baseA[0]
	if aliasB != aliasA {
		t.Fatalf("aliasing divergence on dst=%s s=%q: before in-place=%v, after in-place=%v", d.name, s, aliasB, aliasA)
	}
}

// ps5020Dsts covers the destination edges: nil, non-nil empty,
// exactly-full slices that force growth, and prefixes with spare
// capacity so the append lands in the existing backing array.
func ps5020Dsts() []ps5020Dst {
	return []ps5020Dst{
		{name: "nil", backing: nil, length: 0},
		{name: "empty-nocap", backing: []byte{}, length: 0},
		{name: "empty-cap8", backing: []byte("________"), length: 0},
		{name: "full-len3", backing: []byte("abc"), length: 3},
		{name: "prefix-cap8", backing: []byte("ab______"), length: 2},
		{name: "prefix-cap64", backing: bytes.Repeat([]byte{'~'}, 64), length: 5},
	}
}

// TestEquiv_PS5020_ExhaustiveShort crosses every string of length <= 3
// over an adversarial alphabet with every destination state.
func TestEquiv_PS5020_ExhaustiveShort(t *testing.T) {
	t.Parallel()
	// 'a' and NUL for plain bytes, 0xC3/0xA9 (the two bytes of 'é') so
	// complete and truncated multi-byte sequences both arise, and 0xFF
	// (never valid in UTF-8).
	alphabet := []byte{'a', 0x00, 0xC3, 0xA9, 0xFF}
	var strs []string
	strs = append(strs, "")
	for _, b0 := range alphabet {
		strs = append(strs, string([]byte{b0}))
		for _, b1 := range alphabet {
			strs = append(strs, string([]byte{b0, b1}))
			for _, b2 := range alphabet {
				strs = append(strs, string([]byte{b0, b1, b2}))
			}
		}
	}
	for _, d := range ps5020Dsts() {
		for _, s := range strs {
			ps5020Check(t, d, s)
		}
	}
}

// TestEquiv_PS5020_TargetedSeeds pins the documented edges directly.
func TestEquiv_PS5020_TargetedSeeds(t *testing.T) {
	t.Parallel()
	seeds := []string{
		"",                                       // empty: zero elements appended, nil dst stays nil
		"a",                                      // single byte
		"héllo, wörld",                           // multi-byte runes
		"\uFFFD",                                 // the replacement rune itself
		"\xff\xfe\xfd",                           // raw invalid UTF-8
		"\xc3",                                   // truncated multi-byte sequence
		"nul\x00nul\x00",                         // interior NULs
		strings.Repeat("x", 32),                  // exactly the stack conversion buffer
		strings.Repeat("y", 33),                  // one past it (historical heap edge)
		strings.Repeat("payload\xffline; ", 256), // 4 KiB, invalid UTF-8 sprinkled in
	}
	for _, d := range ps5020Dsts() {
		for _, s := range seeds {
			ps5020Check(t, d, s)
		}
	}
	// s derived from dst's own bytes: string(dst) is a copy in both
	// pipelines, so even a self-append cannot diverge.
	d := ps5020Dst{name: "self", backing: []byte("self-content____________"), length: 12}
	dstB, baseB := d.ps5020Make()
	dstA, baseA := d.ps5020Make()
	before := ps5020Before(dstB, string(dstB))
	after := ps5020After(dstA, string(dstA))
	ps5020Compare(t, d, "self", before, after, baseB, baseA)
}

// TestEquiv_PS5020_Randomized runs full-byte-range strings against
// randomized destination shapes with a fixed seed.
func TestEquiv_PS5020_Randomized(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(0x5020))
	for range 100000 {
		s := make([]byte, rng.Intn(65))
		for i := range s {
			s[i] = byte(rng.Intn(256))
		}
		capacity := rng.Intn(80)
		length := 0
		if capacity > 0 {
			length = rng.Intn(capacity + 1)
		}
		backing := make([]byte, capacity)
		for i := range backing {
			backing[i] = byte(rng.Intn(256))
		}
		d := ps5020Dst{name: "rand", backing: backing, length: length}
		if capacity == 0 && rng.Intn(2) == 0 {
			d.backing = nil // exercise the nil dst too
		}
		ps5020Check(t, d, string(s))
	}
}

// TestEquiv_PS5020_AllocParity pins the perf premise on the current
// toolchain: go1.22+ escape analysis compiles the Before's conversion —
// non-escaping and never mutated, which the direct spread argument
// always is — to a zero-copy alias of the string, so with sufficient
// destination capacity BOTH forms run allocation-free and the pair is
// parity on gc today, exactly as the Doc and MeasuredWin state. If a
// future toolchain regresses this (the Before would allocate again),
// this test fails to force the Doc's honesty back in sync — and the
// After can never allocate more than the Before, since it does strictly
// less work.
func TestEquiv_PS5020_AllocParity(t *testing.T) {
	s := strings.Repeat("z", 1024) // far past any stack conversion buffer
	dst := make([]byte, 0, 2048)
	var sink []byte
	before := testing.AllocsPerRun(100, func() {
		sink = append(dst[:0], []byte(s)...)
	})
	after := testing.AllocsPerRun(100, func() {
		sink = append(dst[:0], s...)
	})
	if after != 0 {
		t.Fatalf("append(dst, s...) allocated %v times with spare capacity — the string-append special form must be allocation-free", after)
	}
	if before != after {
		t.Fatalf("alloc profile diverged: before %v, after %v — the zero-copy parity premise in PS5020's Doc no longer holds on this toolchain; re-measure and update the Doc", before, after)
	}
	_ = sink
}
