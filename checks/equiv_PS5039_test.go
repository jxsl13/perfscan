package checks

// Runtime differential for PS5039: append(dst, string(r)...) vs the
// utf8.AppendRune(dst, r) form the fix emits. The safety claim is that
// the APPENDED BYTES are identical for EVERY int32 value — string(r)
// and utf8.AppendRune dispatch to the same UTF-8 encoding, and both map
// every invalid rune (negative, the surrogate range 0xD800-0xDFFF,
// values above 0x10FFFF) to U+FFFD — and that dst is written the same
// way: in place, preserving capacity, when it has room. This suite pins:
//
//   - byte identity over the whole valid rune space plus adversarial
//     margins (an exhaustive sweep of [-0x10000, 0x120000)), the int32
//     extremes, and a strided sweep of the ENTIRE int32 range;
//   - the widened forms the fix emits for the narrower widths: byte is
//     pinned directly (vet-exempt); for int8/int16/uint16 go vet's
//     stringintconv forbids spelling string(x) here, so — exactly as
//     TestEquiv_PS2139WriteStringRune reduces it — the per-width claim
//     is pinned as rune(x) being value-preserving over each admitted
//     width, which composes with the exhaustive rune-form sweep;
//   - the load-bearing WIDTH gate: for int64(0x100000041) the spec
//     makes string(x) yield "�" while the rewrite's rune(x) would
//     truncate to 'A' — pinned via its two halves (stringintconv again
//     forbids the direct wide spelling);
//   - the in-place path: with spare capacity both spellings write the
//     SAME backing array and preserve its capacity exactly;
//   - the growth path: through noinline wrappers (the pure runtime
//     growslice path) length AND capacity agree from identical starting
//     slices. In INLINED contexts the capacity of a freshly grown
//     slice is an unspecified implementation detail (the compiler may
//     stack-place or round the new backing array differently per
//     spelling) — the accepted PS2112 class PS5017 also documents — so
//     only bytes and length are pinned there;
//   - the perf premise: the After side never allocates when dst has
//     room.

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

// ps5039EquivBefore/After are the exact Before/After shapes of the fix.
func ps5039EquivBefore(dst []byte, r rune) []byte { return append(dst, string(r)...) }
func ps5039EquivAfter(dst []byte, r rune) []byte  { return utf8.AppendRune(dst, r) }

// The byte-operand form the fix emits (byte is stringintconv-exempt, so
// the direct identity is spellable and pinned for it).
func ps5039EquivBeforeByte(dst []byte, c byte) []byte { return append(dst, string(c)...) }
func ps5039EquivAfterByte(dst []byte, c byte) []byte  { return utf8.AppendRune(dst, rune(c)) }

// Noinline twins: the pure runtime growth path, where capacity is
// decided by growslice alone and must agree between the spellings.
//
//go:noinline
func ps5039EquivBeforeNoinline(dst []byte, r rune) []byte { return append(dst, string(r)...) }

//go:noinline
func ps5039EquivAfterNoinline(dst []byte, r rune) []byte { return utf8.AppendRune(dst, r) }

func ps5039EquivCheck(t *testing.T, r rune) {
	t.Helper()
	before := ps5039EquivBefore(nil, r)
	after := ps5039EquivAfter(nil, r)
	if !bytes.Equal(before, after) {
		t.Fatalf("rune %#x: append(nil, string(r)...)=%q != utf8.AppendRune(nil, r)=%q", r, before, after)
	}
}

