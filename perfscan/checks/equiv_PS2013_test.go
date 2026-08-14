package checks

// Runtime differential for PS2013: strings.NewReplacer(old, new).Replace(s)
// vs strings.ReplaceAll(s, old, new), for NON-EMPTY old. The fix's safety
// argument is that both forms replace all non-overlapping occurrences of
// old left-to-right, resuming immediately after each replacement, over
// arbitrary bytes — so for every non-empty old the results are
// byte-identical. This suite pins that claim over:
//
//   - EXHAUSTIVE short inputs: every s of length <= 4 over an adversarial
//     alphabet (ASCII, the two bytes of a multi-byte rune so truncated and
//     complete sequences both arise, and 0xFF which is never valid UTF-8),
//     crossed with every old of length 1..2 over the same alphabet and a
//     spread of new values including "" and invalid UTF-8 — every
//     alignment of matching, overlapping-candidate and boundary-straddling
//     inputs the scan can see;
//   - targeted seeds (overlapping candidates, self-embedding replacements,
//     CJK keys, NULs, invalid UTF-8 keys and payloads, long inputs);
//   - randomized long inputs over the full byte range with a fixed seed.
//
// It also pins the ONE divergent input — old == "" — where NewReplacer
// inserts between every BYTE and ReplaceAll between every RUNE: the gate
// the check's non-empty-old requirement exists for must stay real, or the
// requirement (and this test) should be revisited.

import (
	"math/rand"
	"strings"
	"testing"
)

// ps2013Before is the exact Before-shape of the check.
func ps2013Before(s, old, new string) string {
	return strings.NewReplacer(old, new).Replace(s)
}

// ps2013After is the exact After-shape of the check.
func ps2013After(s, old, new string) string {
	return strings.ReplaceAll(s, old, new)
}

func ps2013Check(t *testing.T, s, old, new string) {
	t.Helper()
	if old == "" {
		t.Fatal("test bug: old must be non-empty — the check never matches an empty old")
	}
	before := ps2013Before(s, old, new)
	after := ps2013After(s, old, new)
	if before != after {
		t.Fatalf("Replace divergence on s=%q old=%q new=%q:\n before=%q\n after=%q", s, old, new, before, after)
	}
}

func TestEquiv_PS2013_ExhaustiveShort(t *testing.T) {
	// 'a'/'b' anchor plain matches; 0xC3 0xA9 are the bytes of é so keys
	// and payloads straddle rune boundaries; 0xFF is never valid UTF-8.
	alphabet := []byte{'a', 'b', 0xC3, 0xA9, 0xFF}
	var strs []string
	var buf [4]byte
	var rec func(n, depth int)
	rec = func(n, depth int) {
		if depth == n {
			strs = append(strs, string(buf[:n]))
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
	news := []string{"", "z", "zz", "\xc3", "é", "aa"}
	for _, old := range strs {
		if old == "" || len(old) > 2 {
			continue
		}
		for _, s := range strs {
			for _, new := range news {
				ps2013Check(t, s, old, new)
			}
		}
	}
}

func TestEquiv_PS2013_TargetedSeeds(t *testing.T) {
	long := strings.Repeat("the payload ", 4096)
	seeds := [][3]string{
		{"aaa", "aa", "b"},                          // overlapping candidates: leftmost non-overlapping wins
		{"aaaa", "aa", "a"},                         // shrinking replacement over overlap
		{"aaaa", "aa", "aaa"},                       // growing, self-embedding replacement (must not rescan output)
		{"abab", "ab", "ab"},                        // no-op replacement value
		{"héllo héllo", "é", "e"},                   // multi-byte key
		{"日本語日本語", "本", "X"},                        // CJK payload and key
		{"\xff\xfe\xff", "\xff", "y"},               // invalid-UTF-8 key over invalid payload
		{" \xc3\x28 \xc3\x28 ", "\xc3", "?"},        // truncated-rune key at every alignment
		{"a\x00b\x00c", "\x00", "-"},                // NUL key
		{"abc", "abc", ""},                          // whole-string to empty
		{"abc", "d", "x"},                           // zero matches (ReplaceAll returns s unchanged)
		{"", "a", "b"},                              // empty s
		{"the " + long + " the", "the", "THE"},      // long payload, many matches
		{strings.Repeat("ab", 8192), "ab", ""},      // long input replaced to nothing
		{strings.Repeat("é", 4096), "é", "\xff"},    // valid key, invalid replacement
		{"x" + strings.Repeat("　", 2048), "　", " "}, // long multi-byte tail
	}
	for _, sd := range seeds {
		ps2013Check(t, sd[0], sd[1], sd[2])
	}
}

func TestEquiv_PS2013_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25082013))
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
	for range 100000 {
		s := gen(full, 32)
		old := gen(full, 4)
		if old == "" {
			old = "\x00"
		}
		ps2013Check(t, s, old, gen(full, 4))
	}
	// Tiny alphabet so matches and overlapping candidates are dense.
	tiny := []byte{'a', 'b', 0xC3, 0xA9}
	for range 100000 {
		s := gen(tiny, 24)
		old := gen(tiny, 3)
		if old == "" {
			old = "a"
		}
		ps2013Check(t, s, old, gen(tiny, 3))
	}
}

// TestEquiv_PS2013_EmptyOldDiverges pins the ONE divergent input the check
// gates out: for old == "", NewReplacer inserts new between every BYTE
// while ReplaceAll inserts between every RUNE, so multi-byte payloads
// differ. If this ever stops failing-to-agree, the non-empty-old gate is
// no longer load-bearing and the check can be revisited.
func TestEquiv_PS2013_EmptyOldDiverges(t *testing.T) {
	s := "héllo"
	before := strings.NewReplacer("", "-").Replace(s)
	after := strings.ReplaceAll(s, "", "-")
	if before == after {
		t.Fatalf("expected byte-vs-rune divergence for empty old on %q, but both forms returned %q — the non-empty-old gate may be obsolete", s, before)
	}
}
