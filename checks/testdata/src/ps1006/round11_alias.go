package ps1006

type round11ClosureHolder struct {
	f func()
}

type round11ValuePointerBox struct{ p *int }

func (box round11ValuePointerBox) reset() { *box.p = 0 }

type round11PointerHolder struct{ pp **int }

type round11NestedClosureHolder struct{ inner round11ClosureHolder }

type round11NestedValuePointerBox struct{ inner round11ValuePointerBox }

func (box round11NestedValuePointerBox) reset() { box.inner.reset() }

type round11ScalarBox struct{ value int }

func (box round11ScalarBox) reset() { box.value = 0 }

type round11Resetter interface{ reset() }

func pointerToPointerRetargetRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			other := 1
			p := &other
			pp := &p
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			*pp = &base
			*p = 0
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0
		out[c+1] = s1
		out[c+2] = s2
		out[c+3] = s3
	}
}

func selectorClosureRetargetRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			holder := round11ClosureHolder{f: func() {}}
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			holder.f = func() { base = 0 }
			holder.f()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0
		out[c+1] = s1
		out[c+2] = s2
		out[c+3] = s3
	}
}

func valueReceiverAliasRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			box := round11ValuePointerBox{p: &base}
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			box.reset()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0
		out[c+1] = s1
		out[c+2] = s2
		out[c+3] = s3
	}
}

func triplePointerRetargetRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			other := 1
			p := &other
			pp := &p
			ppp := &pp
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			**ppp = &base
			*p = 0
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func conditionalPointerRetargetRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			other := 1
			p := &other
			pp := &p
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			if t&1 == 0 {
				*pp = &base
			}
			*p = 0
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func backedgePointerRetargetRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		base, other := 0, 1
		p := &other
		pp := &p
		for t := 0; t < taps; t++ {
			base = t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			*p = 0
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
			*pp = &base
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func aggregatePointerRetargetRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			other := 1
			p := &other
			holder := round11PointerHolder{pp: &p}
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			*holder.pp = &base
			*p = 0
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func assignedAggregatePointerRetargetRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			other := 1
			p := &other
			var holder round11PointerHolder
			holder.pp = &p
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			*holder.pp = &base
			*p = 0
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func methodValueAliasRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			box := round11ValuePointerBox{p: &base}
			mutate := box.reset
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			mutate()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func storedMethodValueAliasRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			box := round11ValuePointerBox{p: &base}
			holder := round11ClosureHolder{f: box.reset}
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			holder.f()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func interfaceValueReceiverAliasRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			var mutate round11Resetter = round11ValuePointerBox{p: &base}
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			mutate.reset()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func nestedClosureRetargetRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			holder := round11NestedClosureHolder{}
			holder.inner.f = func() { base = 0 }
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			holder.inner.f()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func storedClosureFieldRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			holder := round11ClosureHolder{f: func() { base = 0 }}
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			holder.f()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func copiedClosureFieldRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			holder := round11ClosureHolder{}
			holder.f = func() { base = 0 }
			copied := holder
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			copied.f()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func conditionalClosureFieldRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			holder := round11ClosureHolder{f: func() { base = 0 }}
			if t&1 == 0 {
				holder.f = func() {}
			}
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			holder.f()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func overwrittenClosureFieldRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			holder := round11ClosureHolder{f: func() { base = 0 }}
			holder.f = func() {}
			s0 += a[base+c] * w[t]
			holder.f()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func nestedValueReceiverAliasRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			box := round11NestedValuePointerBox{inner: round11ValuePointerBox{p: &base}}
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			box.reset()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func safeScalarValueReceiverRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			box := round11ScalarBox{value: base}
			s0 += a[base+c] * w[t]
			box.reset()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func unrelatedPointerReceiverRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base, other := t*channels, 1
			box := round11ValuePointerBox{p: &other}
			s0 += a[base+c] * w[t]
			box.reset()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}

func unrelatedClosureFieldRound11(a, w, out []float64, taps, channels int) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base, other := t*channels, 1
			holder := round11ClosureHolder{f: func() { other = 0; _ = other }}
			s0 += a[base+c] * w[t]
			holder.f()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
	}
}
