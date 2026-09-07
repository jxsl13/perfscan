package ps2144generic

type composite[T any] struct {
	value T
}

// Direct and nested type-parameter layouts are valid Go but deliberately
// unsupported by PS2144. The regression is primarily that analysis never
// calls Sizes.Sizeof on them and therefore never panics.
func direct[T any](n int) int {
	a := make([]T, n)
	b := make([]T, n)
	return len(a) + len(b)
}

func nested[T any](n int) int {
	a := make([]composite[T], n)
	b := make([]composite[T], n)
	return len(a) + len(b)
}

func array[T any](n int) int {
	a := make([][2]T, n)
	b := make([][2]T, n)
	return len(a) + len(b)
}

func anonymousComposite[T any](n int) int {
	a := make([]struct{ value T }, n)
	b := make([]struct{ value T }, n)
	return len(a) + len(b)
}
