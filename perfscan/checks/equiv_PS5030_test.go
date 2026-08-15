package checks

// Runtime differential for PS5030: strings.IndexAny(s, cut) /
// strings.ContainsAny(s, cut) vs the strings.IndexRune(s, r) /
// strings.ContainsRune(s, r) forms the fix emits — for every cutset that
// is a single valid multi-byte rune (2..4 bytes of UTF-8, not
// utf8.RuneError). The fix's safety argument: IndexAny's general
// fallback (`for i, c := range s { if IndexRune(chars, c) >= 0 ... }`)
// returns the first range offset whose DECODED rune equals r — no other
// haystack rune can probe successfully into the one-rune cutset, because
// every byte of r's encoding is >= 0x80 (an ASCII c finds nothing),
// UTF-8 is self-synchronizing (no other rune's encoding is a substring
// of r's), and an invalid haystack byte decodes to RuneError, which is
// not in the cutset. IndexRune(s, r) returns the first BYTE offset of
// r's encoding; the two coincide because r's lead byte is never a
// continuation byte (so no valid rune of s can span across an occurrence
// of r) and Go's decoder consumes exactly one byte per invalid sequence
// (so `range s` always lands a fresh decode exactly on the occurrence's
// start). ContainsAny is IndexAny >= 0 and ContainsRune is IndexRune >=
// 0, so the membership twins inherit the identity verbatim. This suite
// pins that claim over:
//
//   - EXHAUSTIVE pairs: every haystack of length <= 4 over an
//     adversarial alphabet (ASCII, NUL, all the bytes of 2-, 3- and
//     4-byte encodings so truncated, complete and misaligned sequences
//     arise at every position, bare continuation bytes, and 0xFF)
//     crossed with 2-, 3- and 4-byte cutset runes, printable and not,
//     including U+10FFFF — presence, absence, prefixes-only and
//     boundary positions all arise from the cross product;
//   - targeted seeds: empty haystacks, the rune only at the start, only
//     at the end, repeated, truncated copies of its own encoding right
//     before a real one, its encoding bytes scattered as invalid UTF-8,
//     last-byte-heavy haystacks that force IndexRune's backward-compare
//     scan into its fallback, and long haystacks crossing the SIMD
//     block boundaries;
//   - randomized long haystacks over the full byte range with a fixed
//     seed, with the rune's encoding sometimes spliced in.
//
// It also pins the guards' load-bearing exclusions: the documented
// RuneError contract that keeps "�" cutsets out (today the two
// forms coincide even there — asserted, so a stdlib change is caught by
// a test, not a user), and the perf premise that NO side allocates.

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

// ps5030Before* are the exact Before-shapes of the check; ps5030After*
// are the exact After-shapes the fix emits (call-for-call, including
// under a ! that the fix leaves untouched).
func ps5030BeforeIndex(s, cut string) int       { return strings.IndexAny(s, cut) }
func ps5030BeforeContains(s, cut string) bool   { return strings.ContainsAny(s, cut) }
func ps5030BeforeNot(s, cut string) bool        { return !strings.ContainsAny(s, cut) }
func ps5030AfterIndex(s string, r rune) int     { return strings.IndexRune(s, r) }
func ps5030AfterContains(s string, r rune) bool { return strings.ContainsRune(s, r) }
func ps5030AfterNot(s string, r rune) bool      { return !strings.ContainsRune(s, r) }

// ps5030Runes are the cutset runes the differential crosses with every
// haystack: 2-byte (é, ß, the non-printable NBSP), 3-byte (the em dash,
// あ, €, the non-printable zero-width space), and 4-byte (😀 and the
// maximal U+10FFFF) — every encoding length, printable and not.
var ps5030Runes = []rune{
	0x00E9,   // é      (2 bytes: C3 A9)
	0x00DF,   // ß      (2 bytes: C3 9F)
	0x00A0,   // NBSP   (2 bytes: C2 A0, non-printable)
	0x2014,   // —      (3 bytes: E2 80 94)
	0x3042,   // あ     (3 bytes: E3 81 82)
	0x20AC,   // €      (3 bytes: E2 82 AC)
	0x200B,   // ZWSP   (3 bytes: E2 80 8B, non-printable)
	0x1F600,  // 😀     (4 bytes: F0 9F 98 80)
	0x10FFFF, // MaxRune (4 bytes: F4 8F BF BF)
}

