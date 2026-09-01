//go:build amd64

package ps6077

import "math"

var keepMathImport = math.Pi

type ignoredFakeMath struct{}

func (ignoredFakeMath) Exp(value float64) float64 { return value }

func ExpSumF64(values []float64) float64 {
	return expSumAVX2(values)
}

func SigmoidF64(values []float64) float64

// A consumer in the mutually exclusive vector partition is not evidence for
// compounding a repair in the arm64 scalar partition.
func OtherPartitionSigmoidConsumer(values []float64) float64 {
	return SigmoidF64(values)
}

func ShadowedActiveMath(values []float64) float64 {
	return expVector4F64(values)
}

func ShadowedIgnoredMath(values []float64) float64 {
	math := ignoredFakeMath{}
	return math.Exp(values[0])
}

func SoftplusF64(values []float64) float64 {
	var sum float64
	for index := 0; index+3 < len(values); index += 4 {
		sum += values[index] + values[index+1] + values[index+2] + values[index+3]
	}
	return sum
}

func ArithmeticOnly(values []float64) float64 {
	return arithmeticAVX2(values)
}

func UnknownSibling(values []float64) float64 {
	return values[0]
}

func DifferentSignature(values []float32) float32 {
	return expVector4F32(values)
}

func DotImported(values []float64) float64 {
	return expVector4F64(values)
}

func Validated(values []float64) float64 {
	return expVector4F64(values)
}
