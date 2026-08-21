package ps5110

import "slices"

type ints []int

func left(a, b, c []int) []int {
	return slices.Concat(slices.Concat(a, b), c) // want "slices.Concat materializes and recopies 1 intermediate concatenation result"
}

func deep(a, b, c, d []int) []int {
	return slices.Concat(slices.Concat(slices.Concat(a, b), c), d) // want "slices.Concat materializes and recopies 2 intermediate concatenation result"
}

func branched(a, b, c, d []int) []int {
	return slices.Concat(slices.Concat(a, b), slices.Concat(c, d)) // want "slices.Concat materializes and recopies 2 intermediate concatenation result"
}

func rightCalls(a []int) []int {
	return slices.Concat(a, slices.Concat(source(), source())) // want "slices.Concat materializes and recopies 1 intermediate concatenation result"
}

func parenthesized(a, b, c []int) []int {
	return slices.Concat((slices.Concat((a), b)), c) // want "slices.Concat materializes and recopies 1 intermediate concatenation result"
}

func named(a, b, c ints) ints {
	return slices.Concat[ints](slices.Concat[ints](a, b), c) // want "slices.Concat materializes and recopies 1 intermediate concatenation result"
}

func laterSlice(a, b, c []int, n int) []int {
	return slices.Concat(slices.Concat(a, b), c[:n]) // want "slices.Concat materializes and recopies 1 intermediate concatenation result"
}

func laterLiteral(a, b []int) []int {
	return slices.Concat(slices.Concat(a, b), []int{1, 2}) // want "slices.Concat materializes and recopies 1 intermediate concatenation result"
}

func laterClone(a, b, c []int) []int {
	return slices.Concat(slices.Concat(a, b), slices.Clone(c)) // want "slices.Concat materializes and recopies 1 intermediate concatenation result"
}

func source() []int { return []int{1} }

func mutate(values []int) []int {
	if len(values) != 0 {
		values[0]++
	}
	return values
}

// A later arbitrary call can mutate an inner input after its original copy.
func laterMutation(a, b []int) []int {
	return slices.Concat(slices.Concat(a), mutate(b))
}

func laterAppend(a, b []int) []int {
	return slices.Concat(slices.Concat(a), append(b, 1))
}

func laterReceive(a []int, incoming <-chan []int) []int {
	return slices.Concat(slices.Concat(a), <-incoming)
}

// Only the later branch is flattened. The first snapshot must remain before
// mutate(a), while the second Concat has no observable boundary of its own.
func partialTree(a, b []int) []int {
	return slices.Concat(slices.Concat(a), slices.Concat(mutate(a), b)) // want "slices.Concat materializes and recopies 1 intermediate concatenation result"
}

// Different explicit result types are assignable here but deliberately not
// flattened: retaining the root's concrete generic contract is fail-closed.
func mixedTypes(a, b ints, c []int) []int {
	return slices.Concat[[]int](slices.Concat[ints](a, b), c)
}

func spread(groups [][]int) []int {
	return slices.Concat(slices.Concat(groups...))
}

func empty(a []int) []int {
	return slices.Concat[[]int](slices.Concat[[]int](), a)
}

func untypedNil(a []int) []int {
	return slices.Concat[[]int](slices.Concat[[]int](nil), a)
}

func functionOuter(a, b, c []int) []int {
	concat := slices.Concat[[]int]
	return concat(slices.Concat(a, b), c)
}

func functionInner(a, b, c []int) []int {
	concat := slices.Concat[[]int]
	return slices.Concat(concat(a, b), c)
}

func single(a, b []int) []int {
	return slices.Concat(a, b)
}

func Concat(values ...[]int) []int { return nil }

func user(a, b, c []int) []int {
	return Concat(Concat(a, b), c)
}
