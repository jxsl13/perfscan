package ps1006recursion

type state struct{ value int }

func (s *state) direct() { s.direct() }

func (s *state) left() { s.right() }

func (s *state) right() { s.left() }

func (s *state) readOnly() { _ = s.value }

func (s *state) write() { s.value++ }

func directRecursiveReceiver(a, w []float64, taps, channels int, s *state) float64 {
	var sum float64
	for c := 0; c < channels; c++ {
		for i := 0; i < taps; i++ {
			s.direct()
			sum += a[i*channels+c] * w[i] // want `the inner loop variable is the multiplied`
		}
	}
	return sum
}

func mutualRecursiveReceiver(a, w []float64, taps, channels int, s *state) float64 {
	var sum float64
	for c := 0; c < channels; c++ {
		for i := 0; i < taps; i++ {
			s.left()
			sum += a[i*channels+c] * w[i] // want `the inner loop variable is the multiplied`
		}
	}
	return sum
}

func nonrecursiveReceiver(a, w []float64, taps, channels int, s *state) float64 {
	var sum float64
	for c := 0; c < channels; c++ {
		for i := 0; i < taps; i++ {
			s.readOnly()
			sum += a[i*channels+c] * w[i] // want `the inner loop variable is the multiplied`
		}
	}
	return sum
}
