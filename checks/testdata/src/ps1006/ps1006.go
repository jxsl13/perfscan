package ps1006

func columnSums(a []float64, rows, cols int, out []float64) {
	for c := 0; c < cols; c++ {
		s := 0.0
		for r := 0; r < rows; r++ {
			s += a[r*cols+c] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

// An int accumulator: the fix reuses its type for the scratch slice.
func columnCounts(a []int, rows, cols int, out []int) {
	for c := 0; c < cols; c++ {
		s := 0
		for r := 0; r < rows; r++ {
			s += a[r*cols+c] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

// Fixed arrays and constant bounds prove both non-aliasing and absence of
// bounds panics, so this canonical case remains safely auto-fixable.
func fixedArrayColumnSums(a [12]float64) [4]float64 {
	var out [4]float64
	for c := 0; c < 4; c++ {
		s := 0.0
		for r := 0; r < 3; r++ {
			s += a[r*4+c] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
	return out
}

// Reported but NOT auto-fixed: the final write is not `out[c] = s`, so the
// body is wider than the canonical shape.
func columnSumsScaled(a []float64, rows, cols int, out []float64, scale float64) {
	for c := 0; c < cols; c++ {
		s := 0.0
		for r := 0; r < rows; r++ {
			s += a[r*cols+c] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s * scale
	}
}

// Reported but NOT auto-fixed: a non-zero accumulator seed — make()'s zero
// value would not reproduce it.
func columnSumsBias(a, bias []float64, rows, cols int, out []float64) {
	for c := 0; c < cols; c++ {
		s := bias[c]
		for r := 0; r < rows; r++ {
			s += a[r*cols+c] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

// The inner variable is the additive (contiguous) part: already row-major.
func rowSums(a []float64, rows, cols int, out []float64) {
	for r := 0; r < rows; r++ {
		s := 0.0
		for c := 0; c < cols; c++ {
			s += a[r*cols+c]
		}
		out[r] = s
	}
}

// Not a reduction (writes per inner element): interchange changes nothing
// to accumulate, silent.
func scatter(a, b []float64, rows, cols int) {
	for c := 0; c < cols; c++ {
		for r := 0; r < rows; r++ {
			b[r*cols+c] = a[r*cols+c] * 2
		}
	}
}

// Allocation-free four-output register tile: the tap loop reads adjacent
// channels for each tap and keeps four independent ascending reductions.
func registerTile(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t]
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

// Subtraction is the same independent four-output register-tile shape as
// addition; the main PS1006 reduction detector accepts both compound forms.
func subtractRegisterTile(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 -= a[base+c] * w[t]
			s1 -= a[base+c+1] * w[t]
			s2 -= a[base+c+2] * w[t]
			s3 -= a[base+c+3] * w[t]
		}
		out[c] = s0
		out[c+1] = s1
		out[c+2] = s2
		out[c+3] = s3
	}
}

// The scalar tail is paired with the preceding four-output tile; it should not
// re-open the finding for the already tiled algorithm.
func registerTileWithScalarTail(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t]
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0
		out[c+1] = s1
		out[c+2] = s2
		out[c+3] = s3
	}
	for c := channels - channels%4; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			s += a[base+c] * w[t]
		}
		out[c] = s
	}
}

// Three accumulators are still a strided inner reduction; do not suppress an
// arbitrary small bundle of += statements.
func threeWayRegisterTileStillStrided(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c += 3 {
		var s0, s1, s2 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			if c+1 < channels {
				s1 += a[base+c+1] * w[t]
			}
			if c+2 < channels {
				s2 += a[base+c+2] * w[t]
			}
		}
		out[c] = s0 + s1 + s2
	}
}

// Tap-varying guards do not prove an output-register tile.
func tapVaryingGuardTileStillStrided(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			if t > 0 {
				s1 += a[base+c+1] * w[t]
				s2 += a[base+c+2] * w[t]
				s3 += a[base+c+3] * w[t]
			}
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// A second complete loop is not the scalar tail partition for the preceding
// tile, even when it uses the same body shape.
func registerTileThenCompleteLoopStillStrided(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t]
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0
		out[c+1] = s1
		out[c+2] = s2
		out[c+3] = s3
	}
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] += s
	}
}

// Four accumulators over the same output offset are not an output-register tile.
func sameOffsetAccumulatorsStillStrided(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			s1 += a[base+c] * w[t]
			s2 += a[base+c] * w[t]
			s3 += a[base+c] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Four adjacent offsets from different arrays are not one source tile.
func differentArrayAccumulatorsStillStrided(a, b []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			s1 += b[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += b[base+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Reassigning a derived stride base before use removes the strided proof.
func derivedStrideReassignedBeforeUseSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			base = 0
			s += a[base+c] * w[t]
		}
		out[c] = s
	}
}

// Reassigning after the access does not invalidate the already-seen strided use.
func derivedStrideReassignedAfterUseReports(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			base = 0
		}
		out[c] = s
	}
}

// A shadowed c with the same spelling as the outer loop variable is not the
// outer loop object, so typed matching must keep this silent.
func shadowedOuterNameSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			{
				c := t
				s += a[base+c] * w[t]
			}
		}
		out[c] = s
	}
}

