package ps6108

func fieldPair(index int, packed []byte) (scale, minimum byte) {
	if index < 4 {
		scale = packed[index] & 63
		minimum = packed[index+4] & 63
		return
	}
	scale = (packed[index+4] & 0x0f) | ((packed[index-4] >> 6) << 4)
	minimum = (packed[index+4] >> 4) | ((packed[index] >> 6) << 4)
	return
}

func directFieldPair(index int, packed []byte) (byte, byte) {
	if index < 4 {
		return packed[index] & 63, packed[index+4] & 63
	}
	return (packed[index+4] & 0x0f) | ((packed[index-4] >> 6) << 4),
		(packed[index+4] >> 4) | ((packed[index] >> 6) << 4)
}

func positiveFloat32(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed) // want `2 repeated calls per packed source to pure local helper fieldPair decode statically related fields from the same 12-byte header in an exact 4-trip coefficient loop \(24 source-level packed reads across 1 stream\(s\)\); profile and inspect optimized code.*advisory, no automatic fix`
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func positiveFloat64(raw [12]byte, scale, minimum float64) float64 {
	packed := raw[0:12:12]
	var coefficients [16]float64
	for pair := 0; pair < 4; pair++ {
		logical := 2 * pair
		coefficient := 4 * pair
		s0, m0 := directFieldPair(logical, packed) // want `pure local helper directFieldPair.*exact 4-trip coefficient loop.*1 stream`
		s1, m1 := directFieldPair(1+logical, packed)
		coefficients[coefficient+0] = float64(s0) * scale
		coefficients[coefficient+1] = float64(m0) * minimum
		coefficients[coefficient+2] = float64(s1) * scale
		coefficients[coefficient+3] = float64(m1) * minimum
	}
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func positiveTwoStreams(raw0, raw1 [12]byte, scale0, minimum0, scale1, minimum1 float32) float32 {
	packed0 := raw0[0:12:12]
	packed1 := raw1[0:12:12]
	var coefficients0 [16]float32
	var coefficients1 [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s00, m00 := fieldPair(logical, packed0) // want `48 source-level packed reads across 2 stream\(s\)`
		s01, m01 := fieldPair(logical+1, packed0)
		s10, m10 := fieldPair(logical, packed1)
		s11, m11 := fieldPair(logical+1, packed1)
		coefficients0[coefficient+0] = scale0 * float32(s00)
		coefficients0[coefficient+1] = minimum0 * float32(m00)
		coefficients0[coefficient+2] = scale0 * float32(s01)
		coefficients0[coefficient+3] = minimum0 * float32(m01)
		coefficients1[coefficient+0] = scale1 * float32(s10)
		coefficients1[coefficient+1] = minimum1 * float32(m10)
		coefficients1[coefficient+2] = scale1 * float32(s11)
		coefficients1[coefficient+3] = minimum1 * float32(m11)
	}
	return coefficients0[0] + coefficients0[1] + coefficients0[2] + coefficients0[3] +
		coefficients0[4] + coefficients0[5] + coefficients0[6] + coefficients0[7] +
		coefficients0[8] + coefficients0[9] + coefficients0[10] + coefficients0[11] +
		coefficients0[12] + coefficients0[13] + coefficients0[14] + coefficients0[15] +
		coefficients1[0] + coefficients1[1] + coefficients1[2] + coefficients1[3] +
		coefficients1[4] + coefficients1[5] + coefficients1[6] + coefficients1[7] +
		coefficients1[8] + coefficients1[9] + coefficients1[10] + coefficients1[11] +
		coefficients1[12] + coefficients1[13] + coefficients1[14] + coefficients1[15]
}

func positiveAliasChain(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		next := logical + 1
		s0, m0 := fieldPair(logical, packed) // want `pure local helper fieldPair.*exact 4-trip coefficient loop.*1 stream`
		s1, m1 := fieldPair(next, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func positiveLocalReduction(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed) // want `pure local helper fieldPair.*exact 4-trip coefficient loop.*1 stream`
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	sum := coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
	return sum
}

func tooFewTrips(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [12]float32
	for pair := range 3 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0]
}

func proofWorkTooLarge(raw [132]byte, scale, minimum float32) float32 {
	packed := raw[0:132:132]
	var coefficients [260]float32
	for pair := range 65 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0]
}

