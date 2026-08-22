package checks

// Runtime differential for PS2029: len(strings.SplitN(s, sep, 2)) /
// len(bytes.SplitN(b, sep, 2)) compared against the literal 1 or 2
// (the eight membership shapes) versus strings.Contains /
// bytes.Contains — and, for the chained one-byte fixed points,
// IndexByte >= 0 / < 0. The fix's safety argument is that for a
// NON-EMPTY separator SplitN(s, sep, 2) returns min(k+1, 2) pieces
// (k = non-overlapping occurrences), so its length is exactly 2 iff
// the separator occurs and exactly 1 otherwise — never 0, never more
// than 2 — while the empty separator (the ONE divergence, where SplitN
// rune-explodes and Contains is constant true) is excluded by the
// constant-non-empty guard. This suite pins that claim over:
//
//   - targeted states: the empty haystack, sep == haystack, sep at
//     either end, sep repeated and overlapping ("aa" in "aaa"), sep
//     longer than the haystack, NUL bytes, invalid UTF-8 in both the
//     haystack and the separator, and multi-byte separators whose
//     prefix occurs without the full match;
//   - an exhaustive sweep of ALL haystacks up to length 4 over an
//     adversarial byte alphabet crossed with ALL non-empty separators
//     up to length 2 over the same alphabet (broken UTF-8 lead and
//     continuation bytes included) — every combination must agree for
//     both the strings and the bytes twin;
//   - the chained fixed points: for EVERY one-byte separator value
//     0..255, len(SplitN) == 2 must equal IndexByte >= 0 on both
//     twins;
//   - randomized haystack/separator pairs with a fixed seed;
//   - the nil-haystack edge of the bytes twin;
//   - the perf premise: the After shape allocates nothing, while the
//     Before shape allocates the piece slice.
//
// All eight rewritten comparison shapes are checked for every input.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// ps2029Check pins the core invariant and every rewritten comparison
// shape on one (haystack, separator) pair, for both twins. sep must be
// non-empty — the check's guard — and the caller only passes such.
func ps2029Check(t *testing.T, s, sep string) {
	t.Helper()
	if sep == "" {
		t.Fatalf("test bug: empty separator is outside the guarded domain")
	}
	n := len(strings.SplitN(s, sep, 2))
	if n != 1 && n != 2 {
		t.Fatalf("s=%q sep=%q: len(strings.SplitN(s, sep, 2)) = %d, want 1 or 2 — length algebra broken", s, sep, n)
	}
	if got, want := n == 2, strings.Contains(s, sep); got != want {
		t.Fatalf("s=%q sep=%q: len(SplitN)==2 is %v but Contains is %v — core invariant broken", s, sep, got, want)
	}
	// All eight strings shapes.
	pairs := []struct {
		name          string
		before, after bool
	}{
		{"len==2", len(strings.SplitN(s, sep, 2)) == 2, strings.Contains(s, sep)},
		{"len>=2", len(strings.SplitN(s, sep, 2)) >= 2, strings.Contains(s, sep)},
		{"len>1", len(strings.SplitN(s, sep, 2)) > 1, strings.Contains(s, sep)},
		{"len!=1", len(strings.SplitN(s, sep, 2)) != 1, strings.Contains(s, sep)},
		{"len==1", len(strings.SplitN(s, sep, 2)) == 1, !strings.Contains(s, sep)},
		{"len<=1", len(strings.SplitN(s, sep, 2)) <= 1, !strings.Contains(s, sep)},
		{"len<2", len(strings.SplitN(s, sep, 2)) < 2, !strings.Contains(s, sep)},
		{"len!=2", len(strings.SplitN(s, sep, 2)) != 2, !strings.Contains(s, sep)},
	}
	for _, p := range pairs {
		if p.before != p.after {
			t.Fatalf("s=%q sep=%q: %s diverges: before=%v after=%v", s, sep, p.name, p.before, p.after)
		}
	}
	// The bytes twin on the same bytes.
	bs, bsep := []byte(s), []byte(sep)
	bn := len(bytes.SplitN(bs, bsep, 2))
	if bn != n {
		t.Fatalf("s=%q sep=%q: bytes.SplitN length %d != strings.SplitN length %d", s, sep, bn, n)
	}
	if got, want := bn == 2, bytes.Contains(bs, bsep); got != want {
		t.Fatalf("s=%q sep=%q: bytes len(SplitN)==2 is %v but bytes.Contains is %v", s, sep, got, want)
	}
	// The chained one-byte fixed point, whenever the separator is one
	// byte: len(SplitN)==2 must equal IndexByte >= 0 on both twins.
	if len(sep) == 1 {
		if got, want := n == 2, strings.IndexByte(s, sep[0]) >= 0; got != want {
			t.Fatalf("s=%q sep=%q: len(SplitN)==2 is %v but strings.IndexByte>=0 is %v", s, sep, got, want)
		}
		if got, want := bn == 2, bytes.IndexByte(bs, sep[0]) >= 0; got != want {
			t.Fatalf("s=%q sep=%q: bytes len(SplitN)==2 is %v but bytes.IndexByte>=0 is %v", s, sep, got, want)
		}
	}
}

