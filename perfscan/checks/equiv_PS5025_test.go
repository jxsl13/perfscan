package checks

// Runtime differential for PS5025: strings.LastIndexAny(s, cut) /
// bytes.LastIndexAny(b, cut) vs the LastIndexByte forms the fix emits —
// for every ONE-ASCII-BYTE cutset. The fix's safety argument is branch by
// branch over the stdlib implementations: the len(s) > 8 path builds
// makeASCIISet(cut) (which succeeds for an ASCII cutset) and runs a
// reverse as.contains(s[i]) loop — for a one-byte set exactly the
// s[i] == c comparison LastIndexByte performs; the reverse rune-decoding
// path (len(chars) == 1, target rune c < utf8.RuneSelf) relies on ASCII
// being self-synchronizing in UTF-8 — an ASCII byte is never a
// continuation byte, so utf8.DecodeLastRune yields rune c at exactly the
// byte offsets where s[i] == c, invalid UTF-8 decodes to RuneError
// (never equal to an ASCII target), and the backward walk finds the
// HIGHEST offset first; and the len(s) == 1 fast paths agree — both
// sides return 0 iff s[0] == c (a non-ASCII s[0] is remapped toward
// RuneError, which a one-ASCII-byte cutset never contains). This suite
// pins that claim over:
//
//   - EXHAUSTIVE pairs: every haystack of length <= 3 over an adversarial
//     alphabet (ASCII, NUL, the bytes of multi-byte runes so truncated
//     and complete sequences arise at every alignment, a bare
//     continuation byte, and 0xFF) crossed with ALL 128 ASCII cutset
//     bytes — presence, absence, repeats and boundary positions all
//     arise from the cross product, in both the string and []byte world;
//   - a LENGTH SWEEP across the len(s) > 8 branch boundary: every prefix
//     length 0..16 of adversarial buffers crossed with all 128 ASCII
//     cutset bytes, so the makeASCIISet path (> 8), the reverse
//     rune-decoding path (2..8), the len(s) == 1 fast path, and the
//     empty haystack are each pinned on both sides of the boundary;
//   - targeted seeds: nil and empty haystacks, needle only at the start,
//     only at the end, repeated needles, NUL needles inside invalid
//     UTF-8, ASCII needles adjacent to and between the bytes of split
//     multi-byte runes, and long haystacks;
//   - randomized long haystacks over the full byte range with a fixed
//     seed, probing present and often-absent ASCII bytes and both ends.
//
// It also pins WHY the < 0x80 bound is load-bearing with a concrete
// divergence witness (a single-byte cutset >= 0x80 makes LastIndexAny
// search for the LAST invalid-UTF-8 position, not the byte), and the
// perf premise that NO side allocates.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// ps5025Before* are the exact Before-shapes of the check: a one-ASCII-byte
// cutset through LastIndexAny in both packages.
func ps5025BeforeStr(s, cut string) int        { return strings.LastIndexAny(s, cut) }
func ps5025BeforeByt(b []byte, cut string) int { return bytes.LastIndexAny(b, cut) }

// ps5025After* are the exact After-shapes of the check: LastIndexByte with
// the cutset literal indexed as cut[0], a drop-in call returning the same
// int.
func ps5025AfterStr(s, cut string) int        { return strings.LastIndexByte(s, cut[0]) }
func ps5025AfterByt(b []byte, cut string) int { return bytes.LastIndexByte(b, cut[0]) }

func ps5025Check(t *testing.T, b []byte, c byte) {
	t.Helper()
	if c >= 0x80 {
		t.Fatalf("harness bug: cutset byte %#02x is not ASCII — outside the check's scope", c)
	}
	cut := string([]byte{c})
	s := string(b)

	want := ps5025AfterStr(s, cut)
	if got := ps5025BeforeStr(s, cut); got != want {
		t.Fatalf("strings.LastIndexAny divergence on (%q, %q): before=%d after=%d", s, cut, got, want)
	}
	if got := ps5025AfterByt(b, cut); got != want {
		t.Fatalf("bytes/strings LastIndexByte disagreement on (%q, %q): bytes=%d strings=%d", b, cut, got, want)
	}
	if got := ps5025BeforeByt(b, cut); got != want {
		t.Fatalf("bytes.LastIndexAny divergence on (%q, %q): before=%d after=%d", b, cut, got, want)
	}
}

