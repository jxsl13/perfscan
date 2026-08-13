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
	"math/rand"
	"slices"
	"sort"
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
