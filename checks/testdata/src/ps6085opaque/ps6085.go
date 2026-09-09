package ps6085opaque

import "sync"

var (
	opaqueGrid [4][8]float32
	once       sync.Once
)

// A declaration without a Go body is opaque to source analysis.
func opaqueCallback()

func init() {
	once.Do(opaqueCallback)
	opaqueGrid[0][0] = 1
}

func opaqueCandidate(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	var sum float32
	for lane := range 8 {
		sum += scale * (opaqueGrid[rowIndex&3][lane] + delta)
	}
	return sum
}
