package ps3066

func fourPasses(state, k, q, delta []float64, decay float64) float64 {
	s := 0.0
	for i := range state { // want `4 consecutive loops over the same bound stream state once each; if the per-index work is independent across the loops, merge them so the buffer stays in cache \(a later loop needing ALL of an earlier one's output cannot merge\)`
		state[i] *= decay
	}
	for i := range state {
		s += state[i] * k[i]
	}
	for i := range state {
		state[i] += delta[i]
	}
	for i := range state {
		s += state[i] * q[i]
	}
	return s
}

func twoPasses(state []float64, decay float64) float64 {
	s := 0.0
	for i := range state {
		state[i] *= decay
	}
	for i := range state {
		s += state[i]
	}
	return s
}

func differentBounds(a, b, c []float64) float64 {
	s := 0.0
	for i := range a {
		s += a[i]
	}
	for i := range b {
		s += b[i]
	}
	for i := range c {
		s += c[i]
	}
	return s
}

func interrupted(state []float64, decay float64) float64 {
	s := 0.0
	for i := range state {
		state[i] *= decay
	}
	s = s + 1
	for i := range state {
		s += state[i]
	}
	for i := range state {
		state[i] = 0
	}
	return s
}

// Fix applies: single buffer, every element access is buf[i], no calls, no
// cross-loop scalar flow.
func scaleShiftSquare(buf []float64) {
	for i := range buf { // want `3 consecutive loops over the same bound stream buf once each; if the per-index work is independent across the loops, merge them so the buffer stays in cache \(a later loop needing ALL of an earlier one's output cannot merge\)`
		buf[i] *= 2
	}
	for i := range buf {
		buf[i]++
	}
	for i := range buf {
		buf[i] = buf[i] * buf[i]
	}
}

// Fix applies: read-only passes whose accumulators are confined to their own
// loop.
func threeMoments(a []float64) (float64, float64, float64) {
	m1, m2, m3 := 0.0, 0.0, 0.0
	for i := range a { // want `3 consecutive loops over the same bound stream a once each; if the per-index work is independent across the loops, merge them so the buffer stays in cache \(a later loop needing ALL of an earlier one's output cannot merge\)`
		m1 += a[i]
	}
	for i := range a {
		m2 += a[i] * a[i]
	}
	for i := range a {
		m3 += a[i] * a[i] * a[i]
	}
	return m1, m2, m3
}

// No fix: m is written by the first loop and read by the second, so a merged
// loop would divide by partial sums.
func scalarCarried(buf []float64) float64 {
	m := 0.0
	for i := range buf { // want `3 consecutive loops over the same bound stream buf once each; if the per-index work is independent across the loops, merge them so the buffer stays in cache \(a later loop needing ALL of an earlier one's output cannot merge\)`
		m += buf[i]
	}
	for i := range buf {
		buf[i] /= m
	}
	for i := range buf {
		buf[i] *= 2
	}
	return m
}

// No fix: buf[i-1] is a cross-index read, so merging would see half-updated
// neighbors.
func crossIndex(buf []float64) {
	for i := range buf { // want `3 consecutive loops over the same bound stream buf once each; if the per-index work is independent across the loops, merge them so the buffer stays in cache \(a later loop needing ALL of an earlier one's output cannot merge\)`
		buf[i] *= 2
	}
	for i := range buf {
		buf[i] += 1
	}
	for i := range buf {
		if i > 0 {
			buf[i] += buf[i-1]
		}
	}
}

// No fix: a non-builtin call makes cross-loop independence unprovable.
func callInBody(buf []float64) {
	for i := range buf { // want `3 consecutive loops over the same bound stream buf once each; if the per-index work is independent across the loops, merge them so the buffer stays in cache \(a later loop needing ALL of an earlier one's output cannot merge\)`
		buf[i] = clamp01(buf[i])
	}
	for i := range buf {
		buf[i] *= 2
	}
	for i := range buf {
		buf[i] += 1
	}
}

// No fix: the middle loop reads a SECOND buffer k[i]. Distinct slices can alias
// each other at an offset, so fusing a write of buf[i] with a read of k[i] is
// not provably independent — the fix declines and the diagnostic stays advisory
// (isolates the second-buffer axis from cross-index/scalar/call declines).
func secondBuffer(buf, k []float64) {
	for i := range buf { // want `3 consecutive loops over the same bound stream buf once each; if the per-index work is independent across the loops, merge them so the buffer stays in cache \(a later loop needing ALL of an earlier one's output cannot merge\)`
		buf[i] *= 2
	}
	for i := range buf {
		buf[i] += k[i]
	}
	for i := range buf {
		buf[i] *= 3
	}
}

func clamp01(x float64) float64 {
	if x > 1 {
		return 1
	}
	return x
}
