package checks

import (
	"strings"
	"testing"
)

func TestPS6141ContributionReview(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, old, replacement string
		want                   int
	}{
		{"genuinePrerequisite", "", "", 1},
		{"zeroLaneUpdate", "block+=float32(p[bi].Qs[li])*float32(w[bi].Qs[li])", "block+=0*float32(p[bi].Qs[li])*float32(w[bi].Qs[li])", 0},
		{"zeroOuterUpdate", "sum+=block*p[bi].Scale*w[bi].Scale", "sum+=0*block*p[bi].Scale*w[bi].Scale", 0},
		{"compoundErase", "return sum", "sum*=0;return sum", 0},
		{"integerMaskErase", "return sum", "return float32(int(sum)&0)", 0},
		{"constantTruePanicGuard", "sum:=float32(0)", "if true{panic(\"stop\")};sum:=float32(0)", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := ps6141LanesSynthetic
			if tc.old != "" {
				source = strings.Replace(source, tc.old, tc.replacement, 1)
				if source == ps6141LanesSynthetic {
					t.Fatal("mutation missing")
				}
			}
			pass, owner := ps6141TypedFixture(t, source)
			c := ps6141TestContract()
			c.ConsumerForm = "sourceSummary"
			c.WeightArgument = 1
			c.RowsArgument = -1
			if got := len(ps6141Candidates(pass, owner, &c)); got != tc.want {
				t.Fatalf("candidates=%d want=%d", got, tc.want)
			}
		})
	}
}
