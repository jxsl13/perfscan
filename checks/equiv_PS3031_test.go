package checks

// Runtime differential for PS3031: bytes.TrimFunc(b, unicode.IsSpace)
// vs bytes.TrimSpace(b). The fix's safety argument: bytes.TrimSpace is
// DEFINED as the trim over unicode.IsSpace — its implementation
// literally delegates to TrimFunc(s[lo:], unicode.IsSpace) (resp.
// TrimFunc(s[:hi+1], ...)) the moment its ASCII scan meets a byte >=
// utf8.RuneSelf, and its fast-path table asciiSpace marks exactly
// {'\t', '\n', '\v', '\f', '\r', ' '}, which equals unicode.IsSpace
// restricted to single bytes below utf8.RuneSelf (the other Latin-1
// spaces U+0085 and U+00A0 are multi-byte in UTF-8, so they reach the
// shared rune path in both spellings; a RAW 0x85/0xA0 byte is invalid
// UTF-8, decodes to utf8.RuneError in both, and RuneError is not a
// space).
//
// Unlike the string twin (PS5035), a []byte result HAS a capacity and
// aliasing surface, so byte equality alone is NOT the whole observable
// story — this suite therefore compares the result's data pointer,
// length, CAPACITY, and NIL-NESS too. Every branch of both spellings
// returns a subslice of the SAME backing array at the SAME start
// offset (TrimLeftFunc returns s[i:] with cap = cap-i; TrimSpace's
// fast path returns s[lo:][:hi+1] with cap = cap-lo; the non-ASCII
// fallbacks re-slice at the same absolute offsets — the boundary bytes
// they have already skipped are single-byte ASCII spaces, which are
// self-synchronizing in UTF-8, so the delegated backward rune decode
// sees identical boundaries), and the all-space/empty input yields nil
// on BOTH sides (TrimLeftFunc's i == -1 branch; TrimSpace's explicit
// "returning nil instead of empty slice if all spaces" special case).
// This suite pins that claim over:
//
//   - EXHAUSTIVE short inputs: every byte string of length <= 4 over an
//     adversarial alphabet (each ASCII white-space byte class, a
//     non-space ASCII byte, NUL, the bytes of NBSP U+00A0 and NEL
//     U+0085 and the multi-byte OGHAM SPACE MARK U+1680 and IDEOGRAPHIC
//     SPACE U+3000 — so truncated and complete sequences arise at every
//     position — a bare continuation byte, and 0xFF), each input
//     presented THREE ways: exact-capacity, with spare capacity beyond
//     len (cap arithmetic must track), and at a nonzero offset inside a
//     larger array (the returned pointer must still land inside it);
//   - targeted seeds: empty (nil, empty-non-nil, and re-sliced-to-zero),
//     all-ASCII-space, every Unicode White_Space rune as padding
//     (U+0085, U+00A0, U+1680, U+2000..U+200A, U+2028, U+2029, U+202F,
//     U+205F, U+3000), white space only on one side, RuneError-adjacent
//     shapes (truncated NBSP, bare continuation, 0xFF at the boundary),
//     the literal replacement char (NOT a space in either spelling),
//     and long inputs on and off the ASCII fast path;
//   - randomized inputs with a fixed seed, biased toward boundary white
//     space, multi-byte space runes sliced mid-sequence, invalid bytes,
//     and random spare capacity.
//
// It also pins the perf premise that NO side allocates: both return a
// subslice of b.

import (
	"bytes"
	"math/rand"
	"testing"
	"unicode"
	"unsafe"
)

// ps3031Before is the exact Before-shape of the check; ps3031After is
// the exact After-shape the fix emits. They must agree on EVERY input —
// in bytes, data pointer, length, capacity, and nil-ness.
func ps3031Before(b []byte) []byte { return bytes.TrimFunc(b, unicode.IsSpace) }
func ps3031After(b []byte) []byte  { return bytes.TrimSpace(b) }

