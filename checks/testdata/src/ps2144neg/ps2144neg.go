package ps2144neg

var (
	globalN     = 8
	stored      []int
	storedMany  [][]int
	storedValue int
)

type zero struct{}

type cell [2]int

type retaining struct{}

var retained *retaining

func (r *retaining) retain() { retained = r }

func consume([]int) {}

// One allocation is not a coalescing group.
func single(n int) int {
	a := make([]int, n)
	a[0] = 1
	return a[0]
}

// Different element types or lengths are not the same shape.
func different(n, m int) int {
	a := make([]int, n)
	b := make([]int32, n)
	c := make([]int, m)
	a[0], b[0], c[0] = 1, 2, 3
	return a[0] + int(b[0]) + c[0]
}

// A non-allocation statement separates the declarations.
func nonAdjacent(n int) int {
	a := make([]int, n)
	x := 1
	b := make([]int, n)
	a[0], b[0] = x, x
	return a[0] + b[0]
}

// Blank-assignment-only ownership is outside the substantive-use proof.
func blankOnly(n int) int {
	a := make([]int, n)
	b := make([]int, n)
	_ = a
	_ = b
	return n
}

// Reassignment makes n unstable even though it occurs after the allocations.
func unstableLength(n int) int {
	a := make([]int, n)
	b := make([]int, n)
	a[0], b[0] = 1, 2
	n++
	return a[0] + b[0]
}

// Address-taking can mutate n through an alias and therefore is not stable.
func addressedLength(n int) int {
	a := make([]int, n)
	b := make([]int, n)
	p := &n
	a[0], b[0] = *p, *p
	return a[0] + b[0]
}

// Package variables can change concurrently and are outside the local proof.
func globalLength() int {
	a := make([]int, globalN)
	b := make([]int, globalN)
	a[0], b[0] = 1, 2
	return a[0] + b[0]
}

// Even an immutable local temporary is outside the intentionally bounded
// parameter-or-constant length proof.
func localLength(input int) int {
	n := input
	a := make([]int, n)
	b := make([]int, n)
	a[0], b[0] = 1, 2
	return a[0] + b[0]
}

// len(input) may change if input is reassigned or appended; expression
// interpretation is intentionally outside PS2144's proof.
func lengthExpression(input []int) int {
	a := make([]int, len(input))
	b := make([]int, len(input))
	a[0], b[0] = 1, 2
	return a[0] + b[0]
}

// Explicit capacities are excluded.
func explicitCapacity(n int) int {
	a := make([]int, n, n)
	b := make([]int, n, n)
	a[0], b[0] = 1, 2
	return a[0] + b[0]
}

// Zero length and zero-sized elements do not prove backing-allocation work.
func noPayload(n int) int {
	a := make([]int, 0)
	b := make([]int, 0)
	_ = a
	_ = b
	c := make([]zero, n)
	d := make([]zero, n)
	return len(c) + len(d)
}

// Direct return or storage exposes an independently owned slice.
func returned(n int) []int {
	a := make([]int, n)
	b := make([]int, n)
	a[0], b[0] = 1, 2
	return a
}

func storedSlice(n int) int {
	a := make([]int, n)
	b := make([]int, n)
	stored = a
	storedMany = append(storedMany, b)
	return a[0] + b[0]
}

// Calls and aliases cross the local ownership proof boundary.
func escaped(n int) int {
	a := make([]int, n)
	b := make([]int, n)
	consume(a)
	alias := b
	return alias[0]
}

// append, cap, and reslicing expose lane capacity or create aliases.
func capSensitive(n int) int {
	a := make([]int, n)
	b := make([]int, n)
	a = append(a, 1)
	x := cap(b)
	y := b[:1]
	return a[0] + x + y[0]
}

// An element address retains the backing object and can escape it.
func elementAddress(n int) *int {
	a := make([]int, n)
	b := make([]int, n)
	a[0], b[0] = 1, 2
	return &(a[0])
}

// Captures are outside the direct, synchronous-use proof.
func captured(n int) func() int {
	a := make([]int, n)
	b := make([]int, n)
	a[0], b[0] = 1, 2
	return func() int { return a[0] + b[0] }
}

// Tuple declarations are intentionally outside the bounded syntax.
func tuple(n int) int {
	a, b := make([]int, n), make([]int, n)
	a[0], b[0] = 1, 2
	return a[0] + b[0]
}

// Slicing an array-valued element retains the outer lane's backing storage;
// storing those views would couple otherwise independent lifetimes.
func projectedEscape(n int) int {
	a := make([]cell, n)
	b := make([]cell, n)
	stored = a[0][:]
	stored = b[0][:]
	return a[0][0] + b[0][0]
}

// A pointer-receiver method call on a[i] implicitly takes its address and can
// retain the complete lane backing object.
func implicitPointerReceiver(n int) int {
	a := make([]retaining, n)
	b := make([]retaining, n)
	a[0].retain()
	b[0].retain()
	return 0
}

// Even direct value projections are rejected at storage, return, and send
// boundaries: their ownership is outside the deliberately local proof.
func storedIndex(n int) int {
	a := make([]int, n)
	b := make([]int, n)
	storedValue = a[0]
	storedValue = b[0]
	return storedValue
}

func returnedIndex(n int) (int, int) {
	a := make([]int, n)
	b := make([]int, n)
	a[0], b[0] = 1, 2
	return a[0], b[0]
}

func sentIndex(n int, ch chan<- int) {
	a := make([]int, n)
	b := make([]int, n)
	ch <- a[0]
	ch <- b[0]
}

// Source-proven dead branches and statements after terminal transfers do not
// represent executable backing-object work.
func unreachableFalse(n int) int {
	if false {
		a := make([]int, n)
		b := make([]int, n)
		a[0], b[0] = 1, 2
		return a[0] + b[0]
	}
	return 0
}

func unreachableAfterReturn(n int) int {
	return n
	a := make([]int, n)
	b := make([]int, n)
	a[0], b[0] = 1, 2
	return a[0] + b[0]
}

func unreachableAfterPanic(n int) int {
	panic("stop")
	a := make([]int, n)
	b := make([]int, n)
	a[0], b[0] = 1, 2
	return a[0] + b[0]
}
