package checks

// Runtime differential for PS2020: bytes.Join(bytes.Split(b, sep), new)
// vs bytes.ReplaceAll(b, sep, new) for a NON-EMPTY separator — the bytes
// twin of the PS2015 suite. The fix's safety argument is the Split/Join
// inverse identity: Split cuts b at exactly the non-overlapping
// occurrences of sep found left-to-right via Index (byte-literal — no
// rune interpretation anywhere), Join re-inserts new between the
// fragments, and Replace finds the SAME non-overlapping occurrences with
// the same Index scan and substitutes new at each — so
// p0+new+p1+...+new+pk is byte-for-byte ReplaceAll's output. Beyond the
// string twin, byte slices add two observables the suite pins too:
// NIL-NESS of the result (asserted equal on every case, nil and empty
// inputs included) and ALIASING (both forms must return a slice that
// never shares memory with b — asserted by mutating the results of a
// third computation and re-checking). The suite covers:
//
//   - EXHAUSTIVE short inputs: every b of length <= 5 over an adversarial
//     alphabet (ASCII, the two bytes of a multi-byte rune so truncated and
//     complete sequences both arise, and 0xFF which is never valid UTF-8),
//     crossed with non-empty separators of 1 and 2 bytes including rune
//     fragments, invalid and NUL bytes, and replacements that are nil,
//     empty, equal to the separator, longer than it, and invalid UTF-8;
//   - targeted seeds: nil and empty inputs, long inputs, separator-only
//     inputs, zero-match inputs, NUL separators, CJK separators,
//     sep == new (identity round-trip), overlapping-candidate separators
//     ("aa" over "aaaa");
//   - randomized long inputs over the full byte range and over a tiny
//     separator-dense alphabet with a fixed seed;
//   - the divergence shape the statically-non-empty gate exists for:
//     an empty sep makes Split explode b after each UTF-8 sequence and
//     Join fill the k-1 gaps while ReplaceAll inserts new k+1 times — if
//     that ever stops diverging, the gate is no longer load-bearing and
//     should be revisited.

import (
	"bytes"
	"math/rand"
	"testing"
)

// ps2020Check pins Join(Split(b, sep), new) == ReplaceAll(b, sep, new)
// for a non-empty separator: same bytes AND same nil-ness.
func ps2020Check(t *testing.T, b, sep, repl []byte) {
	t.Helper()
	if len(sep) == 0 {
		t.Fatal("test bug: sep must be non-empty — the empty separator is the divergence shape, never matched by PS2020")
	}
	joined := bytes.Join(bytes.Split(b, sep), repl)
	replaced := bytes.ReplaceAll(b, sep, repl)
	if !bytes.Equal(joined, replaced) {
		t.Fatalf("divergence on b=%q sep=%q new=%q:\n Join(Split)=%q\n ReplaceAll=%q",
			b, sep, repl, joined, replaced)
	}
	if (joined == nil) != (replaced == nil) {
		t.Fatalf("nil-ness divergence on b=%q sep=%q new=%q: Join(Split)==nil is %v, ReplaceAll==nil is %v",
			b, sep, repl, joined == nil, replaced == nil)
	}
}

// ps2020Alphabet anchors plain matches ('a','b'), straddles rune
// boundaries (0xC3 0xA9 are the bytes of é) and includes 0xFF, which is
// never valid UTF-8 — so separator matches can cut runes apart at every
// alignment.
var ps2020Alphabet = []byte{'a', 'b', 0xC3, 0xA9, 0xFF}

func ps2020ShortSlices(maxLen int) [][]byte {
	var out [][]byte
	buf := make([]byte, maxLen)
	var rec func(n, depth int)
	rec = func(n, depth int) {
		if depth == n {
			out = append(out, append([]byte(nil), buf[:n]...))
			return
		}
		for _, c := range ps2020Alphabet {
			buf[depth] = c
			rec(n, depth+1)
		}
	}
	for n := 0; n <= maxLen; n++ {
		rec(n, 0)
	}
	return out
}

func TestEquiv_PS2020_ExhaustiveShort(t *testing.T) {
	slices := ps2020ShortSlices(5)
	// "a" and "aa" exercise dense and adjacent (overlap-candidate)
	// matches over the same inputs; {0xC3} is a rune fragment, {0xFF} an
	// invalid byte and {0x00} a NUL, so matches can cut runes apart; "é"
	// is a complete 2-byte rune whose bytes also appear separately in the
	// alphabet.
	seps := [][]byte{[]byte("a"), []byte("aa"), []byte("ab"), {0xC3}, {0xFF}, {0x00}, []byte("é")}
	// Nil and empty (deletion), 1-byte, separator-equal, longer-than-sep
	// and invalid-UTF-8 replacements.
	repls := [][]byte{nil, {}, []byte(";"), []byte("a"), []byte("; "), {0xFF, 0xC3}, []byte("é")}
	for _, b := range slices {
		for _, sep := range seps {
			for _, repl := range repls {
				ps2020Check(t, b, sep, repl)
			}
		}
	}
}

