package checks

import (
	"strings"
	"testing"
)

func TestPS6141ReviewConstantControlFlow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, prefix string
		want         int
	}{
		{"returnBeforeCandidate", "if true { return 0 };", 0},
		{"panicBeforeCandidate", "if true { panic(\"stop\") };", 0},
		{"infiniteBeforeCandidate", "for true {};", 0},
		{"conditionalReturnStillLive", "if n > 0 { return 0 };", 1},
		{"falseReturnStillLive", "if false { return 0 };", 1},
		{"trueLoopBreakStillLive", "for true { break };", 1},
		{"trueLoopDeadBreak", "for true { if false { break } };", 0},
		{"labelledBreakStillLive", "loop: for true { break loop };", 1},
		{"gotoPastConstantReturn", "goto live; if true { return 0 };live:", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := strings.Replace(ps6141TwoInputSynthetic, "p:=pack(x)", tc.prefix+"p:=pack(x)", 1)
			if source == ps6141TwoInputSynthetic {
				t.Fatal("review mutation did not change owner")
			}
			pass, owner := ps6141TypedFixture(t, source)
			contract := ps6141TestContract()
			contract.ConsumerForm = "twoInputDot"
			contract.WeightArgument = 1
			contract.RowsArgument = -1
			if got := len(ps6141Candidates(pass, owner, &contract)); got != tc.want {
				t.Fatalf("candidate count=%d want %d", got, tc.want)
			}
		})
	}
}
