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
	htmltemplate "html/template"
	"math"
	"math/rand"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"
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

// PS2107 (%g case): fmt.Sprintf("%g", f) -> strconv.FormatFloat(f, 'g', -1, 64)
// for float64, and -> strconv.FormatFloat(float64(f), 'g', -1, 32) for
// float32. %g prints the SHORTEST representation that round-trips at the
// operand's width — exactly FormatFloat's precision -1 with the matching
// bitSize — including the special values (-0 keeps its sign, NaN, ±Inf).
// %e/%f are NOT equivalent (they default to 6 digits) and are not rewritten.
func TestEquiv_PS2107SprintfGFloatToFormatFloat(t *testing.T) {
	f64s := []float64{
		0, math.Copysign(0, -1), 1, -1,
		math.NaN(), math.Inf(1), math.Inf(-1),
		math.MaxFloat64, math.SmallestNonzeroFloat64,
		3.14159, 0.1, 2.0 / 3.0, 1e20, 1e-20,
	}
	// A few thousand computed values: sign/magnitude sweeps plus a
	// deterministic pseudo-random walk over interesting exponents.
	for i := 1; i <= 2000; i++ {
		v := float64(i) * 1.000123
		f64s = append(f64s, v, -v, 1/v, v*1e17, v*1e-17, math.Sqrt(v), math.Exp(-v/100))
	}
	for _, f := range f64s {
		if got, want := fmt.Sprintf("%g", f), strconv.FormatFloat(f, 'g', -1, 64); got != want {
			t.Errorf("fmt.Sprintf(%%g, %v)=%q != strconv.FormatFloat(f, 'g', -1, 64)=%q", f, got, want)
		}
	}

	f32s := []float32{
		0, float32(math.Copysign(0, -1)), 1, -1,
		float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1)),
		math.MaxFloat32, math.SmallestNonzeroFloat32,
		3.14159, 0.1, 2.0 / 3.0, 1e20, 1e-20,
	}
	for i := 1; i <= 2000; i++ {
		v := float32(i) * 1.000123
		f32s = append(f32s, v, -v, 1/v, v*1e17, v*1e-17)
	}
	for _, f := range f32s {
		if got, want := fmt.Sprintf("%g", f), strconv.FormatFloat(float64(f), 'g', -1, 32); got != want {
			t.Errorf("fmt.Sprintf(%%g, float32(%v))=%q != strconv.FormatFloat(float64(f), 'g', -1, 32)=%q", f, got, want)
		}
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

// TestEquiv_PS3007Membership pins both halves of PS3007's strict-comparability
// gate. For STRICTLY comparable key types (int, struct-of-ints, string — types
// whose == can never panic) the map-membership set and slices.Contains answer
// identically for every probe, so the rewrite is safe; that equivalence is
// asserted over random slices with duplicates and random hit/miss probes. For
// an interface key the two forms diverge on panic TIMING: the build loop
// inserts (and so compares/hashes) EVERY element eagerly and panics at build
// time on the first uncomparable dynamic value, while slices.Contains compares
// lazily and panics only if the scan actually reaches that element — never,
// when no probe runs or a match comes first. That divergence is pinned with
// recover() below; if it ever stops holding, the guard could be revisited,
// but weakening the guard while it holds ships a behavior-changing fix.
func TestEquiv_PS3007Membership(t *testing.T) {
	rng := rand.New(rand.NewSource(0x3007))

	// Part 1: strictly-comparable keys — map membership == slices.Contains.
	for trial := 0; trial < 3000; trial++ {
		n := rng.Intn(12) // includes empty slices
		ints := make([]int, n)
		for i := range ints {
			ints[i] = rng.Intn(6) // small domain: duplicates likely
		}
		intSet := make(map[int]bool, len(ints))
		for _, b := range ints {
			intSet[b] = true
		}
		for probe := 0; probe < 8; probe++ {
			tok := rng.Intn(8) // hits and misses
			if got, want := slices.Contains(ints, tok), intSet[tok]; got != want {
				t.Fatalf("trial %d: int key: slices.Contains(%v, %d)=%v != map membership %v", trial, ints, tok, got, want)
			}
		}

		type pair struct{ a, b int }
		pairs := make([]pair, n)
		for i := range pairs {
			pairs[i] = pair{rng.Intn(3), rng.Intn(3)}
		}
		pairSet := make(map[pair]bool, len(pairs))
		for _, b := range pairs {
			pairSet[b] = true
		}
		for probe := 0; probe < 8; probe++ {
			tok := pair{rng.Intn(4), rng.Intn(4)}
			if got, want := slices.Contains(pairs, tok), pairSet[tok]; got != want {
				t.Fatalf("trial %d: struct key: slices.Contains(%v, %v)=%v != map membership %v", trial, pairs, tok, got, want)
			}
		}

		words := make([]string, n)
		for i := range words {
			words[i] = strconv.Itoa(rng.Intn(6))
		}
		wordSet := make(map[string]bool, len(words))
		for _, b := range words {
			wordSet[b] = true
		}
		for probe := 0; probe < 8; probe++ {
			tok := strconv.Itoa(rng.Intn(8))
			if got, want := slices.Contains(words, tok), wordSet[tok]; got != want {
				t.Fatalf("trial %d: string key: slices.Contains(%v, %q)=%v != map membership %v", trial, words, tok, got, want)
			}
		}
	}

	// Part 2: the interface-key divergence that justifies the guard.
	panics := func(f func()) (p bool) {
		defer func() {
			if recover() != nil {
				p = true
			}
		}()
		f()
		return
	}
	buildSet := func(seq []any) {
		set := make(map[any]bool, len(seq))
		for _, b := range seq {
			set[b] = true // eager insert: panics here on an uncomparable element
		}
		_ = set
	}
	scanAll := func(seq, window []any) {
		for _, tok := range window {
			_ = slices.Contains(seq, tok) // lazy compare: panics only on reach
		}
	}

	// Empty probe window: the original panics at build, the rewrite runs zero
	// comparisons and cannot panic.
	seq := []any{1, []int{2}, 3}
	origPanics := panics(func() { buildSet(seq) })
	rewrittenPanics := panics(func() { scanAll(seq, nil) })
	if !origPanics {
		t.Fatal("expected map build over []any{1, []int{2}, 3} to panic on the uncomparable element")
	}
	if rewrittenPanics {
		t.Fatal("expected slices.Contains scans with an empty probe window to NOT panic")
	}
	if origPanics == rewrittenPanics {
		t.Fatal("expected orig-panics != rewritten-panics; the strict-comparability guard looks unnecessary — it is not")
	}

	// Match before the uncomparable element: the rewrite finds 7 and returns
	// without ever comparing against []int{9}; the original still panics.
	seq2 := []any{7, []int{9}}
	if !panics(func() { buildSet(seq2) }) {
		t.Fatal("expected map build over []any{7, []int{9}} to panic")
	}
	if panics(func() { scanAll(seq2, []any{7}) }) {
		t.Fatal("expected slices.Contains(seq, 7) to match before the uncomparable element and NOT panic")
	}
}

// TestEquiv_PS5002SymmetricMirror pins both halves of PS5002's fresh-zero
// gate. The triangle+mirror rewrite replaces the full outer-product
// accumulation m[i][j] += x[i]*x[j]; the mirrored upper cell receives
// init[j][i] + x[j]*x[i] where the original computed init[i][j] + x[i]*x[j],
// so the two are bit-identical exactly when init[i][j] == init[j][i] — i.e.
// when m is SYMMETRIC at loop entry. Part 1 proves the safe case the fix now
// requires: for a FRESH-ZERO matrix (trivially symmetric) the results are
// bit-for-bit identical over random x of varied lengths including ±0, ±Inf
// and NaN. Part 2 is the regression alarm that documents why the gate
// exists: with a non-symmetric initial matrix the two forms diverge — the
// exact bug that used to ship when the fix fired on a parameter matrix.
func TestEquiv_PS5002SymmetricMirror(t *testing.T) {
	clone := func(init [][]float64) [][]float64 {
		m := make([][]float64, len(init))
		for r := range init {
			m[r] = append([]float64(nil), init[r]...)
		}
		return m
	}
	full := func(init [][]float64, x []float64) [][]float64 {
		m := clone(init)
		for i := range x {
			for j := range x {
				m[i][j] += x[i] * x[j]
			}
		}
		return m
	}
	mirrored := func(init [][]float64, x []float64) [][]float64 {
		m := clone(init)
		for i := range x {
			for j := 0; j <= i; j++ {
				m[i][j] += x[i] * x[j]
			}
		}
		for i := range x { // mirror once
			for j := i + 1; j < len(x); j++ {
				m[i][j] = m[j][i]
			}
		}
		return m
	}
	bitsEqual := func(a, b [][]float64) bool {
		for i := range a {
			for j := range a[i] {
				if math.Float64bits(a[i][j]) != math.Float64bits(b[i][j]) {
					return false
				}
			}
		}
		return true
	}

	// Part 1: fresh-zero matrix (what the gated fix can ever see) — the full
	// nest and the triangle+mirror agree bit-for-bit for every x, including
	// the float specials.
	rng := rand.New(rand.NewSource(0x5002))
	specials := []float64{
		0, math.Copysign(0, -1), math.Inf(1), math.Inf(-1), math.NaN(),
		math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64,
		math.MaxFloat64, -math.MaxFloat64, 1, -1, 1e-310,
	}
	for trial := 0; trial < 3000; trial++ {
		n := rng.Intn(10) // includes empty and 1-element vectors
		x := make([]float64, n)
		for i := range x {
			if rng.Intn(3) == 0 {
				x[i] = specials[rng.Intn(len(specials))]
			} else {
				x[i] = rng.NormFloat64() * math.Pow(10, float64(rng.Intn(80)-40))
			}
		}
		zero := make([][]float64, n)
		for r := range zero {
			zero[r] = make([]float64, n)
		}
		a, b := full(zero, x), mirrored(zero, x)
		if !bitsEqual(a, b) {
			t.Fatalf("trial %d: fresh-zero divergence for x=%v:\nfull=%v\nmirrored=%v", trial, x, a, b)
		}
	}

	// Part 2: the divergence that mandates the gate — a NON-SYMMETRIC
	// initial matrix. init[0][1]=10 while init[1][0]=20, so the original
	// leaves 10+1*1=11 in m[0][1] but the mirror copies m[1][0]=20+1*1=21.
	init := [][]float64{{0, 10}, {20, 0}}
	x := []float64{1, 1}
	a, b := full(init, x), mirrored(init, x)
	if bitsEqual(a, b) {
		t.Fatalf("expected divergence for non-symmetric init, got identical %v — if this ever holds, the fresh-zero gate could be revisited, but removing it while it diverges ships a behavior-changing fix", a)
	}
	if a[0][1] != 11 || b[0][1] != 21 {
		t.Fatalf("divergence shape changed: full m[0][1]=%v (want 11), mirrored m[0][1]=%v (want 21)", a[0][1], b[0][1])
	}
}

// TestEquiv_PS4008Matmul pins PS4008's bit-identity claim: the ikj/axpy rewrite
// (zero c[i][j], then for k { for j { c[i][j] += a[i][k]*b[k][j] } }) sums each
// output cell over k in ASCENDING order — exactly the order the serial ijk dot
// accumulator uses — so the two are bitwise identical (Go float64 arithmetic is
// pure IEEE-754 double, no extended-precision spills that would depend on
// evaluation shape). Floating-point addition is non-associative, so a rewrite
// that reordered the accumulation (e.g. cache blocking, or swapping to sum over
// j) would NOT be bit-identical; this asserts the shipped ikj order is. Random
// matrices whose entries mix large/small magnitudes and specials (±0, extremes)
// stress rounding; any single-ULP divergence fails CI.
func TestEquiv_PS4008Matmul(t *testing.T) {
	ijk := func(a, b [][]float64) [][]float64 {
		c := make([][]float64, len(a))
		for i := range a {
			c[i] = make([]float64, len(b[0]))
			for j := range b[0] {
				sum := 0.0
				for k := range b {
					sum += a[i][k] * b[k][j]
				}
				c[i][j] = sum
			}
		}
		return c
	}
	ikj := func(a, b [][]float64) [][]float64 {
		c := make([][]float64, len(a))
		for i := range a {
			c[i] = make([]float64, len(b[0]))
			for j := range b[0] {
				c[i][j] = 0
			}
			for k := range b {
				for j := range b[0] {
					c[i][j] += a[i][k] * b[k][j]
				}
			}
		}
		return c
	}
	rng := rand.New(rand.NewSource(0x4008))
	vals := []float64{1, -1, 1e300, 1e-300, math.Pi, -math.Pi, 0, math.Copysign(0, -1),
		1e16, 1e-16, math.SmallestNonzeroFloat64}
	pick := func() float64 {
		if rng.Intn(3) == 0 {
			return vals[rng.Intn(len(vals))]
		}
		return (rng.Float64() - 0.5) * rng.Float64() * 1e8
	}
	for trial := 0; trial < 1500; trial++ {
		I, K, J := rng.Intn(6)+1, rng.Intn(6)+1, rng.Intn(6)+1
		a := make([][]float64, I)
		for i := range a {
			a[i] = make([]float64, K)
			for k := range a[i] {
				a[i][k] = pick()
			}
		}
		b := make([][]float64, K)
		for k := range b {
			b[k] = make([]float64, J)
			for j := range b[k] {
				b[k][j] = pick()
			}
		}
		c1, c2 := ijk(a, b), ikj(a, b)
		for i := range c1 {
			for j := range c1[i] {
				if math.Float64bits(c1[i][j]) != math.Float64bits(c2[i][j]) {
					t.Fatalf("trial %d cell [%d][%d]: ijk=%x ikj=%x", trial, i, j,
						math.Float64bits(c1[i][j]), math.Float64bits(c2[i][j]))
				}
			}
		}
	}
}

// TestEquiv_PS2008Slab pins PS2008's "bit-identical by construction" claim: the
// slab rewrite `slab := make([]T, len(rows)*d); rows[i] = slab[i*d:(i+1)*d:(i+1)*d]`
// must be observationally identical to per-row `rows[i] = make([]T, d)`. The two
// properties that could break — cross-row aliasing (rows must be DISTINCT) and
// append safety (the 3-index cap must force a realloc, not corrupt the neighbor)
// — are exercised directly and compared between the two forms.
func TestEquiv_PS2008Slab(t *testing.T) {
	perRow := func(n, d int) [][]float64 {
		rows := make([][]float64, n)
		for i := range rows {
			rows[i] = make([]float64, d)
		}
		return rows
	}
	slab := func(n, d int) [][]float64 {
		rows := make([][]float64, n)
		s := make([]float64, len(rows)*d)
		for i := range rows {
			rows[i] = s[i*d : (i+1)*d : (i+1)*d]
		}
		return rows
	}
	for _, nd := range [][2]int{{1, 1}, {3, 4}, {5, 1}, {1, 7}, {8, 8}, {2, 3}} {
		n, d := nd[0], nd[1]
		a, b := perRow(n, d), slab(n, d)
		// len/cap of every row must match (cap==d in both -> append reallocates).
		for i := 0; i < n; i++ {
			if len(a[i]) != len(b[i]) || cap(a[i]) != cap(b[i]) || cap(b[i]) != d {
				t.Fatalf("n=%d d=%d row %d: perRow len/cap=%d/%d slab=%d/%d (want cap %d)",
					n, d, i, len(a[i]), cap(a[i]), len(b[i]), cap(b[i]), d)
			}
		}
		// Fill both identically, then assert element-wise equality.
		fill := func(rows [][]float64) {
			for i := range rows {
				for j := range rows[i] {
					rows[i][j] = float64(i*100 + j)
				}
			}
		}
		fill(a)
		fill(b)
		// DISTINCTNESS: mutating row 0 must not touch row 1 in EITHER form.
		if n >= 2 {
			a[0][0] = -1
			b[0][0] = -1
			if a[1][0] != b[1][0] || b[1][0] != 100 {
				t.Fatalf("n=%d d=%d: cross-row aliasing (a[1][0]=%v b[1][0]=%v want 100)", n, d, a[1][0], b[1][0])
			}
		}
		// APPEND SAFETY: append beyond cap must realloc (neighbor untouched) in both.
		if n >= 2 {
			a[0] = append(a[0], 999)
			b[0] = append(b[0], 999)
			if a[1][0] != b[1][0] {
				t.Fatalf("n=%d d=%d: append corrupted neighbor differently (a[1][0]=%v b[1][0]=%v)", n, d, a[1][0], b[1][0])
			}
		}
	}
}

// TestEquiv_PS1006ColReduce pins PS1006's bit-identity claim AND its panic
// parity. The interchange (scratch sums walked row-major) accumulates each
// output column over r in ASCENDING order — exactly the order the original
// strided reduction uses — so the values are bitwise identical. The
// write-back must be an INDEXED loop (out[c] = sums[c]), not
// copy(out[:cols], sums): the original indexes out by its LENGTH and panics
// at c == len(out) when len(out) < cols, whereas out[:cols] reslices up to
// cap(out) — for len(out) < cols <= cap(out) the copy form silently writes
// into spare capacity and ELIMINATES the panic. This test pins both: bitwise
// value identity on valid inputs, and orig-panics == indexed-panics !=
// copy-panics on a short-but-roomy out.
func TestEquiv_PS1006ColReduce(t *testing.T) {
	orig := func(a []float64, rows, cols int, out []float64) {
		for c := 0; c < cols; c++ {
			s := 0.0
			for r := 0; r < rows; r++ {
				s += a[r*cols+c]
			}
			out[c] = s
		}
	}
	indexed := func(a []float64, rows, cols int, out []float64) {
		psSums := make([]float64, cols)
		for r := 0; r < rows; r++ {
			psBase := r * cols
			for c := 0; c < cols; c++ {
				psSums[c] += a[psBase+c]
			}
		}
		for c := 0; c < cols; c++ {
			out[c] = psSums[c]
		}
	}
	copyForm := func(a []float64, rows, cols int, out []float64) {
		psSums := make([]float64, cols)
		for r := 0; r < rows; r++ {
			psBase := r * cols
			for c := 0; c < cols; c++ {
				psSums[c] += a[psBase+c]
			}
		}
		copy(out[:cols], psSums)
	}

	// 1. CORRECTNESS: bitwise-identical out for random rounding-stressing
	// matrices across varied dims.
	rng := rand.New(rand.NewSource(0x1006))
	vals := []float64{1, -1, 1e300, 1e-300, math.Pi, -math.Pi, 0, math.Copysign(0, -1),
		1e16, 1e-16, math.SmallestNonzeroFloat64}
	pick := func() float64 {
		if rng.Intn(3) == 0 {
			return vals[rng.Intn(len(vals))]
		}
		return (rng.Float64() - 0.5) * rng.Float64() * 1e8
	}
	for trial := 0; trial < 1500; trial++ {
		rows, cols := rng.Intn(7)+1, rng.Intn(7)+1
		a := make([]float64, rows*cols)
		for i := range a {
			a[i] = pick()
		}
		o1, o2 := make([]float64, cols), make([]float64, cols)
		orig(a, rows, cols, o1)
		indexed(a, rows, cols, o2)
		for c := 0; c < cols; c++ {
			if math.Float64bits(o1[c]) != math.Float64bits(o2[c]) {
				t.Fatalf("trial %d rows=%d cols=%d col %d: orig=%x indexed=%x",
					trial, rows, cols, c, math.Float64bits(o1[c]), math.Float64bits(o2[c]))
			}
		}
	}

	// 2. PANIC PARITY (the regression alarm). panics runs f and reports
	// whether it panicked.
	panics := func(f func()) (p bool) {
		defer func() {
			if recover() != nil {
				p = true
			}
		}()
		f()
		return false
	}
	const rows, cols = 3, 4
	a := make([]float64, rows*cols)
	for i := range a {
		a[i] = float64(i + 1)
	}
	// len(out) < cols <= cap(out): the case copy() gets wrong.
	mk := func() []float64 { return make([]float64, 2, 8) }
	origPanics := panics(func() { orig(a, rows, cols, mk()) })
	indexedPanics := panics(func() { indexed(a, rows, cols, mk()) })
	copyPanics := panics(func() { copyForm(a, rows, cols, mk()) })
	if !origPanics || !indexedPanics {
		t.Fatalf("len<cols<=cap: orig-panics=%v indexed-panics=%v, want both true", origPanics, indexedPanics)
	}
	if copyPanics {
		t.Fatalf("len<cols<=cap: copy(out[:cols],...) panicked — the divergence this test pins has vanished; re-audit PS1006's write-back rationale")
	}
	// len(out) < cols with NO spare capacity: everything must panic.
	short := func() []float64 { return make([]float64, 2) }
	if !panics(func() { orig(a, rows, cols, short()) }) ||
		!panics(func() { indexed(a, rows, cols, short()) }) ||
		!panics(func() { copyForm(a, rows, cols, short()) }) {
		t.Fatal("len<cols, cap<cols: all three forms must panic")
	}
	// Partial-write parity: both orig and indexed write out[0],out[1] before
	// panicking at c==2.
	o1, o2 := mk(), mk()
	panics(func() { orig(a, rows, cols, o1) })
	panics(func() { indexed(a, rows, cols, o2) })
	for c := 0; c < 2; c++ {
		if math.Float64bits(o1[c]) != math.Float64bits(o2[c]) {
			t.Fatalf("partial write before panic diverges at %d: orig=%v indexed=%v", c, o1[c], o2[c])
		}
	}
}

// TestEquiv_PS1010ColMean pins PS1010's column-mean interchange. The rewrite
// accumulates each column into a zeroed scratch in i-ASCENDING order (identical
// to the serial column sum), divides by the same len(rows), and writes back with
// an indexed `for j := 0; j < d` loop — NOT copy(mean[:d], ...) — so it preserves
// the original's mean-LENGTH panic (unlike the PS1006 copy bug this check does
// not share). Rectangular inputs must be bitwise-identical; a short mean must
// panic in BOTH forms with the same partial writes.
func TestEquiv_PS1010ColMean(t *testing.T) {
	serial := func(rows [][]float64, mean []float64, d int) {
		for j := 0; j < d; j++ {
			s := 0.0
			for i := range rows {
				s += rows[i][j]
			}
			mean[j] = s / float64(len(rows))
		}
	}
	interchanged := func(rows [][]float64, mean []float64, d int) {
		sums := make([]float64, d)
		for i := range rows {
			for j := 0; j < d; j++ {
				sums[j] += rows[i][j]
			}
		}
		for j := 0; j < d; j++ {
			mean[j] = sums[j] / float64(len(rows))
		}
	}
	rng := rand.New(rand.NewSource(0x1010))
	vals := []float64{1, -1, 1e300, 1e-300, math.Pi, 0, math.Copysign(0, -1), 1e16, math.SmallestNonzeroFloat64}
	pick := func() float64 {
		if rng.Intn(3) == 0 {
			return vals[rng.Intn(len(vals))]
		}
		return (rng.Float64() - 0.5) * rng.Float64() * 1e8
	}
	// Correctness over rectangular matrices.
	for trial := 0; trial < 1500; trial++ {
		nr, d := rng.Intn(6)+1, rng.Intn(6)+1
		rows := make([][]float64, nr)
		for i := range rows {
			rows[i] = make([]float64, d)
			for j := range rows[i] {
				rows[i][j] = pick()
			}
		}
		m1, m2 := make([]float64, d), make([]float64, d)
		serial(rows, m1, d)
		interchanged(rows, m2, d)
		for j := 0; j < d; j++ {
			if math.Float64bits(m1[j]) != math.Float64bits(m2[j]) {
				t.Fatalf("trial %d col %d: serial=%x interchanged=%x", trial, j, math.Float64bits(m1[j]), math.Float64bits(m2[j]))
			}
		}
	}
	// Panic parity: a mean shorter than d panics in BOTH, with identical partial writes.
	rows := [][]float64{{1, 2, 3, 4}, {5, 6, 7, 8}}
	d := 4
	run := func(f func([][]float64, []float64, int)) (mean []float64, panicked bool) {
		mean = make([]float64, 2) // len 2 < d
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		f(rows, mean, d)
		return
	}
	ms, ps := run(serial)
	mi, pi := run(interchanged)
	if !ps || !pi {
		t.Fatalf("short mean must panic in both: serial=%v interchanged=%v", ps, pi)
	}
	if math.Float64bits(ms[0]) != math.Float64bits(mi[0]) || math.Float64bits(ms[1]) != math.Float64bits(mi[1]) {
		t.Fatalf("partial writes diverge before panic: serial=%v interchanged=%v", ms, mi)
	}
}

// TestEquiv_PS1007OuterUnroll pins PS1007's rank-1-update outer unroll. The
// rewrite adds v0 then v1 into the SAME out[d] within one d-loop, so each out[d]
// still accumulates over i in ASCENDING, left-associated order — identical to the
// serial form, with NO reassociation (contrast the separate-accumulator PS6010).
// It is in-place `+=` (no scratch/copy write-back), so out's pre-existing content
// and the accumulation are both preserved bit-for-bit. Odd n exercises the serial
// tail; a non-zero initial out catches any accidental zeroing.
func TestEquiv_PS1007OuterUnroll(t *testing.T) {
	serial := func(w, in, out []float64, n, dim int) {
		for i := 0; i < n; i++ {
			v := w[i]
			for d := 0; d < dim; d++ {
				out[d] += v * in[i*dim+d]
			}
		}
	}
	unrolled := func(w, in, out []float64, n, dim int) {
		i := 0
		for ; i+1 < n; i += 2 {
			v0, v1 := w[i], w[i+1]
			for d := 0; d < dim; d++ {
				out[d] += v0 * in[i*dim+d]
				out[d] += v1 * in[(i+1)*dim+d]
			}
		}
		for ; i < n; i++ {
			v := w[i]
			for d := 0; d < dim; d++ {
				out[d] += v * in[i*dim+d]
			}
		}
	}
	rng := rand.New(rand.NewSource(0x1007))
	vals := []float64{1, -1, 1e300, 1e-300, math.Pi, 0, math.Copysign(0, -1), 1e16, math.SmallestNonzeroFloat64}
	pick := func() float64 {
		if rng.Intn(3) == 0 {
			return vals[rng.Intn(len(vals))]
		}
		return (rng.Float64() - 0.5) * rng.Float64() * 1e8
	}
	for trial := 0; trial < 2000; trial++ {
		n, dim := rng.Intn(7)+1, rng.Intn(6)+1 // n includes odd values -> tail
		w := make([]float64, n)
		for i := range w {
			w[i] = pick()
		}
		in := make([]float64, n*dim)
		for i := range in {
			in[i] = pick()
		}
		o1, o2 := make([]float64, dim), make([]float64, dim)
		for d := range o1 { // non-zero initial out (accumulated into, not overwritten)
			o1[d] = pick()
			o2[d] = o1[d]
		}
		serial(w, in, o1, n, dim)
		unrolled(w, in, o2, n, dim)
		for d := 0; d < dim; d++ {
			if math.Float64bits(o1[d]) != math.Float64bits(o2[d]) {
				t.Fatalf("trial %d n=%d dim=%d out[%d]: serial=%x unrolled=%x", trial, n, dim, d, math.Float64bits(o1[d]), math.Float64bits(o2[d]))
			}
		}
	}
}

// PS2005 (invalid-pattern guard): hoisting a MustCompile of an INVALID literal
// pattern out of a loop is NOT bit-identical — MustCompile panics whenever it
// runs, so the hoisted form panics before the loop even when the loop would
// have executed zero iterations and the original never panicked. This is the
// regression alarm for the ps2005PatternCompiles guard: the fix must be
// withheld (advisory only) for patterns that do not compile.
func TestEquiv_PS2005InvalidPatternPanicRelocation(t *testing.T) {
	orig := func(lines []string) (n int) {
		for _, s := range lines {
			//lint:ignore SA1000 the invalid pattern is the point: it proves MustCompile panics, and the loop-form only panics if it runs
			if regexp.MustCompile("(").MatchString(s) {
				n++
			}
		}
		return n
	}
	hoisted := func(lines []string) (n int) {
		//lint:ignore SA1000 intentionally-invalid pattern: hoisted MustCompile panics before a zero-iteration loop
		psRe := regexp.MustCompile("(")
		for _, s := range lines {
			if psRe.MatchString(s) {
				n++
			}
		}
		return n
	}
	panics := func(f func([]string) int, lines []string) (panicked bool) {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		f(lines)
		return false
	}
	if panics(orig, nil) {
		t.Fatal("original with zero-iteration loop must NOT panic: MustCompile never runs")
	}
	if !panics(hoisted, nil) {
		t.Fatal("hoisted form must panic: MustCompile of an invalid pattern runs before the loop")
	}
}

// PS2003: hoisting strings.Repeat(s, n) out of a loop is NOT bit-identical
// when n can be negative — strings.Repeat panics on count < 0, and for a
// zero-iteration loop the original never evaluates the call (no panic) while
// the hoisted binding evaluates it before the loop (panic). This is the
// regression alarm for the Repeat gate: the hoist must be withheld (advisory
// only) unless the count is a provably non-negative integer constant.
func TestEquiv_PS2003RepeatNegativeCountPanicRelocation(t *testing.T) {
	orig := func(lines []string, s string, n int) {
		for range lines {
			_ = strings.Repeat(s, n)
		}
	}
	hoisted := func(lines []string, s string, n int) {
		psStr := strings.Repeat(s, n)
		for range lines {
			_ = psStr
		}
	}
	panics := func(f func([]string, string, int), lines []string, s string, n int) (panicked bool) {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		f(lines, s, n)
		return false
	}
	if panics(orig, nil, "x", -1) {
		t.Fatal("original with zero-iteration loop must NOT panic: strings.Repeat never runs")
	}
	if !panics(hoisted, nil, "x", -1) {
		t.Fatal("hoisted form must panic: strings.Repeat with a negative count runs before the loop")
	}
	// Sanity: with a non-negative constant count both forms are panic-free —
	// exactly the case the gate keeps hoistable.
	if panics(orig, nil, "x", 3) || panics(hoisted, nil, "x", 3) {
		t.Fatal("non-negative count must not panic in either form")
	}
}

// TestEquiv_PS2116ClearFloatZeroing pins that PS2116's rewrite of a slice-
// zeroing loop `for i := range c { c[i] = 0 }` to `clear(c)` is BIT-identical
// for float and complex element types even when the buffer already holds -0.0,
// NaN, or ±Inf. Both the loop's `c[i] = 0` and clear(c) write the element
// type's zero value (+0.0 / 0+0i), overwriting any prior bit pattern, so the
// post-state is bitwise identical. Motivated by corpus -fix validation on
// gonum/blas/gonum, where PS2116 zeroed []float64/[]complex128 scratch buffers
// (clear(ctmp)/clear(btmp)) and gonum's FP-sensitive BLAS conformance tests
// passed — this locks the float-zero property the runtime clear() relies on.
func TestEquiv_PS2116ClearFloatZeroing(t *testing.T) {
	negZero := math.Copysign(0, -1) // a real negative zero (the `-0.0` literal is +0.0 in Go)
	seed := []float64{0, negZero, math.NaN(), math.Inf(1), math.Inf(-1), 1.5, -3.25, 1e308}
	// float64 slice.
	loopF := append([]float64(nil), seed...)
	clearF := append([]float64(nil), seed...)
	for i := range loopF {
		loopF[i] = 0
	}
	clear(clearF)
	for i := range loopF {
		if math.Float64bits(loopF[i]) != math.Float64bits(clearF[i]) {
			t.Errorf("float64[%d]: loop=%#x clear=%#x", i, math.Float64bits(loopF[i]), math.Float64bits(clearF[i]))
		}
		if math.Float64bits(clearF[i]) != 0 {
			t.Errorf("clear(float64)[%d] = %#x, want +0.0 (0x0)", i, math.Float64bits(clearF[i]))
		}
	}
	// complex128 slice: real+imag both must be +0.0.
	cseed := []complex128{complex(math.NaN(), negZero), complex(negZero, math.Inf(1)), 2 + 3i}
	loopC := append([]complex128(nil), cseed...)
	clearC := append([]complex128(nil), cseed...)
	for i := range loopC {
		loopC[i] = 0
	}
	clear(clearC)
	for i := range loopC {
		lb := [2]uint64{math.Float64bits(real(loopC[i])), math.Float64bits(imag(loopC[i]))}
		cb := [2]uint64{math.Float64bits(real(clearC[i])), math.Float64bits(imag(clearC[i]))}
		if lb != cb || cb != [2]uint64{0, 0} {
			t.Errorf("complex128[%d]: loop=%v clear=%v (want both 0x0)", i, lb, cb)
		}
	}
}

// TestEquiv_PS2128BuilderAccumulator pins the semantic equivalence PS2128 relies
// on: rewriting a string accumulator grown by `+=`/`= x + y` in a loop into a
// strings.Builder (seed -> leading WriteString, each append -> WriteString, each
// post-loop read -> String()) is byte-identical to the original concatenation,
// across empty/single/many/unicode/empty-element inputs, for the empty-init,
// seeded, if-guarded, and multi-read shapes the check fixes. Builder.WriteString
// appends exactly its argument's bytes and String() returns them in order, so
// the equivalence holds by construction — this locks it as a differential guard
// against any future drift in the transform. Complements the golden/adversarial
// fixtures (which check the REWRITE shape) with a runtime output check.
func TestEquiv_PS2128BuilderAccumulator(t *testing.T) {
	inputs := [][]string{
		nil, {}, {""}, {"a"}, {"a", "b", "c"}, {"", "", ""},
		{"x", "", "y", ""}, {"日", "本", "語"}, {"pre", "\x00", "post"},
	}
	// joinDefine: empty init, plain append.
	origDefine := func(items []string) string {
		acc := ""
		for i := 0; i < len(items); i++ {
			acc += items[i]
		}
		return acc
	}
	newDefine := func(items []string) string {
		var acc strings.Builder
		for i := 0; i < len(items); i++ {
			acc.WriteString(items[i])
		}
		return acc.String()
	}
	// seeded: non-empty seed preserved as a leading WriteString.
	origSeeded := func(items []string) string {
		acc := "prefix:"
		for _, it := range items {
			acc += it
		}
		return acc
	}
	newSeeded := func(items []string) string {
		var acc strings.Builder
		acc.WriteString("prefix:")
		for _, it := range items {
			acc.WriteString(it)
		}
		return acc.String()
	}
	// guarded with two post-loop reads: acc AND len(acc) both via String().
	origGuarded := func(items []string) (string, int) {
		acc := ""
		for _, it := range items {
			if it != "" {
				acc += it
			}
		}
		return acc, len(acc)
	}
	newGuarded := func(items []string) (string, int) {
		var acc strings.Builder
		for _, it := range items {
			if it != "" {
				acc.WriteString(it)
			}
		}
		return acc.String(), len(acc.String())
	}
	for _, in := range inputs {
		if got, want := newDefine(in), origDefine(in); got != want {
			t.Errorf("define(%q): builder=%q concat=%q", in, got, want)
		}
		if got, want := newSeeded(in), origSeeded(in); got != want {
			t.Errorf("seeded(%q): builder=%q concat=%q", in, got, want)
		}
		gs, gn := newGuarded(in)
		ws, wn := origGuarded(in)
		if gs != ws || gn != wn {
			t.Errorf("guarded(%q): builder=(%q,%d) concat=(%q,%d)", in, gs, gn, ws, wn)
		}
	}
}

// TestEquiv_PS2103SprintfSpliceToConcat pins the property PS2103 relies on:
// rewriting fmt.Sprintf with a format of only literal text and %s/%v verbs over
// STRING args into plain `+` concatenation is byte-identical. The critical
// safety point is that %s/%v splice each string arg VERBATIM — an arg that
// itself contains '%', "%s", or "%d" is NOT re-interpreted as a verb (fmt only
// parses the FORMAT literal), exactly as concatenation splices it verbatim.
// Escape sequences in the format (\t, \n) are literal bytes in both. Mirrors the
// three fixed golden shapes; args include %-laden, tab/newline, unicode, empty.
func TestEquiv_PS2103SprintfSpliceToConcat(t *testing.T) {
	vals := []string{"", "x", "100%", "%s", "%d", "%!v(int=3)", "a\tb", "line\n", "日本語", ":", "="}
	for _, a := range vals {
		for _, b := range vals {
			// "%s:%s" -> a+":"+b
			if got, want := fmt.Sprintf("%s:%s", a, b), a+":"+b; got != want {
				t.Errorf("%%s:%%s (a=%q b=%q): sprintf=%q concat=%q", a, b, got, want)
			}
			// "%s\t%s!" -> a+"\t"+b+"!"
			if got, want := fmt.Sprintf("%s\t%s!", a, b), a+"\t"+b+"!"; got != want {
				t.Errorf("%%s\\t%%s! (a=%q b=%q): sprintf=%q concat=%q", a, b, got, want)
			}
		}
		// "name=%v" -> "name="+a  (%v over a string == the string, no quoting)
		if got, want := fmt.Sprintf("name=%v", a), "name="+a; got != want {
			t.Errorf("name=%%v (a=%q): sprintf=%q concat=%q", a, got, want)
		}
	}
}

// TestEquiv_PS2120WriteStringSprintfToFprintf pins that PS2120's rewrite of
// `w.WriteString(fmt.Sprintf(f, a...))` to `fmt.Fprintf(w, f, a...)` is
// byte-identical in what it writes AND in its (n, err) return: fmt.Fprintf
// formats into the same bytes fmt.Sprintf produces and writes them to w, so both
// the written content and the byte count match, and both surface w's write error
// identically. Verified over several formats/args by writing each way into two
// bytes.Buffers and comparing bytes + the returned (n, err). Observed applying
// ~11x during corpus validation on gohugoio/hugo. The io.StringWriter fast path
// (WriteString) vs Fprintf's generic path produce the same observable result.
func TestEquiv_PS2120WriteStringSprintfToFprintf(t *testing.T) {
	type tc struct {
		format string
		args   []any
	}
	cases := []tc{
		{"%d", []any{42}},
		{"x=%d,%s", []any{-7, "hi"}},
		{"%s", []any{"100%"}}, // arg-% not re-parsed by either path
		{"%q %v", []any{"a\tb", 3.5}},
		{"no verbs here", nil},
		{"%x", []any{[]byte{0x00, 0xff, 0x10}}},
		{"日本%d語", []any{9}},
	}
	for _, c := range cases {
		var wA, wB bytes.Buffer
		nA, errA := wA.WriteString(fmt.Sprintf(c.format, c.args...)) // original
		nB, errB := fmt.Fprintf(&wB, c.format, c.args...)            // rewritten
		if wA.String() != wB.String() {
			t.Errorf("format %q: WriteString wrote %q but Fprintf wrote %q", c.format, wA.String(), wB.String())
		}
		if nA != nB {
			t.Errorf("format %q: byte count differs: WriteString=%d Fprintf=%d", c.format, nA, nB)
		}
		if (errA == nil) != (errB == nil) {
			t.Errorf("format %q: error mismatch: WriteString=%v Fprintf=%v", c.format, errA, errB)
		}
	}
}

// TestEquiv_PS2109SprintfBytesToAppendf pins that PS2109's rewrite of
// []byte(fmt.Sprintf(f, a...)) to fmt.Appendf(nil, f, a...) is byte-identical.
// fmt.Appendf(nil, f, a...) formats into a fresh []byte exactly as Sprintf
// formats into a string, so converting that string to []byte yields the same
// bytes — Appendf just skips the intermediate string allocation. Verified over
// several formats/args (incl. arg-% not re-parsed, %x []byte, unicode, multiple
// verbs). Observed applying ~13x during corpus validation on grpc-go
// (e.g. []byte(fmt.Sprintf("%s:%s:%s", ...)) -> fmt.Appendf(nil, ...)).
func TestEquiv_PS2109SprintfBytesToAppendf(t *testing.T) {
	type tc struct {
		format string
		args   []any
	}
	cases := []tc{
		{"%d", []any{42}},
		{"%s:%s:%s", []any{"a", "b", "c"}},
		{"%s", []any{"100%"}},
		{"\"%ss\"", []any{"x"}},
		{"%x", []any{[]byte{0x00, 0xff, 0x10}}},
		{"%q %v %d", []any{"a\tb", 3.5, -7}},
		{"日本%d語", []any{9}},
		{"no verbs", nil},
	}
	for _, c := range cases {
		orig := []byte(fmt.Sprintf(c.format, c.args...)) // original
		got := fmt.Appendf(nil, c.format, c.args...)     // rewritten
		if !bytes.Equal(orig, got) {
			t.Errorf("format %q: []byte(Sprintf)=%q != Appendf(nil)=%q", c.format, orig, got)
		}
	}
}

// TestEquiv_PS2107SprintfBinOctToFormat pins PS2107's %b/%o arms: base-2 and
// base-8 integer formatting via fmt is byte-identical to strconv.FormatInt/
// FormatUint with the matching base, for signed and unsigned across the full
// range incl. MinInt64/MaxInt64 (Itoa is base-10 only, hence FormatInt/Uint).
func TestEquiv_PS2107SprintfBinOctToFormat(t *testing.T) {
	ints := []int64{0, -1, 1, math.MaxInt64, math.MinInt64, math.MaxInt32, math.MinInt32, 255, -255}
	for i := int64(-5000); i < 5000; i++ {
		ints = append(ints, i)
	}
	for _, i := range ints {
		if fmt.Sprintf("%b", i) != strconv.FormatInt(i, 2) {
			t.Errorf("%%b(%d): %q != FormatInt base2 %q", i, fmt.Sprintf("%b", i), strconv.FormatInt(i, 2))
		}
		if fmt.Sprintf("%o", i) != strconv.FormatInt(i, 8) {
			t.Errorf("%%o(%d): %q != FormatInt base8 %q", i, fmt.Sprintf("%o", i), strconv.FormatInt(i, 8))
		}
	}
	uints := []uint64{0, 1, math.MaxUint64, math.MaxUint32, 255, 1<<63 - 1, 1 << 63}
	for _, u := range uints {
		if fmt.Sprintf("%b", u) != strconv.FormatUint(u, 2) {
			t.Errorf("%%b(uint %d): %q != FormatUint base2 %q", u, fmt.Sprintf("%b", u), strconv.FormatUint(u, 2))
		}
		if fmt.Sprintf("%o", u) != strconv.FormatUint(u, 8) {
			t.Errorf("%%o(uint %d): %q != FormatUint base8 %q", u, fmt.Sprintf("%o", u), strconv.FormatUint(u, 8))
		}
	}
}

// TestEquiv_PS2107SprintfQuote pins PS2107's %q arms: fmt.Sprintf("%q", s) over a
// string equals strconv.Quote(s), and fmt.Sprintf("%q", r) over a rune equals
// strconv.QuoteRune(r) — including control bytes, escapes, unicode, and the
// out-of-range/invalid runes that both render as U+FFFD.
func TestEquiv_PS2107SprintfQuote(t *testing.T) {
	strs := []string{"", "a", "hello\tworld", "quote\"me", "日本語", "back\\slash", "\x00\x01\x1f", "line\nbreak", "emoji😀", "\x7f"}
	for _, s := range strs {
		if fmt.Sprintf("%q", s) != strconv.Quote(s) {
			t.Errorf("%%q(%q): %q != strconv.Quote %q", s, fmt.Sprintf("%q", s), strconv.Quote(s))
		}
	}
	runes := []rune{'A', '日', 0, 0x10FFFF, 0x110000, utf8.RuneError, -1, 0x1F600, '\t', '\'', '\\', 0x7f, '\n'}
	for _, r := range runes {
		if fmt.Sprintf("%q", r) != strconv.QuoteRune(r) {
			t.Errorf("%%q(rune %d): %q != QuoteRune %q", r, fmt.Sprintf("%q", r), strconv.QuoteRune(r))
		}
	}
}

// TestEquiv_PS5106StringsCompareToOperator pins that strings.Compare(a, b)
// compared to 0 is bit-identical to the direct operator, for all six comparison
// operators and both operand orders (mirrored when Compare is on the right).
// strings.Compare returns exactly -1/0/+1 by definition, so this holds for every
// string pair including empties, prefixes, unicode, and embedded NUL.
func TestEquiv_PS5106StringsCompareToOperator(t *testing.T) {
	vals := []string{"", "a", "b", "ab", "ba", "abc", "abd", "日", "日本", "日本語", "\x00", "a\x00", "z", "aa"}
	for _, a := range vals {
		for _, b := range vals {
			c := strings.Compare(a, b)
			// Compare on the left: operator kept.
			if (c == 0) != (a == b) || (c != 0) != (a != b) ||
				(c < 0) != (a < b) || (c <= 0) != (a <= b) ||
				(c > 0) != (a > b) || (c >= 0) != (a >= b) {
				t.Errorf("left(%q,%q): Compare=%d disagrees with an operator", a, b, c)
			}
			// Compare on the right of 0: operator mirrored. The Yoda spelling
			// (0 OP Compare) is exactly the shape PS5106 rewrites, so it is
			// deliberate here.
			//lint:ignore ST1017 deliberately exercising the 0-on-the-left form PS5106 mirrors
			if (0 == c) != (a == b) || (0 != c) != (a != b) ||
				(0 < c) != (a > b) || (0 <= c) != (a >= b) ||
				(0 > c) != (a < b) || (0 >= c) != (a <= b) {
				t.Errorf("right(%q,%q): 0 OP Compare=%d disagrees with the mirrored operator", a, b, c)
			}
		}
	}
}

// TestEquiv_PS2107SprintfCharToString pins PS2107's %c arm: fmt.Sprintf("%c", r)
// over a rune equals string(r) for the full rune range, including out-of-range
// and negative code points (both render as U+FFFD). Restricted to rune (int32)
// operands — a wider integer would truncate under rune() but %c still prints
// U+FFFD, so those are deliberately not rewritten.
func TestEquiv_PS2107SprintfCharToString(t *testing.T) {
	edge := []rune{'A', '0', '日', 0, 1, 0x7f, 0x80, 0x10FFFF, 0x110000, 0x1FFFFF, utf8.RuneError, -1, -100, 0x1F600, '\n', '\t'}
	for _, r := range edge {
		if fmt.Sprintf("%c", r) != string(r) {
			t.Errorf("%%c(rune %d): %q != string(r) %q", r, fmt.Sprintf("%c", r), string(r))
		}
	}
	for r := rune(-2); r < 0x110020; r++ {
		if fmt.Sprintf("%c", r) != string(r) {
			t.Fatalf("%%c(rune %d) diverges from string(r)", r)
		}
	}
}

// TestEquiv_PS2131MatchStringEqualsPrecompiled pins the equivalence PS2131's
// advice relies on: for a VALID pattern, the (bool) result of the recompiling
// helper regexp.MatchString(pattern, s) equals the reused-regexp form
// regexp.MustCompile(pattern).MatchString(s) — the recommended hoist. (The
// helper additionally returns a nil error for a valid pattern, which the hoisted
// form drops; hence PS2131 is advisory, not an automatic in-place rewrite.)
// Verified for MatchString and Match over several patterns and inputs.
func TestEquiv_PS2131MatchStringEqualsPrecompiled(t *testing.T) {
	pats := []string{"^[a-z]+$", "[0-9]+", "a.c", "^$", "x*", "\\bword\\b"}
	inputs := []string{"", "abc", "a1c", "123", "word here", "AxC"}
	for _, p := range pats {
		re := regexp.MustCompile(p)
		for _, s := range inputs {
			got, err := regexp.MatchString(p, s)
			if err != nil {
				t.Fatalf("MatchString(%q,%q) unexpected err: %v", p, s, err)
			}
			if got != re.MatchString(s) {
				t.Errorf("MatchString(%q,%q)=%v != MustCompile(%q).MatchString=%v", p, s, got, p, re.MatchString(s))
			}
			gotB, _ := regexp.Match(p, []byte(s))
			if gotB != re.Match([]byte(s)) {
				t.Errorf("Match(%q,[]byte(%q)) disagrees with the precompiled form", p, s)
			}
		}
	}
}

// TestEquiv_PS2132NewReplacerReuseEquivalent pins the equivalence PS2132's advice
// relies on: for a fixed set of pairs, a freshly-built strings.NewReplacer(pairs)
// and a reused package-level one produce identical output. Hoisting the
// constructor to package scope is therefore behavior-preserving (it only removes
// the per-call rebuild). Verified over several pair sets and inputs incl. empties,
// overlapping keys, and text with no matches.
func TestEquiv_PS2132NewReplacerReuseEquivalent(t *testing.T) {
	pairSets := [][]string{
		{"&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&#34;", "'", "&#39;"},
		{"a", "b", "c", "d"},
		{"foo", "bar"},
		{"", "X"}, // empty key
	}
	inputs := []string{"", "a<b>&c \"d\" 'e'", "no specials here", "foofoo", "aaa", "&&&<<<"}
	for _, ps := range pairSets {
		reused := strings.NewReplacer(ps...)
		for _, s := range inputs {
			fresh := strings.NewReplacer(ps...).Replace(s)
			if fresh != reused.Replace(s) {
				t.Errorf("pairs=%v input=%q: inline=%q reused=%q", ps, s, fresh, reused.Replace(s))
			}
		}
	}
}

// TestEquiv_PS2133LoadLocationReuseEquivalent pins the equivalence PS2133's
// advice relies on: for a fixed zone name, a per-call time.LoadLocation(name) and
// a package-level one loaded once name the SAME location — hoisting the load to
// package scope is behavior-preserving (it only removes the per-call tzdata read
// and parse). Verified by name equality across several real zones plus the
// fast-path names that PS2133 deliberately leaves alone.
func TestEquiv_PS2133LoadLocationReuseEquivalent(t *testing.T) {
	for _, name := range []string{"America/New_York", "Europe/Berlin", "Asia/Tokyo", "UTC", "Local"} {
		cached, err := time.LoadLocation(name)
		if err != nil {
			t.Skipf("zone %q unavailable in this environment: %v", name, err)
		}
		fresh, err := time.LoadLocation(name)
		if err != nil {
			t.Fatalf("second LoadLocation(%q): %v", name, err)
		}
		if fresh.String() != cached.String() {
			t.Errorf("LoadLocation(%q): fresh=%q != cached=%q", name, fresh.String(), cached.String())
		}
		// Both resolve the same instant to the same wall-clock representation.
		ref := time.Unix(1700000000, 0)
		if ref.In(fresh).Format(time.RFC3339) != ref.In(cached).Format(time.RFC3339) {
			t.Errorf("LoadLocation(%q): In() disagrees between fresh and cached", name)
		}
	}
}

// TestEquiv_PS2134TemplateReuseEquivalent pins the equivalence PS2134's advice
// relies on: for a fixed template text, a freshly parsed template.New(name).Parse
// and a package-level one parsed once render identical output for the same data.
// Hoisting the parse to package scope is therefore behavior-preserving — it only
// removes the per-call re-parse.
func TestEquiv_PS2134TemplateReuseEquivalent(t *testing.T) {
	const src = "Hello {{.Name}}, you have {{.Count}} messages{{if .Admin}} (admin){{end}}."
	cached := template.Must(template.New("t").Parse(src))
	datas := []map[string]any{
		{"Name": "Ann", "Count": 3, "Admin": true},
		{"Name": "", "Count": 0, "Admin": false},
		{"Name": "日本", "Count": 99, "Admin": false},
	}
	for _, d := range datas {
		fresh := template.Must(template.New("t").Parse(src))
		var a, b strings.Builder
		if err := fresh.Execute(&a, d); err != nil {
			t.Fatalf("fresh.Execute: %v", err)
		}
		if err := cached.Execute(&b, d); err != nil {
			t.Fatalf("cached.Execute: %v", err)
		}
		if a.String() != b.String() {
			t.Errorf("data=%v: fresh=%q != cached=%q", d, a.String(), b.String())
		}
	}

	// html/template is covered by the fix too: its contextual auto-escaping is
	// deterministic for a fixed template text, so a shared parsed template renders
	// byte-identically to a fresh per-call parse (including escaped fields).
	const hsrc = "<p>{{.Name}}</p><a href=\"{{.URL}}\">link</a>"
	hcached := htmltemplate.Must(htmltemplate.New("h").Parse(hsrc))
	for _, d := range []map[string]any{
		{"Name": "<b>Ann</b>", "URL": "https://ex.com/?q=1&x=2"},
		{"Name": "plain", "URL": "javascript:alert(1)"},
	} {
		hfresh := htmltemplate.Must(htmltemplate.New("h").Parse(hsrc))
		var a, b strings.Builder
		if err := hfresh.Execute(&a, d); err != nil {
			t.Fatalf("hfresh.Execute: %v", err)
		}
		if err := hcached.Execute(&b, d); err != nil {
			t.Fatalf("hcached.Execute: %v", err)
		}
		if a.String() != b.String() {
			t.Errorf("html data=%v: fresh=%q != cached=%q", d, a.String(), b.String())
		}
	}
}

// TestEquiv_PS2135WriteBytesEquivalent pins the bit-identity PS2135's fix
// relies on: per the io.StringWriter contract, w.WriteString(s) behaves like
// w.Write([]byte(s)), so for a []byte b, WriteString(string(b)) and Write(b)
// write exactly b's bytes and return the same (n, err). Verified over edge
// cases — nil, empty, non-UTF8 bytes (string(b) does NOT sanitize invalid
// UTF-8; the bytes pass through verbatim), and embedded NULs — on both
// bytes.Buffer and strings.Builder, the receivers the fix most often hits.
func TestEquiv_PS2135WriteBytesEquivalent(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		[]byte("plain ascii"),
		{0xff, 0xfe},                     // invalid UTF-8: copied verbatim, not replaced
		{'a', 0x00, 'b', 0x00},           // embedded NULs survive both paths
		[]byte("héllo, 日本語"),             // multi-byte UTF-8
		bytes.Repeat([]byte{0x80}, 1024), // long run of continuation bytes
	}
	for _, b := range cases {
		var viaString, viaWrite bytes.Buffer
		n1, err1 := viaString.WriteString(string(b))
		n2, err2 := viaWrite.Write(b)
		if n1 != n2 || (err1 == nil) != (err2 == nil) {
			t.Errorf("Buffer %x: WriteString(string(b))=(%d,%v) != Write(b)=(%d,%v)", b, n1, err1, n2, err2)
		}
		if n1 != len(b) {
			t.Errorf("Buffer %x: n=%d, want len(b)=%d", b, n1, len(b))
		}
		if !bytes.Equal(viaString.Bytes(), viaWrite.Bytes()) {
			t.Errorf("Buffer %x: contents diverge: %x != %x", b, viaString.Bytes(), viaWrite.Bytes())
		}

		var sbString, sbWrite strings.Builder
		m1, serr1 := sbString.WriteString(string(b))
		m2, serr2 := sbWrite.Write(b)
		if m1 != m2 || (serr1 == nil) != (serr2 == nil) {
			t.Errorf("Builder %x: WriteString(string(b))=(%d,%v) != Write(b)=(%d,%v)", b, m1, serr1, m2, serr2)
		}
		if sbString.String() != sbWrite.String() {
			t.Errorf("Builder %x: contents diverge: %q != %q", b, sbString.String(), sbWrite.String())
		}
	}
}

