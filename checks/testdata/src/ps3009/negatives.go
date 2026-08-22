package ps3009

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"
	"strings"
)

type fakeStrings struct{}

func (fakeStrings) Compare(a, b string) int { return len(b) - len(a) }

type name string

var cmpFn = func(a, b string) int { return strings.Compare(a, b) }

// None of these is the bare ascending strings.Compare comparator; none is
// reported, and none is rewritten.
func negatives(ys []string, xs []int, bs [][]byte, base string) {
	// Swapped operands: strings.Compare(b, a) is a DESCENDING sort —
	// never slices.Sort.
	slices.SortFunc(ys, func(a, b string) int { return strings.Compare(b, a) })

	// Extra work in the body: not a bare comparator.
	slices.SortFunc(ys, func(a, b string) int {
		fmt.Println(a, b)
		return strings.Compare(a, b)
	})

	// A captured variable instead of the second parameter.
	slices.SortFunc(ys, func(a, b string) int { return strings.Compare(a, base) })

	// Arithmetic around the compare: not the bare call.
	slices.SortFunc(ys, func(a, b string) int { return -strings.Compare(a, b) })

	// A DEFINED string type needs conversions to reach strings.Compare —
	// the arguments are no longer the bare parameters, so the exact match
	// fails (the ordering would still agree, but the matcher stays exact).
	ns := []name{"b", "a"}
	slices.SortFunc(ns, func(a, b name) int { return strings.Compare(string(a), string(b)) })

	// cmp.Compare is a different package's comparator; PS3107/PS3006 own
	// that spelling.
	slices.SortFunc(xs, func(a, b int) int { return cmp.Compare(a, b) })

	// bytes.Compare orders []byte, not string; out of scope.
	slices.SortFunc(bs, bytes.Compare)

	// A named comparator value is opaque — only a fresh literal or
	// strings.Compare itself matches.
	slices.SortFunc(ys, cmpFn)

	fmt.Println(ys, xs, bs, ns)
}

// A shadowed strings resolves the Compare selector to a METHOD of the
// local object, not the stdlib package function — never matched.
func shadowedStrings(ys []string) {
	strings := fakeStrings{}
	slices.SortFunc(ys, func(a, b string) int { return strings.Compare(a, b) })
	fmt.Println(ys)
}
