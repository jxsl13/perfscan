package ps6072

type Tensor struct{}

//go:noinline
func (t *Tensor) atRank1(i int) float64 { return float64(i) }

//go:noinline
func (t *Tensor) atRank2(i, j int) float64 { return float64(i + j) }

//go:noinline
func (t *Tensor) atRankN(index []int) float64 { return float64(len(index)) }

func (t *Tensor) AtF64(index ...int) float64 { // want `AtF64 is called from 1 loop call site\(s\) and dispatches through 3 call site\(s\) to 3 distinct //go:noinline specialization targets; Go 1.26's inliner charges 57 per non-inlineable call, so the call-only lower bound is 171 > the default budget 80 before switch/body cost`
	switch len(index) {
	case 1:
		return t.atRank1(index[0])
	case 2:
		return t.atRank2(index[0], index[1])
	default:
		return t.atRankN(index)
	}
}

func Sum(t *Tensor, n int) float64 {
	var total float64
	for i := 0; i < n; i++ {
		total += t.AtF64(i, i)
	}
	return total
}

//go:noinline
func decodeScalar(v int) int { return v }

//go:noinline
func decodeVector(v int) int { return v + 1 }

func decodeFast(vector bool, v int) int { // want `decodeFast is called from 2 loop call site\(s\) and dispatches through 2 call site\(s\) to 2 distinct //go:noinline specialization targets; Go 1.26's inliner charges 57 per non-inlineable call, so the call-only lower bound is 114 > the default budget 80 before switch/body cost`
	if vector {
		return decodeVector(v)
	} else if v >= 0 {
		return decodeScalar(v)
	}
	return v
}

func decodeAll(values []int) int {
	total := 0
	for _, value := range values {
		total += decodeFast(true, value)
		total += decodeFast(false, value)
	}
	return total
}

// Cold specialization wrappers stay silent.
func coldRank(rank int) int {
	if rank == 1 {
		return decodeScalar(rank)
	}
	return decodeVector(rank)
}

func oneNoinlineRank(rank int) int {
	if rank == 1 {
		return decodeScalar(rank)
	}
	return rank + 1
}

func useOneNoinlineRank(n int) {
	for i := 0; i < n; i++ {
		_ = oneNoinlineRank(i)
	}
}

func sequentialFast(v int) int {
	a := decodeScalar(v)
	return a + decodeVector(v)
}

func useSequentialFast(n int) {
	for i := 0; i < n; i++ {
		_ = sequentialFast(i)
	}
}

//go:noinline
func left(v int) int { return v }

//go:noinline
func right(v int) int { return -v }

// A generic branch dispatcher without specialization evidence stays silent.
func choose(flag bool, v int) int {
	if flag {
		return left(v)
	}
	return right(v)
}

func useChoose(n int) {
	for i := 0; i < n; i++ {
		_ = choose(i%2 == 0, i)
	}
}

//perfscan:inline-budget-validated same-binary rank campaign passed.
func validatedRank(flag bool, v int) int {
	if flag {
		return decodeScalar(v)
	}
	return decodeVector(v)
}

func useValidatedRank(n int) {
	for i := 0; i < n; i++ {
		_ = validatedRank(i%2 == 0, i)
	}
}

func capturedFast(flag bool, v int) int {
	if flag {
		return decodeScalar(v)
	}
	return decodeVector(v)
}

func captureInsideLoop(n int) []func() {
	callbacks := make([]func(), 0, n)
	for i := 0; i < n; i++ {
		callbacks = append(callbacks, func() { _ = capturedFast(true, i) })
	}
	return callbacks
}

func rangeExpressionFast(flag bool, v int) int {
	if flag {
		return decodeScalar(v)
	}
	return decodeVector(v)
}

func rangeExpressionOnce() {
	for range rangeExpressionFast(true, 4) {
	}
}

func initFast(flag bool, v int) int {
	if flag {
		return decodeScalar(v)
	}
	return decodeVector(v)
}

func forInitializerOnce() {
	for i := initFast(true, 0); i < 4; i++ {
	}
}

// go:noinline is ordinary prose, not a compiler directive.
func proseScalar(v int) int { return v }

// go:noinline is ordinary prose, not a compiler directive.
func proseVector(v int) int { return v }

func proseFast(flag bool, v int) int {
	if flag {
		return proseScalar(v)
	}
	return proseVector(v)
}

func useProseFast(n int) {
	for i := 0; i < n; i++ {
		_ = proseFast(i%2 == 0, i)
	}
}

func indirectFast(flag bool, v int) int {
	fn := decodeScalar
	if flag {
		fn = decodeVector
	}
	return fn(v)
}

func useIndirectFast(n int) {
	for i := 0; i < n; i++ {
		_ = indirectFast(i%2 == 0, i)
	}
}