func ps3031Check(t *testing.T, b []byte) {
	t.Helper()
	before, after := ps3031Before(b), ps3031After(b)
	switch {
	case (before == nil) != (after == nil):
		t.Fatalf("nil-ness divergence on %q: before nil=%v, after nil=%v", b, before == nil, after == nil)
	case len(before) != len(after):
		t.Fatalf("len divergence on %q: before=%q (len %d), after=%q (len %d)", b, before, len(before), after, len(after))
	case cap(before) != cap(after):
		t.Fatalf("cap divergence on %q: before=%q (cap %d), after=%q (cap %d)", b, before, cap(before), after, cap(after))
	case unsafe.SliceData(before) != unsafe.SliceData(after):
		t.Fatalf("aliasing divergence on %q: before points at %p, after at %p", b, unsafe.SliceData(before), unsafe.SliceData(after))
	case !bytes.Equal(before, after):
		t.Fatalf("byte divergence on %q:\n before=%q\n after=%q", b, before, after)
	}
}

// ps3031CheckShapes runs the differential over the same content in three
// slice shapes: exact capacity, spare capacity beyond len, and a nonzero
// offset inside a larger backing array — the cap/pointer arithmetic of
// TrimLeftFunc's s[i:] and TrimSpace's s[lo:][:hi+1] must agree in all
// of them.
func ps3031CheckShapes(t *testing.T, content []byte) {
	t.Helper()
	exact := append([]byte(nil), content...)
	ps3031Check(t, exact)
	spare := make([]byte, len(content), len(content)+7)
	copy(spare, content)
	ps3031Check(t, spare)
	big := make([]byte, len(content)+10)
	copy(big[3:], content)
	ps3031Check(t, big[3:3+len(content)])
}

