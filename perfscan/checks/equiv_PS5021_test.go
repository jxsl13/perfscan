package checks

// Runtime differential for PS5021: copy(dst, []byte(s)) vs copy(dst, s).
// The fix's safety argument is that copy copies min(len(dst), len(src))
// bytes and returns that count; len([]byte(s)) == len(s), so both forms
// copy the identical byte count, write the identical bytes (byte-level,
// no UTF-8 interpretation — invalid UTF-8 rounds through untouched) and
// return the identical int, while dst and s are each evaluated exactly
// once either way. This suite pins that claim over:
//
//   - EXHAUSTIVE short inputs: every string of length <= 3 over an
//     adversarial alphabet (NUL, ASCII, the three bytes of a multi-byte
//     rune so truncated and complete sequences both arise, and 0xFF),
//     crossed with every destination length 0..5, destinations
//     sentinel-filled so both the written prefix AND the untouched tail
//     are compared;
//   - targeted seeds: empty string into a non-empty dst, zero-length
//     dst, src shorter/equal/longer than dst, NUL-laden and invalid
//     UTF-8 strings, a named string type (the special case accepts any
//     string type), and a 64 KiB string into a small dst;
//   - randomized long inputs over the full byte range with a fixed seed
//     and random dst lengths;
//   - an OVERLAPPING source built with unsafe.String over the
//     destination's own backing array: copy is specified to handle
//     overlapping source and destination, and the Before's conversion
//     (zero-copy on gc >= 1.22, so genuinely aliasing dst; a fresh
//     snapshot elsewhere) must land exactly where the After's direct
//     overlapping memmove does — both are compared against an
//     independently snapshotted model.
//
// Every subtest checks the returned count against the min(len(dst),
// len(s)) model and the destination bytes against a model built by an
// explicit per-byte loop, so a divergence in either the count or any
// written (or wrongly-touched) byte fails.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"unsafe"
)

// ps5021Before is the exact Before-shape of the check.
func ps5021Before(dst []byte, s string) int {
	return copy(dst, []byte(s))
}

// ps5021After is the exact After-shape of the check.
func ps5021After(dst []byte, s string) int {
	return copy(dst, s)
}

// ps5021Model writes the spec's result into a fresh copy of dst with an
// explicit per-byte loop: n = min(len(dst), len(s)) bytes of s land in
// dst[:n], the tail is untouched, and n is returned.
func ps5021Model(dst []byte, s string) (int, []byte) {
	out := append([]byte(nil), dst...)
	n := min(len(dst), len(s))
	for i := 0; i < n; i++ {
		out[i] = s[i]
	}
	return n, out
}

// ps5021Check runs both shapes on identical sentinel-initialized
// destinations and fails on any divergence from the model or between the
// two forms.
func ps5021Check(t *testing.T, dst []byte, s string) {
	t.Helper()
	d1 := append([]byte(nil), dst...)
	d2 := append([]byte(nil), dst...)
	n1 := ps5021Before(d1, s)
	n2 := ps5021After(d2, s)
	wantN, want := ps5021Model(dst, s)
	if n1 != wantN || n2 != wantN {
		t.Fatalf("count diverged: dst=%q s=%q: before=%d after=%d want=%d", dst, s, n1, n2, wantN)
	}
	if !bytes.Equal(d1, want) {
		t.Fatalf("before bytes diverged: dst=%q s=%q: got %q want %q", dst, s, d1, want)
	}
	if !bytes.Equal(d2, want) {
		t.Fatalf("after bytes diverged: dst=%q s=%q: got %q want %q", dst, s, d2, want)
	}
}

// ps5021Sentinel returns a length-n destination filled with a sentinel
// byte, so untouched tail bytes are distinguishable from written ones.
func ps5021Sentinel(n int) []byte {
	d := make([]byte, n)
	for i := range d {
		d[i] = 0xA5
	}
	return d
}

