package checks

// Runtime differential for PS2022: bytes.Equal(b, []byte(s)) vs
// string(b) == s (and the mirror bytes.Equal([]byte(s), b) vs
// s == string(b)) for a plain []byte b and a plain string s. The safety
// argument: bytes.Equal(a, b) is documented and implemented as
// string(a) == string(b), and string([]byte(s)) == s is a byte-for-byte
// round-trip identity for every string s — including invalid UTF-8, NUL
// bytes and the empty string — so the two forms agree on ALL inputs.
// Both compare raw bytes with no rune interpretation, so there is no
// case-folding or normalization to diverge on. nil: bytes.Equal(nil,
// []byte("")) is a len-0 == len-0 comparison (true) and string(nil) is
// "" (true). The suite covers:
//
//   - EXHAUSTIVE short inputs: every []byte of length <= 4 crossed with
//     every string of length <= 4 over an adversarial alphabet (a plain
//     ASCII byte, NUL, the two bytes of a multi-byte rune so truncated
//     and complete sequences both arise, and 0xFF which is never valid
//     UTF-8), plus the nil slice against every short string;
//   - targeted seeds: nil vs empty, equal long inputs, a long shared
//     prefix with a last-byte difference, length mismatches in both
//     directions, embedded NULs, invalid UTF-8 on either side, and a
//     mid-rune byte difference;
//   - randomized long inputs over the full byte range with a fixed
//     seed, plus an equal-pair stream where after must return true;
//   - side-effect order: each operand is evaluated exactly once, in the
//     original left-to-right order, in both the direct and the mirror
//     rewrite.

import (
	"bytes"
	"math/rand"
	"testing"
)

// ps2022Before is the matched shape, verbatim.
func ps2022Before(b []byte, s string) bool {
	return bytes.Equal(b, []byte(s))
}

// ps2022After is the rewrite.
func ps2022After(b []byte, s string) bool {
	return string(b) == s
}

// ps2022BeforeMirror is the mirrored matched shape, verbatim.
func ps2022BeforeMirror(b []byte, s string) bool {
	return bytes.Equal([]byte(s), b)
}

// ps2022AfterMirror is the mirrored rewrite.
func ps2022AfterMirror(b []byte, s string) bool {
	return s == string(b)
}

func ps2022Check(t *testing.T, b []byte, s string) {
	t.Helper()
	if before, after := ps2022Before(b, s), ps2022After(b, s); before != after {
		t.Fatalf("divergence on b=%q s=%q:\n bytes.Equal(b, []byte(s)) = %v\n string(b) == s = %v",
			b, s, before, after)
	}
	if before, after := ps2022BeforeMirror(b, s), ps2022AfterMirror(b, s); before != after {
		t.Fatalf("mirror divergence on b=%q s=%q:\n bytes.Equal([]byte(s), b) = %v\n s == string(b) = %v",
			b, s, before, after)
	}
}

// ps2022Alphabet anchors a plain ASCII byte, NUL, the two bytes of é
// (0xC3 0xA9) so truncated and complete UTF-8 sequences both arise, and
// 0xFF, which is never valid UTF-8.
var ps2022Alphabet = []byte{'a', 0x00, 0xC3, 0xA9, 0xFF}

func TestEquiv_PS2022_ExhaustiveShort(t *testing.T) {
	const maxLen = 4
	var enum func(buf []byte, depth int, emit func([]byte))
	enum = func(buf []byte, depth int, emit func([]byte)) {
		if depth == len(buf) {
			emit(buf)
			return
		}
		for _, c := range ps2022Alphabet {
			buf[depth] = c
			enum(buf, depth+1, emit)
		}
	}
	var slices [][]byte
	slices = append(slices, nil) // the nil slice, distinct from []byte{}
	for n := 0; n <= maxLen; n++ {
		enum(make([]byte, n), 0, func(b []byte) {
			slices = append(slices, append([]byte(nil), b...))
		})
	}
	var strs []string
	for n := 0; n <= maxLen; n++ {
		enum(make([]byte, n), 0, func(b []byte) {
			strs = append(strs, string(b))
		})
	}
	for _, b := range slices {
		for _, s := range strs {
			ps2022Check(t, b, s)
		}
	}
}

