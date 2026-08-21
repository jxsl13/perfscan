package checks

// Runtime differential for PS5005: string(bytes.Trim([]byte(s), cutset))
// vs strings.Trim(s, cutset), plus the TrimLeft/TrimRight siblings. The
// fix's safety argument is that both packages share the same trim
// machinery — the single-ASCII-byte fast path, makeASCIISet/contains for
// all-ASCII cutsets, and rune decoding that yields RuneError width 1 on
// any invalid sequence — and that the two rune-membership tests (bytes'
// range-based containsRune vs strings.ContainsRune/IndexRune) agree on
// every cutset, including cutsets holding invalid UTF-8. So the trim
// boundaries coincide and the resulting string VALUE is identical for
// every (s, cutset) pair. This suite pins that claim over:
//
//   - EXHAUSTIVE short inputs: every byte string of length <= 4 over an
//     adversarial alphabet (ASCII members and non-members, NUL, the
//     bytes of multi-byte cutset runes so truncated and complete
//     sequences both arise, a bare continuation byte, an invalid lead
//     byte, and 0xFF), crossed with cutsets exercising every membership
//     path: empty, single ASCII byte, multi-ASCII set, multi-byte
//     Unicode runes, U+FFFD itself, and raw invalid UTF-8;
//   - targeted seeds (fully-trimmed inputs where bytes historically
//     returns nil, boundary-straddling multi-byte runes, invalid UTF-8
//     on both sides, very long inputs);
//   - randomized long inputs over the full byte range with random
//     cutsets and a fixed seed.
//
// It also pins the perf premise: the strings twins perform zero
// allocations (they return a substring), which is the entire win.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// ps5005Before* are the exact Before-shapes of the check.
func ps5005BeforeTrim(s, cutset string) string  { return string(bytes.Trim([]byte(s), cutset)) }
func ps5005BeforeLeft(s, cutset string) string  { return string(bytes.TrimLeft([]byte(s), cutset)) }
func ps5005BeforeRight(s, cutset string) string { return string(bytes.TrimRight([]byte(s), cutset)) }

// ps5005After* are the exact After-shapes of the check.
func ps5005AfterTrim(s, cutset string) string  { return strings.Trim(s, cutset) }
func ps5005AfterLeft(s, cutset string) string  { return strings.TrimLeft(s, cutset) }
func ps5005AfterRight(s, cutset string) string { return strings.TrimRight(s, cutset) }

func ps5005Check(t *testing.T, s, cutset string) {
	t.Helper()
	if before, after := ps5005BeforeTrim(s, cutset), ps5005AfterTrim(s, cutset); before != after {
		t.Fatalf("Trim divergence on (%q, %q):\n before=%q\n after=%q", s, cutset, before, after)
	}
	if before, after := ps5005BeforeLeft(s, cutset), ps5005AfterLeft(s, cutset); before != after {
		t.Fatalf("TrimLeft divergence on (%q, %q):\n before=%q\n after=%q", s, cutset, before, after)
	}
	if before, after := ps5005BeforeRight(s, cutset), ps5005AfterRight(s, cutset); before != after {
		t.Fatalf("TrimRight divergence on (%q, %q):\n before=%q\n after=%q", s, cutset, before, after)
	}
}

// ps5005Cutsets exercises every membership path of the shared trim
// machinery.
var ps5005Cutsets = []string{
	"",                // both return s unchanged (bytes: after the len-0 nil special case)
	" ",               // single-ASCII-byte fast path
	"\x00",            // single NUL — still the byte fast path
	" \t\r\n.a",       // makeASCIISet path
	"é",               // single multi-byte rune: unicode path, ASCII lookups must miss its 0xC3 0xA9 bytes
	"aéあ",             // mixed ASCII + two-byte + three-byte runes
	"�",               // U+FFFD literally: must match every INVALID sequence in s, not the replacement char alone
	"\xff",            // invalid UTF-8 in the cutset: decodes to RuneError in both membership tests
	"\xa0",            // bare continuation byte as cutset
	"\xc3\x28",        // invalid two-byte sequence in the cutset
	"a\xffあ",          // valid and invalid cutset bytes interleaved
	"\u0085\u00a0 \t", // multi-byte spaces + ASCII: unicode path with ASCII members
}

