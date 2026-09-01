package checks

import "testing"

func TestPS6101CallDepthLimitIsBalancedAndBounded(t *testing.T) {
	t.Parallel()
	engine := &ps6101Engine{callDepth: ps6101CallDepthLimit - 1}
	if !engine.enterCall() {
		t.Fatal("last bounded local-call frame was rejected")
	}
	if engine.enterCall() {
		t.Fatal("local-call frame beyond the analysis bound was accepted")
	}
	if engine.callDepth != ps6101CallDepthLimit {
		t.Fatalf("rejected frame changed depth to %d", engine.callDepth)
	}
	engine.leaveCall()
	if engine.callDepth != ps6101CallDepthLimit-1 {
		t.Fatalf("leaving bounded frame produced depth %d", engine.callDepth)
	}
}
