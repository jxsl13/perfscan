//go:build arm64

package ps6078

const (
	vexpF64Fast  = false // want `HIGH-PRIORITY architecture capability gap: vexpF64Fast is false under \[GOARCH=arm64; //go:build arm64\].*true under \[GOARCH=amd64; //go:build amd64\].*disables optimized branches in equalityExpF64, expF64, scaledExpF64, softplusF64.*leaves softplusF64NEON, vexpF64NEON.*reachable from exported operations ExpF64, PublicPipeline.*enabled same-dtype capabilities vsiluF64Fast.*local vector leaves siluF64NEON, softplusF64NEON, vexpF64NEON`
	vsiluF64Fast = true
	equalF32Fast = false
)

// An external declaration is local evidence that this architecture already
// has vector infrastructure for the same data type.
func siluF64NEON(dst, src []float64)

//perfscan:architecture-capability-validated intentionally unsupported.
const validatedF64Fast = false

const overlapFast = false
