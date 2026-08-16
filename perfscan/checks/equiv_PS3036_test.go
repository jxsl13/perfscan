package checks

// PS3036's runtime differential: slices.CompareFunc with a bare
// strings.Compare comparator — strings.Compare passed directly AND the
// closure spelling the check matches — must return an int BYTE-IDENTICAL to
// slices.Compare on every input the auto-fix accepts. Pinned against the REAL
// stdlib so a future stdlib change that breaks the claim fails CI:
//
//  1. VALUE: slices.Compare's body IS CompareFunc's loop with the comparator
//     fixed to cmp.Compare(s1[i], s2[i]) in source order; on strings
//     cmp.Compare returns exactly strings.Compare's -1/0/+1 (its isNaN
//     branches are dead), so the early-exit index, the propagated int and
//     the ±1 length tie-breaks all agree exactly — not merely in sign. And
//     because strings.Compare only ever yields -1/0/+1, no non-canonical
//     magnitude can leak through CompareFunc's "return the comparator's
//     int" path.
//  2. BYTES, NOT RUNES: both sides order by raw bytes, so invalid UTF-8,
//     0x00/0xFF bytes and high-bit-vs-ASCII orderings agree trivially; this
//     test hammers them anyway.
//  3. SHAPES: no panic paths, no writes, both slices read left to right —
//     the only observable is the returned int, compared exhaustively.
//
// The inputs are adversarial for a lexicographic early-exit loop: equal
// prefixes with a late divergence, one slice a proper prefix of the other,
// empties and nils on either side, first-element divergence, elements that
// are themselves prefix pairs, 0x00/0xFF bytes and invalid UTF-8, plus
// randomized draws from a tiny adversarial alphabet (tie stress) with
// independent lengths, in both argument orders.

import (
	"math/rand"
	"slices"
	"strings"
	"testing"
)

// ps3036Check pins both matched comparator spellings against slices.Compare
// on (a, b) AND (b, a) — the swapped call is a distinct code path through the
// length tie-breaks.
func ps3036Check(t *testing.T, a, b []string) {
	t.Helper()
	for _, pair := range [2][2][]string{{a, b}, {b, a}} {
		s1, s2 := pair[0], pair[1]
		want := slices.Compare(s1, s2)
		if got := slices.CompareFunc(s1, s2, strings.Compare); got != want {
			t.Fatalf("CompareFunc(strings.Compare) != Compare on %q vs %q: %d vs %d", s1, s2, got, want)
		}
		if got := slices.CompareFunc(s1, s2, func(x, y string) int { return strings.Compare(x, y) }); got != want {
			t.Fatalf("CompareFunc(closure) != Compare on %q vs %q: %d vs %d", s1, s2, got, want)
		}
	}
}

func TestEquiv_PS3036CompareFuncStringsCompare(t *testing.T) {
	// Nil/empty on either side: the length tie-breaks alone decide.
	ps3036Check(t, nil, nil)
	ps3036Check(t, nil, []string{})
	ps3036Check(t, []string{}, []string{})
	ps3036Check(t, nil, []string{""})
	ps3036Check(t, []string{}, []string{"a"})
	ps3036Check(t, []string{""}, []string{""})
	ps3036Check(t, []string{""}, []string{"", ""})

	// Element-level byte edge cases, exhaustively paired one- and two-element:
	// empty, prefix pairs, 0x00/0xFF bytes, invalid UTF-8, high-bit vs ASCII.
	pool := []string{
		"", "a", "aa", "ab", "b",
		"\x00", "\x00a", "a\x00", "\xff", "\xff\xfe",
		"\x80world", "world", "wörld", "wo\xc3",
		"prefix", "prefixx", "prefix\x00",
	}
	for i := range pool {
		for j := range pool {
			ps3036Check(t, []string{pool[i]}, []string{pool[j]})
			ps3036Check(t,
				[]string{pool[i], pool[j]},
				[]string{pool[j], pool[i]})
		}
	}

	// Structured shapes at assorted sizes: full equality, first-element
	// divergence, late divergence after an equal prefix, proper prefixes.
	sizes := []int{1, 2, 3, 7, 13, 64, 257, 1024}
	for _, n := range sizes {
		base := make([]string, n)
		for i := range base {
			base[i] = strings.Repeat("k", 1+i%5) + string(rune('a'+i%26))
		}
		equal := slices.Clone(base)
		ps3036Check(t, base, equal) // fully equal -> 0 both ways

		first := slices.Clone(base)
		first[0] += "\x00" // diverges at index 0, by a suffix byte
		ps3036Check(t, base, first)

		last := slices.Clone(base)
		last[n-1] = last[n-1][:len(last[n-1])-1] // equal prefix, last pair diverges by length
		ps3036Check(t, base, last)

		if n > 1 {
			mid := slices.Clone(base)
			mid[n/2] = "\xff" // diverges in the middle, high byte
			ps3036Check(t, base, mid)
		}

		ps3036Check(t, base, base[:n-n/3]) // proper prefix: +1/-1 by length
		ps3036Check(t, base[:0], base)
	}

	// Randomized draws: a tiny adversarial alphabet forces long equal
	// prefixes and frequent full ties; independent lengths exercise every
	// length tie-break.
	r := rand.New(rand.NewSource(3036))
	for trial := 0; trial < 3000; trial++ {
		na, nb := r.Intn(12), r.Intn(12)
		a := make([]string, na)
		b := make([]string, nb)
		for i := range a {
			a[i] = pool[r.Intn(len(pool))]
		}
		for i := range b {
			b[i] = pool[r.Intn(len(pool))]
		}
		ps3036Check(t, a, b)

		// Shared random prefix with a late mutation or truncation.
		n := 1 + r.Intn(40)
		p := make([]string, n)
		for i := range p {
			p[i] = pool[r.Intn(len(pool))]
		}
		q := slices.Clone(p)
		if r.Intn(2) == 0 {
			q[r.Intn(n)] = pool[r.Intn(len(pool))]
		} else {
			q = q[:r.Intn(n+1)]
		}
		ps3036Check(t, p, q)
	}

	// A big pair diverging only at the very last element: full-length scan.
	big := make([]string, 4096)
	for i := range big {
		big[i] = "key " + string(rune('a'+i%26)) + strings.Repeat("x", i%7)
	}
	bigTail := slices.Clone(big)
	bigTail[len(bigTail)-1] += "\x00"
	ps3036Check(t, big, bigTail)
	ps3036Check(t, big, big[:4095])
}
