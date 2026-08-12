package ps4101_copyshadow

// A package-level func named copy shadows the builtin across the whole package.
// PS4101 must NOT rewrite the loop to copy(dst, src) here — that would call this
// func (returning 0, leaving dst zeroed) instead of the builtin. SILENT.
func copy(dst, src []int) int { return 0 }

func fill(src []int) []int {
	dst := make([]int, len(src))
	for i := range src {
		dst[i] = src[i]
	}
	return dst
}
