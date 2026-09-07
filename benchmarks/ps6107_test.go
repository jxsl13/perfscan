package benchmarks

import "testing"

const (
	ps6107Width            = 512
	ps6107RetainedElements = (64 << 10) / 4
)

var ps6107Sink float32

//go:noinline
func ps6107Fill(dst []float32, seed float32) {
	for index := range dst {
		dst[index] = seed + float32(index&15)
	}
}

//go:noinline
func ps6107ConsumeSynchronously(src []float32) float32 {
	var sum float32
	for _, value := range src {
		sum += value
	}
	return sum
}

//go:noinline
func ps6107Before(rows int, seed float32) float32 {
	staging := make([]float32, rows*ps6107Width)
	ps6107Fill(staging, seed)
	return ps6107ConsumeSynchronously(staging)
}

type ps6107Receiver struct {
	staging []float32
}

func (receiver *ps6107Receiver) stagingFor(need int) []float32 {
	// The original make path preserves non-nil zero-length slices and the
	// original invalid-size panic. Oversize calls never replace retained state.
	if need <= 0 || need > ps6107RetainedElements {
		return make([]float32, need)
	}
	if cap(receiver.staging) < need {
		receiver.staging = make([]float32, need)
	}
	return receiver.staging[:need:need]
}

//go:noinline
func (receiver *ps6107Receiver) after(rows int, seed float32) float32 {
	need := rows * ps6107Width
	staging := receiver.stagingFor(need)
	ps6107Fill(staging, seed)
	return ps6107ConsumeSynchronously(staging)
}

func (receiver *ps6107Receiver) release() {
	receiver.staging = nil
}

func BenchmarkPS6107_Before(b *testing.B) {
	rows := [...]int{4, 16, 8, 16}
	b.ReportAllocs()
	for index := range b.N {
		ps6107Sink = ps6107Before(rows[index&3], float32(index))
	}
}

func BenchmarkPS6107_AfterWarmedSteadyState(b *testing.B) {
	rows := [...]int{4, 16, 8, 16}
	receiver := new(ps6107Receiver)
	_ = receiver.after(16, 0) // warm the exact high-water allocation
	b.ReportAllocs()
	b.ResetTimer()
	for index := range b.N {
		ps6107Sink = receiver.after(rows[index&3], float32(index))
	}
}

func BenchmarkPS6107_OversizeFallbackBefore(b *testing.B) {
	b.ReportAllocs()
	for index := range b.N {
		ps6107Sink = ps6107Before(33, float32(index))
	}
}

func BenchmarkPS6107_OversizeFallbackAfter(b *testing.B) {
	receiver := new(ps6107Receiver)
	b.ReportAllocs()
	for index := range b.N {
		ps6107Sink = receiver.after(33, float32(index))
	}
}

func TestPS6107StagingMechanism(t *testing.T) {
	t.Parallel()

	t.Run("before-after-equivalence", func(t *testing.T) {
		t.Parallel()
		receiver := new(ps6107Receiver)
		for _, rows := range []int{0, 4, 16, 8, 16, 33} {
			for _, seed := range []float32{-3, 0, 7} {
				before := ps6107Before(rows, seed)
				after := receiver.after(rows, seed)
				if after != before {
					t.Fatalf("rows=%d seed=%g: after=%g, before=%g", rows, seed, after, before)
				}
			}
		}
	})

	t.Run("exact-growth-and-shrink", func(t *testing.T) {
		t.Parallel()
		receiver := new(ps6107Receiver)
		first := receiver.stagingFor(4 * ps6107Width)
		if len(first) != 4*ps6107Width || cap(first) != 4*ps6107Width || cap(receiver.staging) != 4*ps6107Width {
			t.Fatalf("first growth: len/cap=%d/%d retained=%d", len(first), cap(first), cap(receiver.staging))
		}
		grown := receiver.stagingFor(16 * ps6107Width)
		if len(grown) != 16*ps6107Width || cap(grown) != 16*ps6107Width || cap(receiver.staging) != 16*ps6107Width {
			t.Fatalf("second growth: len/cap=%d/%d retained=%d", len(grown), cap(grown), cap(receiver.staging))
		}
		shrunk := receiver.stagingFor(8 * ps6107Width)
		if len(shrunk) != 8*ps6107Width || cap(shrunk) != 8*ps6107Width || cap(receiver.staging) != 16*ps6107Width {
			t.Fatalf("shrink: len/cap=%d/%d retained=%d", len(shrunk), cap(shrunk), cap(receiver.staging))
		}
	})

	t.Run("zero-preserves-fresh-make-boundary", func(t *testing.T) {
		t.Parallel()
		receiver := new(ps6107Receiver)
		zero := receiver.stagingFor(0)
		if zero == nil {
			t.Fatal("zero-size fallback must preserve make([]float32, 0) non-nilness")
		}
		if receiver.staging != nil {
			t.Fatal("zero-size fallback must not create retained state")
		}
	})

	t.Run("oversize-does-not-replace-high-water", func(t *testing.T) {
		t.Parallel()
		receiver := new(ps6107Receiver)
		retained := receiver.stagingFor(16 * ps6107Width)
		retained[0] = 19
		oversize := receiver.stagingFor(33 * ps6107Width)
		if len(oversize) != 33*ps6107Width || cap(receiver.staging) != 16*ps6107Width || receiver.staging[0] != 19 {
			t.Fatalf("oversize changed retained state: oversize=%d retained=%d value=%g", len(oversize), cap(receiver.staging), receiver.staging[0])
		}
		oversize[0] = 23
		if receiver.staging[0] != 19 {
			t.Fatal("oversize fallback aliases retained staging")
		}
	})

	t.Run("lifecycle-clears-retained-state", func(t *testing.T) {
		t.Parallel()
		receiver := new(ps6107Receiver)
		_ = receiver.stagingFor(16 * ps6107Width)
		receiver.release()
		if receiver.staging != nil {
			t.Fatal("release did not clear retained staging")
		}
	})
}