// ps5025Alphabet stresses every comparison hazard: ASCII, NUL, the bytes
// of the two-byte U+00E9 (C3 A9) and three-byte U+3042 (E3 81 82) so
// truncated and complete multi-byte sequences arise at every alignment
// (exercising the RuneError-adjacent decode paths the ASCII bound steers
// clear of), a bare continuation byte, and 0xFF (never valid UTF-8).
var ps5025Alphabet = []byte{'a', 'z', 0x00, 0xC3, 0xA9, 0xE3, 0x81, 0x82, 0xFF}

func TestEquiv_PS5025_ExhaustiveShort(t *testing.T) {
	var enumerate func(buf []byte, depth int, emit func([]byte))
	enumerate = func(buf []byte, depth int, emit func([]byte)) {
		if depth == len(buf) {
			emit(bytes.Clone(buf))
			return
		}
		for _, c := range ps5025Alphabet {
			buf[depth] = c
			enumerate(buf, depth+1, emit)
		}
	}
	var haystacks [][]byte
	for n := 0; n <= 3; n++ {
		enumerate(make([]byte, n), 0, func(b []byte) { haystacks = append(haystacks, b) })
	}
	for _, b := range haystacks {
		// ALL 128 ASCII cutset bytes (the check's exact scope): absent,
		// present once, present repeatedly, at either boundary — every
		// combination arises from the cross product. The one-byte
		// haystacks pin both len(s) == 1 fast paths, including their
		// s[0] >= 0x80 remap branches against an ASCII cutset.
		for c := 0; c < 0x80; c++ {
			ps5025Check(t, b, byte(c))
		}
	}
}

// TestEquiv_PS5025_LengthSweep pins the len(s) > 8 branch boundary: below
// it strings.LastIndexAny decodes runes backward, above it it builds the
// ASCII set and scans raw bytes — both must agree with LastIndexByte at
// every length. Every prefix length 0..16 of adversarial buffers is
// crossed with all 128 ASCII cutset bytes.
func TestEquiv_PS5025_LengthSweep(t *testing.T) {
	bufs := [][]byte{
		[]byte("a/b/c=d/e:f.g-h!"),                        // ASCII with repeated separators
		[]byte("z\x00z\xC3\xA9z\xE3\x81\x82z\xFFz\x81za"), // needles interleaved with multi-byte, invalid and NUL bytes
		bytes.Repeat([]byte{'/'}, 17),                     // the needle everywhere
		{0xC3, 0xA9, 0xE3, 0x81, 0x82, 0xFF, 0x81, 0xC3, 0x28, 'x', 0xC3, 0xA9, 0xE3, 0x81, 0x82, 0xFF, 0x81}, // almost no ASCII at all
	}
	for _, buf := range bufs {
		for n := 0; n <= len(buf); n++ {
			for c := 0; c < 0x80; c++ {
				ps5025Check(t, buf[:n], byte(c))
			}
		}
	}
}

func TestEquiv_PS5025_TargetedSeeds(t *testing.T) {
	long := bytes.Repeat([]byte("payload="), 4096)
	seeds := [][]byte{
		nil,
		{},
		[]byte("z"),
		[]byte("\xff"), // one-byte NON-UTF-8 haystack: both len(s)==1 RuneError-remap branches
		[]byte("za"), []byte("az"), []byte("zz"),
		[]byte("zaz"), []byte("aza"),
		[]byte("only one z here"),
		[]byte("z at both ends and in the z middle z"),
		[]byte("no needle at all"),
		[]byte("\x00\x00a\x00"),                         // NUL runs
		[]byte("\xff\xfe\xfd"),                          // invalid UTF-8 throughout
		[]byte("z\xc3\xa9\xc3\xa9"),                     // needle before complete multi-byte runes (backward decode crosses them first)
		[]byte("\xc3z\xa9"),                             // needle between the bytes of a split rune
		[]byte("éz…z€"),                                 // multi-byte runes around the needle
		bytes.Repeat([]byte("z"), 1000),                 // needle everywhere
		long,                                            // long, needle absent
		append(bytes.Clone(long), 'z'),                  // long, needle only at the very end
		append([]byte("z"), long...),                    // long, needle only at the very start
		append(append(bytes.Clone(long), 'z'), long...), // long, needle in the middle
	}
	probes := []byte{'z', 'a', '=', 0x00, 0x7F, '/'}
	for _, b := range seeds {
		for _, c := range probes {
			ps5025Check(t, b, c)
		}
	}
}

