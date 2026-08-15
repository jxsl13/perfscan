package ps3023

import (
	"cmp"
	"slices"
)

// A type-parameter element is fixed too: cmp.Compare already forced T to
// satisfy cmp.Ordered, which is exactly slices.Compare's constraint, and the
// byte-identity argument is structural, independent of the instantiation. The
// cmp import survives here — cmp.Ordered in the signature still references it.
func generic[S ~[]T, T cmp.Ordered](a, b S) int {
	return slices.CompareFunc(a, b, cmp.Compare) // want `slices\.CompareFunc with a bare cmp\.Compare comparator`
}