func TestEquiv_PS5005_ExhaustiveShort(t *testing.T) {
	alphabet := []byte{
		' ', '\t', // ASCII cutset members
		'a', '.', // ASCII members/non-members depending on cutset
		0x00,       // NUL
		0xC3, 0xA9, // C3 A9 = U+00E9; each byte alone is invalid UTF-8
		0xE3, 0x81, 0x82, // E3 81 82 = U+3042; prefixes are truncated sequences
		0xA0, // bare continuation byte
		0xFF, // never valid in UTF-8
	}
	var buf [4]byte
	var rec func(n, depth int)
	rec = func(n, depth int) {
		if depth == n {
			s := string(buf[:n])
			for _, cutset := range ps5005Cutsets {
				ps5005Check(t, s, cutset)
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

func TestEquiv_PS5005_TargetedSeeds(t *testing.T) {
	long := strings.Repeat("payload/", 4096)
	seeds := []string{
		"",
		"-",
		"----", // trimmed to nothing: bytes historically returns nil, string(nil) == ""
		"--x--",
		"ééxé",                          // multi-byte cutset runes on both ends
		"あ日本あ",                          // three-byte members wrapping three-byte non-members
		"\xc3\x28",                      // invalid two-byte sequence
		"-\xc3\x28-",                    // invalid UTF-8 inside the trimmed span
		"\x80\x80",                      // bare continuation bytes only
		"-\xff\xfe\x80-",                // invalid bytes wrapped in members
		"\xc3",                          // truncated U+00E9 prefix at the very edge
		"\xc3-",                         // truncated sequence then a member
		"-\xc3",                         // member then truncated sequence
		"\xe3\x81",                      // truncated U+3042 prefix
		"a\x00b",                        // NUL inside the kept span
		"�x�",                           // literal replacement chars at the edges
		"//" + long + "//",              // long payload, trimmed ends
		strings.Repeat("-", 8192) + "x", // long all-member prefix
		"x" + strings.Repeat("あ", 2048), // long multi-byte-member suffix
		strings.Repeat("-é", 4096),      // long input trimmed to nothing on the unicode path
	}
	cutsets := append([]string{"-", "-/", "éあ-", "�-", "\xff-"}, ps5005Cutsets...)
	for _, s := range seeds {
		for _, cutset := range cutsets {
			ps5005Check(t, s, cutset)
		}
	}
}

func TestEquiv_PS5005_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25005005))
	randBytes := func(n int, pick func() byte) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = pick()
		}
		return string(b)
	}
	// Full byte range for both s and cutset: mostly invalid UTF-8 at
	// random alignments, cutsets that flip between the ASCII-set and
	// unicode membership paths at random.
	anyByte := func() byte { return byte(rng.Intn(256)) }
	for range 50000 {
		s := randBytes(rng.Intn(33), anyByte)
		cutset := randBytes(rng.Intn(7), anyByte)
		ps5005Check(t, s, cutset)
	}
	// Member-heavy alphabet so trims routinely straddle rune boundaries
	// of the multi-byte cutset runes.
	heavy := []byte{'-', ' ', 0xC3, 0xA9, 0xE3, 0x81, 0x82, 0xFF, 'x'}
	heavyByte := func() byte { return heavy[rng.Intn(len(heavy))] }
	for range 50000 {
		s := randBytes(rng.Intn(25), heavyByte)
		cutset := randBytes(1+rng.Intn(5), heavyByte)
		ps5005Check(t, s, cutset)
	}
}

// TestEquiv_PS5005_AfterIsZeroAlloc pins the perf premise: the rewritten
// forms allocate nothing (they return a substring of the input), which
// is exactly the win the check promises. The Before-form's copies are
// measured in benchmarks/ps5005_test.go rather than asserted here, since
// escape analysis could legitimately elide them in some shapes.
func TestEquiv_PS5005_AfterIsZeroAlloc(t *testing.T) {
	s := "--== a trimmed payload that is clearly long enough to heap-allocate ==--"
	const cutset = "-= "
	var sink string
	allocs := testing.AllocsPerRun(100, func() {
		sink = strings.Trim(s, cutset)
		sink = strings.TrimLeft(s, cutset)
		sink = strings.TrimRight(s, cutset)
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the strings Trim family allocated %v times per run; the zero-copy premise of PS5005 no longer holds", allocs)
	}
}
