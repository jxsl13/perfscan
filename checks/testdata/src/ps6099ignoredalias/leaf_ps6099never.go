//go:build ps6099never

package ps6099ignoredalias

import "ps6099ignoredalias/simdops"

type Scalar = float64
type Band []Scalar

func ApplyExp(dst Band) {
	simdops.ExpF64(dst)
}
