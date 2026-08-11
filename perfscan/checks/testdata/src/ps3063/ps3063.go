package ps3063

func parallelFor(n int, f func(int)) {
	for i := 0; i < n; i++ {
		f(i)
	}
}

func buildBasis(i int)     {}
func fusedSpline(i, j int) {}
func work(i int)           {}

func forward(n, m int) {
	parallelFor(n, buildBasis)
	for i := 0; i < n; i++ { // want `this nest runs serial in a function that already fans out elsewhere — the parallel transform is proven available; check the writes own disjoint output, then band the outer loop and gate with BOTH a bit-exact digest and -race`
		for j := 0; j < m; j++ {
			fusedSpline(i, j)
		}
	}
}

// No fan-out call in this function: PS3063 is silent (PS3059/PS3034
// territory, different check).
func serialOnly(n, m int) {
	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			fusedSpline(i, j)
		}
	}
}

// The nest itself fans out: nothing left serial.
func allParallel(n, m int) {
	parallelFor(n, buildBasis)
	for i := 0; i < n; i++ {
		parallelFor(m, work)
	}
}

// A single-level loop is not a nest: silent (PS3065 territory).
func singleLevel(n int) {
	parallelFor(n, buildBasis)
	for i := 0; i < n; i++ {
		work(i)
	}
}
