package crossover

import (
	"strings"
	"testing"
)

func TestExactResultIdentifiersDoNotRewriteNeighborOrSelector(t *testing.T) {
	t.Parallel()
	result, err := replaceResultIdentifiers("len(_r0._r0) + len(_result) + len(_r00)", map[string]string{"_r0": "_s0"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "_s0._r0") || !strings.Contains(result, "_result") || !strings.Contains(result, "_r00") {
		t.Fatalf("rewrote non-result identifier: %s", result)
	}
}
