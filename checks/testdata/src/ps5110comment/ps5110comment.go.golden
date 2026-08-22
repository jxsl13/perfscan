package ps5110comment

import "slices"

func concat(a, b, c []int) []int {
	return slices.Concat(( /* preserve snapshot rationale */ slices.Concat(a, b)), c) // want "slices.Concat materializes and recopies 1 intermediate concatenation result"
}
