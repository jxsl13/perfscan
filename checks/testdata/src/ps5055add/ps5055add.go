package ps5055add

import "slices"

var keep = slices.Contains([]int{1}, 2)

func addBytes(a, b []byte) int {
	return slices.Compare(a, b) // want `slices\.Compare over byte slices runs the generic element loop`
}
