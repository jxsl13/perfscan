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
