package ps3083

type key struct{ block int64 }

func topBlocks(q float64) []int64 { return nil }

func use(bool) {}

func perPassMap(queries []float64, keys []key, k int) {
	for _, q := range queries {
		sel := make(map[int64]bool, k) // want `int-keyed map sel is allocated every outer iteration and probed in a nested loop; for dense keys a slice with a generation-counter stamp removes the per-pass allocation and the hash per probe`
		for _, b := range topBlocks(q) {
			sel[b] = true
		}
		for _, kk := range keys {
			use(sel[kk.block])
		}
	}
}

// Built per pass but only written in the nested loop: the build is not a
// probe, silent.
func buildOnly(queries []float64) map[int64]bool {
	var last map[int64]bool
	for _, q := range queries {
		sel := make(map[int64]bool)
		for _, b := range topBlocks(q) {
			sel[b] = true
		}
		last = sel
	}
	return last
}

// Allocated once outside the loop: silent (PS3003 may still apply to the
// probe itself).
func hoisted(queries []float64, keys []key) {
	sel := make(map[int64]bool)
	for _, q := range queries {
		for _, b := range topBlocks(q) {
			sel[b] = true
		}
		for _, kk := range keys {
			use(sel[kk.block])
		}
	}
}

// String keys have no dense range to densify: silent.
func stringKeys(queries []float64, names []string) {
	for range queries {
		sel := make(map[string]bool)
		for _, n := range names {
			sel[n] = true
		}
		for _, n := range names {
			use(sel[n])
		}
	}
}
