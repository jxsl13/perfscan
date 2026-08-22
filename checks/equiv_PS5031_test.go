package checks

// Runtime differential for PS5031: every membership comparison of
// strings.LastIndex(s, sub) against -1/0 vs the strings.Contains(s, sub)
// form the fix emits. The fix's safety argument: LastIndex(s, sub)
// returns >= 0 exactly when sub occurs in s (n == 0 -> len(s) >= 0;
// n == len(s) -> 0 or -1 by direct comparison; n > len(s) -> -1; the
// general reverse Rabin-Karp finds the last occurrence, which exists
// exactly when ANY occurrence exists), and Contains(s, sub) is
// Index(s, sub) >= 0 — the identical membership predicate computed
// forward. Both sides match RAW bytes with no rune decoding, case
// folding, or UTF-8 validation, so the booleans agree on every input,
// valid UTF-8 or not. This suite pins that claim over:
//
//   - EXHAUSTIVE pairs: every haystack of length <= 3 over an
//     adversarial alphabet (ASCII, NUL, a UTF-8 lead byte, a bare
//     continuation byte, and 0xFF — so truncated and misaligned
//     sequences arise at every position) crossed with every needle of
//     length <= 3 over the same alphabet — presence, absence, empty
//     needle, needle == haystack, needle longer than haystack, and
//     boundary positions all arise from the cross product;
//   - targeted seeds: empty both sides, the needle only at the start,
//     only at the end, repeated, overlapping repeats ("aaa"/"aa"),
//     nearly-matching prefixes that force Rabin-Karp hash collisions to
//     be re-verified, NUL-bearing and invalid-UTF-8 needles, and
//     needle-equal-to-haystack;
//   - randomized long haystacks (up to 4 KiB) over the full byte range
//     with a fixed seed, with the needle sometimes spliced in and
//     sometimes absent — long enough to cross the bytealg SIMD cutovers
//     on the Contains side and the rolling-hash loop on the LastIndex
//     side.
//
// It also pins the perf premise that NO side allocates.

import (
	"math/rand"
	"strings"
	"testing"
)

// ps5031Before* are the exact Before-shapes of the check (all six
// comparison forms); ps5031After* are the exact After-shapes the fix
// emits. Every Before must agree with its After on EVERY input.
func ps5031BeforeNE(s, sub string) bool { return strings.LastIndex(s, sub) != -1 }
func ps5031BeforeGT(s, sub string) bool { return strings.LastIndex(s, sub) > -1 }
func ps5031BeforeGE(s, sub string) bool { return strings.LastIndex(s, sub) >= 0 }
func ps5031BeforeEQ(s, sub string) bool { return strings.LastIndex(s, sub) == -1 }
func ps5031BeforeLT(s, sub string) bool { return strings.LastIndex(s, sub) < 0 }
func ps5031BeforeLE(s, sub string) bool { return strings.LastIndex(s, sub) <= -1 }

func ps5031AfterHas(s, sub string) bool  { return strings.Contains(s, sub) }
func ps5031AfterNone(s, sub string) bool { return !strings.Contains(s, sub) }

func ps5031Check(t *testing.T, s, sub string) {
	t.Helper()
	has := ps5031AfterHas(s, sub)
	none := ps5031AfterNone(s, sub)
	if has == none {
		t.Fatalf("Contains and !Contains agree?! s=%q sub=%q", s, sub)
	}
	if got := ps5031BeforeNE(s, sub); got != has {
		t.Errorf("LastIndex(%q, %q) != -1 = %v, Contains = %v", s, sub, got, has)
	}
	if got := ps5031BeforeGT(s, sub); got != has {
		t.Errorf("LastIndex(%q, %q) > -1 = %v, Contains = %v", s, sub, got, has)
	}
	if got := ps5031BeforeGE(s, sub); got != has {
		t.Errorf("LastIndex(%q, %q) >= 0 = %v, Contains = %v", s, sub, got, has)
	}
	if got := ps5031BeforeEQ(s, sub); got != none {
		t.Errorf("LastIndex(%q, %q) == -1 = %v, !Contains = %v", s, sub, got, none)
	}
	if got := ps5031BeforeLT(s, sub); got != none {
		t.Errorf("LastIndex(%q, %q) < 0 = %v, !Contains = %v", s, sub, got, none)
	}
	if got := ps5031BeforeLE(s, sub); got != none {
		t.Errorf("LastIndex(%q, %q) <= -1 = %v, !Contains = %v", s, sub, got, none)
	}
}

// ps5031Alphabet is the adversarial byte alphabet for the exhaustive
// cross product: plain ASCII, NUL, a two-byte UTF-8 lead byte, a bare
// continuation byte, and 0xFF — every mix of valid, truncated and
// misaligned UTF-8 arises somewhere in the product.
var ps5031Alphabet = []byte{'a', 'b', 0x00, 0xC3, 0xA9, 0xFF}

