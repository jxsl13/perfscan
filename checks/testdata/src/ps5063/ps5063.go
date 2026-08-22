package ps5063

import "slices"

type ID int

// --- POSITIVES ---

func eqInt(a, b []int) bool {
	return slices.Compare(a, b) == 0 // want `slices\.Compare\(\.\.\.\) == 0 tests equality only`
}

func neqString(a, b []string) bool {
	return slices.Compare(a, b) != 0 // want `slices\.Compare\(\.\.\.\) != 0 tests equality only`
}

// Zero on the left.
func zeroLeft(a, b []int64) bool {
	return 0 == slices.Compare(a, b) // want `slices\.Compare\(\.\.\.\) == 0 tests equality only`
}

func zeroLeftNeq(a, b []int) bool {
	return 0 != slices.Compare(a, b) // want `slices\.Compare\(\.\.\.\) != 0 tests equality only`
}

// A named integer element type is still ordered with no NaN.
func namedElem(a, b []ID) bool {
	return slices.Compare(a, b) == 0 // want `slices\.Compare\(\.\.\.\) == 0 tests equality only`
}

// --- NEGATIVES: silent ---

// Ordering, not equality.
func ordering(a, b []int) bool {
	return slices.Compare(a, b) < 0
}

// Compared against a non-zero literal.
func nonZero(a, b []int) bool {
	return slices.Compare(a, b) == 1
}

// Float elements: cmp.Compare orders two NaNs as equal, == does not.
func floatElem(a, b []float64) bool {
	return slices.Compare(a, b) == 0
}

// Byte elements are PS5055's / PS5101's domain.
func byteElem(a, b []byte) bool {
	return slices.Compare(a, b) == 0
}
