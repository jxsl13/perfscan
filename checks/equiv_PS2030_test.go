package checks

// Runtime differential for PS2030: len(bytes.Fields(b)) compared
// against the literal 0 or 1 (the six blankness shapes) versus
// len(bytes.TrimSpace(b)) compared the same way. The fix's safety
// argument is that "b has at least one field" and "TrimSpace leaves
// at least one byte" are the SAME predicate — both functions classify
// runes with the identical whitespace set (the asciiSpace table below
// utf8.RuneSelf, unicode.IsSpace above it: Fields via its ASCII fast
// path falling back to FieldsFunc(b, unicode.IsSpace), TrimSpace via
// its ASCII fast path falling back to TrimFunc(b, unicode.IsSpace)).
// Note the two lens count DIFFERENT things on non-blank input (fields
// vs bytes) — the equivalence is zero-vs-nonzero only, which is
// exactly why the check is restricted to the six zero/nonzero shapes.
// This suite pins the claim over:
//
//   - targeted states: nil, the empty slice, every ASCII space byte
//     alone and combined, non-ASCII spaces (NBSP U+00A0, NEL U+0085,
//     the Unicode Zs runs U+2000..U+200A, U+2028/U+2029, U+3000),
//     ordinary content, content flanked by exotic spaces, and invalid
//     UTF-8 (lone continuation and lead bytes, truncated NBSP), where
//     utf8.RuneError is not a space in either function;
//   - an exhaustive sweep of ALL byte slices up to length 3 over an
//     adversarial byte alphabet (ASCII space and non-space, the NBSP
//     and NEL byte pairs' constituents, 0xFF, 0x80) — adjacent bytes
//     form multi-byte space runes and broken prefixes of them;
//   - an exhaustive per-rune sweep of every Unicode code point
//     (including surrogates, which []byte(string(r)) turns into
//     RuneError bytes): the encoding of r is blank under Fields iff it
//     is blank under TrimSpace, and both agree with unicode.IsSpace;
//   - randomized byte-slices and rune-slices with a fixed seed;
//   - the perf premise: the After shape allocates nothing, while the
//     Before shape allocates once b has a field.
//
// All six rewritten comparison shapes are checked for every input.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"unicode"
)

// ps2030Before* are the exact Before-shapes of the check.
func ps2030BeforeEq0(b []byte) bool { return len(bytes.Fields(b)) == 0 }
func ps2030BeforeLt1(b []byte) bool { return len(bytes.Fields(b)) < 1 }
func ps2030BeforeLe0(b []byte) bool { return len(bytes.Fields(b)) <= 0 }
func ps2030BeforeNe0(b []byte) bool { return len(bytes.Fields(b)) != 0 }
func ps2030BeforeGt0(b []byte) bool { return len(bytes.Fields(b)) > 0 }
func ps2030BeforeGe1(b []byte) bool { return len(bytes.Fields(b)) >= 1 }

// ps2030After* are the exact After-shapes of the check: the SAME
// comparison with only the Fields identifier renamed.
func ps2030AfterEq0(b []byte) bool { return len(bytes.TrimSpace(b)) == 0 }
func ps2030AfterLt1(b []byte) bool { return len(bytes.TrimSpace(b)) < 1 }
func ps2030AfterLe0(b []byte) bool { return len(bytes.TrimSpace(b)) <= 0 }
func ps2030AfterNe0(b []byte) bool { return len(bytes.TrimSpace(b)) != 0 }
func ps2030AfterGt0(b []byte) bool { return len(bytes.TrimSpace(b)) > 0 }
func ps2030AfterGe1(b []byte) bool { return len(bytes.TrimSpace(b)) >= 1 }

// ps2030Check pins every rewritten comparison shape on one input, and
// the underlying invariant itself: len(bytes.Fields(b)) == 0 iff
// len(bytes.TrimSpace(b)) == 0.
func ps2030Check(t *testing.T, b []byte) {
	t.Helper()
	if got, want := len(bytes.Fields(b)) == 0, len(bytes.TrimSpace(b)) == 0; got != want {
		t.Fatalf("input %q: len(Fields)==0 is %v but len(TrimSpace)==0 is %v — core invariant broken", b, got, want)
	}
	pairs := []struct {
		name          string
		before, after func([]byte) bool
	}{
		{"len==0", ps2030BeforeEq0, ps2030AfterEq0},
		{"len<1", ps2030BeforeLt1, ps2030AfterLt1},
		{"len<=0", ps2030BeforeLe0, ps2030AfterLe0},
		{"len!=0", ps2030BeforeNe0, ps2030AfterNe0},
		{"len>0", ps2030BeforeGt0, ps2030AfterGt0},
		{"len>=1", ps2030BeforeGe1, ps2030AfterGe1},
	}
	for _, p := range pairs {
		if got, want := p.before(b), p.after(b); got != want {
			t.Fatalf("input %q: %s diverges: before=%v after=%v", b, p.name, got, want)
		}
	}
}

