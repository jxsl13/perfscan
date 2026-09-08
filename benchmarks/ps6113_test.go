package benchmarks

import (
	"math"
	"slices"
	"testing"
)

type ps6113Recorder struct {
	events int
	work   int
}

func (recorder *ps6113Recorder) project(source, temporary []float32) {
	recorder.events++
	for index := range source {
		temporary[index] = source[index] * 1.25
		recorder.work++
	}
}

func (recorder *ps6113Recorder) residual(destination, temporary []float32) {
	recorder.events++
	for index := range destination {
		destination[index] += temporary[index]
		recorder.work++
	}
}

func (recorder *ps6113Recorder) projectAdd(source, destination []float32) {
	recorder.events++
	for index := range source {
		projected := source[index] * 1.25
		destination[index] += projected
		recorder.work += 2
	}
}

func TestPS6113DocumentedWorkPair(t *testing.T) {
	t.Parallel()
	source := []float32{1, -2, 3.5, 4, -5.25, 6}
	initial := []float32{10, 20, -30, 40, 50, -60}
	beforeOutput := slices.Clone(initial)
	afterOutput := slices.Clone(initial)
	temporary := make([]float32, len(source))
	before := &ps6113Recorder{}
	after := &ps6113Recorder{}

	before.project(source, temporary)
	before.residual(beforeOutput, temporary)
	after.projectAdd(source, afterOutput)

	if !slices.Equal(beforeOutput, afterOutput) {
		t.Fatalf("work-pair output differs: before=%v after=%v", beforeOutput, afterOutput)
	}
	if before.work != after.work || before.events != 2 || after.events != 1 {
		t.Fatalf("work/event accounting differs: before=%+v after=%+v", before, after)
	}
	if ps6113Digest(beforeOutput) != ps6113Digest(afterOutput) {
		t.Fatal("continuation digest differs")
	}
}

func ps6113Digest(values []float32) uint64 {
	var digest uint64 = 1469598103934665603
	for _, value := range values {
		digest ^= uint64(math.Float32bits(value))
		digest *= 1099511628211
	}
	return digest
}
