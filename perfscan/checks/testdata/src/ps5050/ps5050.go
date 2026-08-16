package ps5050

import (
	"bytes"
	"slices"
)

var _ = bytes.Equal

// A non-byte slices use keeps the slices import alive, so converting the
// byte-slice calls below never orphans it.
var keepSlices = slices.Contains([]int{1}, 2)

type ByteBuf []byte

type Nibble byte

// --- fixable: byte slice + byte operand ---

func plainIndex(b []byte, c byte) int {
	return slices.Index(b, c) // want `slices\.Index over a byte slice`
}

func plainContains(b []byte, c byte) bool {
	return slices.Contains(b, c) // want `slices\.Contains over a byte slice`
}

func negatedContains(b []byte, c byte) bool {
	return !slices.Contains(b, c) // want `slices\.Contains over a byte slice`
}

func comparisonContains(b []byte, c byte, flag bool) bool {
	return slices.Contains(b, c) == flag // want `slices\.Contains over a byte slice`
}

func andContains(b []byte, c byte, other bool) bool {
	return slices.Contains(b, c) && other // want `slices\.Contains over a byte slice`
}

// A named slice whose underlying type is []byte is assignable to IndexByte's
// []byte parameter.
func namedSlice(bb ByteBuf, c byte) int {
	return slices.Index(bb, c) // want `slices\.Index over a byte slice`
}

// --- advisory: reported, no fix ---

// go/defer requires a call expression; the comparison cannot be spliced there.
func goDeferContains(b []byte, c byte) {
	go slices.Contains(b, c) // want `slices\.Contains over a byte slice`
}

func commentInSelector(b []byte, c byte) int {
	return slices. /*keep*/ Index(b, c) // want `slices\.Index over a byte slice`
}

// --- negatives: silent ---

func nonByteSlice(xs []int, v int) int {
	return slices.Index(xs, v)
}

func namedElem(ns []Nibble, n Nibble) int {
	return slices.Index(ns, n)
}