// Split exclusive branches do not add up to a guaranteed four-output tile.
func exclusiveBranchAccumulatorsStillStrided(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			if flag {
				s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
				s1 += a[base+c+1] * w[t]
			} else {
				s2 += a[base+c+2] * w[t]
				s3 += a[base+c+3] * w[t]
			}
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// A full tile in only one branch is not guaranteed for every tap iteration.
func fullTileOnlyInOneBranchStillStrided(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			if flag {
				s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
				s1 += a[base+c+1] * w[t]
				s2 += a[base+c+2] * w[t]
				s3 += a[base+c+3] * w[t]
			}
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Both branches contain the same complete four-output tile, so every path keeps
// the allocation-free register-tile shape.
func guaranteedBranchTileSilent(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			if flag {
				s0 += a[base+c] * w[t]
				s1 += a[base+c+1] * w[t]
				s2 += a[base+c+2] * w[t]
				s3 += a[base+c+3] * w[t]
			} else {
				s0 += a[base+c] * w[t]
				s1 += a[base+c+1] * w[t]
				s2 += a[base+c+2] * w[t]
				s3 += a[base+c+3] * w[t]
			}
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

func earlyContinueBeforeTileStillStrided(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			if flag {
				continue
			}
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

func earlyBreakBeforeTileStillStrided(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			if flag {
				break
			}
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

func earlyReturnBeforeTileStillStrided(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			if flag {
				return
			}
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

func earlyPanicBeforeTileStillStrided(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			if flag {
				panic("skip tile")
			}
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

func earlyGotoBeforeTileStillStrided(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			if flag {
				goto done
			}
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
done:
}

func switchContinueBeforeTileStillStrided(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			switch {
			case flag:
				continue
			}
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

func nestedLoopContinueBeforeTileSilent(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			for probe := 0; probe < 1; probe++ {
				if flag {
					continue
				}
			}
			s0 += a[base+c] * w[t]
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// A sibling inner loop with a valid register tile does not suppress this later
// serial strided reduction.
func siblingTileDoesNotSuppressLaterInner(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t]
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s0 + s1 + s2 + s3 + s
	}
}

// Four adjacent reads into the same accumulator are still one serial chain.
func sameAccumulatorFourOffsetsStillStrided(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			s += a[base+c+1] * w[t]
			s += a[base+c+2] * w[t]
			s += a[base+c+3] * w[t]
		}
		out[c] = s
	}
}

func blockNestedReductionReports(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			{
				s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			}
		}
		out[c] = s
	}
}

func labeledNestedReductionReports(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			if false {
				goto label
			}
		label:
			{
				s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			}
		}
		out[c] = s
	}
}

func elseIfReductionReports(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			if flag {
			} else if !flag {
				s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			}
		}
		out[c] = s
	}
}

func switchReductionReports(a []float64, w []float64, taps, channels int, out []float64, mode int) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			switch mode {
			case 1:
				s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			}
		}
		out[c] = s
	}
}