func TestEquiv_PS2029_TargetedInputs(t *testing.T) {
	type pair struct{ s, sep string }
	inputs := []pair{
		{"", ","},             // empty haystack: [""], len 1; Contains false
		{",", ","},            // sep == haystack: ["",""], len 2
		{"a,b", ","},          // the classic
		{",ab", ","},          // sep at the start
		{"ab,", ","},          // sep at the end
		{"a,b,c", ","},        // repeated: SplitN stops at the first
		{"aaa", "aa"},         // overlapping occurrences
		{"aa", "aaa"},         // sep longer than the haystack
		{"ab", "=>"},          // absent multi-byte sep
		{"a=>b", "=>"},        // present multi-byte sep
		{"a=b", "=>"},         // prefix of sep occurs, full sep does not
		{"a==>b", "=>"},       // sep preceded by its own first byte
		{"\x00a\x00", "\x00"}, // NUL separator
		{"héllo wörld", "ö"},  // multi-byte rune separator
		{"h\xffx", "\xff"},    // invalid UTF-8 separator, present
		{"hx", "\xff"},        // invalid UTF-8 separator, absent
		{"\xc2\xa0", "\xc2"},  // broken lead byte as separator
		{"\xc2\xa0", "\xa0"},  // continuation byte as separator
		{strings.Repeat("a", 4096), "b"},
		{strings.Repeat("a", 4096) + "b", "b"},
		{"b" + strings.Repeat("a", 4096), "b"},
		{strings.Repeat("k=v&", 1024), "&"},
	}
	for _, in := range inputs {
		ps2029Check(t, in.s, in.sep)
	}
	// The nil-haystack edge of the bytes twin: SplitN(nil, sep, 2) is
	// [nil] (length 1) and Contains(nil, sep) is false for a non-empty
	// separator.
	if got := len(bytes.SplitN(nil, []byte(","), 2)); got != 1 {
		t.Fatalf("len(bytes.SplitN(nil, sep, 2)) = %d, want 1", got)
	}
	if bytes.Contains(nil, []byte(",")) {
		t.Fatalf("bytes.Contains(nil, non-empty sep) = true, want false")
	}
}

// TestEquiv_PS2029_EmptySepDivergence documents WHY the non-empty
// guard exists: for the empty separator the two sides genuinely
// differ, so the check must never fire there. This is the divergence
// the guard excludes, pinned so it stays excluded.
func TestEquiv_PS2029_EmptySepDivergence(t *testing.T) {
	// SplitN(s, "", 2) rune-explodes: its length is min(2, rune count)
	// — 0 for the empty haystack — while Contains(s, "") is always true.
	if got := len(strings.SplitN("", "", 2)); got != 0 {
		t.Fatalf("len(strings.SplitN(\"\", \"\", 2)) = %d, want 0", got)
	}
	if got := len(strings.SplitN("a", "", 2)); got != 1 {
		t.Fatalf("len(strings.SplitN(\"a\", \"\", 2)) = %d, want 1", got)
	}
	if got := len(strings.SplitN("ab", "", 2)); got != 2 {
		t.Fatalf("len(strings.SplitN(\"ab\", \"\", 2)) = %d, want 2", got)
	}
	if !strings.Contains("", "") || !strings.Contains("a", "") {
		t.Fatalf("strings.Contains(s, \"\") must be constant true")
	}
	// (len == 2) != Contains for both "" and "a": the identity is
	// genuinely broken without the guard.
	for _, s := range []string{"", "a"} {
		if (len(strings.SplitN(s, "", 2)) == 2) == strings.Contains(s, "") {
			t.Fatalf("s=%q: empty separator did NOT diverge — the guard would be unnecessary", s)
		}
	}
}

