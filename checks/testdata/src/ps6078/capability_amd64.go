//go:build amd64

package ps6078

const (
	vexpF64Fast  = true
	vsiluF64Fast = true
	equalF32Fast = false
	// Keep the native package type-correct without creating a literal
	// capability variant for the overlap-only analyzer fixture.
	overlapFast = false && true
)

const validatedF64Fast = true