func ps5030Check(t *testing.T, s string, r rune) {
	t.Helper()
	cut := string(r)
	if len(cut) < 2 || !utf8.ValidRune(r) || r == utf8.RuneError {
		t.Fatalf("harness bug: rune %U is outside the check's scope", r)
	}
	wantIdx := ps5030AfterIndex(s, r)
	if got := ps5030BeforeIndex(s, cut); got != wantIdx {
		t.Fatalf("strings.IndexAny divergence on (%q, %q): before=%d after=%d", s, cut, got, wantIdx)
	}
	wantHas := wantIdx >= 0
	if got := ps5030BeforeContains(s, cut); got != wantHas {
		t.Fatalf("strings.ContainsAny divergence on (%q, %q): before=%v after=%v", s, cut, got, wantHas)
	}
	if got := ps5030AfterContains(s, r); got != wantHas {
		t.Fatalf("strings.ContainsRune spelling divergence on (%q, %q): got %v want %v", s, cut, got, wantHas)
	}
	// The ! forms the fix leaves in place around the renamed call.
	if got := ps5030BeforeNot(s, cut); got != !wantHas {
		t.Fatalf("!strings.ContainsAny divergence on (%q, %q): before=%v after=%v", s, cut, got, !wantHas)
	}
	if got := ps5030AfterNot(s, r); got != !wantHas {
		t.Fatalf("!strings.ContainsRune spelling divergence on (%q, %q): got %v want %v", s, cut, got, !wantHas)
	}
}

// ps5030Alphabet stresses every comparison hazard: ASCII, NUL, ALL the
// distinct bytes of the tested runes' encodings (so lead bytes without
// their continuations, continuations without a lead, misaligned overlaps
// and complete encodings arise at every position of the exhaustive
// haystacks), and 0xFF (never valid UTF-8).
var ps5030Alphabet = func() []byte {
	seen := map[byte]bool{'a': true, 0x00: true, 0xFF: true}
	out := []byte{'a', 0x00, 0xFF}
	for _, r := range ps5030Runes {
		for _, b := range []byte(string(r)) {
			if !seen[b] {
				seen[b] = true
				out = append(out, b)
			}
		}
	}
	return out
}()

func TestEquiv_PS5030_ExhaustiveShort(t *testing.T) {
	var haystacks []string
	var enumerate func(buf []byte, depth int)
	enumerate = func(buf []byte, depth int) {
		haystacks = append(haystacks, string(buf))
		if depth == 0 {
			return
		}
		for _, c := range ps5030Alphabet {
			enumerate(append(buf, c), depth-1)
		}
	}
	enumerate(nil, 4)
	for _, s := range haystacks {
		for _, r := range ps5030Runes {
			ps5030Check(t, s, r)
		}
	}
}

func TestEquiv_PS5030_TargetedSeeds(t *testing.T) {
	long := strings.Repeat("payload=value ", 2048)
	for _, r := range ps5030Runes {
		enc := string(r)
		trunc := enc[:len(enc)-1] // the encoding minus its last byte: a truncated, invalid prefix
		cont := enc[1:]           // the continuation bytes without their lead
		seeds := []string{
			"",
			enc,                         // exactly the rune
			enc + enc + enc,             // repeated back-to-back
			"x" + enc,                   // only at the very end
			enc + "x",                   // only at the very start
			trunc,                       // truncated prefix alone (invalid, must be a miss)
			trunc + enc,                 // truncated prefix immediately before a real occurrence
			trunc + trunc + enc + trunc, // several truncations around one real occurrence
			cont + enc,                  // bare continuation bytes before the real occurrence
			enc[:1] + "x" + enc,         // lead byte split from its continuations
			"éßあ€😀" + enc,               // other multi-byte runes before the occurrence
			"\xff\xfe" + enc + "\x80",   // invalid UTF-8 around the occurrence
			strings.Repeat(enc[len(enc)-1:], 64) + enc, // last-byte-heavy: forces IndexRune's backward compare into its fallback
			strings.Repeat(trunc, 200),                 // many near-misses, no occurrence
			long,                                       // long, no occurrence
			long + enc,                                 // long, occurrence only at the very end
			enc + long,                                 // long, occurrence only at the very start
			long + enc + long,                          // long, occurrence in the middle
			long + trunc,                               // long ending in a truncated near-miss
		}
		for _, s := range seeds {
			ps5030Check(t, s, r)
		}
		// Cross-pollination: every OTHER rune's seeds must be misses or
		// hits for r on their own merits — probe a few mixed haystacks.
		for _, other := range ps5030Runes {
			if other == r {
				continue
			}
			ps5030Check(t, string(other)+enc, r)
			ps5030Check(t, enc[:1]+string(other), r)
		}
	}
}

