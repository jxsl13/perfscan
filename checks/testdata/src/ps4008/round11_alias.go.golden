package ps4008

type round11ClosureHolder struct {
	f func()
}

type round11ValuePointerBox struct{ p *int }

func (box round11ValuePointerBox) reset() { *box.p = 0 }

func pointerToPointerRetargetRound11(a, b [][]float64, out []float64, rows, cols int) {
	for i := 0; i < rows; i++ {
		for j := 0; j+3 < cols; j += 4 {
			var s0, s1, s2, s3 float64
			for k := 0; k < rows; k++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := k
				other := 0
				p := &other
				pp := &p
				s0 += a[i][base] * b[base][j]
				*pp = &base
				*p = 0
				s1 += a[i][base] * b[base][j+1]
				s2 += a[i][base] * b[base][j+2]
				s3 += a[i][base] * b[base][j+3]
			}
			out[j] = s0 + s1 + s2 + s3
		}
	}
}

func selectorClosureRetargetRound11(a, b [][]float64, out []float64, rows, cols int) {
	for i := 0; i < rows; i++ {
		for j := 0; j+3 < cols; j += 4 {
			var s0, s1, s2, s3 float64
			for k := 0; k < rows; k++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := k
				holder := round11ClosureHolder{f: func() {}}
				s0 += a[i][base] * b[base][j]
				holder.f = func() { base = 0 }
				holder.f()
				s1 += a[i][base] * b[base][j+1]
				s2 += a[i][base] * b[base][j+2]
				s3 += a[i][base] * b[base][j+3]
			}
			out[j] = s0 + s1 + s2 + s3
		}
	}
}

func valueReceiverAliasRound11(a, b [][]float64, out []float64, rows, cols int) {
	for i := 0; i < rows; i++ {
		for j := 0; j+3 < cols; j += 4 {
			var s0, s1, s2, s3 float64
			for k := 0; k < rows; k++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := k
				box := round11ValuePointerBox{p: &base}
				s0 += a[i][base] * b[base][j]
				box.reset()
				s1 += a[i][base] * b[base][j+1]
				s2 += a[i][base] * b[base][j+2]
				s3 += a[i][base] * b[base][j+3]
			}
			out[j] = s0 + s1 + s2 + s3
		}
	}
}
