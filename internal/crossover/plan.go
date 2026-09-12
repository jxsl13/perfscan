package crossover

import (
	"errors"
	"slices"
)

// Invocation is one fresh-process observation, not a subbenchmark aggregate.
// Controls deliberately run the serial selection in both arms of each cell.
type Invocation struct {
	Pair      int    `json:"pair"`
	Size      int    `json:"size"`
	Procs     int    `json:"gomaxprocs"`
	Scope     string `json:"scope"`
	Phase     string `json:"phase"`
	Arm       string `json:"arm"`
	Selection string `json:"selection"`
}

// Schedule predeclares a complete balanced matrix. Shape coverage joins the
// current observed boundary, never a caller's independently chosen threshold.
func Schedule(boundary int64, sizes, procs []int, pairs int) ([]Invocation, error) {
	if boundary < 2 || pairs < 2 || pairs%2 != 0 || pairs > 100 || len(sizes) < 2 || len(sizes) > 32 || len(procs) == 0 || len(procs) > 8 {
		return nil, errors.New("invalid bounded crossover matrix")
	}
	if pairs*len(sizes)*len(procs)*16 > 100000 {
		return nil, errors.New("crossover matrix exceeds bounded invocation budget")
	}
	below, above := false, false
	for i, n := range sizes {
		if n < 1 || slices.Contains(sizes[:i], n) {
			return nil, errors.New("duplicate or invalid shape")
		}
		below = below || int64(n) < boundary
		above = above || int64(n) >= boundary
	}
	if !below || !above {
		return nil, errors.New("refreshed evidence must cover below and at/above the observed boundary")
	}
	for i, p := range procs {
		if p < 2 || p > 256 || slices.Contains(procs[:i], p) {
			return nil, errors.New("parallel evidence requires distinct GOMAXPROCS >= 2")
		}
	}
	result := make([]Invocation, 0, pairs*len(sizes)*len(procs)*4*4)
	for pair := 1; pair <= pairs; pair++ {
		shapeOrder := slices.Clone(sizes)
		procOrder := slices.Clone(procs)
		scopes := []string{"policy", "forced-operation", "production", "original-benchmark"}
		phases := []string{"control", "candidate"}
		arms := []string{"serial", "parallel"}
		if pair%2 == 0 {
			slices.Reverse(shapeOrder)
			slices.Reverse(procOrder)
			slices.Reverse(scopes)
			slices.Reverse(phases)
			slices.Reverse(arms)
		}
		for _, size := range shapeOrder {
			for _, p := range procOrder {
				for _, scope := range scopes {
					for _, phase := range phases {
						for _, arm := range arms {
							selection := arm
							if phase == "control" {
								selection = "serial"
							}
							result = append(result, Invocation{Pair: pair, Size: size, Procs: p, Scope: scope, Phase: phase, Arm: arm, Selection: selection})
						}
					}
				}
			}
		}
	}
	return result, nil
}