// TestEquiv_PS2029_Exhaustive sweeps ALL haystacks up to length 4 over
// an adversarial byte alphabet (separator byte, its repetition fodder,
// an ordinary byte, a UTF-8 lead byte, a continuation byte) crossed
// with ALL non-empty separators up to length 2 over the same alphabet.
// Adjacency builds overlapping matches, broken UTF-8 and
// prefix-but-no-match shapes — every combination must agree on both
// twins and all eight shapes.
func TestEquiv_PS2029_Exhaustive(t *testing.T) {
	alphabet := []byte{',', 'a', '=', 0xC2, 0xA0}
	var haystacks []string
	var grow func(prefix []byte, depth int)
	grow = func(prefix []byte, depth int) {
		haystacks = append(haystacks, string(prefix))
		if depth == 0 {
			return
		}
		for _, b := range alphabet {
			grow(append(prefix, b), depth-1)
		}
	}
	grow(nil, 4)
	var seps []string
	for _, b := range alphabet {
		seps = append(seps, string([]byte{b}))
		for _, c := range alphabet {
			seps = append(seps, string([]byte{b, c}))
		}
	}
	for _, s := range haystacks {
		for _, sep := range seps {
			ps2029Check(t, s, sep)
		}
	}
}

// TestEquiv_PS2029_AllOneByteSeps drives every one-byte separator
// value 0..255 — the chained IndexByte fixed point's whole domain —
// against haystacks that contain it at the start, middle, end, not at
// all, and as part of multi-byte garbage.
func TestEquiv_PS2029_AllOneByteSeps(t *testing.T) {
	for v := 0; v < 256; v++ {
		sep := string([]byte{byte(v)})
		other := byte(v + 1)
		hay := []string{
			"",
			sep,
			sep + "tail",
			"head" + sep,
			"mid" + sep + "dle",
			string([]byte{other, other, other}),
			string([]byte{other, byte(v), other}),
		}
		for _, s := range hay {
			ps2029Check(t, s, sep)
		}
	}
}

// TestEquiv_PS2029_Randomized drives random haystack/separator pairs
// (arbitrary garbage bytes, separator-dense haystacks) through every
// comparison shape with a fixed seed.
func TestEquiv_PS2029_Randomized(t *testing.T) {
	rng := rand.New(rand.NewSource(0x2029))
	for range 5000 {
		sep := make([]byte, 1+rng.Intn(3))
		for i := range sep {
			sep[i] = byte(rng.Intn(256))
		}
		b := make([]byte, rng.Intn(64))
		for i := range b {
			if rng.Intn(4) == 0 {
				// Seed separator fragments so the present branch is
				// actually exercised.
				b[i] = sep[rng.Intn(len(sep))]
			} else {
				b[i] = byte(rng.Intn(256))
			}
		}
		ps2029Check(t, string(b), string(sep))
	}
}

// TestEquiv_PS2029_AfterAllocFree pins the perf premise: the After
// shape allocates nothing, while the Before shape allocates the piece
// slice — that slice is the entire delta the rewrite removes.
func TestEquiv_PS2029_AfterAllocFree(t *testing.T) {
	line := strings.Repeat("key-", 100) + "=>" + strings.Repeat("-value", 100)
	bline := []byte(line)
	sep := "=>"
	bsep := []byte("=>")
	var sink bool
	if avg := testing.AllocsPerRun(100, func() { sink = strings.Contains(line, sep) }); avg != 0 {
		t.Errorf("strings.Contains allocates %v times per run, want 0", avg)
	}
	if avg := testing.AllocsPerRun(100, func() { sink = bytes.Contains(bline, bsep) }); avg != 0 {
		t.Errorf("bytes.Contains allocates %v times per run, want 0", avg)
	}
	if avg := testing.AllocsPerRun(100, func() { sink = len(strings.SplitN(line, sep, 2)) == 2 }); avg == 0 {
		t.Log("len(strings.SplitN(s, sep, 2)) == 2 did not allocate — the compiler learned to elide SplitN; the win claim may need re-framing")
	}
	_ = sink
}
