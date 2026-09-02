//go:build ps6099never

package ps6099ignoredunknown

import "ps6099ignoredunknown/simdops"

func ApplyExpF64(dst []float64) {
	simdops.ExpF64(dst)
}