func TestEquiv_PS2030_TargetedInputs(t *testing.T) {
	inputs := [][]byte{
		nil,                                                                               // Fields -> empty slice; TrimSpace -> nil re-slice
		{},                                                                                // empty but non-nil
		[]byte(" "), []byte("\t"), []byte("\n"), []byte("\v"), []byte("\f"), []byte("\r"), // every asciiSpace byte alone
		[]byte(" \t\n\v\f\r "),                                 // all of them combined
		[]byte("a"), []byte(" a"), []byte("a "), []byte(" a "), // ordinary content, flanked
		[]byte("one two  three"),
		[]byte("\u00a0"),             // NBSP: IsSpace-true, non-ASCII in both
		[]byte("\u0085"),             // NEL: IsSpace-true, non-ASCII in both
		[]byte("\u00a0 \u0085\t"),    // mixed ASCII and non-ASCII spaces
		[]byte("\u2000\u2001\u200a"), // the Zs run U+2000..U+200A (sampled)
		[]byte("\u2028\u2029\u3000"), // line/paragraph separators, ideographic space
		[]byte("\u00a0x\u3000"),      // content between exotic spaces
		[]byte("\u200b"),             // ZERO WIDTH SPACE is NOT IsSpace: a field in both
		{0xFF}, {0x80},               // invalid UTF-8: RuneError, not a space, in both
		{0xC2},                               // truncated NBSP lead byte: RuneError in both
		{0xC2, 0xA0},                         // well-formed NBSP from raw bytes
		[]byte(" \xff "), []byte("\t\x80\t"), // invalid bytes flanked by real spaces
		[]byte("héllo wörld"),
		bytes.Repeat([]byte(" "), 4096),
		append(bytes.Repeat([]byte(" "), 4096), 'x'),
		append([]byte("x"), bytes.Repeat([]byte(" "), 4096)...),
		bytes.Repeat([]byte("a b "), 1024),
	}
	for _, b := range inputs {
		ps2030Check(t, b)
	}
}

// TestEquiv_PS2030_ExhaustiveShortSlices sweeps ALL byte slices up to
// length 3 over an adversarial byte alphabet: an ASCII space and
// non-space, the constituent bytes of multi-byte space runes (0xC2/0xA0
// forms NBSP, 0xC2/0x85 forms NEL, 0xE3/0x80 begins U+3000), a lone
// continuation byte and 0xFF. Adjacency builds well-formed non-ASCII
// spaces, truncated prefixes of them, and arbitrary garbage — every
// combination must agree.
func TestEquiv_PS2030_ExhaustiveShortSlices(t *testing.T) {
	alphabet := []byte{' ', '\t', 'a', 0xC2, 0xA0, 0x85, 0xE3, 0x80, 0xFF}
	var sweep func(prefix []byte, depth int)
	sweep = func(prefix []byte, depth int) {
		ps2030Check(t, prefix)
		if depth == 0 {
			return
		}
		for _, c := range alphabet {
			sweep(append(prefix[:len(prefix):len(prefix)], c), depth-1)
		}
	}
	sweep(nil, 3)
}

// TestEquiv_PS2030_AllRunes sweeps every Unicode code point:
// []byte(string(r)) (which converts surrogates and out-of-range values
// to the RuneError encoding) is blank under Fields iff it is blank
// under TrimSpace, and both agree with unicode.IsSpace — the shared
// classifier the safety argument rests on.
func TestEquiv_PS2030_AllRunes(t *testing.T) {
	for r := rune(0); r <= unicode.MaxRune; r++ {
		b := []byte(string(r))
		before, after := ps2030BeforeEq0(b), ps2030AfterEq0(b)
		if before != after {
			t.Fatalf("rune %U: len(Fields)==0 is %v but len(TrimSpace)==0 is %v", r, before, after)
		}
		// The decoded rune of string(r) is r itself for valid scalar
		// values and RuneError for surrogates; either way both sides
		// must equal unicode.IsSpace of that decoded rune.
		if want := unicode.IsSpace([]rune(string(b))[0]); before != want {
			t.Fatalf("rune %U: blankness %v disagrees with unicode.IsSpace %v", r, before, want)
		}
	}
}

// TestEquiv_PS2030_Randomized drives random byte-slices (arbitrary
// garbage, space-heavy) and random rune-slices through every
// comparison shape with a fixed seed.
func TestEquiv_PS2030_Randomized(t *testing.T) {
	rng := rand.New(rand.NewSource(0x2030))
	spacey := []byte{' ', '\t', '\n', '\v', '\f', '\r', 0xC2, 0xA0, 0x85, 0xE3, 0x80, 0xFF, 'a'}
	for range 2000 {
		// Arbitrary bytes.
		b := make([]byte, rng.Intn(64))
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		ps2030Check(t, b)
		// Space-heavy bytes: mostly whitespace and space-rune fragments,
		// so the all-blank branch is actually exercised.
		c := make([]byte, rng.Intn(64))
		for i := range c {
			c[i] = spacey[rng.Intn(len(spacey))]
		}
		ps2030Check(t, c)
		// Random valid runes biased toward the space table.
		var sb strings.Builder
		for range rng.Intn(24) {
			if rng.Intn(2) == 0 {
				sb.WriteRune(rune(rng.Intn(0x3001)))
			} else {
				sb.WriteRune(rune(rng.Intn(unicode.MaxRune + 1)))
			}
		}
		ps2030Check(t, []byte(sb.String()))
	}
}

// TestEquiv_PS2030_AfterAllocFree pins the perf premise: the After
// shape allocates nothing, while the Before shape allocates the whole
// [][]byte of fields — that slice is the entire delta the rewrite
// removes.
func TestEquiv_PS2030_AfterAllocFree(t *testing.T) {
	line := bytes.Repeat([]byte("field "), 64)
	var sink bool
	if avg := testing.AllocsPerRun(100, func() { sink = ps2030AfterEq0(line) }); avg != 0 {
		t.Errorf("len(bytes.TrimSpace(b)) == 0 allocates %v times per run, want 0", avg)
	}
	if avg := testing.AllocsPerRun(100, func() { sink = ps2030BeforeEq0(line) }); avg == 0 {
		t.Log("len(bytes.Fields(b)) == 0 did not allocate — the compiler learned to elide Fields; the win claim may need re-framing")
	}
	_ = sink
}
