package benchmarks

import "testing"

var (
	ps6083Packed = uint32(0x00fac688)
	ps6083Input  = [8]float32{0.5, -1.25, 2, -4.5, 8, 0.125, -0.75, 3.5}
	ps6083Output [8]float32
	ps6083Sink   float32
)

//go:noinline
func ps6083Before(output, input *[8]float32, packed uint32) {
	for index := range 8 {
		output[index] = input[index] * float32(2*((packed>>(3*index))&7)+1)
	}
}

//go:noinline
func ps6083After(output, input *[8]float32, packed uint32) {
	lookup := [8]float32{1, 3, 5, 7, 9, 11, 13, 15}
	for index := range 8 {
		output[index] = input[index] * lookup[(packed>>(3*index))&7]
	}
}

func BenchmarkPS6083_Before(b *testing.B) {
	for range b.N {
		ps6083Before(&ps6083Output, &ps6083Input, ps6083Packed)
	}
	ps6083Sink = ps6083Output[len(ps6083Output)-1]
}

func BenchmarkPS6083_After(b *testing.B) {
	for range b.N {
		ps6083After(&ps6083Output, &ps6083Input, ps6083Packed)
	}
	ps6083Sink = ps6083Output[len(ps6083Output)-1]
}
