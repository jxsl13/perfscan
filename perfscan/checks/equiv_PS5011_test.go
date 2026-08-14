package checks

// Runtime differential for PS5011:
// string(bytes.Replace/ReplaceAll([]byte(s), []byte(old), []byte(new), n))
// vs strings.Replace/ReplaceAll(s, old, new, n). The fix's safety
// argument is that []byte(x) round-trips every operand byte-exactly and
// that the two packages run the same literal byte-substring
// search-and-splice algorithm — no rune decoding for matching, no case
// folding, no normalization — agreeing on the n semantics (negative
// means all, zero means none), the empty-old rune-insertion shape (both
// count runes with identical invalid-UTF-8 treatment: RuneError width
// 1), overlap handling (the search resumes after a consumed match), and
// the no-match and fully-replaced extremes. So the produced string VALUE
// is identical for every (s, old, new, n). This suite pins that claim
// over:
//
//   - EXHAUSTIVE short inputs: every byte string of length <= 3 over an
//     adversarial alphabet (ASCII, NUL, the bytes of multi-byte runes so
//     truncated and complete sequences both arise, a bare continuation
//     byte, and 0xFF), crossed with old/new sets holding empty strings,
//     single and multi-byte ASCII, multi-byte runes, U+FFFD itself, raw
//     invalid UTF-8, and self-overlapping patterns, crossed with every n
//     regime (negative, zero, fewer than / exactly / more than the
//     match count);
//   - targeted seeds (empty-old insertion over invalid UTF-8, matches
//     straddling nothing / everything, old == new, len(old) == len(new)
//     byte-swap shape, very long inputs with periodic matches);
//   - randomized long inputs over the full byte range with a fixed
//     seed.
//
// It also pins the no-match fast path that is part of the perf premise:
// strings.Replace/ReplaceAll return s itself with zero allocations when
// there is nothing to do, where the bytes form pays two full copies.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// ps5011Before* are the exact Before-shapes of the check.
func ps5011BeforeReplace(s, old, new string, n int) string {
	return string(bytes.Replace([]byte(s), []byte(old), []byte(new), n))
}

func ps5011BeforeReplaceAll(s, old, new string) string {
	return string(bytes.ReplaceAll([]byte(s), []byte(old), []byte(new)))
}

// ps5011After* are the exact After-shapes of the check.
func ps5011AfterReplace(s, old, new string, n int) string {
	return strings.Replace(s, old, new, n)
}

func ps5011AfterReplaceAll(s, old, new string) string {
	return strings.ReplaceAll(s, old, new)
}

func ps5011Check(t *testing.T, s, old, new string, n int) {
	t.Helper()
	if before, after := ps5011BeforeReplace(s, old, new, n), ps5011AfterReplace(s, old, new, n); before != after {
		t.Fatalf("Replace divergence on (%q, %q, %q, %d):\n before=%q\n after=%q", s, old, new, n, before, after)
	}
	if n < 0 {
		if before, after := ps5011BeforeReplaceAll(s, old, new), ps5011AfterReplaceAll(s, old, new); before != after {
			t.Fatalf("ReplaceAll divergence on (%q, %q, %q):\n before=%q\n after=%q", s, old, new, before, after)
		}
	}
}

// ps5011Pairs exercises every matching path: the empty-old rune
// insertion, single-byte and multi-byte literals, self-overlapping
// patterns, U+FFFD vs raw invalid UTF-8, and NUL.
var ps5011Pairs = []string{
	"",         // empty old: insert-between-every-rune; empty new: pure deletion
	"a",        // single ASCII byte
	"aa",       // self-overlapping candidate
	"ab",       // multi-byte ASCII literal
	"\x00",     // NUL
	"é",        // two-byte rune
	"あ",        // three-byte rune
	"�",        // U+FFFD literally: must NOT match raw invalid sequences
	"\xff",     // invalid UTF-8
	"\xc3",     // lead byte of é alone — a truncated sequence
	"\xc3\xa9", // the é bytes spelled raw
	"ZZ",       // longer replacement than most inputs
}

