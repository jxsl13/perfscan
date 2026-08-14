package checks

// Runtime differential for PS2014: strings.Split(s, sep)[i] vs
// strings.SplitN(s, sep, i+2)[i] for constant 1 <= i <= 16, plus the
// bytes twin. The fix's safety argument is that SplitN(s, sep, i+2)
// produces min(k+1, i+2) pieces for k separator occurrences and ONLY the
// last produced piece (index i+1) can be the un-split remainder — every
// earlier piece is boundary-for-boundary identical to Split's, so piece
// i (strictly before i+1) is identical whenever it exists, and it exists
// for exactly the same inputs (min(k+1, i+2) >= i+1 iff k+1 >= i+1), so
// panic behavior is identical too. This suite pins that claim over:
//
//   - EXHAUSTIVE short inputs: every s of length <= 5 over an adversarial
//     alphabet (ASCII, the two bytes of a multi-byte rune so truncated and
//     complete sequences both arise, and 0xFF which is never valid UTF-8),
//     crossed with separators including the EMPTY string (the rune-explode
//     path), 1- and 2-byte ASCII, a rune fragment and an invalid byte, for
//     every index i in 1..4 — every alignment of matching, adjacent-empty
//     and boundary-straddling pieces the scan can see, INCLUDING all the
//     out-of-range shapes, which must agree on being out of range;
//   - the bytes twin over the same grid, additionally checking len, cap,
//     backing-array aliasing and nil-ness of piece i (pieces are capped
//     subslices except a final remainder, which both forms share);
//   - randomized long inputs over the full byte range and over a tiny
//     separator-dense alphabet with a fixed seed, i in 1..16;
//   - the divergence shape the +2 exists for: SplitN(s, sep, i+1)[i] is
//     the UN-SPLIT remainder and differs from Split(s, sep)[i] whenever
//     more separators follow — if that ever stops diverging, the n = i+2
//     arithmetic is no longer load-bearing and should be revisited.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// ps2014Check pins Split(s, sep)[i] == SplitN(s, sep, i+2)[i] including
// range/panic parity: both sides must agree on whether index i exists.
func ps2014Check(t *testing.T, s, sep string, i int) {
	t.Helper()
	if i < 1 {
		t.Fatal("test bug: i must be >= 1 — index 0 is PS2009's domain, never matched by PS2014")
	}
	full := strings.Split(s, sep)
	lim := strings.SplitN(s, sep, i+2)
	if (len(full) > i) != (len(lim) > i) {
		t.Fatalf("range divergence on s=%q sep=%q i=%d: Split has %d pieces, SplitN(%d) has %d — one side panics on [%d] and the other does not",
			s, sep, i, len(full), i+2, len(lim), i)
	}
	if len(full) > i && full[i] != lim[i] {
		t.Fatalf("piece divergence on s=%q sep=%q i=%d:\n Split[%d]=%q\n SplitN(%d)[%d]=%q",
			s, sep, i, i, full[i], i+2, i, lim[i])
	}
}

// ps2014CheckBytes is the bytes twin: beyond byte equality it pins len,
// cap, nil-ness and backing-array identity of piece i, since a []byte
// piece is a live view into the input and callers may append to it.
func ps2014CheckBytes(t *testing.T, s, sep []byte, i int) {
	t.Helper()
	full := bytes.Split(s, sep)
	lim := bytes.SplitN(s, sep, i+2)
	if (len(full) > i) != (len(lim) > i) {
		t.Fatalf("bytes range divergence on s=%q sep=%q i=%d: Split has %d pieces, SplitN(%d) has %d",
			s, sep, i, len(full), i+2, len(lim))
	}
	if len(full) <= i {
		return
	}
	f, l := full[i], lim[i]
	if !bytes.Equal(f, l) {
		t.Fatalf("bytes piece divergence on s=%q sep=%q i=%d: Split[%d]=%q SplitN(%d)[%d]=%q", s, sep, i, i, f, i+2, i, l)
	}
	if (f == nil) != (l == nil) {
		t.Fatalf("nil-ness divergence on s=%q sep=%q i=%d: Split piece nil=%v, SplitN piece nil=%v", s, sep, i, f == nil, l == nil)
	}
	if len(f) != len(l) || cap(f) != cap(l) {
		t.Fatalf("len/cap divergence on s=%q sep=%q i=%d: Split len=%d cap=%d, SplitN len=%d cap=%d",
			s, sep, i, len(f), cap(f), len(l), cap(l))
	}
	if len(f) > 0 && &f[0] != &l[0] {
		t.Fatalf("aliasing divergence on s=%q sep=%q i=%d: pieces point at different backing arrays", s, sep, i)
	}
}

// ps2014Alphabet anchors plain matches ('a','b'), straddles rune
// boundaries (0xC3 0xA9 are the bytes of é) and includes 0xFF, which is
// never valid UTF-8 — so the empty-separator rune-explode path sees
// complete, truncated and invalid sequences at every alignment.
var ps2014Alphabet = []byte{'a', 'b', 0xC3, 0xA9, 0xFF}

func ps2014ShortStrings(maxLen int) []string {
	var strs []string
	buf := make([]byte, maxLen)
	var rec func(n, depth int)
	rec = func(n, depth int) {
		if depth == n {
			strs = append(strs, string(buf[:n]))
			return
		}
		for _, c := range ps2014Alphabet {
			buf[depth] = c
			rec(n, depth+1)
		}
	}
	for n := 0; n <= maxLen; n++ {
		rec(n, 0)
	}
	return strs
}

