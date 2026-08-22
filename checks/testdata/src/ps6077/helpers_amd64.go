//go:build amd64

package ps6077

// External helper declarations keep the amd64 fixture type-correct while
// still modeling assembly/vector leaves for the cross-partition analyzer.
func portableAVX2(values []float64) float64
func expSumAVX2(values []float64) float64
func arithmeticAVX2(values []float64) float64
func expVector4F32(values []float32) float32
func expVector4F64(values []float64) float64