func TestEquiv_PS2022_TargetedSeeds(t *testing.T) {
	long := bytes.Repeat([]byte("alpha beta gamma delta "), 128)
	type seed struct {
		b []byte
		s string
	}
	seeds := []seed{
		{nil, ""},                                // nil vs empty: both true
		{[]byte{}, ""},                           // empty vs empty
		{nil, "x"},                               // nil vs non-empty
		{[]byte("x"), ""},                        // non-empty vs empty
		{long, string(long)},                     // equal long inputs
		{long, string(long[:len(long)-1]) + "!"}, // long shared prefix, last byte differs
		{long, string(long) + "tail"},            // length mismatch, b shorter
		{append(long, "tail"...), string(long)},  // length mismatch, b longer
		{[]byte("a\x00b"), "a\x00b"},             // embedded NUL, equal
		{[]byte("a\x00b"), "a\x00c"},             // embedded NUL, differs after it
		{[]byte("héllo"), "héllo"},               // valid multi-byte, equal
		{[]byte("h\xc3llo"), "héllo"},            // truncated rune vs complete rune
		{[]byte("\xff\xfe"), "\xff\xfe"},         // invalid UTF-8, equal
		{[]byte("\xff\xfe"), "\xff\xff"},         // invalid UTF-8, differs
		{[]byte("日本語"), "日本語"},                   // multi-byte runes, equal
		{[]byte("日本語"), "日本誤"},                   // differs inside the last rune
	}
	for _, sd := range seeds {
		ps2022Check(t, sd.b, sd.s)
	}
}

func TestEquiv_PS2022_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25082022))
	gen := func(min, max int) []byte {
		b := make([]byte, min+rng.Intn(max-min+1))
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		return b
	}
	// Independent pairs: almost always unequal, at random lengths, over
	// the full byte range (mostly invalid UTF-8).
	for range 100000 {
		ps2022Check(t, gen(0, 64), string(gen(0, 64)))
	}
	// Equal pairs: the rewrite must return true exactly when the call
	// does, so feed it pairs that ARE equal — with an occasional one-byte
	// or one-length perturbation so near-misses are covered too.
	for range 100000 {
		b := gen(0, 64)
		s := string(b)
		switch rng.Intn(4) {
		case 0:
			if len(b) > 0 {
				i := rng.Intn(len(b))
				b = append([]byte(nil), b...)
				b[i] ^= byte(1 + rng.Intn(255))
			}
		case 1:
			b = append(append([]byte(nil), b...), byte(rng.Intn(256)))
		}
		ps2022Check(t, b, s)
	}
}

// TestEquiv_PS2022_SideEffectOrder pins that each operand is evaluated
// exactly once, in the original left-to-right order, in both rewrites:
// b then s for bytes.Equal(b, []byte(s)) -> string(b) == s, and s then b
// for bytes.Equal([]byte(s), b) -> s == string(b).
func TestEquiv_PS2022_SideEffectOrder(t *testing.T) {
	run := func(f func(g func() []byte, h func() string) bool) []string {
		var log []string
		g := func() []byte { log = append(log, "b"); return []byte("v") }
		h := func() string { log = append(log, "s"); return "v" }
		if !f(g, h) {
			t.Fatal("expected the equal pair to compare true")
		}
		return log
	}
	check := func(name string, got, want []string) {
		if len(got) != len(want) {
			t.Fatalf("%s: evaluated %v, want %v", name, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s: evaluated %v, want %v", name, got, want)
			}
		}
	}
	check("before", run(func(g func() []byte, h func() string) bool {
		return bytes.Equal(g(), []byte(h()))
	}), []string{"b", "s"})
	check("after", run(func(g func() []byte, h func() string) bool {
		return string(g()) == h()
	}), []string{"b", "s"})
	check("before mirror", run(func(g func() []byte, h func() string) bool {
		return bytes.Equal([]byte(h()), g())
	}), []string{"s", "b"})
	check("after mirror", run(func(g func() []byte, h func() string) bool {
		return h() == string(g())
	}), []string{"s", "b"})
}
