// The dynamic end-to-end test copies the actual owner operation/benchmark and
// supplies controlled raw evidence. No unsigned fake JSON attestation fires this
// advisory; this plain analysistest fixture covers its opt-in silent boundary.
package ps6131

const measuredParallelThreshold = 16

func serial(out, in []float32) {
	for i := range out {
		out[i] = -in[i]
	}
}
func worker(n int, body func(int, int)) { body(0, n) }
func parallel(out, in []float32) {
	worker(len(out), func(lo, hi int) { serial(out[lo:hi], in[lo:hi]) })
}

func dispatch(in []float32) []float32 {
	out := make([]float32, len(in))
	if len(out) < measuredParallelThreshold {
		serial(out, in)
	} else {
		parallel(out, in)
	}
	return out
}
