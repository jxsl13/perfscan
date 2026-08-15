package checks

// Runtime differential for PS5036: every membership comparison of
// bytes.LastIndex(b, sub) against -1/0 vs the bytes.Contains(b, sub)
// form the fix emits. The fix's safety argument: LastIndex(b, sub)
// returns >= 0 exactly when sub occurs in b (n == 0 -> len(b) >= 0;
// n == 1 -> LastIndexByte; n == len(b) -> 0 or -1 by direct comparison;
// n > len(b) -> -1; the general reverse Rabin-Karp finds the last
// occurrence, which exists exactly when ANY occurrence exists), and
// Contains(b, sub) is Index(b, sub) != -1 — the identical membership
// predicate computed forward. Both sides match RAW bytes with no rune
// decoding, case folding, or UTF-8 validation, so the booleans agree on
// every input, valid UTF-8 or not, nil or empty slices included. This
// suite pins that claim over:
//
//   - EXHAUSTIVE pairs: every haystack of length <= 3 over an
//     adversarial alphabet (ASCII, NUL, a UTF-8 lead byte, a bare
//     continuation byte, and 0xFF — so truncated and misaligned
//     sequences arise at every position) crossed with every needle of
//     length <= 3 over the same alphabet — presence, absence, empty
//     needle, needle == haystack, needle longer than haystack, and
//     boundary positions all arise from the cross product — plus nil on
//     either side;
//   - targeted seeds: nil/empty both sides, the needle only at the
//     start, only at the end, repeated, overlapping repeats
//     ("aaa"/"aa"), nearly-matching prefixes that force Rabin-Karp hash
//     collisions to be re-verified, NUL-bearing and invalid-UTF-8
//     needles, and needle-equal-to-haystack;
//   - randomized long haystacks (up to 4 KiB) over the full byte range
//     with a fixed seed, with the needle sometimes spliced in and
//     sometimes absent — long enough to cross the bytealg SIMD cutovers
//     on the Contains side and the rolling-hash loop on the LastIndex
//     side.
//
// It also pins the perf premise that NO side allocates.

import (
	"bytes"
	"math/rand"
	"testing"
)

// ps5036Before* are the exact Before-shapes of the check (all six
// comparison forms); ps5036After* are the exact After-shapes the fix
// emits. Every Before must agree with its After on EVERY input.
func ps5036BeforeNE(b, sub []byte) bool { return bytes.LastIndex(b, sub) != -1 }
func ps5036BeforeGT(b, sub []byte) bool { return bytes.LastIndex(b, sub) > -1 }
func ps5036BeforeGE(b, sub []byte) bool { return bytes.LastIndex(b, sub) >= 0 }
func ps5036BeforeEQ(b, sub []byte) bool { return bytes.LastIndex(b, sub) == -1 }
func ps5036BeforeLT(b, sub []byte) bool { return bytes.LastIndex(b, sub) < 0 }
func ps5036BeforeLE(b, sub []byte) bool { return bytes.LastIndex(b, sub) <= -1 }

func ps5036AfterHas(b, sub []byte) bool  { return bytes.Contains(b, sub) }
func ps5036AfterNone(b, sub []byte) bool { return !bytes.Contains(b, sub) }

func ps5036Check(t *testing.T, b, sub []byte) {
	t.Helper()
	has := ps5036AfterHas(b, sub)
	none := ps5036AfterNone(b, sub)
	if has == none {
		t.Fatalf("Contains and !Contains agree?! b=%q sub=%q", b, sub)
	}
	if got := ps5036BeforeNE(b, sub); got != has {
		t.Errorf("LastIndex(%q, %q) != -1 = %v, Contains = %v", b, sub, got, has)
	}
	if got := ps5036BeforeGT(b, sub); got != has {
		t.Errorf("LastIndex(%q, %q) > -1 = %v, Contains = %v", b, sub, got, has)
	}
	if got := ps5036BeforeGE(b, sub); got != has {
		t.Errorf("LastIndex(%q, %q) >= 0 = %v, Contains = %v", b, sub, got, has)
	}
	if got := ps5036BeforeEQ(b, sub); got != none {
		t.Errorf("LastIndex(%q, %q) == -1 = %v, !Contains = %v", b, sub, got, none)
	}
	if got := ps5036BeforeLT(b, sub); got != none {
		t.Errorf("LastIndex(%q, %q) < 0 = %v, !Contains = %v", b, sub, got, none)
	}
	if got := ps5036BeforeLE(b, sub); got != none {
		t.Errorf("LastIndex(%q, %q) <= -1 = %v, !Contains = %v", b, sub, got, none)
	}
}

// ps5036Alphabet is the adversarial byte alphabet for the exhaustive
// cross product: plain ASCII, NUL, a two-byte UTF-8 lead byte, a bare
// continuation byte, and 0xFF — every mix of valid, truncated and
// misaligned UTF-8 arises somewhere in the product.
var ps5036Alphabet = []byte{'a', 'b', 0x00, 0xC3, 0xA9, 0xFF}