func selectReductionReports(a []float64, w []float64, taps, channels int, out []float64, ready <-chan struct{}) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			select {
			case <-ready:
				s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			default:
			}
		}
		out[c] = s
	}
}

// The scalar tail must match the preceding tile's source. A tile over a does
// not resolve a tail reduction over b.
func registerTileThenDifferentSourceTailStillStrided(a, b []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t]
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0
		out[c+1] = s1
		out[c+2] = s2
		out[c+3] = s3
	}
	for c := channels - channels%4; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			s += b[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

// The scalar tail must also match the previous tile's stride expression.
func registerTileThenDifferentStrideTailStillStrided(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t]
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0
		out[c+1] = s1
		out[c+2] = s2
		out[c+3] = s3
	}
	for c := channels - channels%4; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * (channels + 1)
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func consumePS1006Index(int) {}

func mutatePS1006Index(p *int) { *p = 0 }

func resetPS1006AndTrue(p *int) bool {
	*p = 0
	return true
}

func resetPS1006AndInt(p *int) int {
	*p = 0
	return 0
}

func resetPS1006Any(p *int) any {
	*p = 0
	return 0
}

type ps1006PointerBox struct{ p *int }

func resetPS1006InterfacePointer(value any) {
	if p, ok := value.(*int); ok {
		*p = 0
	}
}

func resetPS1006PointerBox(box ps1006PointerBox) { *box.p = 0 }

func (box *ps1006PointerBox) reset() { *box.p = 0 }

type ps1006Holder struct {
	a     []float64
	width int
}

func simultaneousStrideAssignmentReports(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			prev := base
			prev, base = 0, prev
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			_ = prev
		}
		out[c] = s
	}
}

func simultaneousStrideAssignmentOverwriteSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			prev := 0
			base, prev = prev, base
			s += a[base+c] * w[t]
			_ = prev
		}
		out[c] = s
	}
}

func byValueCallKeepsStrideReports(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			consumePS1006Index(base)
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func pointerAliasMutationClearsStrideSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			p := &base
			*p = 0
			s += a[base+c] * w[t]
		}
		out[c] = s
	}
}

func pointerArgumentMutationClearsStrideSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			mutatePS1006Index(&base)
			s += a[base+c] * w[t]
		}
		out[c] = s
	}
}

func pointerArgumentMutationKeepsUnrelatedStrideReports(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base1 := t * channels
			base2 := t * channels
			mutatePS1006Index(&base1)
			s += a[base2+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func transitivePointerAliasMutationKeepsUnrelatedStrideReports(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base1 := t * channels
			base2 := t * channels
			p := &base1
			q := p
			*q = 0
			s += a[base2+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func futurePointerAliasAssignmentDoesNotKillUnrelatedStrideReports(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base1 := t * channels
			base2 := t * channels
			p := &base1
			*p = 0
			p = &base2
			_ = p
			s += a[base2+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func overwrittenPointerAliasMutationKeepsPreviousStrideReports(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base1 := t * channels
			base2 := t * channels
			p := &base1
			p = &base2
			*p = 0
			s += a[base1+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func branchPointerAliasMutationKeepsUnreachedStrideReports(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base1 := t * channels
			base2 := t * channels
			p := &base1
			if flag {
				p = &base2
			}
			*p = 0
			s += a[base2+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func branchLocalPointerAliasMutationKeepsUnrelatedStrideReports(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base1 := t * channels
			base2 := t * channels
			if flag {
				p := &base1
				*p = 0
				s += a[base2+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			}
		}
		out[c] = s
	}
}

func nestedBranchPointerAliasMutationKeepsUnrelatedStrideReports(a []float64, w []float64, taps, channels int, out []float64, outer, inner bool) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base1 := t * channels
			base2 := t * channels
			if outer {
				p := &base1
				if inner {
					*p = 0
					s += a[base2+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
				}
			}
		}
		out[c] = s
	}
}

func manyInvariantBranchesBeforeReductionStayStrided(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			if flag {
			}
			if flag {
			}
			if flag {
			}
			if flag {
			}
			if flag {
			}
			if flag {
			}
			if flag {
			}
			if flag {
			}
			if flag {
			}
			if flag {
			}
			if flag {
			}
			if flag {
			}
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func closureAliasMutationClearsStrideSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			mutate := func() { base = 0 }
			mutate()
			s += a[base+c] * w[t]
		}
		out[c] = s
	}
}

func closureAliasMutationKeepsUnrelatedStrideReports(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base1 := t * channels
			base2 := t * channels
			mutate := func() { base1 = 0 }
			mutate()
			_ = base1
			s += a[base2+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func overwrittenClosureAliasMutationKeepsPreviousStrideReports(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base1 := t * channels
			base2 := t * channels
			mutate := func() { base1 = 0 }
			mutate = func() { base2 = 0 }
			mutate()
			_ = base2
			s += a[base1+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func aggregateAliasWriteDoesNotCreateStrideSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			bases := []int{t * channels}
			bases[0] = 0
			s += a[bases[0]+c] * w[t]
		}
		out[c] = s
	}
}

func aggregatePointerAliasMutationClearsStrideSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			box := struct{ p *int }{p: &base}
			*box.p = 0
			s += a[base+c] * w[t]
		}
		out[c] = s
	}
}

func transitiveAggregatePointerAliasMutationClearsStrideSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			p := &base
			box := struct{ p *int }{p: p}
			*box.p = 0
			s += a[base+c] * w[t]
		}
		out[c] = s
	}
}

