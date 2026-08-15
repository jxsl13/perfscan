package checks

// Runtime differential for PS2028: len(strings.Fields(s)) compared
// against the literal 0 or 1 (the six blankness shapes) versus
// strings.TrimSpace(s) == "" / != "". The fix's safety argument is
// that "s has at least one field" and "TrimSpace leaves something"
// are the SAME predicate — both functions classify runes with the
// identical whitespace set (the asciiSpace table below utf8.RuneSelf,
// unicode.IsSpace above it: Fields via its ASCII fast path falling
// back to FieldsFunc(s, unicode.IsSpace), TrimSpace via its ASCII fast
// path falling back to TrimFunc(s, unicode.IsSpace)). This suite pins
// that claim over:
//
//   - targeted states: the empty string, every ASCII space byte alone
//     and combined, non-ASCII spaces (NBSP U+00A0, NEL U+0085, the
//     Unicode Zs runs U+2000..U+200A, U+2028/U+2029, U+3000), ordinary
//     content, content flanked by exotic spaces, and invalid UTF-8
//     (lone continuation and lead bytes, truncated NBSP), where
//     utf8.RuneError is not a space in either function;
//   - an exhaustive sweep of ALL strings up to length 3 over an
//     adversarial byte alphabet (ASCII space and non-space, the NBSP
//     and NEL byte pairs' constituents, 0xFF, 0x80) — adjacent bytes
//     form multi-byte space runes and broken prefixes of them;
//   - an exhaustive per-rune sweep of every Unicode code point
//     (including surrogates, which string(r) turns into RuneError):
//     string(r) is blank under Fields iff it is blank under TrimSpace;
//   - randomized byte-strings and rune-strings with a fixed seed;
//   - the perf premise: the After shape allocates nothing, while the
//     Before shape allocates once s has a field.
//
// All six rewritten comparison shapes are checked for every input.

import (
	"math/rand"
	"strings"
	"testing"
	"unicode"
)

// ps2028Before* are the exact Before-shapes of the check.
func ps2028BeforeEq0(s string) bool { return len(strings.Fields(s)) == 0 }
func ps2028BeforeLt1(s string) bool { return len(strings.Fields(s)) < 1 }
func ps2028BeforeLe0(s string) bool { return len(strings.Fields(s)) <= 0 }
func ps2028BeforeNe0(s string) bool { return len(strings.Fields(s)) != 0 }
func ps2028BeforeGt0(s string) bool { return len(strings.Fields(s)) > 0 }
func ps2028BeforeGe1(s string) bool { return len(strings.Fields(s)) >= 1 }

// ps2028After* are the exact After-shapes of the check.
func ps2028AfterEq0(s string) bool { return strings.TrimSpace(s) == "" }
func ps2028AfterLt1(s string) bool { return strings.TrimSpace(s) == "" }
func ps2028AfterLe0(s string) bool { return strings.TrimSpace(s) == "" }
func ps2028AfterNe0(s string) bool { return strings.TrimSpace(s) != "" }
func ps2028AfterGt0(s string) bool { return strings.TrimSpace(s) != "" }
func ps2028AfterGe1(s string) bool { return strings.TrimSpace(s) != "" }

// ps2028Check pins every rewritten comparison shape on one input, and
// the underlying invariant itself: len(Fields(s)) == 0 iff
// TrimSpace(s) == "".
func ps2028Check(t *testing.T, s string) {
	t.Helper()
	if got, want := len(strings.Fields(s)) == 0, strings.TrimSpace(s) == ""; got != want {
		t.Fatalf("input %q: len(Fields)==0 is %v but TrimSpace==\"\" is %v — core invariant broken", s, got, want)
	}
	pairs := []struct {
		name          string
		before, after func(string) bool
	}{
		{"len==0", ps2028BeforeEq0, ps2028AfterEq0},
		{"len<1", ps2028BeforeLt1, ps2028AfterLt1},
		{"len<=0", ps2028BeforeLe0, ps2028AfterLe0},
		{"len!=0", ps2028BeforeNe0, ps2028AfterNe0},
		{"len>0", ps2028BeforeGt0, ps2028AfterGt0},
		{"len>=1", ps2028BeforeGe1, ps2028AfterGe1},
	}
	for _, p := range pairs {
		if got, want := p.before(s), p.after(s); got != want {
			t.Fatalf("input %q: %s diverges: before=%v after=%v", s, p.name, got, want)
		}
	}
}

