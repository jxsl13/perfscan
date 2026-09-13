// Synthetic complete source fixture, not an authentic activation pilot.
package ps6141

func pack(x []float32) []int8 {
	p := make([]int8, len(x))
	for i := range x {
		p[i] = int8(x[i])
	}
	return p
}
func dot(p, w []int8) int {
	if len(p) != len(w) {
		panic("shape")
	}
	sum := 0
	for i := range p {
		sum += int(p[i]) * int(w[i])
	}
	return sum
}
func immediate(x []float32, w []int8) int {
	p := pack(x) // want "fresh activation quantization/packing immediately feeds one source-proved dot boundary"
	return dot(p, w)
}
func reused(x []float32, w []int8) int { p := pack(x); first := dot(p, w); return first + dot(p, w) }
func scratch(p, w []int8) int          { return dot(p, w) }
func rows(x []float32, w []int8, n int) int {
	p := pack(x)
	sum := 0
	for range n {
		sum += dot(p, w)
	}
	return sum
}
func fused(x []float32, w []int8) int {
	sum := 0
	for i := range x {
		sum += int(int8(x[i])) * int(w[i])
	}
	return sum
}