// ps5031Strings returns every string of length <= maxLen over the
// alphabet.
func ps5031Strings(maxLen int) []string {
	out := []string{""}
	prev := []string{""}
	for range maxLen {
		var next []string
		for _, p := range prev {
			for _, c := range ps5031Alphabet {
				next = append(next, p+string(c))
			}
		}
		out = append(out, next...)
		prev = next
	}
	return out
}

// TestPS5031EquivalenceExhaustive crosses every haystack of length <= 3
// with every needle of length <= 3 over the adversarial alphabet:
// 259 x 259 pairs covering empty needle (LastIndex = len(s), Contains
// true), absent needle, needle == haystack, needle longer than haystack,
// and matches at every boundary — all six comparison shapes each.
func TestPS5031EquivalenceExhaustive(t *testing.T) {
	all := ps5031Strings(3)
	for _, s := range all {
		for _, sub := range all {
			ps5031Check(t, s, sub)
		}
	}
}

// TestPS5031EquivalenceSeeds pins the classic divergence hunting
// grounds: overlapping repeats, nearly-matching prefixes that make the
// reverse rolling hash re-verify candidates, needles at either end only,
// NUL-bearing and invalid-UTF-8 needles, and the n == len(s) /
// n > len(s) fast paths.
func TestPS5031EquivalenceSeeds(t *testing.T) {
	cases := []struct{ s, sub string }{
		{"", ""},
		{"", "x"},
		{"x", ""},
		{"aaa", "aa"},                        // overlapping repeats
		{"aaaa", "aaa"},                      // more overlap
		{"ababab", "abab"},                   // periodic
		{"mississippi", "issi"},              // classic overlap
		{"needle in the haystack", "needle"}, // start only
		{"the haystack ends with needle", "needle"},          // end only
		{"needle needle needle", "needle"},                   // repeated
		{"nearly needlX but needle", "needle"},               // near-miss prefix first
		{"abcdefgh", "abcdefgh"},                             // n == len(s), equal
		{"abcdefgh", "abcdefgx"},                             // n == len(s), unequal
		{"short", "longer than the haystack"},                // n > len(s)
		{"\x00a\x00b\x00", "\x00b"},                          // NUL bytes
		{"caf\xc3\xa9 latte", "\xc3\xa9"},                    // valid UTF-8 needle
		{"caf\xc3 broken \xa9", "\xc3\xa9"},                  // split encoding — absent
		{"raw \xff\xfe bytes", "\xff\xfe"},                   // invalid UTF-8 both sides
		{strings.Repeat("ab", 2048) + "cd", "cd"},            // late match, long
		{"cd" + strings.Repeat("ab", 2048), "cd"},            // early match, long
		{strings.Repeat("a", 4096), "b"},                     // absent, long
		{strings.Repeat("a", 4096), strings.Repeat("a", 33)}, // heavy overlap, long
	}
	for _, c := range cases {
		ps5031Check(t, c.s, c.sub)
		ps5031Check(t, c.sub, c.s) // and swapped
	}
}

// TestPS5031EquivalenceRandom hammers randomized long haystacks over the
// full byte range with a fixed seed; the needle is sometimes spliced in
// and sometimes left absent, at lengths that cross the bytealg cutovers
// on the Contains side.
func TestPS5031EquivalenceRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5031))
	randBytes := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		return string(b)
	}
	for range 2000 {
		s := randBytes(rng.Intn(4096))
		sub := randBytes(2 + rng.Intn(24))
		if rng.Intn(2) == 0 && len(s) > 0 {
			// Splice the needle in at a random offset.
			at := rng.Intn(len(s))
			s = s[:at] + sub + s[at:]
		}
		ps5031Check(t, s, sub)
	}
}

// TestPS5031NoAllocs pins the perf premise: neither side of the rewrite
// allocates.
func TestPS5031NoAllocs(t *testing.T) {
	s := strings.Repeat("service=checkout region=eu-west-1 shard=07 ", 256)
	sub := "region="
	absent := "REGION="
	for name, fn := range map[string]func() bool{
		"before/present": func() bool { return ps5031BeforeNE(s, sub) },
		"before/absent":  func() bool { return ps5031BeforeNE(s, absent) },
		"after/present":  func() bool { return ps5031AfterHas(s, sub) },
		"after/absent":   func() bool { return ps5031AfterHas(s, absent) },
	} {
		var sink bool
		if n := testing.AllocsPerRun(10, func() { sink = fn() }); n != 0 {
			t.Errorf("%s allocates %v times per run", name, n)
		}
		_ = sink
	}
}