func TestEquiv_PS2014_ExhaustiveShort(t *testing.T) {
	strs := ps2014ShortStrings(5)
	// The empty separator exercises rune-explode; "a" and "aa" exercise
	// dense and adjacent matches over the same inputs; "\xC3" is a rune
	// fragment and "\xFF" an invalid byte, so matches can cut runes apart.
	seps := []string{"", "a", "aa", "ab", "\xC3", "\xFF", "é"}
	for _, s := range strs {
		for _, sep := range seps {
			for i := 1; i <= 4; i++ {
				ps2014Check(t, s, sep, i)
			}
		}
	}
}

func TestEquiv_PS2014_ExhaustiveShortBytes(t *testing.T) {
	strs := ps2014ShortStrings(5)
	seps := []string{"", "a", "aa", "ab", "\xC3", "\xFF", "é"}
	for _, s := range strs {
		for _, sep := range seps {
			for i := 1; i <= 4; i++ {
				ps2014CheckBytes(t, []byte(s), []byte(sep), i)
			}
		}
	}
	// nil input and nil separator: both explode/scan paths must agree.
	for i := 1; i <= 4; i++ {
		ps2014CheckBytes(t, nil, []byte(","), i)
		ps2014CheckBytes(t, nil, nil, i)
		ps2014CheckBytes(t, []byte("a,b,c,d,e"), nil, i)
	}
}

func TestEquiv_PS2014_TargetedSeeds(t *testing.T) {
	long := strings.Repeat("field-", 4096)
	type seed struct {
		s, sep string
	}
	seeds := []seed{
		{"a,b,c,d,e,f", ","},                    // plain CSV, in range for all i tested
		{"a,b", ","},                            // exactly 1 separator: [1] is the FINAL piece (both remainders), [2] out of range in both
		{"a,b,c", ","},                          // exactly 2: [2] is the final piece
		{",,,,", ","},                           // empty fields everywhere
		{"", ","},                               // empty input: 1 piece, all i out of range
		{"", ""},                                // empty input, empty sep: 0 pieces
		{"héllo,wörld,日本,語", ","},               // multibyte fields
		{"日本語日本語", "本"},                         // CJK separator
		{"a\xFFb\xFFc\xFFd", "\xFF"},            // invalid-UTF-8 separator
		{"\xC3\x28\xC3\x28\xC3\x28", "\xC3"},    // truncated-rune separator at every alignment
		{"héllo", ""},                           // rune-explode over multibyte
		{"\xFF\xFE\xFD\xFC", ""},                // rune-explode over invalid UTF-8: one piece per byte
		{"aaaaaa", "aa"},                        // adjacent matches, no gap
		{"aaaaaaa", "aa"},                       // odd tail after adjacent matches
		{"x" + long + "x", "-"},                 // long, separator-dense
		{long + long, "field"},                  // separator == field text
		{"a\x00b\x00c\x00d", "\x00"},            // NUL separator
		{strings.Repeat("é", 64), "é"},          // input consists ONLY of separators
		{strings.Repeat("　", 64) + "end", "　"},  // long multibyte separator run
		{"no-separator-here at all", "MISSING"}, // zero matches: 1 piece, all i out of range
	}
	for _, sd := range seeds {
		for i := 1; i <= 16; i++ {
			ps2014Check(t, sd.s, sd.sep, i)
			ps2014CheckBytes(t, []byte(sd.s), []byte(sd.sep), i)
		}
	}
}

func TestEquiv_PS2014_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25082014))
	gen := func(al []byte, max int) string {
		b := make([]byte, rng.Intn(max+1))
		for i := range b {
			b[i] = al[rng.Intn(len(al))]
		}
		return string(b)
	}
	full := make([]byte, 256)
	for i := range full {
		full[i] = byte(i)
	}
	// Full byte range: mostly invalid UTF-8 at random alignments.
	for range 50000 {
		s := gen(full, 48)
		sep := gen(full, 3)
		i := 1 + rng.Intn(16)
		ps2014Check(t, s, sep, i)
		ps2014CheckBytes(t, []byte(s), []byte(sep), i)
	}
	// Tiny alphabet so separator matches are dense and pieces short:
	// in-range and out-of-range shapes both arise for every i.
	tiny := []byte{'a', 'b', 0xC3, 0xA9}
	for range 50000 {
		s := gen(tiny, 32)
		sep := gen(tiny, 2)
		i := 1 + rng.Intn(16)
		ps2014Check(t, s, sep, i)
		ps2014CheckBytes(t, []byte(s), []byte(sep), i)
	}
}

// TestEquiv_PS2014_NPlusOneDiverges pins the divergence the n = i+2
// arithmetic exists to avoid: with n = i+1, piece i IS the un-split
// remainder and differs from Split's piece i whenever more separators
// follow. If this ever stops diverging, the +2 is no longer load-bearing
// and the check should be revisited.
func TestEquiv_PS2014_NPlusOneDiverges(t *testing.T) {
	s, sep, i := "a,b,c,d,e", ",", 2
	full := strings.Split(s, sep)[i]        // "c"
	wrong := strings.SplitN(s, sep, i+1)[i] // "c,d,e" — the remainder
	if full == wrong {
		t.Fatalf("expected SplitN(s, sep, i+1)[i] to be the un-split remainder and diverge on s=%q sep=%q i=%d, but both returned %q — the i+2 limit may be obsolete", s, sep, i, full)
	}
	right := strings.SplitN(s, sep, i+2)[i]
	if full != right {
		t.Fatalf("SplitN(s, sep, i+2)[i]=%q must equal Split(s, sep)[i]=%q", right, full)
	}
}
