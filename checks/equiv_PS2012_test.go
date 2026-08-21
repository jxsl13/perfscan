package checks

// Runtime differential for PS2012: string(bytes.TrimSpace([]byte(s)))
// vs strings.TrimSpace(s). The fix's safety argument is that both
// TrimSpace implementations trim exactly the unicode.IsSpace runes and
// decode malformed input with the same utf8 RuneError-width-1 rule, so
// the surviving span — and therefore the resulting string VALUE — is
// identical for every input. This suite pins that claim over:
//
//   - EXHAUSTIVE short inputs: every byte string of length <= 4 over an
//     adversarial alphabet holding all five ASCII whitespace escapes and
//     the space, the bytes of the multi-byte spaces U+0085 / U+00A0 /
//     U+3000 (so truncated and complete sequences both arise), a NUL, a
//     bare continuation byte, and 0xFF — every alignment of valid,
//     invalid, and boundary-straddling UTF-8 the trim loop can see;
//   - targeted seeds (all-whitespace, Unicode-space-wrapped, invalid
//     UTF-8 mid-string, very long inputs);
//   - randomized long inputs over the full byte range with a fixed seed.
//
// It also pins the perf premise: strings.TrimSpace performs zero
// allocations (it returns a substring), which is the entire win.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// ps2012Before is the exact Before-shape of the check.
func ps2012Before(s string) string { return string(bytes.TrimSpace([]byte(s))) }

// ps2012After is the exact After-shape of the check.
func ps2012After(s string) string { return strings.TrimSpace(s) }

func ps2012Check(t *testing.T, s string) {
	t.Helper()
	before := ps2012Before(s)
	after := ps2012After(s)
	if before != after {
		t.Fatalf("TrimSpace divergence on %q:\n before=%q\n after=%q", s, before, after)
	}
}

func TestEquiv_PS2012_ExhaustiveShort(t *testing.T) {
	alphabet := []byte{
		'\t', '\n', '\v', '\f', '\r', ' ', // the ASCII IsSpace set
		'a', '.', // non-space anchors
		0x00,       // NUL (not a space; must survive)
		0xC2, 0x85, // C2 85 = U+0085 NEL (a space); each byte alone is invalid UTF-8
		0xA0,       // C2 A0 = U+00A0 NBSP (a space); A0 alone is a bare continuation byte
		0xE3, 0x80, // E3 80 80 = U+3000 ideographic space; prefixes of it are truncated sequences
		0xFF, // never valid in UTF-8
	}
	var buf [4]byte
	var rec func(n, depth int)
	rec = func(n, depth int) {
		if depth == n {
			ps2012Check(t, string(buf[:n]))
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

func TestEquiv_PS2012_TargetedSeeds(t *testing.T) {
	long := strings.Repeat("payload ", 4096)
	seeds := []string{
		"",
		" ",
		"\t\n\v\f\r \t\n\v\f\r", // all-ASCII-whitespace, both ends
		" \u0085\u00a0\u1680\u2000\u2028\u2029\u3000", // entirely Unicode spaces (NEL, NBSP, ogham, en quad, line+para sep, ideographic)
		"\u00a0x\u00a0",                      // NBSP-wrapped
		"\u3000\u65e5\u672c\u8a9e\u3000",     // ideographic-space-wrapped multi-byte payload
		"\u200bx\u200b",                      // ZERO WIDTH SPACE is NOT IsSpace - must survive
		"\xc3\x28",                           // classic invalid two-byte sequence
		" \xc3\x28 ",                         // invalid UTF-8 inside the trimmed span
		"\x80\x80",                           // bare continuation bytes only
		" \xff\xfe\x80 ",                     // invalid bytes wrapped in spaces
		"\xc2",                               // truncated NEL/NBSP prefix at the very edge
		"\xc2 ",                              // truncated sequence then a space
		" \xc2",                              // space then truncated sequence
		"\xe3\x80",                           // truncated ideographic-space prefix
		"a\x00b",                             // NUL inside the kept span
		"\r\nwindows line\r\n",               // the classic line-trim input
		" " + long + " ",                     // long payload, trimmed ends
		strings.Repeat(" ", 8192) + "x",      // long all-space prefix
		"x" + strings.Repeat("\u3000", 2048), // long multi-byte-space suffix
		strings.Repeat("\t  ", 4096),         // long input trimmed to nothing
	}
	for _, s := range seeds {
		ps2012Check(t, s)
	}
}

func TestEquiv_PS2012_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25012012))
	// Full byte range: mostly invalid UTF-8 at random alignments.
	for range 100000 {
		b := make([]byte, rng.Intn(33))
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		ps2012Check(t, string(b))
	}
	// Whitespace-heavy alphabet so trims routinely straddle rune
	// boundaries of the multi-byte spaces.
	heavy := []byte{' ', '\t', '\n', 0xC2, 0x85, 0xA0, 0xE3, 0x80, 'x'}
	for range 100000 {
		b := make([]byte, rng.Intn(25))
		for i := range b {
			b[i] = heavy[rng.Intn(len(heavy))]
		}
		ps2012Check(t, string(b))
	}
}

// TestEquiv_PS2012_AfterIsZeroAlloc pins the perf premise: the rewritten
// form allocates nothing (it returns a substring of its input), which is
// exactly the win the check promises. The Before-form's two copies are
// measured in benchmarks/ps2012_test.go rather than asserted here, since
// escape analysis could legitimately elide them in some shapes.
func TestEquiv_PS2012_AfterIsZeroAlloc(t *testing.T) {
	s := "  \t trimmed payload that is clearly long enough to heap-allocate \r\n "
	var sink string
	allocs := testing.AllocsPerRun(100, func() {
		sink = strings.TrimSpace(s)
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("strings.TrimSpace allocated %v times per run; the zero-copy premise of PS2012 no longer holds", allocs)
	}
}
