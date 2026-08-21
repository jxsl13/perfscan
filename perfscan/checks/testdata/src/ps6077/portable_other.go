//go:build !arm64 && !amd64

package ps6077

import "math"

// This deliberately broad fallback is excluded even though amd64 has a
// vector sibling: unsupported architectures are not implementation gaps.
func PortableExp(values []float64) float64 {
	return math.Exp(values[0])
}