func TestEquiv_PS5030_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25030))
	for range 20000 {
		b := make([]byte, rng.Intn(300))
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		r := ps5030Runes[rng.Intn(len(ps5030Runes))]
		if rng.Intn(2) == 0 && len(b) >= 4 {
			// Splice the encoding in at a random offset so guaranteed
			// hits (possibly clobbered by the next splice — either way a
			// well-defined haystack) arise alongside random misses.
			copy(b[rng.Intn(len(b)-3):], string(r))
		}
		ps5030Check(t, string(b), r)
	}
}

// TestEquiv_PS5030_RuneErrorGuardPin pins the r != utf8.RuneError guard.
// IndexRune's DOCUMENTED contract for RuneError is a different question —
// "the first instance of any invalid UTF-8 byte sequence" — so the check
// conservatively refuses to rewrite a "�" cutset rather than tie
// its safety to the observation that, in the CURRENT stdlib, both forms
// happen to return the first position whose decode is U+FFFD (a real
// U+FFFD and every invalid sequence alike). This test asserts today's
// coincidence on both a bare invalid byte and a real, well-encoded
// U+FFFD: if the stdlib ever separates the two behaviors (as the
// documentation reserves the right to), this fails and PS5030's guard is
// re-audited — by a test, not a linter user.
func TestEquiv_PS5030_RuneErrorGuardPin(t *testing.T) {
	// Built at runtime: an invalid-UTF-8 CONSTANT would (correctly) trip
	// vet/staticcheck — feeding IndexAny invalid UTF-8 is the point.
	invalid := string([]byte{0xff}) + "x�y"
	wellFormed := "ab�cd"
	for _, s := range []string{invalid, wellFormed, "plain ascii", ""} {
		if got, want := strings.IndexAny(s, "�"), strings.IndexRune(s, utf8.RuneError); got != want {
			t.Fatalf("IndexAny(%q, %q) = %d but IndexRune(s, RuneError) = %d: the stdlib separated the RuneError behaviors — re-audit PS5030's RuneError guard", s, "�", got, want)
		}
	}
	if got := strings.IndexAny(invalid, "�"); got != 0 {
		t.Fatalf("IndexAny over a leading invalid byte = %d, want 0 (RuneError-decode membership)", got)
	}
	if got := strings.IndexAny(wellFormed, "�"); got != 2 {
		t.Fatalf("IndexAny over a real U+FFFD = %d, want 2", got)
	}
}

// TestEquiv_PS5030_BothSidesZeroAlloc pins the perf premise: neither side
// allocates (the cutset is a string constant and the rune a constant —
// nothing is constructed per call), so the entire win is scan cost: the
// per-rune decode-and-probe fallback collapses to one substring scan.
func TestEquiv_PS5030_BothSidesZeroAlloc(t *testing.T) {
	s := "service=checkout region=eu-west-1 shard=07 cost=42€ final—ok"
	var sinkI int
	var sinkB bool
	allocs := testing.AllocsPerRun(100, func() {
		sinkI = strings.IndexAny(s, "—")
		sinkI += strings.IndexRune(s, '—')
		sinkB = strings.ContainsAny(s, "€")
		sinkB = sinkB != strings.ContainsRune(s, '€')
	})
	_, _ = sinkI, sinkB
	if allocs != 0 {
		t.Fatalf("the IndexAny/IndexRune quartet allocated %v times per run; PS5030's like-for-like premise no longer holds", allocs)
	}
}