// TestEquiv_PS2136StrconvBytesToAppend pins that PS2136's rewrite of
// []byte(strconv.FOO(args...)) to strconv.AppendFOO(nil, args...) is
// byte-identical for every matched verb. In the standard library each
// Format*/Quote*/Itoa is implemented as string(Append*(nil, ...)), so the
// Append form yields exactly the bytes the []byte conversion copies out of
// the string — and every output is at least one byte long (a digit,
// "true"/"false", or the opening quote), so there is no nil-vs-empty-slice
// corner: the equivalence holds including nil-ness.
func TestEquiv_PS2136StrconvBytesToAppend(t *testing.T) {
	t.Run("Itoa", func(t *testing.T) {
		ints := []int{0, 1, -1, 9, 10, -10, 123456, -123456, math.MaxInt, math.MinInt, math.MaxInt32, math.MinInt32}
		for i := -1100; i <= 1100; i++ {
			ints = append(ints, i)
		}
		for _, n := range ints {
			orig := []byte(strconv.Itoa(n))             // original
			got := strconv.AppendInt(nil, int64(n), 10) // rewritten (Itoa special case)
			if !bytes.Equal(orig, got) || got == nil {
				t.Errorf("Itoa(%d): []byte=%q != AppendInt=%q", n, orig, got)
			}
		}
	})
	t.Run("FormatInt", func(t *testing.T) {
		ints := []int64{0, 1, -1, math.MaxInt64, math.MinInt64, math.MaxInt32, math.MinInt32, 255, -255, 1 << 40}
		for _, x := range ints {
			for _, base := range []int{2, 8, 10, 16, 36} {
				orig := []byte(strconv.FormatInt(x, base))
				got := strconv.AppendInt(nil, x, base)
				if !bytes.Equal(orig, got) || got == nil {
					t.Errorf("FormatInt(%d,%d): %q != %q", x, base, orig, got)
				}
			}
		}
	})
	t.Run("FormatUint", func(t *testing.T) {
		uints := []uint64{0, 1, math.MaxUint64, math.MaxUint32, 255, 1 << 63, 1<<63 - 1}
		for _, u := range uints {
			for _, base := range []int{2, 8, 10, 16, 36} {
				orig := []byte(strconv.FormatUint(u, base))
				got := strconv.AppendUint(nil, u, base)
				if !bytes.Equal(orig, got) || got == nil {
					t.Errorf("FormatUint(%d,%d): %q != %q", u, base, orig, got)
				}
			}
		}
	})
	t.Run("FormatFloat", func(t *testing.T) {
		floats := []float64{
			0, math.Copysign(0, -1), 1.5, -1.5, math.Pi, 1e300, -1e-300,
			math.NaN(), math.Inf(1), math.Inf(-1),
			math.MaxFloat64, math.SmallestNonzeroFloat64, 3.4028235e38, // ~MaxFloat32
		}
		type shape struct {
			fmt  byte
			prec int
			bits int
		}
		shapes := []shape{
			{'g', -1, 64}, {'g', -1, 32}, {'e', 6, 64}, {'f', 3, 64},
			{'b', -1, 64}, {'x', -1, 64}, {'E', 2, 32}, {'G', 10, 64},
		}
		for _, f := range floats {
			for _, s := range shapes {
				orig := []byte(strconv.FormatFloat(f, s.fmt, s.prec, s.bits))
				got := strconv.AppendFloat(nil, f, s.fmt, s.prec, s.bits)
				if !bytes.Equal(orig, got) || got == nil {
					t.Errorf("FormatFloat(%v,%q,%d,%d): %q != %q", f, s.fmt, s.prec, s.bits, orig, got)
				}
			}
		}
	})
	t.Run("FormatBool", func(t *testing.T) {
		for _, v := range []bool{true, false} {
			orig := []byte(strconv.FormatBool(v))
			got := strconv.AppendBool(nil, v)
			if !bytes.Equal(orig, got) || got == nil {
				t.Errorf("FormatBool(%v): %q != %q", v, orig, got)
			}
		}
	})
	t.Run("QuoteStrings", func(t *testing.T) {
		strs := []string{
			"", "a", "hello\tworld", "quote\"me", "back\\slash", "line\nbreak",
			"日本語", "emoji😀", "\x00\x01\x1f", "\x7f", "\xff\xfe", // invalid UTF-8
			" ", " ", "mixed 日本 ascii", "'single'",
		}
		for _, s := range strs {
			if orig, got := []byte(strconv.Quote(s)), strconv.AppendQuote(nil, s); !bytes.Equal(orig, got) || got == nil {
				t.Errorf("Quote(%q): %q != %q", s, orig, got)
			}
			if orig, got := []byte(strconv.QuoteToASCII(s)), strconv.AppendQuoteToASCII(nil, s); !bytes.Equal(orig, got) || got == nil {
				t.Errorf("QuoteToASCII(%q): %q != %q", s, orig, got)
			}
			if orig, got := []byte(strconv.QuoteToGraphic(s)), strconv.AppendQuoteToGraphic(nil, s); !bytes.Equal(orig, got) || got == nil {
				t.Errorf("QuoteToGraphic(%q): %q != %q", s, orig, got)
			}
		}
	})
	t.Run("QuoteRunes", func(t *testing.T) {
		runes := []rune{
			'A', '0', '日', 0, 1, '\t', '\n', '\'', '\\', 0x7f, 0x80,
			utf8.RuneError, -1, -100, 0x10FFFF, 0x110000, 0x1F600, 0x2028, 0x00a0,
		}
		for _, r := range runes {
			if orig, got := []byte(strconv.QuoteRune(r)), strconv.AppendQuoteRune(nil, r); !bytes.Equal(orig, got) || got == nil {
				t.Errorf("QuoteRune(%d): %q != %q", r, orig, got)
			}
			if orig, got := []byte(strconv.QuoteRuneToASCII(r)), strconv.AppendQuoteRuneToASCII(nil, r); !bytes.Equal(orig, got) || got == nil {
				t.Errorf("QuoteRuneToASCII(%d): %q != %q", r, orig, got)
			}
			if orig, got := []byte(strconv.QuoteRuneToGraphic(r)), strconv.AppendQuoteRuneToGraphic(nil, r); !bytes.Equal(orig, got) || got == nil {
				t.Errorf("QuoteRuneToGraphic(%d): %q != %q", r, orig, got)
			}
		}
	})
}
