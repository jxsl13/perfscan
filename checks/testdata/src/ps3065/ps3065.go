package ps3065

func parallelFor(n int, f func(int)) {
	for i := 0; i < n; i++ {
		f(i)
	}
}

// kernelColumn loops over rows: expensive per item.
func kernelColumn(x []float64, j int) float64 {
	s := 0.0
	for _, v := range x {
		s += v * float64(j)
	}
	return s
}

func cheap(j int) float64 { return float64(j) }

func fill(cache []float64, x []float64) {
	for j := range cache { // want `each iteration writes its own slot from kernelColumn, which itself loops — depth-1 but expensive per item, and a fan-out helper exists in this package; band the loop \(bit-identical; expect Amdahl, gate with -race\)`
		cache[j] = kernelColumn(x, j)
	}
}

// The callee does not loop: cheap per item, silent.
func fillCheap(cache []float64) {
	for j := range cache {
		cache[j] = cheap(j)
	}
}

// The function already fans out: silent.
func fillParallel(cache []float64, x []float64) {
	parallelFor(len(cache), func(j int) {
		cache[j] = kernelColumn(x, j)
	})
}