func TestEquiv_PS2020_TargetedSeeds(t *testing.T) {
	long := bytes.Repeat([]byte("field-"), 4096)
	type seed struct {
		b, sep, repl []byte
	}
	seeds := []seed{
		{[]byte("a,b,c,d,e"), []byte(","), []byte("; ")},                                    // the doc's Before/After
		{nil, []byte(","), []byte(";")},                                                     // nil input: one nil piece, zero matches
		{[]byte{}, []byte(","), []byte(";")},                                                // empty non-nil input
		{[]byte("a"), []byte(","), []byte(";")},                                             // no match: single piece round-trip
		{[]byte("no-separator-here at all"), []byte("MISSING"), []byte("x")},                // zero matches on a long input
		{[]byte(",,,,"), []byte(","), []byte(";")},                                          // separators only: empty fields everywhere
		{[]byte(",a,"), []byte(","), []byte("::")},                                          // leading and trailing separators
		{[]byte("aaaa"), []byte("aa"), []byte("b")},                                         // overlapping candidates: non-overlapping left-to-right match
		{[]byte("aaaaa"), []byte("aa"), []byte("b")},                                        // odd tail after adjacent matches
		{[]byte("a,b,c"), []byte(","), []byte(",")},                                         // sep == new: identity round-trip
		{[]byte("a,b"), []byte(","), nil},                                                   // nil replacement: deletion
		{[]byte("héllo,wörld,日本,語"), []byte(","), []byte("、")},                              // multibyte fields, multibyte replacement
		{[]byte("日本語日本語"), []byte("本"), []byte("ほん")},                                       // CJK separator
		{[]byte("a\xFFb\xFFc"), []byte{0xFF}, []byte("?")},                                  // invalid-UTF-8 separator
		{[]byte("\xC3\x28\xC3\x28"), []byte{0xC3}, []byte{}},                                // truncated-rune separator, deleting
		{[]byte("a\x00b\x00c"), []byte{0x00}, []byte(" ")},                                  // NUL separator
		{bytes.Repeat([]byte("é"), 64), []byte("é"), []byte("e")},                           // input consists ONLY of separators
		{append(append([]byte("x"), long...), 'x'), []byte("-"), []byte("_")},               // long, separator-dense
		{append(append([]byte(nil), long...), long...), []byte("field"), []byte("FIELD")},   // separator == field text, longer replacement
		{append(bytes.Repeat([]byte("　"), 64), []byte("end")...), []byte("　"), []byte(".")}, // long multibyte separator run
		{[]byte("tail-sep-grows"), []byte("-"), bytes.Repeat([]byte("+"), 8)},               // replacement much longer than separator
	}
	for _, sd := range seeds {
		ps2020Check(t, sd.b, sd.sep, sd.repl)
	}
}

func TestEquiv_PS2020_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25082020))
	gen := func(al []byte, min, max int) []byte {
		b := make([]byte, min+rng.Intn(max-min+1))
		for i := range b {
			b[i] = al[rng.Intn(len(al))]
		}
		return b
	}
	full := make([]byte, 256)
	for i := range full {
		full[i] = byte(i)
	}
	// Full byte range: mostly invalid UTF-8 at random alignments.
	for range 50000 {
		b := gen(full, 0, 48)
		sep := gen(full, 1, 3) // non-empty by construction
		repl := gen(full, 0, 4)
		ps2020Check(t, b, sep, repl)
	}
	// Tiny alphabet so separator matches are dense and adjacent, and the
	// replacement often equals or contains the separator.
	tiny := []byte{'a', 'b', 0xC3, 0xA9}
	for range 50000 {
		b := gen(tiny, 0, 32)
		sep := gen(tiny, 1, 2)
		repl := gen(tiny, 0, 3)
		ps2020Check(t, b, sep, repl)
	}
}

// TestEquiv_PS2020_NoAliasing pins the slice-specific half of the safety
// argument: NEITHER form may return a slice sharing memory with the
// input, so the rewrite cannot change mutation visibility. (Split's
// fragments DO alias b, but Join copies out of them; ReplaceAll copies
// too, on the zero-match path included.)
func TestEquiv_PS2020_NoAliasing(t *testing.T) {
	for _, sep := range [][]byte{[]byte(","), []byte("x")} { // matching and zero-match separators
		b := []byte("a,b,c")
		joined := bytes.Join(bytes.Split(b, sep), []byte(";"))
		replaced := bytes.ReplaceAll(b, sep, []byte(";"))
		for i := range joined {
			joined[i] = 'J'
		}
		for i := range replaced {
			replaced[i] = 'R'
		}
		if !bytes.Equal(b, []byte("a,b,c")) {
			t.Fatalf("sep=%q: mutating a result wrote through to the input: b=%q — a form aliases its input and the rewrite could change mutation visibility", sep, b)
		}
	}
}

// TestEquiv_PS2020_EmptySepDiverges pins the divergence the
// statically-non-empty gate exists for: Split(b, empty) explodes b after
// each UTF-8 sequence into its k subslices and Join fills the k-1 gaps,
// while ReplaceAll(b, empty, new) matches the empty slice before and
// after every rune and inserts new k+1 times. Nil and empty separators
// behave identically. If this ever stops diverging, the gate is no
// longer load-bearing and the check should be revisited.
func TestEquiv_PS2020_EmptySepDiverges(t *testing.T) {
	b, repl := []byte("ab"), []byte(";")
	for _, sep := range [][]byte{nil, {}} {
		joined := bytes.Join(bytes.Split(b, sep), repl) // "a;b"
		replaced := bytes.ReplaceAll(b, sep, repl)      // ";a;b;"
		if bytes.Equal(joined, replaced) {
			t.Fatalf("expected Join(Split(b, %q), %q) and ReplaceAll(b, %q, %q) to diverge on b=%q, but both returned %q — the non-empty-separator gate may be obsolete",
				sep, repl, sep, repl, b, joined)
		}
	}
	// Even the empty input diverges: Split(empty, empty) has ZERO pieces
	// so the join is empty, while ReplaceAll(empty, empty, new) inserts
	// new once.
	if got := bytes.Join(bytes.Split(nil, nil), repl); bytes.Equal(got, bytes.ReplaceAll(nil, nil, repl)) {
		t.Fatalf("expected divergence on the empty input with the empty separator, both returned %q", got)
	}
}
