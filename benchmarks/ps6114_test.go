package benchmarks

import (
	"slices"
	"testing"
)

func TestPS6114DocumentedWorkPair(t *testing.T) {
	t.Parallel()
	const batch, stride = 4, 5
	packed := make([]float64, batch*stride)
	for index := range packed {
		packed[index] = float64(index) - 7.5
	}

	beforeWork := 0
	transformed := make([]float64, len(packed))
	for index, value := range packed {
		transformed[index] = ps6114RowLocal(value)
		beforeWork++
	}
	before := make([]float64, batch)
	for index := range batch {
		before[index] = transformed[index*stride]
	}

	afterWork := 0
	after := make([]float64, batch)
	for index := range batch {
		after[index] = ps6114RowLocal(packed[index*stride])
		afterWork++
	}

	if !slices.Equal(before, after) {
		t.Fatalf("selected output differs: before=%v after=%v", before, after)
	}
	if beforeWork != batch*stride || afterWork != batch || beforeWork-afterWork != batch*(stride-1) {
		t.Fatalf("row-work accounting differs: before=%d after=%d", beforeWork, afterWork)
	}
	if ps6114Digest(before) != ps6114Digest(after) {
		t.Fatal("continuation digest differs")
	}
}

func ps6114RowLocal(value float64) float64 { return value*1.25 + 0.5 }

func ps6114Digest(values []float64) float64 {
	result := 0.0
	for index, value := range values {
		result += float64(index+1) * value
	}
	return result
}