func TestEquiv_PS2028_TargetedInputs(t *testing.T) {
	inputs := []string{
		"",                                // Fields -> empty slice; TrimSpace -> ""
		" ", "\t", "\n", "\v", "\f", "\r", // every asciiSpace byte alone
		" \t\n\v\f\r ",         // all of them combined
		"a", " a", "a ", " a ", // ordinary content, flanked
		"one two  three",
		"\u00a0",             // NBSP: IsSpace-true, non-ASCII in both
		"\u0085",             // NEL: IsSpace-true, non-ASCII in both
		"\u00a0 \u0085\t",    // mixed ASCII and non-ASCII spaces
		"\u2000\u2001\u200a", // the Zs run U+2000..U+200A (sampled)
		"\u2028\u2029\u3000", // line/paragraph separators, ideographic space
		"\u00a0x\u3000",      // content between exotic spaces
		"\u200b",             // ZERO WIDTH SPACE is NOT IsSpace: a field in both
		"\xff", "\x80",       // invalid UTF-8: RuneError, not a space, in both
		"\xc2",               // truncated NBSP lead byte: RuneError in both
		"\xc2\xa0",           // well-formed NBSP from raw bytes
		" \xff ", "\t\x80\t", // invalid bytes flanked by real spaces
		"héllo wörld",
		strings.Repeat(" ", 4096),
		strings.Repeat(" ", 4096) + "x",
		"x" + strings.Repeat(" ", 4096),
		strings.Repeat("a b ", 1024),
	}
	for _, s := range inputs {
		ps2028Check(t, s)
	}
}

// TestEquiv_PS2028_ExhaustiveShortStrings sweeps ALL strings up to
// length 3 over an adversarial byte alphabet: an ASCII space and
// non-space, the constituent bytes of multi-byte space runes (0xC2/0xA0
// forms NBSP, 0xC2/0x85 forms NEL, 0xE3/0x80 begins U+3000), a lone
// continuation byte and 0xFF. Adjacency builds well-formed non-ASCII
// spaces, truncated prefixes of them, and arbitrary garbage — every
// combination must agree.
func TestEquiv_PS2028_ExhaustiveShortStrings(t *testing.T) {
	alphabet := []byte{' ', '\t', 'a', 0xC2, 0xA0, 0x85, 0xE3, 0x80, 0xFF}
	var sweep func(prefix []byte, depth int)
	sweep = func(prefix []byte, depth int) {
		ps2028Check(t, string(prefix))
		if depth == 0 {
			return
		}
		for _, b := range alphabet {
			sweep(append(prefix, b), depth-1)
		}
	}
	sweep(nil, 3)
}

// TestEquiv_PS2028_AllRunes sweeps every Unicode code point: string(r)
// (which converts surrogates and out-of-range values to RuneError) is
// blank under Fields iff it is blank under TrimSpace, and both agree
// with unicode.IsSpace — the shared classifier the safety argument
// rests on.
func TestEquiv_PS2028_AllRunes(t *testing.T) {
	for r := rune(0); r <= unicode.MaxRune; r++ {
		s := string(r)
		before, after := ps2028BeforeEq0(s), ps2028AfterEq0(s)
		if before != after {
			t.Fatalf("rune %U: len(Fields)==0 is %v but TrimSpace==\"\" is %v", r, before, after)
		}
		// The decoded rune of string(r) is r itself for valid scalar
		// values and RuneError for surrogates; either way both sides
		// must equal unicode.IsSpace of that decoded rune.
		if want := unicode.IsSpace([]rune(s)[0]); before != want {
			t.Fatalf("rune %U: blankness %v disagrees with unicode.IsSpace %v", r, before, want)
		}
	}
}

// TestEquiv_PS2028_Randomized drives random byte-strings (arbitrary
// garbage, space-heavy) and random rune-strings through every
// comparison shape with a fixed seed.
func TestEquiv_PS2028_Randomized(t *testing.T) {
	rng := rand.New(rand.NewSource(0x2028))
	spacey := []byte{' ', '\t', '\n', '\v', '\f', '\r', 0xC2, 0xA0, 0x85, 0xE3, 0x80, 0xFF, 'a'}
	for range 2000 {
		// Arbitrary bytes.
		b := make([]byte, rng.Intn(64))
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		ps2028Check(t, string(b))
		// Space-heavy bytes: mostly whitespace and space-rune fragments,
		// so the all-blank branch is actually exercised.
		c := make([]byte, rng.Intn(64))
		for i := range c {
			c[i] = spacey[rng.Intn(len(spacey))]
		}
		ps2028Check(t, string(c))
		// Random valid runes biased toward the space table.
		var sb strings.Builder
		for range rng.Intn(24) {
			if rng.Intn(2) == 0 {
				sb.WriteRune(rune(rng.Intn(0x3001)))
			} else {
				sb.WriteRune(rune(rng.Intn(unicode.MaxRune + 1)))
			}
		}
		ps2028Check(t, sb.String())
	}
}

// TestEquiv_PS2028_AfterAllocFree pins the perf premise: the After
// shape allocates nothing, while the Before shape allocates the whole
// []string of fields — that slice is the entire delta the rewrite
// removes.
func TestEquiv_PS2028_AfterAllocFree(t *testing.T) {
	line := strings.Repeat("field ", 64)
	var sink bool
	if avg := testing.AllocsPerRun(100, func() { sink = ps2028AfterEq0(line) }); avg != 0 {
		t.Errorf("strings.TrimSpace(s) == \"\" allocates %v times per run, want 0", avg)
	}
	if avg := testing.AllocsPerRun(100, func() { sink = ps2028BeforeEq0(line) }); avg == 0 {
		t.Log("len(strings.Fields(s)) == 0 did not allocate — the compiler learned to elide Fields; the win claim may need re-framing")
	}
	_ = sink
}