func TestEquiv_PS5011_ExhaustiveShort(t *testing.T) {
	alphabet := []byte{
		'a', 'b', // ASCII match/non-match material
		0x00,       // NUL
		0xC3, 0xA9, // C3 A9 = U+00E9; each byte alone is invalid UTF-8
		0xFF, // never valid in UTF-8
	}
	ns := []int{-2, -1, 0, 1, 2, 5}
	var buf [3]byte
	var rec func(n, depth int)
	rec = func(n, depth int) {
		if depth == n {
			s := string(buf[:n])
			for _, old := range ps5011Pairs {
				for _, new := range ps5011Pairs {
					for _, k := range ns {
						ps5011Check(t, s, old, new, k)
					}
				}
			}
			return
		}
		for _, c := range alphabet {
			buf[depth] = c
			rec(n, depth+1)
		}
	}
	for n := 0; n <= len(buf); n++ {
		rec(n, 0)
	}
}

func TestEquiv_PS5011_TargetedSeeds(t *testing.T) {
	long := strings.Repeat("key=val;", 4096)
	type seed struct{ s, old, new string }
	seeds := []seed{
		{"", "", "-"},                            // empty everything: one insertion
		{"aaa", "a", "bb"},                       // every byte replaced, result grows
		{"aaaa", "aa", "b"},                      // overlap: two non-overlapping matches, not three
		{"hello world", "o", "0"},                // plain
		{"hello", "x", "y"},                      // NO match: the fast-path parity case
		{"aXbXc", "X", ""},                       // deletion
		{"aXbXc", "X", "X"},                      // old == new
		{"swap", "a", "o"},                       // len(old) == len(new) == 1 byte swap
		{"\xff\xfe", "", "."},                    // empty old over invalid UTF-8: RuneError width 1 each
		{"\xc3\xa9\xc3", "\xc3", "?"},            // raw lead byte matches inside AND after a valid rune
		{"straße", "ß", "ss"},                    // multi-byte old, ASCII new
		{"a\x00b\x00c", "\x00", " "},             // NUL replacement
		{"ééé", "é", "e"},                        // adjacent multi-byte matches
		{"�x�", "�", "!"},                        // literal U+FFFD only, raw invalid must not match
		{long, "key", "KEY"},                     // long input, periodic matches
		{long, ";", ""},                          // long input, shrinking result
		{long, "absent", "-"},                    // long input, no match
		{strings.Repeat("ab", 8192), "ab", "ba"}, // long full-coverage replacement
	}
	ns := []int{-1, 0, 1, 2, 3, 1 << 20}
	for _, sd := range seeds {
		for _, n := range ns {
			ps5011Check(t, sd.s, sd.old, sd.new, n)
		}
	}
}

func TestEquiv_PS5011_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25011))
	randBytes := func(n int, pick func() byte) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = pick()
		}
		return string(b)
	}
	// Full byte range for all three operands: mostly invalid UTF-8 at
	// random alignments, old lengths flipping between the empty-old rune
	// path and the substring-search path at random.
	anyByte := func() byte { return byte(rng.Intn(256)) }
	for range 50000 {
		s := randBytes(rng.Intn(40), anyByte)
		old := randBytes(rng.Intn(4), anyByte)
		new := randBytes(rng.Intn(5), anyByte)
		ps5011Check(t, s, old, new, rng.Intn(8)-2)
	}
	// Match-heavy alphabet so replacements routinely hit, overlap, and
	// straddle multi-byte rune boundaries.
	heavy := []byte{'a', 'b', 0xC3, 0xA9, 0xFF}
	heavyByte := func() byte { return heavy[rng.Intn(len(heavy))] }
	for range 50000 {
		s := randBytes(rng.Intn(30), heavyByte)
		old := randBytes(rng.Intn(3), heavyByte)
		new := randBytes(rng.Intn(4), heavyByte)
		ps5011Check(t, s, old, new, rng.Intn(8)-2)
	}
}

// TestEquiv_PS5011_NoMatchFastPathIsZeroAlloc pins the perf premise's
// strongest point: when old does not occur, the rewritten forms return s
// itself with zero allocations, where the Before-shape pays a copy of s
// into []byte AND a copy back into a fresh string. The general Before/
// After costs are measured in benchmarks/ps5011_test.go.
func TestEquiv_PS5011_NoMatchFastPathIsZeroAlloc(t *testing.T) {
	s := "a payload that is comfortably long enough to heap-allocate on copy"
	var sink string
	allocs := testing.AllocsPerRun(100, func() {
		sink = strings.ReplaceAll(s, "absent", "-")
		sink = strings.Replace(s, "absent", "-", -1)
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the strings Replace no-match fast path allocated %v times per run; the fast-path premise of PS5011 no longer holds", allocs)
	}
}
