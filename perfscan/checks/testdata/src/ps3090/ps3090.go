package ps3090

import "sync"

// Vocabulary for this fixture: fanOutHelpers = {parallelFor}.

func parallelFor(n int, body func(lo, hi int)) { body(0, n) }

func notFanout(n int, body func(lo, hi int)) { body(0, n) }

func f(x float64) float64 { return x * 2 }

// --- POSITIVES ---

// Captures the dst + src slices (payload escape).
func SiLU(dst, src []float64) {
	parallelFor(len(dst), func(lo, hi int) { // want `closure passed to fan-out helper parallelFor captures`
		for i := lo; i < hi; i++ {
			dst[i] = f(src[i])
		}
	})
}

// Captures a call-local barrier pointer (*sync.Mutex) plus a slice.
func WithBarrier(dst []float64) {
	mu := &sync.Mutex{}
	parallelFor(len(dst), func(lo, hi int) { // want `closure passed to fan-out helper parallelFor captures`
		mu.Lock()
		for i := lo; i < hi; i++ {
			dst[i] = 0
		}
		mu.Unlock()
	})
}

// --- GUARDS: none reported ---

// A non-capturing closure (uses only its own parameters and locals).
func NoCapture(n int) {
	parallelFor(n, func(lo, hi int) {
		s := lo + hi
		_ = s
	})
}

// Captures only a value-typed variable (an int bound), not a reference.
func ValueCaptureOnly(n int) {
	bound := n * 2
	parallelFor(n, func(lo, hi int) {
		if lo < bound {
			_ = hi
		}
	})
}

// Captures a package-level slice: shared state, not a per-call escape.
var globalBuf []float64

func UsesGlobal(n int) {
	parallelFor(n, func(lo, hi int) {
		for i := lo; i < hi; i++ {
			globalBuf[i] = 0
		}
	})
}

// The closure is passed to a function that is NOT a fan-out helper.
func NotAFanout(dst []float64) {
	notFanout(len(dst), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			dst[i] = 0
		}
	})
}

// The only reference-typed variable is the closure's OWN local, not a capture.
func OwnLocal(n int) {
	parallelFor(n, func(lo, hi int) {
		local := make([]int, hi-lo)
		for i := range local {
			local[i] = i
		}
		_ = local
	})
}