func aggregatePointerAliasMutationKeepsUnrelatedStrideReports(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base1 := t * channels
			base2 := t * channels
			box := struct{ p *int }{p: &base1}
			*box.p = 0
			s += a[base2+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func interfacePointerAliasCallClearsStrideSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			var alias any = &base
			resetPS1006InterfacePointer(alias)
			s += a[base+c] * w[t]
		}
		out[c] = s
	}
}

func aggregatePointerAliasCallClearsStrideSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			box := ps1006PointerBox{p: &base}
			resetPS1006PointerBox(box)
			s += a[base+c] * w[t]
		}
		out[c] = s
	}
}

func aggregatePointerReceiverClearsStrideSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			box := ps1006PointerBox{p: &base}
			box.reset()
			s += a[base+c] * w[t]
		}
		out[c] = s
	}
}

func ifConditionMutationClearsStrideSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			if resetPS1006AndTrue(&base) {
				s += a[base+c] * w[t]
			}
		}
		out[c] = s
	}
}

func switchTagMutationClearsStrideSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			switch resetPS1006AndInt(&base) {
			case 0:
				s += a[base+c] * w[t]
			}
		}
		out[c] = s
	}
}

func typeSwitchAssignMutationClearsStrideSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			switch resetPS1006Any(&base).(type) {
			default:
				s += a[base+c] * w[t]
			}
		}
		out[c] = s
	}
}

func switchCaseMutationClearsStrideSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			switch {
			case resetPS1006AndTrue(&base):
				s += a[base+c] * w[t]
			}
		}
		out[c] = s
	}
}

func selectReceiveClearsStrideSilent(a []float64, w []float64, taps, channels int, out []float64, ch <-chan int) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			select {
			case base = <-ch:
				s += a[base+c] * w[t]
			}
		}
		out[c] = s
	}
}

func unconditionalContinueBeforeReductionSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			continue
			s += a[t*channels+c] * w[t]
		}
		out[c] = s
	}
}

func unconditionalReturnBeforeReductionSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			return
			s += a[t*channels+c] * w[t]
		}
		out[c] = s
	}
}

func forwardGotoSkipsReductionSilent(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			goto done
			s += a[t*channels+c] * w[t]
		done:
		}
		out[c] = s
	}
}

