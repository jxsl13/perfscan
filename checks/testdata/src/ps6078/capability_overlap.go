//go:build arm64 && goexperiment.simd

package ps6078

// This true partition overlaps the broader arm64 false partition, so it is
// not evidence of an architecture capability difference.
const overlapFast = true
