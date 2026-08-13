package checks_test

// This suite pins the SEMANTIC equivalences that perfscan's stdlib-based
// auto-fixes rely on but analysistest cannot check: analysistest verifies a
// check produces the right rewritten TEXT, not that the rewrite is
// behavior-identical. Here we actually RUN the before/after forms over edge
// cases and assert byte-for-byte identical results, so a future Go stdlib change
// (or an ill-considered widening) that breaks a bit-identity claim fails CI
// rather than silently shipping a behavior-changing fix.

import (
	"bytes"
	"cmp"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// PS2119: `for _, v := range strings.Split(s, sep)` -> `for v := range
// strings.SplitSeq(s, sep)` (Go 1.24). The rewrite is bit-identical only if
// SplitSeq yields exactly the Split slice's elements in order, for EVERY
// separator — including "" (rune split) and multi-byte seps.
func TestEquiv_SplitSeq(t *testing.T) {
	inputs := []string{"", "a", "a,b,c", ",", ",,", "a,", ",a", "a,,b", "héllo", "日本,語", "x,y,z,"}
	seps := []string{",", "", "a", ",,", "日", "xyz", "\x00"}
	for _, s := range inputs {
		for _, sep := range seps {
			var seq []string
			for v := range strings.SplitSeq(s, sep) {
				seq = append(seq, v)
			}
			if got := strings.Split(s, sep); !slices.Equal(seq, got) {
				t.Errorf("strings.SplitSeq(%q,%q)=%q != range Split=%q", s, sep, seq, got)
			}
			var after []string
			for v := range strings.SplitAfterSeq(s, sep) {
				after = append(after, v)
			}
			if got := strings.SplitAfter(s, sep); !slices.Equal(after, got) {
				t.Errorf("strings.SplitAfterSeq(%q,%q)=%q != range SplitAfter=%q", s, sep, after, got)
			}

			var bseq [][]byte
			for v := range bytes.SplitSeq([]byte(s), []byte(sep)) {
				bseq = append(bseq, v)
			}
			if got := bytes.Split([]byte(s), []byte(sep)); !equalByteSlices(bseq, got) {
				t.Errorf("bytes.SplitSeq(%q,%q) != range Split", s, sep)
			}
		}
	}
}

// PS2119 (Fields arm): `for _, v := range strings.Fields(s)` -> `for v := range
// strings.FieldsSeq(s)` (Go 1.24). Bit-identical only if FieldsSeq yields
// exactly the Fields slice's elements in order — across every whitespace shape
// (leading/trailing/internal runs, tabs/newlines, Unicode spaces, all-space,
// empty). Fields splits on unicode.IsSpace, which is why "" seps don't apply
// here; this is the arm TestEquiv_SplitSeq does not exercise.
func TestEquiv_FieldsSeq(t *testing.T) {
	inputs := []string{
		"", " ", "   ", "a", "a b c", "  a  b  ", "\tx\ny\r\nz\t",
		"one", "  leading", "trailing  ", "日本 語  x", "a b", // NBSP is a space to IsSpace
		"mix\t \n of  \t whitespace",
	}
	for _, s := range inputs {
		var seq []string
		for v := range strings.FieldsSeq(s) {
			seq = append(seq, v)
		}
		if got := strings.Fields(s); !slices.Equal(seq, got) {
			t.Errorf("strings.FieldsSeq(%q)=%q != range Fields=%q", s, seq, got)
		}

		var bseq [][]byte
		for v := range bytes.FieldsSeq([]byte(s)) {
			bseq = append(bseq, v)
		}
		if got := bytes.Fields([]byte(s)); !equalByteSlices(bseq, got) {
			t.Errorf("bytes.FieldsSeq(%q) != range Fields", s)
		}
	}
}

// PS2119 (argument snapshot): both the eager and the lazy form evaluate the
// range operand — Split(s, sep) or SplitSeq(s, sep) — exactly ONCE at loop
// entry, and a string argument is an immutable value copy. Reassigning s
// inside the body therefore affects NEITHER form; this pins that the fix
// needs no "s reassigned in body" guard for the strings arm.
func TestEquiv_SplitSeq_ArgSnapshot(t *testing.T) {
	s1 := "a,b,c"
	var eager []string
	for _, v := range strings.Split(s1, ",") {
		eager = append(eager, v)
		s1 = "x,y"
	}
	s2 := "a,b,c"
	var lazy []string
	for v := range strings.SplitSeq(s2, ",") {
		lazy = append(lazy, v)
		s2 = "x,y"
	}
	want := []string{"a", "b", "c"}
	if !slices.Equal(eager, want) || !slices.Equal(lazy, want) {
		t.Errorf("reassigning s in the body must affect neither form: eager=%q lazy=%q want=%q", eager, lazy, want)
	}
}

// PS2119 (bytes arm is ADVISORY-ONLY — this pins the divergences that forbid
// the auto-fix). Unlike strings, a []byte source is mutable and the fragments
// alias it, and the two forms are observably different in three ways:
//
//  1. bytes.Split pins every fragment boundary BEFORE the body runs;
//     bytes.SplitSeq re-runs Index lazily between yields, so a body write
//     into the source's backing array changes the number and content of
//     later fragments under the Seq form only.
//  2. genSplit's FINAL fragment is the raw tail `s` (cap reaches the
//     source's spare capacity) while splitSeq clamps every fragment to its
//     length — cap(lastPiece) diverges.
//  3. Consequently, append to the final piece writes into the source
//     buffer's spare capacity under the eager form but allocates a fresh
//     array under the lazy one.
//
// If a future stdlib change makes any arm below fail, re-evaluate the
// advisory (mutation, arm 1, is inherent to laziness and cannot converge).
func TestEquiv_SplitSeq_BytesAliasingDivergence(t *testing.T) {
	// 1: boundary divergence under body mutation.
	b := []byte("a,b,c")
	var eager []string
	for _, v := range bytes.Split(b, []byte{','}) {
		eager = append(eager, string(v))
		b[3] = 'X' // clobber the 2nd separator after the 1st yield
	}
	b2 := []byte("a,b,c")
	var lazy []string
	for v := range bytes.SplitSeq(b2, []byte{','}) {
		lazy = append(lazy, string(v))
		b2[3] = 'X'
	}
	if want := []string{"a", "b", "c"}; !slices.Equal(eager, want) {
		t.Errorf("bytes.Split under body mutation: got %q, want %q (eager boundaries must be pinned)", eager, want)
	}
	if want := []string{"a", "bXc"}; !slices.Equal(lazy, want) {
		t.Errorf("bytes.SplitSeq under body mutation: got %q, want %q (lazy re-scan)", lazy, want)
	}

	// 2: cap of the final fragment.
	src := make([]byte, 0, 64)
	src = append(src, "a,b"...)
	frags := bytes.Split(src, []byte{','})
	eagerLast := frags[len(frags)-1]
	var lazyLast []byte
	for v := range bytes.SplitSeq(src, []byte{','}) {
		lazyLast = v
	}
	if cap(lazyLast) != len(lazyLast) {
		t.Errorf("bytes.SplitSeq final fragment must be cap-clamped: len=%d cap=%d", len(lazyLast), cap(lazyLast))
	}
	if cap(eagerLast) == cap(lazyLast) {
		t.Errorf("cap parity reached (eager=%d lazy=%d): the bytes advisory's cap arm no longer holds — re-check genSplit", cap(eagerLast), cap(lazyLast))
	}

	// 3: append to the final piece — eager spills into src's spare capacity.
	mk := func() []byte { b := make([]byte, 0, 16); return append(b, "a,b"...) }
	e := mk()
	for _, v := range bytes.Split(e, []byte{','}) {
		v = append(v, '!')
		_ = v
	}
	l := mk()
	for v := range bytes.SplitSeq(l, []byte{','}) {
		v = append(v, '!')
		_ = v
	}
	if e[:4][3] != '!' {
		t.Errorf("eager append to final piece must spill into the source's spare capacity, got %q", e[:4])
	}
	if l[:4][3] == '!' {
		t.Errorf("lazy append to final piece must NOT touch the source buffer, got %q", l[:4])
	}
}

// PS2112: append(append([]T(nil), a...), b...) -> slices.Concat(a, b). Concat
// must yield exactly the chained-append result INCLUDING nil-ness on the edges.
// The subtle part: slices.Concat is nil-PRESERVING for all-empty inputs (it does
// Grow(nil, 0), which returns nil) — UNLIKE slices.Clone, which returns a non-nil
// empty. Since the chained append onto []T(nil) also yields nil when everything
// is empty, the two agree; this pins that agreement so a future change to Grow's
// zero behavior (or Concat's) would fail rather than silently ship a nil/non-nil
// divergence.
func TestEquiv_Concat(t *testing.T) {
	var nilS []int
	empt := []int{}
	full := []int{1, 2, 3}
	cases := [][2][]int{
		{nilS, nilS}, // all-nil -> both nil
		{empt, empt}, // all-empty non-nil -> both STILL nil (Grow(nil,0))
		{nilS, empt}, // mixed empty -> nil
		{full, nilS}, // one side empty
		{nilS, full}, // other side empty
		{full, full}, // both non-empty
		{empt, full}, // empty + non-empty
	}
	for _, c := range cases {
		a, b := c[0], c[1]
		got := append(append([]int(nil), a...), b...)
		want := slices.Concat(a, b)
		if (got == nil) != (want == nil) || !slices.Equal(got, want) {
			t.Errorf("append(append([]int(nil), %v...), %v...)=%v (nil=%v) != slices.Concat=%v (nil=%v)",
				a, b, got, got == nil, want, want == nil)
		}
	}
	// bytes form (PS2112 handles any element type): all-empty stays nil, too.
	if gb, wb := append(append([]byte(nil), []byte(nil)...), []byte{}...), slices.Concat([]byte(nil), []byte{}); (gb == nil) != (wb == nil) {
		t.Errorf("bytes all-empty: chained nil=%v != slices.Concat nil=%v", gb == nil, wb == nil)
	}
	if gb, wb := append(append([]byte(nil), []byte("x")...), []byte("yz")...), slices.Concat([]byte("x"), []byte("yz")); !bytes.Equal(gb, wb) {
		t.Errorf("bytes non-empty: chained append != slices.Concat")
	}
}

// PS5102: buf.WriteRune('x') -> buf.WriteByte('x') when the rune is a single
// UTF-8 byte (r < utf8.RuneSelf). For those runes WriteRune emits exactly the
// one byte WriteByte does; for r >= RuneSelf it emits a multi-byte encoding, so
// the rewrite's r < RuneSelf guard is load-bearing — pinned in both directions,
// for strings.Builder and bytes.Buffer (the two writers PS5102 targets).
func TestEquiv_WriteRuneByte(t *testing.T) {
	for r := rune(0); r < utf8.RuneSelf; r++ {
		var sb strings.Builder
		sb.WriteRune(r)
		var bb bytes.Buffer
		bb.WriteByte(byte(r))
		if sb.String() != bb.String() {
			t.Fatalf("WriteRune(%#U)=%q != WriteByte(%#x)=%q", r, sb.String(), byte(r), bb.String())
		}
	}
	// A multi-byte rune MUST differ — otherwise the single-byte guard would be
	// unnecessary and a widened check would silently corrupt UTF-8.
	var sb strings.Builder
	sb.WriteRune('é') // U+00E9 -> 0xC3 0xA9
	var bb bytes.Buffer
	bb.WriteByte(byte('é')) // 0xE9
	if sb.String() == bb.String() {
		t.Fatal("expected WriteRune != WriteByte for a multi-byte rune (the r<RuneSelf guard is load-bearing)")
	}
}

// PS3104: sort.Ints/sort.Strings -> slices.Sort. Both must produce the identical
// ordering (they share pdqsort since go1.21); pinned across random + tie-heavy
// inputs so a divergence would be caught.
func TestEquiv_SlicesSort(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for trial := 0; trial < 3000; trial++ {
		n := r.Intn(50)
		base := make([]int, n)
		for i := range base {
			base[i] = r.Intn(6) // small domain -> many ties
		}
		a := slices.Clone(base)
		b := slices.Clone(base)
		sort.Ints(a)
		slices.Sort(b)
		if !slices.Equal(a, b) {
			t.Fatalf("sort.Ints != slices.Sort on %v: %v vs %v", base, a, b)
		}
	}
}

// PS3002: sort.Slice(x, func(i,j) bool) -> slices.SortFunc(x, func(a,b) int)
// with cmp.Compare. Both share pdqsort, so a comparator inducing the same order
// yields the identical permutation (incl. ties) — the whole basis of the
// PS3002 multi-field / descending fixes.
func TestEquiv_SortSliceToSortFunc(t *testing.T) {
	type kv struct{ a, b int }
	r := rand.New(rand.NewSource(2))
	for trial := 0; trial < 3000; trial++ {
		n := r.Intn(40)
		base := make([]kv, n)
		for i := range base {
			base[i] = kv{r.Intn(4), r.Intn(4)} // ties across the first key
		}
		x := slices.Clone(base)
		y := slices.Clone(base)
		// original: multi-field ascending bool comparator
		sort.Slice(x, func(i, j int) bool {
			if x[i].a != x[j].a {
				return x[i].a < x[j].a
			}
			return x[i].b < x[j].b
		})
		// rewritten: cmp.Compare tie-break chain
		slices.SortFunc(y, func(p, q kv) int {
			if p.a != q.a {
				return cmp.Compare(p.a, q.a)
			}
			return cmp.Compare(p.b, q.b)
		})
		if !slices.Equal(x, y) {
			t.Fatalf("sort.Slice != slices.SortFunc on trial %d", trial)
		}
	}
}

// PS3002: the TWO-RETURN guard pair spelling
//
//	if x[i].a < x[j].a { return true }
//	if x[i].a > x[j].a { return false }
//
// plus a bare `return false` tail ("all fields equal") is the same total
// order as the emitted `if p.a != q.a { return cmp.Compare(p.a, q.a) }` chain
// ending in `return 0`: when a field differs exactly one if of its pair fires
// with the '<' verdict, when all are equal both comparators say "equal"
// (false ↔ 0). Both sorts share pdqsort, so equal-order comparators must give
// the identical permutation — pinned over tie-heavy random data (small value
// domain, an id field to expose any tie reordering), STABLE and UNSTABLE.
func TestEquiv_SortSliceTwoReturnPair(t *testing.T) {
	type kv struct{ a, b, id int }
	intCmp := func(p, q kv) int {
		if p.a != q.a {
			return cmp.Compare(p.a, q.a)
		}
		if p.b != q.b {
			return cmp.Compare(p.b, q.b)
		}
		return 0
	}
	r := rand.New(rand.NewSource(4))
	for trial := 0; trial < 3000; trial++ {
		n := r.Intn(40)
		base := make([]kv, n)
		for i := range base {
			base[i] = kv{r.Intn(4), r.Intn(4), i} // small domain -> many ties
		}
		boolCmp := func(x []kv) func(i, j int) bool {
			return func(i, j int) bool {
				if x[i].a < x[j].a {
					return true
				}
				if x[i].a > x[j].a {
					return false
				}
				if x[i].b < x[j].b {
					return true
				}
				if x[i].b > x[j].b {
					return false
				}
				return false
			}
		}
		x := slices.Clone(base)
		y := slices.Clone(base)
		sort.Slice(x, boolCmp(x))
		slices.SortFunc(y, intCmp)
		if !slices.Equal(x, y) {
			t.Fatalf("unstable: pair comparator != cmp chain on trial %d: %v vs %v", trial, x, y)
		}
		xs := slices.Clone(base)
		ys := slices.Clone(base)
		sort.SliceStable(xs, boolCmp(xs))
		slices.SortStableFunc(ys, intCmp)
		if !slices.Equal(xs, ys) {
			t.Fatalf("stable: pair comparator != cmp chain on trial %d: %v vs %v", trial, xs, ys)
		}

		// FALSE-FIRST descending pair on a (the '<' if returns false and comes
		// first, the '>' if returns true and carries the direction), then an
		// ascending final compare on b — the swapped-halves spelling the check
		// also accepts, emitted as cmp.Compare(q.a, p.a) then cmp.Compare(p.b,
		// q.b). Must be the identical permutation, stable and unstable.
		ffBool := func(x []kv) func(i, j int) bool {
			return func(i, j int) bool {
				if x[i].a < x[j].a {
					return false
				}
				if x[i].a > x[j].a {
					return true
				}
				return x[i].b < x[j].b
			}
		}
		ffInt := func(p, q kv) int {
			if p.a != q.a {
				return cmp.Compare(q.a, p.a)
			}
			return cmp.Compare(p.b, q.b)
		}
		fx := slices.Clone(base)
		fy := slices.Clone(base)
		sort.Slice(fx, ffBool(fx))
		slices.SortFunc(fy, ffInt)
		if !slices.Equal(fx, fy) {
			t.Fatalf("unstable: false-first desc pair != cmp chain on trial %d: %v vs %v", trial, fx, fy)
		}
		fxs := slices.Clone(base)
		fys := slices.Clone(base)
		sort.SliceStable(fxs, ffBool(fxs))
		slices.SortStableFunc(fys, ffInt)
		if !slices.Equal(fxs, fys) {
			t.Fatalf("stable: false-first desc pair != cmp chain on trial %d: %v vs %v", trial, fxs, fys)
		}
	}
}

// PS2116 / PS3102: a zeroing loop -> clear(s); a delete loop -> clear(m).
func TestEquiv_Clear(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for trial := 0; trial < 1000; trial++ {
		n := r.Intn(20)
		a := make([]int, n)
		b := make([]int, n)
		for i := range a {
			a[i] = r.Int()
			b[i] = a[i]
		}
		for i := range a {
			a[i] = 0
		}
		clear(b)
		if !slices.Equal(a, b) {
			t.Fatal("zeroing loop != clear(slice)")
		}

		m1 := map[int]int{}
		m2 := map[int]int{}
		for i := 0; i < n; i++ {
			m1[r.Intn(10)] = i
		}
		for k, v := range m1 {
			m2[k] = v
		}
		for k := range m1 {
			delete(m1, k)
		}
		clear(m2)
		if len(m1) != 0 || len(m2) != 0 {
			t.Fatal("delete loop != clear(map)")
		}
	}
}

// PS2121: len(strings.Split(s, sep)) -> strings.Count(s, sep)+1 (and the bytes
// analog). The identity holds for every NON-EMPTY separator — the only case the
// check ever rewrites — across all inputs including "". The test also pins the
// DIVERGENCE at sep=="", which is exactly why ps2121SepNonEmpty is load-bearing:
// were the guard ever widened to admit a variable or empty separator, this fails
// rather than shipping a fix that changes the counted value.
func TestEquiv_CountPlusOne(t *testing.T) {
	inputs := []string{"", "a", "a,b,c", ",", ",,", "a,", ",a", "a,,b", "héllo", "日本,語", "x,y,z,", "abcabc"}
	seps := []string{",", "a", ",,", "abc", "日", "\x00", "bc"} // all provably non-empty
	for _, s := range inputs {
		for _, sep := range seps {
			if got, want := strings.Count(s, sep)+1, len(strings.Split(s, sep)); got != want {
				t.Errorf("strings: Count(%q,%q)+1=%d != len(Split)=%d", s, sep, got, want)
			}
			bs, bsep := []byte(s), []byte(sep)
			if got, want := bytes.Count(bs, bsep)+1, len(bytes.Split(bs, bsep)); got != want {
				t.Errorf("bytes: Count(%q,%q)+1=%d != len(Split)=%d", s, sep, got, want)
			}
		}
	}
	// The empty separator is exactly where the identity breaks: Count(s,"") is
	// runes+1 while Split(s,"") is one piece per rune. If this ever stops
	// diverging the guard could be relaxed; until then it must stay.
	if strings.Count("abc", "")+1 == len(strings.Split("abc", "")) {
		t.Fatal("expected len(Split)!=Count+1 for the empty separator (guard would be unnecessary)")
	}
}

// PS2110: the nil/empty truth table behind append([]T(nil), s...) and
// append([]T{}, s...) -> slices.Clone(s) (bytes.Clone for []byte). The rewrite
// is bit-identical only when the divergent input is provably impossible, which
// is precisely what ps2110SliceFacts proves: neverEmpty for the []T(nil) form,
// neverNil for the []T{} form. This pins both the safe matches and the two
// nil-ness divergences the guards exist to avoid.
func TestEquiv_CloneNilEmpty(t *testing.T) {
	var nilS []int
	emptyS := []int{}

	// []T(nil) form == Clone whenever s is non-empty (the neverEmpty case)...
	for _, s := range [][]int{{1, 2, 3}, {0}} {
		got, want := append([]int(nil), s...), slices.Clone(s)
		if (got == nil) != (want == nil) || !slices.Equal(got, want) {
			t.Errorf("append([]int(nil), %v...) != slices.Clone (nil form, non-empty)", s)
		}
	}
	// ...but diverges on an empty non-nil s: append yields nil, Clone non-nil.
	if (append([]int(nil), emptyS...) == nil) == (slices.Clone(emptyS) == nil) {
		t.Fatal("expected append([]int(nil), emptyNonNil...) to diverge from slices.Clone in nil-ness")
	}

	// []T{} form == Clone whenever s is non-nil (the neverNil case)...
	for _, s := range [][]int{{1, 2, 3}, emptyS} {
		got, want := append([]int{}, s...), slices.Clone(s)
		if (got == nil) != (want == nil) || !slices.Equal(got, want) {
			t.Errorf("append([]int{}, %v...) != slices.Clone (empty form, non-nil)", s)
		}
	}
	// ...but diverges on a nil s: append yields non-nil empty, Clone nil.
	if (append([]int{}, nilS...) == nil) == (slices.Clone(nilS) == nil) {
		t.Fatal("expected append([]int{}, nil...) to diverge from slices.Clone in nil-ness")
	}

	// bytes.Clone shares the exact contract for the []byte element case.
	if (append([]byte(nil), []byte{}...) == nil) == (bytes.Clone([]byte{}) == nil) {
		t.Fatal("expected append([]byte(nil), emptyNonNil...) to diverge from bytes.Clone")
	}
	if !bytes.Equal(append([]byte(nil), []byte("xy")...), bytes.Clone([]byte("xy"))) {
		t.Fatal("append([]byte(nil), nonempty...) != bytes.Clone")
	}
}

// PS5101: bytes.Compare(a,b)==0 <-> bytes.Equal(a,b).
// PS5104: (strings/bytes.Count(s,sub)>0) <-> Contains(s,sub).
// PS5105: (strings/bytes.Index(s,sub)==0) <-> HasPrefix(s,sub).
// All three are blind (no per-call-site guard), so the equivalence must hold for
// EVERY input — including "", invalid UTF-8, and sub longer than s.
func TestEquiv_CompareContainsPrefix(t *testing.T) {
	strs := []string{"", "a", "ab", "abc", "abcabc", "héllo", "日本語", "\x00", "bca", "\xff\xfe"}
	subs := []string{"", "a", "ab", "abc", "bc", "x", "日", "abcabc", "\x00", "\xff"}
	for _, a := range strs {
		for _, b := range strs {
			// Comparing bytes.Compare==0 against bytes.Equal IS the point here
			// (that is exactly the PS5101 rewrite); staticcheck's S1004 would
			// have us delete one side of the equivalence.
			//lint:ignore S1004 pinning the PS5101 rewrite requires both spellings
			if (bytes.Compare([]byte(a), []byte(b)) == 0) != bytes.Equal([]byte(a), []byte(b)) {
				t.Errorf("bytes.Compare==0 != bytes.Equal on %q,%q", a, b)
			}
		}
		for _, sub := range subs {
			if (strings.Count(a, sub) > 0) != strings.Contains(a, sub) {
				t.Errorf("strings.Count>0 != Contains on %q,%q", a, sub)
			}
			if (bytes.Count([]byte(a), []byte(sub)) > 0) != bytes.Contains([]byte(a), []byte(sub)) {
				t.Errorf("bytes.Count>0 != Contains on %q,%q", a, sub)
			}
			if (strings.Index(a, sub) == 0) != strings.HasPrefix(a, sub) {
				t.Errorf("strings.Index==0 != HasPrefix on %q,%q", a, sub)
			}
			if (bytes.Index([]byte(a), []byte(sub)) == 0) != bytes.HasPrefix([]byte(a), []byte(sub)) {
				t.Errorf("bytes.Index==0 != HasPrefix on %q,%q", a, sub)
			}
		}
	}
}

// PS2124: strings.Join([]string{a, b, ...}, sep) -> interleaved concatenation.
// The check only fires on an inline literal, so the element count is known; pin
// the boundary lengths (0, 1, 2) the interleaving edit special-cases.
func TestEquiv_JoinLiteral(t *testing.T) {
	sep := "-"
	if strings.Join([]string{"a", "b", "c"}, sep) != "a"+sep+"b"+sep+"c" {
		t.Error("Join 3-elem != interleaved concat")
	}
	if strings.Join([]string{"x"}, sep) != "x" {
		t.Error("Join 1-elem != the element (no separator)")
	}
	if strings.Join([]string{}, sep) != "" {
		t.Error("Join empty literal != empty string")
	}
	if strings.Join([]string{"", ""}, sep) != ""+sep+"" {
		t.Error("Join two empties != a single separator")
	}
}

// PS2125: len([]rune(s)) -> utf8.RuneCountInString(s); len([]byte(s)) -> len(s).
// Invalid UTF-8 is included: []rune decodes each bad byte to U+FFFD and
// RuneCountInString counts it identically, so the identity must still hold.
func TestEquiv_LenConversions(t *testing.T) {
	for _, s := range []string{"", "a", "abc", "héllo", "日本語", "a\x00b", "\xff\xfe", "é"} {
		if len([]rune(s)) != utf8.RuneCountInString(s) {
			t.Errorf("len([]rune(%q))=%d != RuneCountInString=%d", s, len([]rune(s)), utf8.RuneCountInString(s))
		}
		if len([]byte(s)) != len(s) {
			t.Errorf("len([]byte(%q)) != len(s)", s)
		}
	}
}

// PS2107: fmt.Sprintf of a single bare verb -> the direct strconv/hex call.
// Why each family is bit-identical:
//
//   - %d over any integer: fmt formats integers by plain base-10 division with
//     a leading '-' for negatives — exactly strconv's algorithm (fmt actually
//     defers to the same digit logic). strconv.Itoa(i) == FormatInt(int64(i), 10)
//     by definition, and FormatInt handles MinInt64 correctly (it works in
//     uint64 magnitude space, so the -x overflow trap does not exist there).
//     For unsigned kinds the widening uint64(u) is value-preserving for every
//     width up to uint64 itself, so FormatUint(uint64(u), 10) prints the same
//     digits %d does, including MaxUint64 at full width.
//   - %t over bool: fmt prints exactly "true"/"false"; strconv.FormatBool
//     returns exactly those two strings.
//   - %x over []byte: fmt hex-dumps the bytes as lowercase hex, two digits per
//     byte, no separator — precisely hex.EncodeToString's output, including
//     nil and empty both yielding "".
//
// PS2107 only rewrites UNNAMED basic/[]byte types (a named type could carry a
// fmt.Formatter that fmt would honor), so unnamed values are what we pin here.
// (%s over a string is an identity owned by PS2130, deliberately not tested.)
func TestEquiv_Sprintf2107(t *testing.T) {
	// %d / int -> strconv.Itoa. MinInt64 is the critical edge: a naive
	// "negate then format" would overflow; both fmt and strconv must not.
	ints := []int{
		0, 1, -1, 7, -7, 42, -37, 123456789, -987654321,
		math.MaxInt32, math.MinInt32,
		math.MaxInt64, math.MinInt64,
	}
	for _, i := range ints {
		if got, want := fmt.Sprintf("%d", i), strconv.Itoa(i); got != want {
			t.Errorf("fmt.Sprintf(%%d, %d)=%q != strconv.Itoa=%q", i, got, want)
		}
	}

	// %d / unsigned widths -> strconv.FormatUint(uint64(u), 10). MaxUint64 is
	// the full-width edge; each smaller width's max pins the value-preserving
	// widening.
	for _, u := range []uint{0, 1, 42, math.MaxUint32, math.MaxUint} {
		if got, want := fmt.Sprintf("%d", u), strconv.FormatUint(uint64(u), 10); got != want {
			t.Errorf("fmt.Sprintf(%%d, uint(%d))=%q != strconv.FormatUint=%q", u, got, want)
		}
	}
	for _, u := range []uint8{0, 1, 42, math.MaxUint8} {
		if got, want := fmt.Sprintf("%d", u), strconv.FormatUint(uint64(u), 10); got != want {
			t.Errorf("fmt.Sprintf(%%d, uint8(%d))=%q != strconv.FormatUint=%q", u, got, want)
		}
	}
	for _, u := range []uint16{0, 1, 42, math.MaxUint8 + 1, math.MaxUint16} {
		if got, want := fmt.Sprintf("%d", u), strconv.FormatUint(uint64(u), 10); got != want {
			t.Errorf("fmt.Sprintf(%%d, uint16(%d))=%q != strconv.FormatUint=%q", u, got, want)
		}
	}
	for _, u := range []uint32{0, 1, 42, math.MaxUint16 + 1, math.MaxUint32} {
		if got, want := fmt.Sprintf("%d", u), strconv.FormatUint(uint64(u), 10); got != want {
			t.Errorf("fmt.Sprintf(%%d, uint32(%d))=%q != strconv.FormatUint=%q", u, got, want)
		}
	}
	for _, u := range []uint64{0, 1, 42, math.MaxUint32 + 1, math.MaxUint64} {
		if got, want := fmt.Sprintf("%d", u), strconv.FormatUint(u, 10); got != want {
			t.Errorf("fmt.Sprintf(%%d, uint64(%d))=%q != strconv.FormatUint=%q", u, got, want)
		}
	}

	// %d / non-int signed widths -> strconv.FormatInt(int64(x), 10). The
	// widening int64(x) is value-preserving for every signed width, and
	// FormatInt handles MinInt64 without a negation overflow.
	for _, x := range []int8{0, 1, -1, 42, math.MinInt8, math.MaxInt8} {
		if got, want := fmt.Sprintf("%d", x), strconv.FormatInt(int64(x), 10); got != want {
			t.Errorf("fmt.Sprintf(%%d, int8(%d))=%q != strconv.FormatInt=%q", x, got, want)
		}
	}
	for _, x := range []int16{0, 1, -1, math.MinInt8 - 1, math.MinInt16, math.MaxInt16} {
		if got, want := fmt.Sprintf("%d", x), strconv.FormatInt(int64(x), 10); got != want {
			t.Errorf("fmt.Sprintf(%%d, int16(%d))=%q != strconv.FormatInt=%q", x, got, want)
		}
	}
	for _, x := range []int32{0, 1, -1, math.MinInt16 - 1, math.MinInt32, math.MaxInt32} {
		if got, want := fmt.Sprintf("%d", x), strconv.FormatInt(int64(x), 10); got != want {
			t.Errorf("fmt.Sprintf(%%d, int32(%d))=%q != strconv.FormatInt=%q", x, got, want)
		}
	}
	for _, x := range []int64{0, 1, -1, math.MinInt32 - 1, math.MinInt64, math.MaxInt64} {
		if got, want := fmt.Sprintf("%d", x), strconv.FormatInt(x, 10); got != want {
			t.Errorf("fmt.Sprintf(%%d, int64(%d))=%q != strconv.FormatInt=%q", x, got, want)
		}
	}

	// %t / bool -> strconv.FormatBool.
	for _, b := range []bool{true, false} {
		if got, want := fmt.Sprintf("%t", b), strconv.FormatBool(b); got != want {
			t.Errorf("fmt.Sprintf(%%t, %v)=%q != strconv.FormatBool=%q", b, got, want)
		}
	}

	// %x / []byte -> hex.EncodeToString: lowercase, two digits per byte, no
	// separator. all256 covers every byte value, so any per-byte digit
	// divergence (case, padding) would surface.
	all256 := make([]byte, 256)
	for b := 0; b < 256; b++ {
		all256[b] = byte(b)
	}
	byteCases := [][]byte{
		nil,
		{},
		{0x00},
		{0x0a},
		{0xff},
		{0x00, 0x0f, 0xff, 0xa5},
		all256,
	}
	for _, bs := range byteCases {
		if got, want := fmt.Sprintf("%x", bs), hex.EncodeToString(bs); got != want {
			t.Errorf("fmt.Sprintf(%%x, %v)=%q != hex.EncodeToString=%q", bs, got, want)
		}
	}
	// nil and empty must both render as "" on BOTH sides.
	if fmt.Sprintf("%x", []byte(nil)) != "" || hex.EncodeToString(nil) != "" {
		t.Error("nil []byte must render as \"\" under both the x verb and hex.EncodeToString")
	}
	if fmt.Sprintf("%x", []byte{}) != "" || hex.EncodeToString([]byte{}) != "" {
		t.Error("empty []byte must render as \"\" under both the x verb and hex.EncodeToString")
	}
}

// PS3101: `for ... { bytes.Contains(line, []byte(sep)) }` -> hoist
// `bsep := []byte(sep)` above the loop. The hoist shares ONE []byte buffer
// across every iteration instead of a fresh copy per iteration, so it is
// bit-identical only if every function the check whitelists as "read-only"
// (bytesReadOnlyFuncs in ps3101.go) neither mutates its []byte argument nor
// aliases it into its result. Here every whitelisted func is run against a
// shared hoisted buffer over many iterations: results must equal the
// fresh-conversion results AND the shared buffer's bytes must be unchanged
// after every call. A future widening of the whitelist (e.g. bytes.Split,
// which ALIASES its argument into the result, or bytes.Replace) must extend
// this table or fail review.
func TestEquiv_PS3101HoistSharedReadOnly(t *testing.T) {
	seps := []string{"", "a", ",", "ab", "日本", "\x00", "aba"}
	lines := [][]byte{nil, {}, []byte("a"), []byte("abab"), []byte("日本語ab"), []byte("\x00a\x00"), []byte("zzz")}
	pred := func(r rune) bool { return r == 'a' || r == '日' }
	for _, sep := range seps {
		hoisted := []byte(sep) // the shared buffer PS3101's fix introduces
		snapshot := string(hoisted)
		for iter := 0; iter < 3; iter++ { // sharing across iterations is the point
			for _, line := range lines {
				fresh := func() []byte { return []byte(sep) }
				type res struct {
					name string
					a, b any
				}
				checks := []res{
					{"Compare", bytes.Compare(line, hoisted), bytes.Compare(line, fresh())},
					{"Contains", bytes.Contains(line, hoisted), bytes.Contains(line, fresh())},
					{"ContainsAny", bytes.ContainsAny(hoisted, "ab日"), bytes.ContainsAny(fresh(), "ab日")},
					{"ContainsFunc", bytes.ContainsFunc(hoisted, pred), bytes.ContainsFunc(fresh(), pred)},
					{"ContainsRune", bytes.ContainsRune(hoisted, 'a'), bytes.ContainsRune(fresh(), 'a')},
					{"Count", bytes.Count(line, hoisted), bytes.Count(line, fresh())},
					{"Equal", bytes.Equal(line, hoisted), bytes.Equal(line, fresh())},
					{"EqualFold", bytes.EqualFold(line, hoisted), bytes.EqualFold(line, fresh())},
					{"HasPrefix", bytes.HasPrefix(line, hoisted), bytes.HasPrefix(line, fresh())},
					{"HasSuffix", bytes.HasSuffix(line, hoisted), bytes.HasSuffix(line, fresh())},
					{"Index", bytes.Index(line, hoisted), bytes.Index(line, fresh())},
					{"IndexAny", bytes.IndexAny(hoisted, "ab日"), bytes.IndexAny(fresh(), "ab日")},
					{"IndexByte", bytes.IndexByte(hoisted, 'a'), bytes.IndexByte(fresh(), 'a')},
					{"IndexFunc", bytes.IndexFunc(hoisted, pred), bytes.IndexFunc(fresh(), pred)},
					{"IndexRune", bytes.IndexRune(hoisted, 'a'), bytes.IndexRune(fresh(), 'a')},
					{"LastIndex", bytes.LastIndex(line, hoisted), bytes.LastIndex(line, fresh())},
					{"LastIndexAny", bytes.LastIndexAny(hoisted, "ab日"), bytes.LastIndexAny(fresh(), "ab日")},
					{"LastIndexByte", bytes.LastIndexByte(hoisted, 'a'), bytes.LastIndexByte(fresh(), 'a')},
					{"LastIndexFunc", bytes.LastIndexFunc(hoisted, pred), bytes.LastIndexFunc(fresh(), pred)},
				}
				for _, c := range checks {
					if c.a != c.b {
						t.Errorf("bytes.%s(sep=%q, line=%q): shared hoisted buffer=%v != fresh conversion=%v", c.name, sep, line, c.a, c.b)
					}
				}
				if string(hoisted) != snapshot {
					t.Fatalf("shared hoisted buffer MUTATED by a whitelisted read-only func: %q -> %q (sep=%q, line=%q)", snapshot, hoisted, sep, line)
				}
			}
		}
	}
}

func equalByteSlices(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// TestEquiv_Sincos pins PS5008's bit-identity claim: math.Sincos(x) returns
// exactly the same (sin, cos) bits as separate math.Sin(x)/math.Cos(x). The
// fusion is only bit-identical because Go's math.Sincos shares Sin/Cos's exact
// argument reduction and polynomials — an implementation detail that a future
// Go release could change, which would silently make the PS5008 rewrite
// behavior-changing. This asserts bitwise identity over edge cases (NaN, ±Inf,
// ±0, subnormal, huge magnitudes that stress argument reduction) plus dense and
// large-magnitude deterministic sweeps; any single ULP divergence fails CI.
func TestEquiv_Sincos(t *testing.T) {
	check := func(x float64) {
		s1, c1 := math.Sin(x), math.Cos(x)
		s2, c2 := math.Sincos(x)
		if math.Float64bits(s1) != math.Float64bits(s2) {
			t.Errorf("sin divergence at x=%v: Sin=%#x Sincos.s=%#x", x, math.Float64bits(s1), math.Float64bits(s2))
		}
		if math.Float64bits(c1) != math.Float64bits(c2) {
			t.Errorf("cos divergence at x=%v: Cos=%#x Sincos.c=%#x", x, math.Float64bits(c1), math.Float64bits(c2))
		}
	}
	for _, x := range []float64{
		0, math.Copysign(0, -1), 1, -1, math.Pi, math.Pi / 2, math.Pi / 4, 2 * math.Pi,
		math.Inf(1), math.Inf(-1), math.NaN(), math.SmallestNonzeroFloat64, math.MaxFloat64,
		1e-300, 1e300, 1 << 30, 1e18, -1e18,
	} {
		check(x)
	}
	// Dense sweep near the origin and a large-magnitude sweep (argument-reduction
	// stress). Deterministic step, no rand — reproducible failures.
	for i := -50000; i <= 50000; i++ {
		check(float64(i) * 0.000313)
	}
	for i := 0; i < 50000; i++ {
		check(float64(i) * 1234.5678)
	}
}

// psClamp3077 is a verbatim copy of the helper PS3077's fix emits
// (ps3077HelperText in ps3077.go); the tests below pin the emitted code's
// semantics, so any edit to the helper must be mirrored here.
func psClamp3077(v, lo, hi float64) float64 {
	r := v
	if r <= lo {
		r = lo
	}
	if r >= hi {
		r = hi
	}
	return r
}

// equivSpecialFloats is the float64 special-value set the PS3077/PS3082
// bit-identity pins iterate: both zeros, both infinities, NaN, subnormals,
// the extremes, and a couple of ordinary values.
var equivSpecialFloats = []float64{
	0, math.Copysign(0, -1), 1, -1, 0.5, -0.5, 2.5, -2.5, 100.5, -100.5, 255, 256,
	math.Inf(1), math.Inf(-1), math.NaN(),
	math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64, 1e-310, -1e-310,
	math.MaxFloat64, -math.MaxFloat64,
}

// TestEquiv_PS3077ClampConstBounds pins the bit-identity that lets PS3077
// rewrite math.Min(math.Max(v, lo), hi) — and the math.Max(math.Min(v, hi),
// lo) order — to the psClamp branch form when BOTH bounds are compile-time
// constants. A Go constant can never be negative zero (untyped -0.0 is
// exactly 0, converting to +0.0) or NaN (no constant expression produces
// one), and for every other bound pair psClamp is bit-for-bit identical to
// the math pair for EVERY input v: both zeros, both infinities, NaN,
// subnormals, the extremes, each exact boundary and its neighbors, and a
// dense sweep. Any single bit of divergence fails CI.
func TestEquiv_PS3077ClampConstBounds(t *testing.T) {
	// Safe constant bound pairs (what the gated fix can ever see).
	boundPairs := [][2]float64{{0, 1}, {-1, 1}, {0, 255}, {-100.5, 100.5}, {0.25, 0.75}}
	for _, bp := range boundPairs {
		lo, hi := bp[0], bp[1]
		check := func(v float64) {
			want := math.Min(math.Max(v, lo), hi)
			got := psClamp3077(v, lo, hi)
			if math.Float64bits(got) != math.Float64bits(want) {
				t.Errorf("psClamp(%v, %v, %v)=%#x != math.Min(math.Max(...))=%#x",
					v, lo, hi, math.Float64bits(got), math.Float64bits(want))
			}
			// The reverse spelling PS3077 also rewrites must agree too.
			if wantRev := math.Max(math.Min(v, hi), lo); math.Float64bits(got) != math.Float64bits(wantRev) {
				t.Errorf("psClamp(%v, %v, %v)=%#x != math.Max(math.Min(...))=%#x",
					v, lo, hi, math.Float64bits(got), math.Float64bits(wantRev))
			}
		}
		for _, v := range equivSpecialFloats {
			check(v)
		}
		// Each exact boundary and its ULP neighbors.
		for _, b := range []float64{lo, hi} {
			check(b)
			check(math.Nextafter(b, math.Inf(-1)))
			check(math.Nextafter(b, math.Inf(1)))
		}
		// Dense deterministic sweep across and beyond every bound pair.
		for i := -30000; i <= 30000; i++ {
			check(float64(i) * 0.0173)
		}
	}
}

// TestEquiv_PS3077ClampVariableBoundDivergence pins WHY the constant-bound
// gate exists: with a -0 or NaN BOUND — values only a runtime variable can
// carry, never a Go constant — psClamp is NOT the math pair. If any arm
// below starts agreeing, the gate could be revisited; until then weakening
// it to admit variable bounds ships a behavior-changing fix.
func TestEquiv_PS3077ClampVariableBoundDivergence(t *testing.T) {
	negZero := math.Copysign(0, -1)
	// lo = -0, v = +0: math.Max(+0, -0) is +0 so the pair returns +0, but
	// psClamp's '+0 <= -0' comparison is true and returns -0.
	if math.Float64bits(psClamp3077(0, negZero, 1)) == math.Float64bits(math.Min(math.Max(0, negZero), 1)) {
		t.Error("expected psClamp(+0, -0, 1) to DIFFER from math.Min(math.Max(+0, -0), 1); the constant-bound gate looks unnecessary — it is not")
	}
	// hi = NaN: math.Min(x, NaN) is NaN, but psClamp's 'x >= NaN' is false
	// and v passes through unclamped.
	if math.Float64bits(psClamp3077(0.5, 0, math.NaN())) == math.Float64bits(math.Min(math.Max(0.5, 0), math.NaN())) {
		t.Error("expected psClamp(0.5, 0, NaN) to DIFFER from math.Min(math.Max(0.5, 0), NaN)")
	}
	// lo = NaN: same failed-comparison fallthrough on the lower bound.
	if math.Float64bits(psClamp3077(0.5, math.NaN(), 1)) == math.Float64bits(math.Min(math.Max(0.5, math.NaN()), 1)) {
		t.Error("expected psClamp(0.5, NaN, 1) to DIFFER from math.Min(math.Max(0.5, NaN), 1)")
	}
}

// psFmax3082 and psFmin3082 are verbatim copies of the wrappers PS3082's fix
// emits (ps3082FmaxText/ps3082FminText in ps3082.go); the pin below tracks
// the real emitted code, so any edit to the helpers must be mirrored here.
func psFmax3082(a, b float64) float64 {
	if r := max(a, b); r == r {
		return r
	}
	return math.Max(a, b)
}

func psFmin3082(a, b float64) float64 {
	if r := min(a, b); r == r {
		return r
	}
	return math.Min(a, b)
}

// TestEquiv_PS3082MinMaxWrapper pins PS3082's bit-identity claim: the
// builtin-with-NaN-fallback wrappers return exactly math.Max/math.Min's bits
// for EVERY pair. The builtins and the math functions disagree only on
// NaN-vs-Inf pairs (math.Max documents +Inf as beating NaN, math.Min -Inf),
// which is exactly when the builtin returns NaN and the wrapper delegates —
// so the full ±0/±Inf/NaN cross product plus a numeric sweep must agree
// bit-for-bit, including the -0/+0 ordering both sides define.
func TestEquiv_PS3082MinMaxWrapper(t *testing.T) {
	check := func(a, b float64) {
		if got, want := psFmax3082(a, b), math.Max(a, b); math.Float64bits(got) != math.Float64bits(want) {
			t.Errorf("psFmax(%v, %v)=%#x != math.Max=%#x", a, b, math.Float64bits(got), math.Float64bits(want))
		}
		if got, want := psFmin3082(a, b), math.Min(a, b); math.Float64bits(got) != math.Float64bits(want) {
			t.Errorf("psFmin(%v, %v)=%#x != math.Min=%#x", a, b, math.Float64bits(got), math.Float64bits(want))
		}
	}
	for _, a := range equivSpecialFloats {
		for _, b := range equivSpecialFloats {
			check(a, b)
		}
	}
	// Deterministic numeric sweep: dense pairs, including equal values and
	// opposite signs, plus each special value against the sweep.
	for i := -2000; i <= 2000; i++ {
		x := float64(i) * 0.317
		check(x, x)
		check(x, -x)
		check(x, x+1)
		for _, s := range equivSpecialFloats {
			check(x, s)
			check(s, x)
		}
	}
}

// TestEquiv_PS3005IndirectKeySort pins PS3005's float/NaN safety. PS3005 is the
// only sort-rewrite check that deliberately ALLOWS float keys (PS3002/3104/3105
// exclude or name-limit them) — its bit-identity rests entirely on the rewrite
// keeping the SAME '<' predicate, only changing where the value is loaded from
// (m[idx[a]][f] -> key[idx[a]] where key[i]=m[i][f]). Because both forms feed
// sort.Slice the identical comparator RESULTS over the identical values, the
// deterministic pdqsort yields the identical permutation — even for a NaN key,
// where '<' is an inconsistent comparator (all-false), because BOTH sorts run
// that same inconsistent comparator on the same input. This asserts the two
// permutations match bit-for-bit over many random 2-D matrices whose sort
// column carries NaN, +/-0, +/-Inf and ordinary values.
func TestEquiv_PS3005IndirectKeySort(t *testing.T) {
	rng := rand.New(rand.NewSource(0x3005))
	pool := []float64{math.NaN(), math.Inf(1), math.Inf(-1), 0, math.Copysign(0, -1),
		1, -1, 2.5, -2.5, math.MaxFloat64, -math.MaxFloat64, math.SmallestNonzeroFloat64}
	const f = 1 // sort column
	for trial := 0; trial < 4000; trial++ {
		n := rng.Intn(12) + 1
		m := make([][]float64, n)
		for i := range m {
			m[i] = []float64{rng.Float64(), pool[rng.Intn(len(pool))], rng.Float64()}
		}

		// Original: comparator dereferences the 2-D structure.
		idxA := make([]int, n)
		for i := range idxA {
			idxA[i] = i
		}
		sort.Slice(idxA, func(a, b int) bool { return m[idxA[a]][f] < m[idxA[b]][f] })

		// Rewritten: flat key column, SAME '<' predicate.
		key := make([]float64, len(m))
		for i := range m {
			key[i] = m[i][f]
		}
		idxB := make([]int, n)
		for i := range idxB {
			idxB[i] = i
		}
		sort.Slice(idxB, func(a, b int) bool { return key[idxB[a]] < key[idxB[b]] })

		if !slices.Equal(idxA, idxB) {
			t.Fatalf("trial %d: permutation diverged\n orig=%v\n rewritten=%v\n column=%v",
				trial, idxA, idxB, func() []float64 {
					c := make([]float64, n)
					for i := range m {
						c[i] = m[i][f]
					}
					return c
				}())
		}
	}
}