func branchJoinStrideReports(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := 0
			if flag {
				base = t * channels
			} else {
				base = t * channels
			}
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func branchJoinMixedStrideReports(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := 0
			if flag {
				base = t * channels
			} else {
				base = 0
			}
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func branchJoinDifferentStrideKeysStillReports(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := 0
			if flag {
				base = t * channels
			} else {
				base = t * (channels + 1)
			}
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func switchBreakKeepsStrideReports(a []float64, w []float64, taps, channels int, out []float64, mode int) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			switch mode {
			case 0:
				break
			default:
				break
			}
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func switchFallthroughKeepsStrideReports(a []float64, w []float64, taps, channels int, out []float64, mode int) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := 0
			switch mode {
			case 0:
				base = t * channels
				fallthrough
			default:
			}
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func switchNestedBreakKeepsStrideReports(a []float64, w []float64, taps, channels int, out []float64, mode int, flag bool) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			switch mode {
			case 0:
				if flag {
					break
				}
				base = 0
			default:
				base = 0
			}
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func forwardGotoKeepsStrideAtLabelReports(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			if flag {
				goto use
			}
			base = 0
		use:
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func backwardGotoKeepsStrideAtLabelReports(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := 0
		use:
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			base = t * channels
			goto use
		}
		out[c] = s
	}
}

func backwardGotoToEmptyLabelKeepsStrideReports(a []float64, w []float64, taps, channels int, out []float64) {
	for c := 0; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := 0
		use:
			;
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			base = t * channels
			goto use
		}
		out[c] = s
	}
}

func holderSelectorRegisterTileWithTailSilent(holder ps1006Holder, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += holder.a[base+c] * w[t]
			s1 += holder.a[base+c+1] * w[t]
			s2 += holder.a[base+c+2] * w[t]
			s3 += holder.a[base+c+3] * w[t]
		}
		out[c] = s0
		out[c+1] = s1
		out[c+2] = s2
		out[c+3] = s3
	}
	for c := channels - channels%4; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			s += holder.a[base+c] * w[t]
		}
		out[c] = s
	}
}

func selectorShadowedBaseTailStillStrided(holder ps1006Holder, other ps1006Holder, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += holder.a[base+c] * w[t]
			s1 += holder.a[base+c+1] * w[t]
			s2 += holder.a[base+c+2] * w[t]
			s3 += holder.a[base+c+3] * w[t]
		}
		out[c] = s0
		out[c+1] = s1
		out[c+2] = s2
		out[c+3] = s3
	}
	for c := channels - channels%4; c < channels; c++ {
		holder := other
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * channels
			s += holder.a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func selectorStrideRegisterTileWithTailSilent(holder ps1006Holder, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * holder.width
			s0 += holder.a[base+c] * w[t]
			s1 += holder.a[base+c+1] * w[t]
			s2 += holder.a[base+c+2] * w[t]
			s3 += holder.a[base+c+3] * w[t]
		}
		out[c] = s0
		out[c+1] = s1
		out[c+2] = s2
		out[c+3] = s3
	}
	for c := channels - channels%4; c < channels; c++ {
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * holder.width
			s += holder.a[base+c] * w[t]
		}
		out[c] = s
	}
}

func selectorShadowedStrideTailStillStrided(a []float64, cfg ps1006Holder, other ps1006Holder, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * cfg.width
			s0 += a[base+c] * w[t]
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0
		out[c+1] = s1
		out[c+2] = s2
		out[c+3] = s3
	}
	for c := channels - channels%4; c < channels; c++ {
		cfg := other
		s := 0.0
		for t := 0; t < taps; t++ {
			base := t * cfg.width
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

func nestedOptionalTailGuardDifferentAccumulatorsEachPathSilent(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3, alt float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t]
			if c+1 < channels {
				if flag {
					s1 += a[base+c+1] * w[t]
				} else {
					alt += a[base+c+1] * w[t]
				}
			}
			if c+2 < channels {
				s2 += a[base+c+2] * w[t]
			}
			if c+3 < channels {
				s3 += a[base+c+3] * w[t]
			}
		}
		out[c] = s0 + s1 + s2 + s3 + alt
	}
}