// ps5036Slices returns nil plus every byte slice of length <= maxLen
// over the alphabet.
func ps5036Slices(maxLen int) [][]byte {
	out := [][]byte{nil, {}}
	prev := [][]byte{{}}
	for range maxLen {
		var next [][]byte
		for _, p := range prev {
			for _, c := range ps5036Alphabet {
				next = append(next, append(append([]byte{}, p...), c))
			}
		}
		out = append(out, next...)
		prev = next
	}
	return out
}

// TestPS5036EquivalenceExhaustive crosses every haystack of length <= 3
// (nil and empty included) with every needle of length <= 3 over the
// adversarial alphabet: 260 x 260 pairs covering nil/empty needle
// (LastIndex = len(b), Contains true), absent needle, needle ==
// haystack, needle longer than haystack, and matches at every boundary
// — all six comparison shapes each.
func TestPS5036EquivalenceExhaustive(t *testing.T) {
	all := ps5036Slices(3)
	for _, b := range all {
		for _, sub := range all {
			ps5036Check(t, b, sub)
		}
	}
}

// TestPS5036EquivalenceSeeds pins the classic divergence hunting
// grounds: overlapping repeats, nearly-matching prefixes that make the
// reverse rolling hash re-verify candidates, needles at either end only,
// NUL-bearing and invalid-UTF-8 needles, and the n == 1 / n == len(b) /
// n > len(b) fast paths.
func TestPS5036EquivalenceSeeds(t *testing.T) {
	cases := []struct{ b, sub []byte }{
		{nil, nil},
		{nil, []byte("x")},
		{[]byte("x"), nil},
		{[]byte(""), []byte("")},
		{[]byte("aaa"), []byte("aa")},                                             // overlapping repeats
		{[]byte("aaaa"), []byte("aaa")},                                           // more overlap
		{[]byte("ababab"), []byte("abab")},                                        // periodic
		{[]byte("mississippi"), []byte("issi")},                                   // classic overlap
		{[]byte("needle in the haystack"), []byte("needle")},                      // start only
		{[]byte("the haystack ends with needle"), []byte("needle")},               // end only
		{[]byte("needle needle needle"), []byte("needle")},                        // repeated
		{[]byte("nearly needlX but needle"), []byte("needle")},                    // near-miss prefix first
		{[]byte("abcdefgh"), []byte("abcdefgh")},                                  // n == len(b), equal
		{[]byte("abcdefgh"), []byte("abcdefgx")},                                  // n == len(b), unequal
		{[]byte("short"), []byte("longer than the haystack")},                     // n > len(b)
		{[]byte("split/path/name"), []byte("/")},                                  // n == 1 fast path
		{[]byte("\x00a\x00b\x00"), []byte("\x00b")},                               // NUL bytes
		{[]byte("caf\xc3\xa9 latte"), []byte("\xc3\xa9")},                         // valid UTF-8 needle
		{[]byte("caf\xc3 broken \xa9"), []byte("\xc3\xa9")},                       // split encoding — absent
		{[]byte("raw \xff\xfe bytes"), []byte("\xff\xfe")},                        // invalid UTF-8 both sides
		{append(bytes.Repeat([]byte("ab"), 2048), "cd"...), []byte("cd")},         // late match, long
		{append([]byte("cd"), bytes.Repeat([]byte("ab"), 2048)...), []byte("cd")}, // early match, long
		{bytes.Repeat([]byte("a"), 4096), []byte("b")},                            // absent, long
		{bytes.Repeat([]byte("a"), 4096), bytes.Repeat([]byte("a"), 33)},          // heavy overlap, long
	}
	for _, c := range cases {
		ps5036Check(t, c.b, c.sub)
		ps5036Check(t, c.sub, c.b) // and swapped
	}
}

// TestPS5036EquivalenceRandom hammers randomized long haystacks over the
// full byte range with a fixed seed; the needle is sometimes spliced in
// and sometimes left absent, at lengths that cross the bytealg cutovers
// on the Contains side.
func TestPS5036EquivalenceRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5036))
	randBytes := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		return b
	}
	for range 2000 {
		b := randBytes(rng.Intn(4096))
		sub := randBytes(2 + rng.Intn(24))
		if rng.Intn(2) == 0 && len(b) > 0 {
			// Splice the needle in at a random offset.
			at := rng.Intn(len(b))
			spliced := append([]byte{}, b[:at]...)
			spliced = append(spliced, sub...)
			spliced = append(spliced, b[at:]...)
			b = spliced
		}
		ps5036Check(t, b, sub)
	}
}

// TestPS5036NoAllocs pins the perf premise: neither side of the rewrite
// allocates.
func TestPS5036NoAllocs(t *testing.T) {
	b := bytes.Repeat([]byte("service=checkout region=eu-west-1 shard=07 "), 256)
	sub := []byte("region=")
	absent := []byte("REGION=")
	for name, fn := range map[string]func() bool{
		"before/present": func() bool { return ps5036BeforeNE(b, sub) },
		"before/absent":  func() bool { return ps5036BeforeNE(b, absent) },
		"after/present":  func() bool { return ps5036AfterHas(b, sub) },
		"after/absent":   func() bool { return ps5036AfterHas(b, absent) },
	} {
		var sink bool
		if n := testing.AllocsPerRun(10, func() { sink = fn() }); n != 0 {
			t.Errorf("%s allocates %v times per run", name, n)
		}
		_ = sink
	}
}
