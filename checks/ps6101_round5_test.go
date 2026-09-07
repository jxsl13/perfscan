package checks

import "testing"

func TestPS6101ExactLoopTransferBudgetIsSharedAndBounded(t *testing.T) {
	t.Parallel()
	work := func(requested int) int {
		t.Helper()
		budget := &ps6101LoopTransferBudget{
			remaining: ps6101ExactLoopTransferLimit, abstractRemaining: ps6101ExactLoopTransferLimit,
		}
		outer := &ps6101Engine{loopTransfers: budget}
		inner := &ps6101Engine{loopTransfers: budget}
		for index := 0; index < requested; index++ {
			engine := outer
			if index%2 != 0 {
				engine = inner
			}
			engine.takeExactLoopTransfer()
		}
		if budget.remaining != ps6101ExactLoopTransferLimit-budget.spent {
			t.Fatalf("loop transfer accounting = remaining %d, spent %d", budget.remaining, budget.spent)
		}
		return budget.spent
	}
	if got := work(3 * 3 * 3); got != 27 {
		t.Fatalf("small exact transfer work = %d, want 27", got)
	}
	small, large := work(12*12*12), work(24*24*24)
	if small != ps6101ExactLoopTransferLimit || large != small {
		t.Fatalf("nested exact transfer work grew from %d to %d; want shared limit %d", small, large, ps6101ExactLoopTransferLimit)
	}

	abstractBudget := &ps6101LoopTransferBudget{abstractRemaining: ps6101ExactLoopTransferLimit}
	abstractOuter := &ps6101Engine{loopTransfers: abstractBudget}
	abstractInner := &ps6101Engine{loopTransfers: abstractBudget}
	for index := 0; index < 3*ps6101ExactLoopTransferLimit; index++ {
		engine := abstractOuter
		if index%2 != 0 {
			engine = abstractInner
		}
		engine.takeAbstractLoopTransfer()
	}
	if abstractBudget.abstractSpent != ps6101ExactLoopTransferLimit || abstractBudget.abstractRemaining != 0 {
		t.Fatalf(
			"nested abstract transfer work = spent %d, remaining %d; want spent %d, remaining 0",
			abstractBudget.abstractSpent, abstractBudget.abstractRemaining, ps6101ExactLoopTransferLimit,
		)
	}
}
