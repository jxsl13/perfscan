package ps5055

import (
	"bytes"
	"slices"
)

var _ = bytes.Contains

// A non-byte slices use keeps the slices import alive, so the byte-slice
// rewrites below never orphan it.
var keepSlices = slices.Contains([]int{1}, 2)

type ByteBuf []byte

type Nibble byte

// --- fixable ---

func equalBytes(a, b []byte) bool {
	return slices.Equal(a, b) // want `slices\.Equal over byte slices runs the generic element loop`
}

func compareBytes(a, b []byte) int {
	return slices.Compare(a, b) // want `slices\.Compare over byte slices runs the generic element loop`
}

// A named slice whose underlying type is []byte qualifies.
func namedSlice(a, b ByteBuf) bool {
	return slices.Equal(a, b) // want `slices\.Equal over byte slices runs the generic element loop`
}

// --- advisory: reported, no fix ---

func commentInSelector(a, b []byte) bool {
	return slices. /*keep*/ Equal(a, b) // want `slices\.Equal over byte slices runs the generic element loop`
}

// --- negatives: silent ---

func nonByteSlice(a, b []int) bool {
	return slices.Equal(a, b)
}

func namedElem(a, b []Nibble) bool {
	return slices.Equal(a, b)
}

// A different slices function.
func other(a, b []byte) bool {
	return slices.ContainsFunc(a, func(x byte) bool { return x == b[0] })
}