func TestEquiv_PS3031_ExhaustiveShort(t *testing.T) {
	alphabet := []byte{
		' ', '\t', '\n', '\v', '\f', '\r', // the asciiSpace table, complete
		'a',        // ASCII non-space
		0x00,       // NUL (not a space)
		0xC2, 0xA0, // C2 A0 = NBSP U+00A0: IsSpace-true, multi-byte
		0x85,             // second byte of NEL U+0085 (C2 85) and a bare invalid byte alone
		0xE1, 0x9A, 0x80, // E1 9A 80 = OGHAM SPACE MARK U+1680
		0xE3, 0x80, // truncated IDEOGRAPHIC SPACE U+3000 prefix (E3 80 80)
		0xFF, // never valid in UTF-8
	}
	var buf [4]byte
	var rec func(n, depth int)
	rec = func(n, depth int) {
		if depth == n {
			ps3031CheckShapes(t, buf[:n])
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

func TestEquiv_PS3031_TargetedSeeds(t *testing.T) {
	// The empty shapes first, spelled precisely: a nil slice, an empty
	// non-nil slice, and a re-sliced-to-zero slice with capacity behind
	// it. Both spellings return nil for each (TrimSpace's explicit
	// special case; TrimFunc via TrimLeftFunc's i == -1 branch).
	ps3031Check(t, nil)
	ps3031Check(t, []byte{})
	ps3031Check(t, []byte(" x ")[:0])

	// Every White_Space rune Go's unicode.IsSpace reports true for.
	spaces := []rune{
		'\t', '\n', '\v', '\f', '\r', ' ',
		0x85, 0xA0, 0x1680,
		0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005,
		0x2006, 0x2007, 0x2008, 0x2009, 0x200A,
		0x2028, 0x2029, 0x202F, 0x205F, 0x3000,
	}
	seeds := []string{
		" ",
		" \t\r\n\v\f ",              // all-ASCII-space: both sides yield nil
		"x",                         // nothing to trim
		"  leading only",            //
		"trailing only\t\t",         //
		"  both sides  ",            //
		" interior  space  kept ",   // interior runs must survive verbatim
		"\u00a0nbsp padded\u00a0",   // multi-byte space at both ends
		"\u0085nel\u0085",           //
		"\u3000ideographic\u3000",   //
		"\u200bZWSP is NOT a space", // U+200B: IsSpace-false in Go
		"\ufeffBOM is not a space",  //
		"�",                         // literal RuneError: not a space either way
		" �\t",                      // RuneError inside the trimmed region's edges
		"\xc2",                      // truncated NBSP at the very start
		"\xc2 ",                     //
		" \xc2",                     // truncated NBSP after trailing-space start
		"\x85",                      // bare NEL byte: invalid alone, decodes RuneError
		"\xa0\xa0",                  // bare continuation bytes only
		"\xffdata\xff",              // invalid bytes as hard boundaries
		" \xff ",                    // ASCII space around an invalid byte
		"a\x00b",                    // NUL inside
		" \u00a0 \t\u3000 mixed run of spaces \u2028\r\n ",
		string(bytes.Repeat([]byte(" "), 4096)),                       // long all-space
		string(bytes.Repeat([]byte("x"), 4096)),                       // long no-trim ASCII fast path
		"  " + string(bytes.Repeat([]byte("x"), 4096)) + "\t\t",       // long ASCII with trimmed edges
		"\u00a0" + string(bytes.Repeat([]byte("x"), 4096)) + "\u00a0", // long input on the delegated rune path
		string(bytes.Repeat([]byte("\u3000"), 1024)),                  // long all-unicode-space
		string(bytes.Repeat([]byte(" x"), 2048)) + string(bytes.Repeat([]byte(" "), 64)),
	}
	for _, r := range spaces {
		seeds = append(seeds,
			string(r),
			string(r)+"core"+string(r),
			string(r)+string(r)+"x",
			"x"+string(r)+string(r),
			// The rune sliced to just its first byte on either boundary:
			// invalid UTF-8 that must NOT be trimmed.
			string(string(r)[:1])+"y"+string(string(r)[:1]),
		)
	}
	for _, s := range seeds {
		ps3031CheckShapes(t, []byte(s))
	}
}

func TestEquiv_PS3031_Randomized(t *testing.T) {
	rng := rand.New(rand.NewSource(0x30031))
	spaceRunes := []rune{'\t', '\n', ' ', 0x85, 0xA0, 0x1680, 0x2003, 0x2028, 0x202F, 0x3000}
	pickers := []func() byte{
		func() byte { return byte(rng.Intn(128)) },        // ASCII
		func() byte { return " \t\r\n\v\f"[rng.Intn(6)] }, // ASCII spaces
		func() byte { // bytes of multi-byte space runes (sliced mid-rune on purpose)
			enc := string(spaceRunes[rng.Intn(len(spaceRunes))])
			return enc[rng.Intn(len(enc))]
		},
		func() byte { return byte(0x80 + rng.Intn(64)) }, // bare continuations
		func() byte { return byte(rng.Intn(256)) },       // anything
	}
	for range 200000 {
		pick := pickers[rng.Intn(len(pickers))]
		b := make([]byte, rng.Intn(25))
		for i := range b {
			b[i] = pick()
		}
		// Half the time, splice complete space runes onto the ends so
		// deep trims of valid multi-byte white space arise often.
		if rng.Intn(2) == 0 {
			var padded []byte
			for range rng.Intn(4) {
				padded = append(padded, string(spaceRunes[rng.Intn(len(spaceRunes))])...)
			}
			padded = append(padded, b...)
			for range rng.Intn(4) {
				padded = append(padded, string(spaceRunes[rng.Intn(len(spaceRunes))])...)
			}
			b = padded
		}
		// Random spare capacity: the cap arithmetic must hold whatever
		// slack sits behind len.
		withCap := make([]byte, len(b), len(b)+rng.Intn(16))
		copy(withCap, b)
		ps3031Check(t, withCap)
	}
}

// TestEquiv_PS3031_NoAlloc pins the perf premise: neither spelling
// allocates — both return a subslice of b.
func TestEquiv_PS3031_NoAlloc(t *testing.T) {
	b := []byte("  \t padded payload with interior  spaces \u00a0 ")
	var out []byte
	if avg := testing.AllocsPerRun(100, func() { out = ps3031Before(b) }); avg != 0 {
		t.Errorf("TrimFunc allocates: %v allocs/op", avg)
	}
	if avg := testing.AllocsPerRun(100, func() { out = ps3031After(b) }); avg != 0 {
		t.Errorf("TrimSpace allocates: %v allocs/op", avg)
	}
	_ = out
}