func nestedOptionalTailGuardExclusiveSlotCollisionStillStrided(a []float64, w []float64, taps, channels int, out []float64, flag bool) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			if c+1 < channels {
				if flag {
					s1 += a[base+c+1] * w[t]
				} else {
					s0 += a[base+c+1] * w[t]
				}
			}
			if c+2 < channels {
				s2 += a[base+c+2] * w[t]
			}
			if c+3 < channels {
				s3 += a[base+c+3] * w[t]
			}
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Guards using a different bound are not output-lane tail guards. When limit
// excludes them, only s0 runs and the serial strided reduction remains.
func differentBoundGuardTileStillStrided(a, w []float64, taps, channels, limit int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			if c+1 < limit {
				s1 += a[base+c+1] * w[t]
			}
			if c+2 < limit {
				s2 += a[base+c+2] * w[t]
			}
			if c+3 < limit {
				s3 += a[base+c+3] * w[t]
			}
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// A range tap loop is the same supported reduction shape as a counted tap
// loop, so its matching scalar remainder stays covered by the main tile.
func rangeTapRegisterTileWithScalarTail(a, w []float64, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := range w {
			base := t * channels
			s0 += a[base+c] * w[t]
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0
		out[c+1] = s1
		out[c+2] = s2
		out[c+3] = s3
	}
	for c := channels - channels%4; c < channels; c++ {
		s := 0.0
		for t := range w {
			base := t * channels
			s += a[base+c] * w[t]
		}
		out[c] = s
	}
}

// A complete tile for a does not resolve the independent serial reduction
// from b. Suppression has to account for every strided source/key in the loop.
func completeTileBesideUnresolvedSourceStillStrided(a, b, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3, serial float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
			serial += b[base+c] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3 + serial
	}
}

