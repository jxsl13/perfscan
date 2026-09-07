package benchmarks

import "testing"

var (
	ps6095Input = func() [256]float64 {
		var values [256]float64
		for index := range values {
			values[index] = float64(index%31-15) / 7
		}
		return values
	}()
	ps6095Output [256]float64
)

//go:noinline
func ps6095Before(output, input []float64, weight, denominator float64) {
	for index := range output {
		output[index] = input[index] * (weight / denominator)
	}
}

//go:noinline
func ps6095After(output, input []float64, weight, denominator float64) {
	quotient := weight / denominator
	for index := range output {
		output[index] = input[index] * quotient
	}
}

func BenchmarkPS6095_Before(b *testing.B) {
	for range b.N {
		ps6095Before(ps6095Output[:], ps6095Input[:], 3.25, 0.875)
	}
	sinkF = ps6095Output[len(ps6095Output)-1]
}

func BenchmarkPS6095_After(b *testing.B) {
	for range b.N {
		ps6095After(ps6095Output[:], ps6095Input[:], 3.25, 0.875)
	}
	sinkF = ps6095Output[len(ps6095Output)-1]
}
