package benchmarks

import "testing"

const (
	ps6100Size  = 4000
	ps6100Steps = 300
)

var (
	ps6100Alpha = func() []float64 {
		values := make([]float64, ps6100Size)
		for index := range values {
			values[index] = float64(index%101) / 100
		}
		return values
	}()
	ps6100Labels = func() []int {
		values := make([]int, ps6100Size)
		for index := range values {
			if index&1 == 0 {
				values[index] = 1
			} else {
				values[index] = -1
			}
		}
		return values
	}()
	ps6100Sink int
)

func BenchmarkPS6100Before(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		alpha := append([]float64(nil), ps6100Alpha...)
		ps6100Sink = ps6100RunBefore(alpha)
	}
}

func BenchmarkPS6100After(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		alpha := append([]float64(nil), ps6100Alpha...)
		ps6100Sink = ps6100RunAfter(alpha)
	}
}

func TestPS6100BenchmarkEquivalence(t *testing.T) {
	t.Parallel()
	beforeAlpha := append([]float64(nil), ps6100Alpha...)
	afterAlpha := append([]float64(nil), ps6100Alpha...)
	before, after := ps6100RunBefore(beforeAlpha), ps6100RunAfter(afterAlpha)
	if before != after {
		t.Fatalf("membership totals differ: before=%d after=%d", before, after)
	}
	for index := range beforeAlpha {
		if beforeAlpha[index] != afterAlpha[index] {
			t.Fatalf("final alpha differs at %d: before=%v after=%v", index, beforeAlpha[index], afterAlpha[index])
		}
	}
}

func ps6100RunBefore(alpha []float64) int {
	hits := 0
	for step := 0; step < ps6100Steps; step++ {
		for index := range alpha {
			if ps6100Labels[index] > 0 && alpha[index] < 0.8 {
				hits++
			}
		}
		for index := range alpha {
			if ps6100Labels[index] < 0 && alpha[index] > 0.2 {
				hits++
			}
		}
		left, right := step*17%len(alpha), step*43%len(alpha)
		alpha[left] = float64((step*29)%101) / 100
		alpha[right] = float64((step*71)%101) / 100
	}
	return hits
}

func ps6100RunAfter(alpha []float64) int {
	status := make([]uint8, len(alpha))
	for index := range status {
		status[index] = ps6100Classify(alpha, index)
	}
	hits := 0
	for step := 0; step < ps6100Steps; step++ {
		for index := range status {
			if status[index]&1 != 0 {
				hits++
			}
		}
		for index := range status {
			if status[index]&2 != 0 {
				hits++
			}
		}
		left, right := step*17%len(alpha), step*43%len(alpha)
		alpha[left] = float64((step*29)%101) / 100
		alpha[right] = float64((step*71)%101) / 100
		status[left] = ps6100Classify(alpha, left)
		status[right] = ps6100Classify(alpha, right)
	}
	return hits
}

func ps6100Classify(alpha []float64, index int) uint8 {
	var status uint8
	if ps6100Labels[index] > 0 && alpha[index] < 0.8 {
		status |= 1
	}
	if ps6100Labels[index] < 0 && alpha[index] > 0.2 {
		status |= 2
	}
	return status
}