func TestEquiv_PS5039AppendStringRune(t *testing.T) {
	// Exhaustive over the valid rune space plus adversarial margins:
	// negatives, the surrogate range, and values above MaxRune must all
	// encode as U+FFFD in both forms.
	for r := rune(-0x10000); r < 0x120000; r++ {
		ps5039EquivCheck(t, r)
	}
	// The int32 extremes, then a strided sweep of the ENTIRE int32 range.
	for _, r := range []rune{math.MinInt32, math.MinInt32 + 1, -1 << 24, 1 << 30, math.MaxInt32 - 1, math.MaxInt32} {
		ps5039EquivCheck(t, r)
	}
	for i := int64(math.MinInt32); i <= math.MaxInt32; i += 65537 {
		ps5039EquivCheck(t, rune(i))
	}

	// The byte form directly; the other narrow widths reduce to rune(x)
	// being value-preserving (see the file comment).
	for v := 0; v < 256; v++ {
		c := byte(v)
		if b, a := ps5039EquivBeforeByte(nil, c), ps5039EquivAfterByte(nil, c); !bytes.Equal(b, a) {
			t.Fatalf("byte %d: %q != %q", v, b, a)
		}
	}
	for i := -32768; i <= 65535; i++ {
		if i >= -128 && i <= 127 {
			if x := int8(i); int64(rune(x)) != int64(x) {
				t.Fatalf("rune(int8 %d) not value-preserving", i)
			}
		}
		if i >= -32768 && i <= 32767 {
			if x := int16(i); int64(rune(x)) != int64(x) {
				t.Fatalf("rune(int16 %d) not value-preserving", i)
			}
		}
		if i >= 0 {
			if x := uint16(i); int64(rune(x)) != int64(x) {
				t.Fatalf("rune(uint16 %d) not value-preserving", i)
			}
		}
	}

	// The WIDTH gate is load-bearing: string(int64(0x100000041)) is
	// "�" by the spec's full-value conversion, while the rewrite's
	// rune(x) truncates to 'A'. stringintconv forbids the direct wide
	// spelling here, so the divergence is pinned via its two halves.
	wide := int64(0x100000041)
	if got := string(rune(wide)); got != "A" {
		t.Fatalf("string(rune(int64(0x100000041)))=%q, expected the truncated %q", got, "A")
	}
	if got := string(rune(0x110000)); got != "�" {
		t.Fatalf("string(out-of-range rune)=%q, expected %q — the spec's U+FFFD conversion changed?!", got, "�")
	}

	// Appending onto existing content, not just nil.
	seeds := [][]byte{nil, {}, []byte("abc"), []byte("héllo \xff\x80 日本語")}
	for _, seed := range seeds {
		for _, r := range []rune{'a', 0, 0x7F, 0x80, 0x7FF, 0x800, 0xD7FF, 0xD800, 0xDFFF, 0xE000, 0xFFFF, 0x10000, 0x10FFFF, 0x110000, -1, math.MinInt32, math.MaxInt32} {
			b := ps5039EquivBefore(append([]byte(nil), seed...), r)
			a := ps5039EquivAfter(append([]byte(nil), seed...), r)
			if !bytes.Equal(b, a) {
				t.Fatalf("seed %q rune %#x: %q != %q", seed, r, b, a)
			}
		}
	}

	// In-place path: with spare capacity both spellings write the SAME
	// backing array and preserve its capacity exactly.
	for _, r := range []rune{'a', 0xE9, 0x65E5, 0x10FFFF, -1, 0xD800, 0x110000} {
		back1 := make([]byte, 16)
		back2 := make([]byte, 16)
		b := ps5039EquivBefore(back1[:3:16], r)
		a := ps5039EquivAfter(back2[:3:16], r)
		if cap(b) != 16 || cap(a) != 16 || &b[0] != &back1[0] || &a[0] != &back2[0] {
			t.Fatalf("rune %#x: in-place append diverged (capB=%d capA=%d)", r, cap(b), cap(a))
		}
		if !bytes.Equal(b, a) {
			t.Fatalf("rune %#x: in-place bytes diverged: %q != %q", r, b, a)
		}
	}

	// Growth path through the noinline twins: the pure runtime growslice
	// path requests the same new length in both spellings, so length AND
	// capacity agree from identical starting slices.
	for _, r := range []rune{'a', 0xE9, 0x65E5, 0x10FFFF, -1} {
		for _, capN := range []int{0, 1, 2, 3, 4, 5, 8, 16, 100} {
			b := ps5039EquivBeforeNoinline(make([]byte, 0, capN), r)
			a := ps5039EquivAfterNoinline(make([]byte, 0, capN), r)
			if !bytes.Equal(b, a) || len(b) != len(a) || cap(b) != cap(a) {
				t.Fatalf("rune %#x from cap %d: before len=%d cap=%d %q | after len=%d cap=%d %q",
					r, capN, len(b), cap(b), b, len(a), cap(a), a)
			}
		}
	}

	// A realistic transcoding loop must produce identical buffers.
	input := strings.Repeat("naïve — 日本語テキスト mixed £𝄞😀 line �\n", 8) + "\xff tail"
	bufB := make([]byte, 0, 4*len(input))
	bufA := make([]byte, 0, 4*len(input))
	for _, r := range input {
		bufB = ps5039EquivBefore(bufB, r)
		bufA = ps5039EquivAfter(bufA, r)
	}
	if !bytes.Equal(bufB, bufA) {
		t.Fatalf("transcoding loop diverged: %q != %q", bufB, bufA)
	}
	if string(bufB) != strings.ToValidUTF8(input, "�") {
		t.Fatalf("transcoding loop lost the range-loop U+FFFD semantics")
	}
}

// TestEquiv_PS5039NoAllocs pins the perf premise: with room in dst the
// After side never allocates (the encoder writes straight into dst's
// backing array). The Before side's temporary string may or may not
// heap-allocate depending on escape analysis, so it is not pinned.
func TestEquiv_PS5039NoAllocs(t *testing.T) {
	dst := make([]byte, 0, 64)
	var sink []byte
	if n := testing.AllocsPerRun(100, func() { sink = utf8.AppendRune(dst[:0], '日') }); n != 0 {
		t.Errorf("utf8.AppendRune into spare capacity allocates: %v allocs/op", n)
	}
	_ = sink
}
