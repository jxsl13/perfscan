package ps1006

type round12PointerBox struct{ p *int }

func (box round12PointerBox) resetIndirect() { *box.p = 0 }

type round12Resetter interface{ resetIndirect() }

func capturedIndirectMutationRound12(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			p := &base
			mutate := func() { *p = 0 }
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			mutate()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func capturedRetargetRound12(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base, other := t*channels, 1
			p := &other
			pp := &p
			mutate := func() { *pp = &base; *p = 0 }
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			mutate()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func interfaceCallableRound12(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			var box round12Resetter = round12PointerBox{p: &base}
			mutate := func() { box.resetIndirect() }
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			mutate()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func nestedCallableRound12(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			p := &base
			inner := func() { *p = 0 }
			mutate := func() { inner() }
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			mutate()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func switchFallthroughDependencyRound12(a []float64, rows, cols int, flag bool) float64 {
	var sum float64
	for c := 0; c < cols; c++ {
		for r := 0; r < rows; r++ {
			base := 0
			switch flag {
			case true:
				base = r * cols
				fallthrough
			default:
				sum += a[base+c] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			}
		}
	}
	return sum
}

// Without fallthrough, the assignment in the first case cannot reach the use.
func switchNoFallthroughControlRound12(a []float64, rows, cols int, flag bool) float64 {
	var sum float64
	for c := 0; c < cols; c++ {
		for r := 0; r < rows; r++ {
			base := 0
			switch flag {
			case true:
				base = r * cols
			default:
				sum += a[base+c]
			}
		}
	}
	return sum
}
