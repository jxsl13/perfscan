package ps6084grouped

//go:noescape
func pairLeaf(packed0, packed1 *byte, scratch0, scratch1 *float32) (float32, float32)

//go:noescape
func interfaceLeaf(packed0, packed1 *byte, scratch0, scratch1 any) (float32, float32)

func decode(value byte) float32 { return float32(value) }

func opaque(value byte) float32

func live(packed0, packed1 []byte, trips int) {
	for block := 0; block < trips; block++ {
		var scratch0, scratch1 [4]float32
		for lane := range 4 { // want `fixed scratch arrays scratch0, scratch1.*paired packed sources packed0, packed1.*pairLeaf.*path that may repeat`
			scratch0[lane] = float32(packed0[lane])
			scratch1[lane] = float32(packed1[lane])
		}
		_, _ = pairLeaf(&packed0[0], &packed1[0], &scratch0[0], &scratch1[0])
	}
}

func liveWithPredecoded(packed0, packed1 []byte, trips int) {
	for block := 0; block < trips; block++ {
		decoded0, decoded1 := decode(packed0[0]), decode(packed1[0])
		var scratch0, scratch1 [2]float32
		for lane := range 2 { // want `fixed scratch arrays scratch0, scratch1.*paired packed sources packed0, packed1.*pairLeaf`
			scratch0[lane] = decoded0 + float32(packed0[lane])
			scratch1[lane] = decoded1 + float32(packed1[lane])
		}
		_, _ = pairLeaf(&packed0[0], &packed1[0], &scratch0[0], &scratch1[0])
	}
}

func partialStaysSilent(packed0, packed1 []byte, trips int) {
	for block := 0; block < trips; block++ {
		var scratch0, scratch1 [4]float32
		for lane := range 3 {
			scratch0[lane] = float32(packed0[lane])
			scratch1[lane] = float32(packed1[lane])
		}
		_, _ = pairLeaf(&packed0[0], &packed1[0], &scratch0[0], &scratch1[0])
	}
}

func duplicateStaysSilent(packed0, packed1 []byte, trips int) {
	for block := 0; block < trips; block++ {
		var scratch0, scratch1 [4]float32
		for lane := range 4 {
			scratch0[lane] = float32(packed0[lane])
			scratch0[lane] = float32(packed0[lane])
			scratch1[lane] = float32(packed1[lane])
		}
		_, _ = pairLeaf(&packed0[0], &packed1[0], &scratch0[0], &scratch1[0])
	}
}

func mixedRootsStaySilent(packed0, packed1 []byte, trips int) {
	for block := 0; block < trips; block++ {
		var scratch0, scratch1 [4]float32
		for lane := range 4 {
			scratch0[lane] = float32(packed0[lane]) + float32(packed1[lane])
			scratch1[lane] = float32(packed1[lane])
		}
		_, _ = pairLeaf(&packed0[0], &packed1[0], &scratch0[0], &scratch1[0])
	}
}

func opaqueRHSStaysSilent(packed0, packed1 []byte, trips int) {
	for block := 0; block < trips; block++ {
		var scratch0, scratch1 [4]float32
		for lane := range 4 {
			scratch0[lane] = opaque(packed0[lane])
			scratch1[lane] = float32(packed1[lane])
		}
		_, _ = pairLeaf(&packed0[0], &packed1[0], &scratch0[0], &scratch1[0])
	}
}

func interfaceBoundaryStaysSilent(packed0, packed1 []byte, trips int) {
	for block := 0; block < trips; block++ {
		var scratch0, scratch1 [4]float32
		for lane := range 4 {
			scratch0[lane] = float32(packed0[lane])
			scratch1[lane] = float32(packed1[lane])
		}
		_, _ = interfaceLeaf(&packed0[0], &packed1[0], &scratch0[0], &scratch1[0])
	}
}

func oneTripStaysSilent(packed0, packed1 []byte) {
	for block := 0; block < 1; block++ {
		var scratch0, scratch1 [4]float32
		for lane := range 4 {
			scratch0[lane] = float32(packed0[lane])
			scratch1[lane] = float32(packed1[lane])
		}
		_, _ = pairLeaf(&packed0[0], &packed1[0], &scratch0[0], &scratch1[0])
	}
}

func laterReadStaysSilent(packed0, packed1 []byte, trips int) float32 {
	var result float32
	for block := 0; block < trips; block++ {
		var scratch0, scratch1 [4]float32
		for lane := range 4 {
			scratch0[lane] = float32(packed0[lane])
			scratch1[lane] = float32(packed1[lane])
		}
		_, _ = pairLeaf(&packed0[0], &packed1[0], &scratch0[0], &scratch1[0])
		result += scratch0[0]
	}
	return result
}