func oneCallOnly(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [8]float32
	for pair := range 4 {
		logical := pair
		coefficient := pair * 2
		s0, m0 := fieldPair(logical, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
	}
	return coefficients[0]
}

func dynamicLayout(raw [12]byte, limit int, scale, minimum float32) float32 {
	packed := raw[0:limit:limit]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0]
}

func reachableHighBranchOutOfBounds(raw [11]byte, scale, minimum float32) float32 {
	packed := raw[0:11:11]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0]
}

func mutatedSource(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	packed[0] = 0
	return coefficients[0]
}

func deadStores(raw [12]byte, scale, minimum float32) {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
}

var escapedCoefficients [16]float32

func outputEscape(raw [12]byte, scale, minimum float32) {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	escapedCoefficients = coefficients
}

var impureMask byte = 63

func impureFieldPair(index int, packed []byte) (byte, byte) {
	if index < 4 {
		return packed[index] & impureMask, packed[index+4] & 63
	}
	return packed[index], packed[index+4]
}

func signedFieldPair(index int, packed []byte) (int8, int8) {
	if index < 4 {
		return int8(packed[index]), int8(packed[index+4])
	}
	return int8(packed[index]), int8(packed[index+4])
}

func helperRejected(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := impureFieldPair(logical, packed)
		s1, m1 := impureFieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0]
}

func dynamicCallIndex(raw [12]byte, offset int, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair*2 + offset
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0]
}

func extraResultUse(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		_ = s0
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0]
}

func outOfBoundsOutputIndex(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 5
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0]
}

func sourceAlias(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	alias := packed
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	_ = alias
	return coefficients[0]
}

func outputAddress(raw [12]byte, scale, minimum float32) *float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return &coefficients[0]
}

// The following negatives retain all sixteen coefficient values. They keep
// each intended guard independently visible instead of also failing because
// only coefficient zero was observed.

func nonzeroStartRetained(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := 1; pair < 5; pair++ {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func stepTwoRetained(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := 0; pair < 8; pair += 2 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func descendingRetained(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := 7; pair > 3; pair-- {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func dynamicLayoutRetained(raw [12]byte, limit int, scale, minimum float32) float32 {
	packed := raw[0:limit:limit]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func reachableBoundsRetained(raw [11]byte, scale, minimum float32) float32 {
	packed := raw[0:11:11]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func mutatedSourceRetained(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	packed[0] = 0
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func sourceAliasRetained(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	alias := packed
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	_ = alias
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func impureHelperRetained(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := impureFieldPair(logical, packed)
		s1, m1 := impureFieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func signedHelperRetained(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := signedFieldPair(logical, packed)
		s1, m1 := signedFieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func extraResultRetained(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		_ = s0
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func capturedOutputRetained(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	captured := func() float32 { return coefficients[0] }
	_ = captured
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func outOfBoundsOutputRetained(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 5
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func unicodeRangeRetained(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range "éééé" {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func conditionalLoopRetained(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	if false {
		for pair := range 4 {
			logical := pair * 2
			coefficient := pair * 4
			s0, m0 := fieldPair(logical, packed)
			s1, m1 := fieldPair(logical+1, packed)
			coefficients[coefficient+0] = scale * float32(s0)
			coefficients[coefficient+1] = minimum * float32(m0)
			coefficients[coefficient+2] = scale * float32(s1)
			coefficients[coefficient+3] = minimum * float32(m1)
		}
	}
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func unreachableLoopRetained(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	return 0
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func unreachableReturnReadRetained(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return 0
	return coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
}

func unreachableReductionReturnRetained(raw [12]byte, scale, minimum float32) float32 {
	packed := raw[0:12:12]
	var coefficients [16]float32
	for pair := range 4 {
		logical := pair * 2
		coefficient := pair * 4
		s0, m0 := fieldPair(logical, packed)
		s1, m1 := fieldPair(logical+1, packed)
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	sum := coefficients[0] + coefficients[1] + coefficients[2] + coefficients[3] +
		coefficients[4] + coefficients[5] + coefficients[6] + coefficients[7] +
		coefficients[8] + coefficients[9] + coefficients[10] + coefficients[11] +
		coefficients[12] + coefficients[13] + coefficients[14] + coefficients[15]
	return 0
	return sum
}
