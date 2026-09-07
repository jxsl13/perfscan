package ps6095

import "runtime"

type ps6095LifetimeWitness struct{}

var ps6095LifetimeFinalized bool

//go:noinline
func quotientWithWitness(numerator, denominator float64, witness *ps6095LifetimeWitness) float64 {
	return numerator / denominator
}

// Negative: caching the helper call before the loop would move the witness's
// last use and allow its finalizer to run while the original program still
// passes it on every iteration. Helpers with reference or unused parameters
// therefore receive no purity summary and no automatic lifetime-shortening
// fix.
func helperPointerLifetime(output []float64, numerator, denominator float64) {
	witness := new(ps6095LifetimeWitness)
	runtime.SetFinalizer(witness, func(*ps6095LifetimeWitness) {
		ps6095LifetimeFinalized = true
	})
	for index := range output {
		runtime.GC()
		output[index] = quotientWithWitness(numerator, denominator, witness)
	}
}