// The outer/guard bound is not a stable tile invariant when it is assigned
// inside the tap loop.
func assignedTileBoundStillStrided(a, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			channels--
			if c+1 < channels {
				s1 += a[base+c+1] * w[t]
			}
			if c+2 < channels {
				s2 += a[base+c+2] * w[t]
			}
			if c+3 < channels {
				s3 += a[base+c+3] * w[t]
			}
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Escaping the bound through a pointer likewise invalidates the optional-tail
// proof, even when the callee's mutation is not visible locally.
func escapedTileBoundStillStrided(a, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			mutatePS1006Index(&channels)
			if c+1 < channels {
				s1 += a[base+c+1] * w[t]
			}
			if c+2 < channels {
				s2 += a[base+c+2] * w[t]
			}
			if c+3 < channels {
				s3 += a[base+c+3] * w[t]
			}
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Changing the outer output variable in the inner body destroys the adjacent
// lane proof, including when the change happens through an alias.
func aliasedOuterMutationTileStillStrided(a, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			outer := &c
			mutatePS1006Index(outer)
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Rebinding a slice source between lanes means the four syntactically adjacent
// reads no longer share one source value.
func reassignedTileSourceStillStrided(a, b, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			a = b
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Direct stride inputs are value-sensitive as well as identity-sensitive.
func reassignedDirectStrideStillStrided(a, w []float64, taps, channels, stride int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[t*stride+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			stride = channels
			s1 += a[t*stride+c+1] * w[t]
			s2 += a[t*stride+c+2] * w[t]
			s3 += a[t*stride+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Every matching strided read in one accumulator RHS participates in the
// proof. A complete tile for a cannot hide the unmatched b read in lane zero.
func multipleRHSStridedKeysStillStrided(a, b, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c]*w[t] + b[base+c]*w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// An address escape before the output loop is part of the bound's stability
// history, even though the indirect mutation inside the tap loop names only p.
func priorEscapedTileBoundStillStrided(a, w []float64, taps, channels int, out []float64) {
	p := &channels
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			mutatePS1006Index(p)
			if c+1 < channels {
				s1 += a[base+c+1] * w[t]
			}
			if c+2 < channels {
				s2 += a[base+c+2] * w[t]
			}
			if c+3 < channels {
				s3 += a[base+c+3] * w[t]
			}
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// A source alias created before the output loop can rebind the slice between
// lanes without a direct assignment to a in the inspected tap-loop body.
func priorEscapedTileSourceStillStrided(a, b, w []float64, taps, channels int, out []float64) {
	pa := &a
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			*pa = b
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// The same prior-escape rule applies to scalar stride inputs: an indirect
// write can change the address calculation between adjacent lanes.
func priorEscapedDirectStrideStillStrided(a, w []float64, taps, channels, stride int, out []float64) {
	ps := &stride
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[t*stride+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			*ps = channels
			s1 += a[t*stride+c+1] * w[t]
			s2 += a[t*stride+c+2] * w[t]
			s3 += a[t*stride+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Multiple sources in one RHS are resolved when each source/stride key has
// the same complete four-offset tile with distinct lane accumulators.
func completeMultipleRHSStridedKeysSilent(a, b, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c]*w[t] + b[base+c]*w[t]
			s1 += a[base+c+1]*w[t] + b[base+c+1]*w[t]
			s2 += a[base+c+2]*w[t] + b[base+c+2]*w[t]
			s3 += a[base+c+3]*w[t] + b[base+c+3]*w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Every mutable object inside an indexed stride expression is an invariant.
// Changing axis between lanes invalidates the otherwise complete tile.
func indexedStrideAxisMutationStillStrided(a, w []float64, strides []int, axis, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[t*strides[axis]+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			axis++
			s1 += a[t*strides[axis]+c+1] * w[t]
			s2 += a[t*strides[axis]+c+2] * w[t]
			s3 += a[t*strides[axis]+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// A closure created before the tile can mutate the output bound without the
// call expression itself mentioning that captured object.
func priorClosureBoundMutationStillStrided(a, w []float64, taps, channels int, out []float64) {
	mutate := func() { channels = 1 }
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			mutate()
			if c+1 < channels {
				s1 += a[base+c+1] * w[t]
			}
			if c+2 < channels {
				s2 += a[base+c+2] * w[t]
			}
			if c+3 < channels {
				s3 += a[base+c+3] * w[t]
			}
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Source rebinding through a captured variable is subject to the same
// function-wide stability history.
func priorClosureSourceMutationStillStrided(a, b, w []float64, taps, channels int, out []float64) {
	mutate := func() { a = b }
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			mutate()
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Scalar stride captures also invalidate the proof when their closure is
// invoked between adjacent lanes.
func priorClosureStrideMutationStillStrided(a, w []float64, taps, channels, stride int, out []float64) {
	mutate := func() { stride = channels }
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[t*stride+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			mutate()
			s1 += a[t*stride+c+1] * w[t]
			s2 += a[t*stride+c+2] * w[t]
			s3 += a[t*stride+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// len and cap are value-pure stride inputs. Their referenced slice objects are
// still indexed for writes/escapes, but the calls themselves do not defeat a
// complete register tile.
func lenStrideCallTileSilent(a, w []float64, strides []int, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[t*len(strides)+c] * w[t]
			s1 += a[t*len(strides)+c+1] * w[t]
			s2 += a[t*len(strides)+c+2] * w[t]
			s3 += a[t*len(strides)+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

func capStrideCallTileSilent(a, w []float64, strides []int, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[t*cap(strides)+c] * w[t]
			s1 += a[t*cap(strides)+c+1] * w[t]
			s2 += a[t*cap(strides)+c+2] * w[t]
			s3 += a[t*cap(strides)+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

func convertedStrideCallTileSilent(a, w []float64, taps, channels, stride int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[t*int(stride)+c] * w[t]
			s1 += a[t*int(stride)+c+1] * w[t]
			s2 += a[t*int(stride)+c+2] * w[t]
			s3 += a[t*int(stride)+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// A type conversion is a proven value-producing call, so wrapping the entire
// canonical multiplication does not manufacture the arbitrary-call hazard.
func wrappedConvertedStrideTileSilent(a, w []float64, taps, channels, stride int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[int(t*stride)+c] * w[t]
			s1 += a[int(t*stride)+c+1] * w[t]
			s2 += a[int(t*stride)+c+2] * w[t]
			s3 += a[int(t*stride)+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Pure conversion calls are still part of the base identity. Different
// narrowing widths can wrap at different tap products, so they do not prove
// that these four reads form one adjacent-output tile.
func mismatchedNarrowingStrideConversionsStillStrided(a, w []float64, taps, channels, stride int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[int(int8(t*stride))+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			s1 += a[int(int16(t*stride))+c+1] * w[t]
			s2 += a[int(int32(t*stride))+c+2] * w[t]
			s3 += a[int(int64(t*stride))+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Identical narrowing is conservative but sound for the tile proof: every
// lane wraps the same base before its adjacent constant offset is added.
func matchingNarrowingStrideConversionsTileSilent(a, w []float64, taps, channels, stride int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[int(int8(t*stride))+c] * w[t]
			s1 += a[int(int8(t*stride))+c+1] * w[t]
			s2 += a[int(int8(t*stride))+c+2] * w[t]
			s3 += a[int(int8(t*stride))+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

type ps1006NarrowWord = int8

// Identical source spelling is not type identity: the nested block shadows
// the package alias with a wider conversion, so the lane bases diverge once
// the tap product exceeds int8.
func shadowedConversionTypesStillStrided(a, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[int(ps1006NarrowWord(t*channels))+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			{
				type ps1006NarrowWord = int16
				s1 += a[int(ps1006NarrowWord(t*channels))+c+1] * w[t]
				s2 += a[int(ps1006NarrowWord(t*channels))+c+2] * w[t]
				s3 += a[int(ps1006NarrowWord(t*channels))+c+3] * w[t]
			}
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Converting an int expression to int is an identity operation and must not
// split an otherwise complete adjacent-output tile into different keys.
func mixedNoOpConversionTileSilent(a, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[t*channels+c] * w[t]
			s1 += a[int(t*channels)+c+1] * w[t]
			s2 += a[t*channels+c+2] * w[t]
			s3 += a[int(t*channels)+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// A pointer rebind at the bottom of the tap loop reaches the dereference on
// its backedge. From the second tap onward it can mutate base between lanes.
func backedgePointerAliasStillStrided(a, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var base, other int
		pointer := &other
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base = t * channels
			s0 += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			*pointer = 0
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
			pointer = &base
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

var ps1006NextStrideValue = 8

func nextPS1006Stride() int {
	ps1006NextStrideValue++
	return ps1006NextStrideValue
}

func nextPS1006Base(base int) int {
	ps1006NextStrideValue++
	return base + ps1006NextStrideValue
}

// Finding the canonical multiplication nested inside an arbitrary wrapper is
// not enough to prove one adjacent-access tile: every call may return a
// different base even when all four argument expressions are identical.
func sideEffectBaseWrapperStillStrided(a, w []float64, taps, channels, stride int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[nextPS1006Base(t*stride)+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			s1 += a[nextPS1006Base(t*stride)+c+1] * w[t]
			s2 += a[nextPS1006Base(t*stride)+c+2] * w[t]
			s3 += a[nextPS1006Base(t*stride)+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

// Identical source text does not make an arbitrary call return the same value
// for adjacent lanes.
func sideEffectStrideCallStillStrided(a, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[t*nextPS1006Stride()+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			s1 += a[t*nextPS1006Stride()+c+1] * w[t]
			s2 += a[t*nextPS1006Stride()+c+2] * w[t]
			s3 += a[t*nextPS1006Stride()+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

type ps1006StrideSource struct{ stride int }

func (source *ps1006StrideSource) Next() int {
	source.stride++
	return source.stride
}

func sideEffectStrideMethodStillStrided(a, w []float64, source *ps1006StrideSource, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[t*source.Next()+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			s1 += a[t*source.Next()+c+1] * w[t]
			s2 += a[t*source.Next()+c+2] * w[t]
			s3 += a[t*source.Next()+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}

var ps1006PackageStride = 8

func mutatePS1006PackageStride() { ps1006PackageStride++ }

// An unrelated-looking impure call can mutate a package-scope proof input.
func packageStrideCallEffectStillStrided(a, w []float64, taps, channels int, out []float64) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			s0 += a[t*ps1006PackageStride+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			mutatePS1006PackageStride()
			s1 += a[t*ps1006PackageStride+c+1] * w[t]
			s2 += a[t*ps1006PackageStride+c+2] * w[t]
			s3 += a[t*ps1006PackageStride+c+3] * w[t]
		}
		out[c] = s0 + s1 + s2 + s3
	}
}
