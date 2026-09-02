package ps1006

// A label on the immediately preceding four-lane loop does not change the
// tile. The matching scalar remainder must therefore stay silent.
func labeledRegisterTileThenMatchingTailSilentRound31(a, w []float64, taps, channels int, out []float64) {
	if false {
		goto mainTile
	}
mainTile:
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

// A reachable break targeting the outer tile can leave complete four-lane
// iterations uncomputed. It must not make the scalar remainder look resolved.
func labeledRegisterTileBreakKeepsTailDiagnosticRound31(a, w []float64, taps, channels int, out []float64, stop bool) {
mainTile:
	for c := 0; c+3 < channels; c += 4 {
		if stop {
			break mainTile
		}
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
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

// A labeled continue can likewise skip the complete tile body while still
// reaching the scalar remainder after the outer loop terminates.
func labeledRegisterTileContinueKeepsTailDiagnosticRound31(a, w []float64, taps, channels int, out []float64, skip bool) {
mainTile:
	for c := 0; c+3 < channels; c += 4 {
		if skip {
			continue mainTile
		}
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
			s += a[base+c] * w[t] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = s
	}
}

// Once all four lanes have completed, a labeled continue is the ordinary
// outer-loop backedge and must not invalidate the matching tail proof.
func labeledContinueAfterRegisterTileSilentRound31(a, w []float64, taps, channels int, out []float64, skip bool) {
mainTile:
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t]
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
		if skip {
			continue mainTile
		}
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

func continueAfterRegisterTileSilentRound31(a, w []float64, taps, channels int, out []float64, skip bool) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t]
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
		if skip {
			continue
		}
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

// A forward goto after the tile stays within the current iteration and lands
// immediately before its natural end.
func forwardGotoAfterRegisterTileSilentRound31(a, w []float64, taps, channels int, out []float64, jump bool) {
	for c := 0; c+3 < channels; c += 4 {
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t]
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
		if jump {
			goto done
		}
	done:
		_ = c
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

// Constant-dead exits do not create a live path around the tile.
func deadBreakBeforeRegisterTileSilentRound31(a, w []float64, taps, channels int, out []float64) {
mainTile:
	for c := 0; c+3 < channels; c += 4 {
		if false {
			break mainTile
		}
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t]
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
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

func deadSwitchBreakBeforeRegisterTileSilentRound31(a, w []float64, taps, channels int, out []float64) {
mainTile:
	for c := 0; c+3 < channels; c += 4 {
		switch 0 {
		case 1:
			break mainTile
		}
		var s0, s1, s2, s3 float64
		for t := 0; t < taps; t++ {
			base := t * channels
			s0 += a[base+c] * w[t]
			s1 += a[base+c+1] * w[t]
			s2 += a[base+c+2] * w[t]
			s3 += a[base+c+3] * w[t]
		}
		out[c], out[c+1], out[c+2], out[c+3] = s0, s1, s2, s3
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
