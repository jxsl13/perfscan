package benchmarks

import (
	"math"
	"testing"
)

const ps6085Rows = 2048

var (
	ps6085Base     [ps6085Rows][8]float32
	ps6085Expanded [2][ps6085Rows][8]float32
	ps6085Output   [8]float32
	ps6085Digest   float32
)

func init() {
	for row := range ps6085Base {
		for lane := range ps6085Base[row] {
			value := float32((row*17+lane*5)%257-128) * 0.03125
			ps6085Base[row][lane] = value
			ps6085Expanded[0][row][lane] = value + float32(0.125)
			ps6085Expanded[1][row][lane] = value + float32(-0.125)
		}
	}
}

//go:noinline
func ps6085Before(output *[8]float32, row int, state uint32, scale float32) {
	delta := float32(0.125)
	if state&1 != 0 {
		delta = float32(-0.125)
	}
	lookup := &ps6085Base[row&(ps6085Rows-1)]
	for lane := range 8 {
		output[lane] = scale * (lookup[lane] + delta)
	}
}

//go:noinline
func ps6085After(output *[8]float32, row int, state uint32, scale float32) {
	lookup := &ps6085Expanded[state&1][row&(ps6085Rows-1)]
	for lane := range 8 {
		output[lane] = scale * lookup[lane]
	}
}

func TestPS6085StateExpandedTablePreservesFloat32Operations(t *testing.T) {
	t.Parallel()
	for row := range ps6085Rows {
		for state := range uint32(2) {
			for _, scale := range []float32{-3.5, -0, 0.25, 1, 7.75} {
				var before, after [8]float32
				ps6085Before(&before, row, state, scale)
				ps6085After(&after, row, state, scale)
				for lane := range before {
					if math.Float32bits(before[lane]) != math.Float32bits(after[lane]) {
						t.Fatalf("row=%d state=%d scale=%v lane=%d: before=%08x after=%08x", row, state, scale, lane, math.Float32bits(before[lane]), math.Float32bits(after[lane]))
					}
				}
			}
		}
	}
}

func BenchmarkPS6085_Before(b *testing.B) {
	var output [8]float32
	for iteration := range b.N {
		row := iteration >> 1
		state := uint32(iteration & 1)
		ps6085Before(&output, row, state, 0.75)
	}
	ps6085Output = output
	ps6085Digest = output[len(output)-1]
}

func BenchmarkPS6085_After(b *testing.B) {
	var output [8]float32
	for iteration := range b.N {
		row := iteration >> 1
		state := uint32(iteration & 1)
		ps6085After(&output, row, state, 0.75)
	}
	ps6085Output = output
	ps6085Digest = output[len(output)-1]
}
