package ps4008

type round12PointerBox struct{ p *int }

func (box round12PointerBox) resetIndirect() { *box.p = 0 }

type round12Resetter interface{ resetIndirect() }

func dotCapturedIndirectRound12(a, b [][]float64, out []float64, rows, cols int) {
	for i := 0; i < rows; i++ {
		for j := 0; j+3 < cols; j += 4 {
			var s0, s1, s2, s3 float64
			for k := 0; k < rows; k++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := k
				p := &base
				mutate := func() { *p = 0 }
				s0 += a[i][base] * b[base][j]
				mutate()
				s1 += a[i][base] * b[base][j+1]
				s2 += a[i][base] * b[base][j+2]
				s3 += a[i][base] * b[base][j+3]
			}
			out[j] = s0 + s1 + s2 + s3
		}
	}
}

func dotCapturedRetargetRound12(a, b [][]float64, out []float64, rows, cols int) {
	for i := 0; i < rows; i++ {
		for j := 0; j+3 < cols; j += 4 {
			var s0, s1, s2, s3 float64
			for k := 0; k < rows; k++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base, other := k, 0
				p := &other
				pp := &p
				mutate := func() { *pp = &base; *p = 0 }
				s0 += a[i][base] * b[base][j]
				mutate()
				s1 += a[i][base] * b[base][j+1]
				s2 += a[i][base] * b[base][j+2]
				s3 += a[i][base] * b[base][j+3]
			}
			out[j] = s0 + s1 + s2 + s3
		}
	}
}

func dotInterfaceCallableRound12(a, b [][]float64, out []float64, rows, cols int) {
	for i := 0; i < rows; i++ {
		for j := 0; j+3 < cols; j += 4 {
			var s0, s1, s2, s3 float64
			for k := 0; k < rows; k++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := k
				var box round12Resetter = round12PointerBox{p: &base}
				mutate := func() { box.resetIndirect() }
				s0 += a[i][base] * b[base][j]
				mutate()
				s1 += a[i][base] * b[base][j+1]
				s2 += a[i][base] * b[base][j+2]
				s3 += a[i][base] * b[base][j+3]
			}
			out[j] = s0 + s1 + s2 + s3
		}
	}
}

func dotNestedCallableRound12(a, b [][]float64, out []float64, rows, cols int) {
	for i := 0; i < rows; i++ {
		for j := 0; j+3 < cols; j += 4 {
			var s0, s1, s2, s3 float64
			for k := 0; k < rows; k++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := k
				p := &base
				inner := func() { *p = 0 }
				mutate := func() { inner() }
				s0 += a[i][base] * b[base][j]
				mutate()
				s1 += a[i][base] * b[base][j+1]
				s2 += a[i][base] * b[base][j+2]
				s3 += a[i][base] * b[base][j+3]
			}
			out[j] = s0 + s1 + s2 + s3
		}
	}
}

// A closure that only updates an unrelated scalar must not hide a complete
// four-lane tile.
func dotValueOnlyClosureControlRound12(a, b [][]float64, out []float64, rows, cols int) {
	for i := 0; i < rows; i++ {
		for j := 0; j+3 < cols; j += 4 {
			var s0, s1, s2, s3 float64
			for k := 0; k < rows; k++ {
				base, unrelated := k, 0
				mutate := func() { unrelated++ }
				s0 += a[i][base] * b[base][j]
				mutate()
				s1 += a[i][base] * b[base][j+1]
				s2 += a[i][base] * b[base][j+2]
				s3 += a[i][base] * b[base][j+3]
				_ = unrelated
			}
			out[j] = s0 + s1 + s2 + s3
		}
	}
}

func dotSwitchFallthroughDependencyRound12(a, b [][]float64, out []float64, rows, cols int, flag bool) {
	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			var sum float64
			for k := 0; k < rows; k++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := 0
				switch flag {
				case true:
					base = k
					fallthrough
				default:
					sum += a[i][base] * b[base][j]
				}
			}
			out[j] = sum
		}
	}
}

func dotSwitchNoFallthroughControlRound12(a, b [][]float64, out []float64, rows, cols int, flag bool) {
	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			var sum float64
			for k := 0; k < rows; k++ {
				base := 0
				switch flag {
				case true:
					base = k
				default:
					sum += a[i][base] * b[base][j]
				}
			}
			out[j] = sum
		}
	}
}
