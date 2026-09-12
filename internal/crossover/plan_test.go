package crossover

import "testing"

func TestScheduleCompleteBoundaryControlsAndBalancedOrder(t *testing.T) {
	t.Parallel()
	matrix, err := Schedule(16, []int{8, 16}, []int{2, 4}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(matrix) != 128 {
		t.Fatalf("incomplete matrix: %d", len(matrix))
	}
	for _, sample := range matrix {
		if sample.Phase == "control" && sample.Selection != "serial" {
			t.Fatal("control changed binary selection")
		}
		if sample.Phase == "candidate" && sample.Selection != sample.Arm {
			t.Fatal("candidate arm lost")
		}
	}
	first, last := matrix[0], matrix[len(matrix)-1]
	if first.Pair != 1 || last.Pair != 2 || first.Size != last.Size || first.Procs != last.Procs || first.Scope != last.Scope || first.Phase != last.Phase || first.Arm != last.Arm {
		t.Fatalf("orders are not paired reversals: %+v %+v", first, last)
	}
}

func TestScheduleRejectsIncompleteOrUnbalancedRefresh(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		sizes, procs []int
		pairs        int
	}{
		{"below_only", []int{4, 8}, []int{2}, 2}, {"above_only", []int{16, 32}, []int{2}, 2}, {"one_shape", []int{16}, []int{2}, 2}, {"one_processor", []int{8, 16}, []int{1}, 2}, {"odd_pairs", []int{8, 16}, []int{2}, 3}, {"duplicate_shape", []int{8, 8, 16}, []int{2}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Schedule(16, tc.sizes, tc.procs, tc.pairs); err == nil {
				t.Fatal("accepted incomplete refreshed evidence")
			}
		})
	}
}
