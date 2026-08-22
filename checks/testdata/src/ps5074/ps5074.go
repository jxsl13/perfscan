package ps5074

import (
	"bytes"
	"maps"
	"slices"
	"strings"
)

func byteClone(b []byte) []byte {
	return bytes.Clone(bytes.Clone(b)) // want `bytes\.Clone is nested 2 times`
}

func stringClone(s string) string {
	return strings.Clone(strings.Clone(s)) // want `strings\.Clone is nested 2 times`
}

func sliceClone(s []int) []int {
	return slices.Clone(slices.Clone(s)) // want `slices\.Clone is nested 2 times`
}

func explicitGeneric(s []int) []int {
	return slices.Clone[[]int](slices.Clone[[]int](s)) // want `slices\.Clone is nested 2 times`
}

func mapClone(m map[string]int) map[string]int {
	return maps.Clone(maps.Clone(m)) // want `maps\.Clone is nested 2 times`
}

func triple(b []byte) []byte {
	return bytes.Clone(bytes.Clone(bytes.Clone(b))) // want `bytes\.Clone is nested 3 times`
}

func independentNested(b []byte) []byte {
	return bytes.Clone(bytes.Clone(append(bytes.Clone(bytes.Clone(b)), b...))) // want `bytes\.Clone is nested 2 times` `bytes\.Clone is nested 2 times`
}

// The diagnostic remains, but deleting this comment would be unsafe.
func commented(b []byte) []byte {
	return bytes.Clone( /* retain */ bytes.Clone(b)) // want `bytes\.Clone is nested 2 times`
}

// --- negatives ---

func single(b []byte) []byte { return bytes.Clone(b) }

func Clone[T any](v T) T { return v }

func userClone(b []byte) []byte { return Clone(Clone(b)) }

func converted(s string) []byte {
	return bytes.Clone([]byte(strings.Clone(s)))
}

type namedInts []int

// Removing the outer generic call would change the interface's dynamic type
// from []int to namedInts.
func mixedGenericResult(s namedInts) any {
	return slices.Clone[[]int](slices.Clone[namedInts](s))
}
