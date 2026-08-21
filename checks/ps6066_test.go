package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6066(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6066.Analyzer, "ps6066")
}

func TestPS6066AffineArithmeticRejectsOverflow(t *testing.T) {
	const (
		maxInt64 = int64(1<<63 - 1)
		minInt64 = int64(-1 << 63)
	)
	if _, ok := ps6066SafeAdd(maxInt64, 1); ok {
		t.Fatal("positive affine addition overflow must be rejected")
	}
	if _, ok := ps6066SafeAdd(minInt64, -1); ok {
		t.Fatal("negative affine addition overflow must be rejected")
	}
	if _, ok := ps6066SafeMul(maxInt64, 2); ok {
		t.Fatal("affine multiplication overflow must be rejected")
	}
	if _, ok := ps6066SafeMul(minInt64, -1); ok {
		t.Fatal("minInt64 * -1 overflow must be rejected")
	}
	if value, ok := ps6066SafeMul(-7, -9); !ok || value != 63 {
		t.Fatalf("ordinary affine multiplication = (%d, %t), want (63, true)", value, ok)
	}
}