func TestEquiv_PS5025_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25025025))
	randBytes := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		return b
	}
	for range 20000 {
		b := randBytes(rng.Intn(257))
		// A random ASCII byte (sometimes present), a byte from a narrow
		// range (often absent), and — when b starts/ends ASCII — its
		// boundary bytes (guaranteed hits at the boundaries).
		ps5025Check(t, b, byte(rng.Intn(0x80)))
		ps5025Check(t, b, byte(rng.Intn(4)))
		if len(b) > 0 && b[0] < 0x80 {
			ps5025Check(t, b, b[0])
		}
		if len(b) > 0 && b[len(b)-1] < 0x80 {
			ps5025Check(t, b, b[len(b)-1])
		}
	}
}

// TestEquiv_PS5025_NonASCIIDivergenceWitness pins WHY the check's < 0x80
// cutset bound is load-bearing (and why single non-ASCII-byte cutsets are
// excluded entirely, not even advisory): for a one-byte cutset >= 0x80,
// LastIndexAny remaps the cutset rune to utf8.RuneError and returns the
// LAST INVALID-UTF-8 position — NOT the last occurrence of the byte. On
// the witness "\x28\xc3" (ASCII then a dangling two-byte lead) with
// cutset "\xff", LastIndexAny finds position 1 while LastIndexByte finds
// nothing. If the stdlib ever changed this behavior, this test — not a
// linter user — finds out.
func TestEquiv_PS5025_NonASCIIDivergenceWitness(t *testing.T) {
	// Both built at runtime: the invalid-UTF-8 CONSTANTS "\x28\xc3" and
	// "\xff" would (correctly) trip staticcheck's SA1011 — feeding
	// LastIndexAny invalid UTF-8 is exactly the point of this witness.
	s := string([]byte{0x28, 0xc3})
	cut := string([]byte{0xff})
	if got := strings.LastIndexAny(s, cut); got != 1 {
		t.Fatalf("strings.LastIndexAny(%q, %q) = %d, want 1 (RuneError remap): the stdlib changed — re-audit PS5025's ASCII bound", s, cut, got)
	}
	if got := strings.LastIndexByte(s, cut[0]); got != -1 {
		t.Fatalf("strings.LastIndexByte(%q, %#02x) = %d, want -1", s, cut[0], got)
	}
	if got := bytes.LastIndexAny([]byte(s), cut); got != 1 {
		t.Fatalf("bytes.LastIndexAny(%q, %q) = %d, want 1 (RuneError remap): the stdlib changed — re-audit PS5025's ASCII bound", s, cut, got)
	}
	if got := bytes.LastIndexByte([]byte(s), cut[0]); got != -1 {
		t.Fatalf("bytes.LastIndexByte(%q, %#02x) = %d, want -1", s, cut[0], got)
	}
}

// TestEquiv_PS5025_BothSidesZeroAlloc pins the perf premise: neither side
// allocates (the cutset is a string constant; the makeASCIISet bitset the
// Before side builds lives on the stack) — the entire win is instruction
// count: the set build, the rune decoding, and the call-frame chain.
func TestEquiv_PS5025_BothSidesZeroAlloc(t *testing.T) {
	s := "service=checkout region=eu-west-1 shard=07 status=ok final=z"
	b := []byte(s)
	var sink int
	allocs := testing.AllocsPerRun(100, func() {
		sink = strings.LastIndexAny(s, "z")
		sink += bytes.LastIndexAny(b, "z")
		sink += strings.LastIndexByte(s, "z"[0])
		sink += bytes.LastIndexByte(b, "z"[0])
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the LastIndexAny/LastIndexByte quartet allocated %v times per run; PS5025's like-for-like premise no longer holds", allocs)
	}
}
