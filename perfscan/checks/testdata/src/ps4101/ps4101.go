package ps4101

func rangeIdx(dst, src []float64) {
	for i := range src { // want `element-copy loop from src to dst is a single memmove; replace with copy\(dst, src\)`
		dst[i] = src[i]
	}
}

func rangeVal(dst, src []int) {
	for i, v := range src { // want `element-copy loop from src to dst is a single memmove; replace with copy\(dst, src\)`
		dst[i] = v
	}
}

func counted(dst, src []byte) {
	for i := 0; i < len(src); i++ { // want `element-copy loop from src to dst is a single memmove; replace with copy\(dst, src\)`
		dst[i] = src[i]
	}
}

// A transform is not a copy: silent.
func scaled(dst, src []float64, k float64) {
	for i := range src {
		dst[i] = src[i] * k
	}
}

// Different element types cannot memmove: silent.
func widened(dst []float64, src []float32) {
	for i := range src {
		dst[i] = float64(src[i])
	}
}

// Extra work in the body: silent.
func twoStatements(dst, src []int) {
	n := 0
	for i := range src {
		dst[i] = src[i]
		n++
	}
	_ = n
}
