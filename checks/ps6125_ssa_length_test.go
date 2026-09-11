package checks

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6125SSALengthStability(t *testing.T) {
	t.Parallel()
	const source = `package extent
func produce(rows int) {}
func sliceLength(value []int) { value[0] = 7; produce(len(value)) }
func stringLength(value string) { produce(len(value)) }
func mapLength(value map[int]int) { delete(value, 0); produce(len(value)) }
func channelLength(value chan int) { <-value; produce(len(value)) }
`
	for _, test := range []struct {
		name  string
		known bool
	}{
		{"sliceLength", true}, {"stringLength", true},
		{"mapLength", false}, {"channelLength", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			function := ps6125TestSSA(t, source).Func(test.name)
			var pool ps6125ExtentPool
			length := ps6125SymbolicExtent(pool.identity(function.Params[0].Object(), nil, true))
			flow := ps6125AnalyzeSSAExtents(function, nil, map[*ssa.Parameter]ps6125Extent{function.Params[0]: length})
			found := 0
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					call, ok := instruction.(*ssa.Call)
					if !ok || call.Call.StaticCallee() == nil || call.Call.StaticCallee().Name() != "produce" {
						continue
					}
					fact := flow.scalar(call.Call.Args[0])
					if test.known {
						if fact.state != ps6125Integer || !ps6125SameExtent(fact.extent, length) {
							t.Fatal("stable descriptor length was lost")
						}
					} else if fact.state != ps6125Unknown {
						t.Fatal("mutable object length reused an entry fact after effects")
					}
					found++
				}
			}
			if found != 1 {
				t.Fatalf("found %d length observations, want one", found)
			}
		})
	}
}