func TestPS5021EquivalenceExhaustive(t *testing.T) {
	t.Parallel()
	// NUL, ASCII, the three bytes of 中 (0xE4 0xB8 0xAD — so complete
	// and truncated multi-byte sequences both arise), and 0xFF (never
	// valid UTF-8).
	alphabet := []byte{0x00, 'a', 0xE4, 0xB8, 0xAD, 0xFF}
	var srcs []string
	srcs = append(srcs, "")
	for _, a := range alphabet {
		srcs = append(srcs, string([]byte{a}))
		for _, b := range alphabet {
			srcs = append(srcs, string([]byte{a, b}))
			for _, c := range alphabet {
				srcs = append(srcs, string([]byte{a, b, c}))
			}
		}
	}
	for _, s := range srcs {
		for dstLen := 0; dstLen <= 5; dstLen++ {
			ps5021Check(t, ps5021Sentinel(dstLen), s)
		}
	}
}

func TestPS5021EquivalenceSeeds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		dstLen int
		s      string
	}{
		{0, ""},
		{0, "abc"},
		{8, ""},
		{8, "abc"},                          // src shorter
		{3, "abc"},                          // equal lengths
		{2, "abcdef"},                       // src longer
		{8, "\x00\x00\x00"},                 // NULs are copied, not terminators
		{8, "\xff\xfe\xfd"},                 // invalid UTF-8 rounds through
		{8, "\xe4\xb8"},                     // truncated multi-byte rune
		{16, "hello, 世界"},                   // multi-byte runes, byte-level
		{7, strings.Repeat("\x80", 7)},      // bare continuation bytes
		{5, strings.Repeat("x", 1<<16)},     // 64 KiB into a small dst
		{1 << 16, strings.Repeat("y", 100)}, // small src into a huge dst
	}
	for _, c := range cases {
		ps5021Check(t, ps5021Sentinel(c.dstLen), c.s)
	}
}

// ps5021Str pins the named-string-operand form: the builtin's special
// case accepts any string type, and the rewrite keeps the operand
// verbatim, so both forms see the identical value.
type ps5021Str string

func TestPS5021EquivalenceNamedString(t *testing.T) {
	t.Parallel()
	before := func(dst []byte, s ps5021Str) int { return copy(dst, []byte(s)) }
	after := func(dst []byte, s ps5021Str) int { return copy(dst, s) }
	for _, s := range []ps5021Str{"", "a", "\xff\x00\xe4", "hello, 世界"} {
		for dstLen := 0; dstLen <= 12; dstLen += 3 {
			d1, d2 := ps5021Sentinel(dstLen), ps5021Sentinel(dstLen)
			n1, n2 := before(d1, s), after(d2, s)
			wantN, want := ps5021Model(ps5021Sentinel(dstLen), string(s))
			if n1 != wantN || n2 != wantN || !bytes.Equal(d1, want) || !bytes.Equal(d2, want) {
				t.Fatalf("named string diverged: dst=%d s=%q: before=(%d,%q) after=(%d,%q) want=(%d,%q)",
					dstLen, s, n1, d1, n2, d2, wantN, want)
			}
		}
	}
}

func TestPS5021EquivalenceRandomized(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(0x5021))
	for i := 0; i < 200; i++ {
		src := make([]byte, rng.Intn(8192))
		rng.Read(src)
		dst := make([]byte, rng.Intn(8192))
		rng.Read(dst)
		ps5021Check(t, dst, string(src))
	}
}

// TestPS5021EquivalenceOverlap pins the aliasing claim: even when s is
// built (via unsafe.String) over the destination's OWN backing array —
// so the After's direct copy is an overlapping memmove, and on gc >= 1.22
// the Before's zero-copied conversion aliases dst just the same — both
// forms produce the bytes of an independent snapshot model, because copy
// is specified to behave as if the source were read out first.
func TestPS5021EquivalenceOverlap(t *testing.T) {
	t.Parallel()
	const base = "abcdefgh"
	for off := 0; off < len(base); off++ {
		for n := 1; off+n <= len(base); n++ {
			d1 := []byte(base)
			d2 := []byte(base)
			s1 := unsafe.String(&d1[off], n)
			s2 := unsafe.String(&d2[off], n)
			// Model from an immutable snapshot of the aliased window.
			snap := base[off : off+n]
			wantN, want := ps5021Model([]byte(base), snap)
			n1 := ps5021Before(d1, s1)
			n2 := ps5021After(d2, s2)
			if n1 != wantN || n2 != wantN || !bytes.Equal(d1, want) || !bytes.Equal(d2, want) {
				t.Fatalf("overlap diverged: off=%d n=%d: before=(%d,%q) after=(%d,%q) want=(%d,%q)",
					off, n, n1, d1, n2, d2, wantN, want)
			}
		}
	}
}
