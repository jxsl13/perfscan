package crossover

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/benchmarkevidence"
)

func sample(n int, bytes, allocs, elapsed int64) benchmarkevidence.Observation {
	return benchmarkevidence.Observation{Allocation: benchmarkevidence.Allocation{N: n, MemBytes: bytes, MemAllocs: allocs, BytesPerOp: bytes / int64(n), BytesRemainder: bytes % int64(n), AllocsPerOp: allocs / int64(n), AllocsRemainder: allocs % int64(n)}, ElapsedNanos: elapsed}
}

func TestReadObservationStrictRawIdentities(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(sample(8, 19, 3, 101))
	if err != nil {
		t.Fatal(err)
	}
	valid := string(encoded) + "\nPASS\n"
	if _, err := ReadObservation([]byte(valid), 8); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, text string
		n          int
	}{
		{"duplicate", strings.Replace(valid, `"elapsedNanos":101`, `"elapsedNanos":101,"elapsedNanos":101`, 1), 8},
		{"wrong_identity", strings.Replace(valid, `"bytesRemainder":3`, `"bytesRemainder":0`, 1), 8},
		{"missing_allocs", strings.Replace(valid, `"memAllocs":3,`, "", 1), 8},
		{"failed", strings.Replace(valid, "PASS", "FAIL", 1), 8}, {"extra_line", valid + "extra\n", 8}, {"fixed_work_mismatch", valid, 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ReadObservation([]byte(tc.text), tc.n); err == nil {
				t.Fatalf("accepted invalid raw observation: %s", tc.text)
			}
		})
	}
}

func TestCompareAdaptiveDoesNotInventMatchedTotals(t *testing.T) {
	t.Parallel()
	adaptive, err := Compare(sample(8, 19, 3, 101), sample(16, 41, 7, 201))
	if err != nil {
		t.Fatal(err)
	}
	if adaptive.MatchedRawTotals || adaptive.RawBytesDelta != 0 || adaptive.RawAllocsDelta != 0 || adaptive.Caution == "" || adaptive.BytesPerOpDifference != "3/16" || adaptive.NanosPerOpDifference != "-1/16" {
		t.Fatalf("adaptive totals misrepresented: %+v", adaptive)
	}
	fixed, err := Compare(sample(8, 19, 3, 101), sample(8, 41, 7, 201))
	if err != nil {
		t.Fatal(err)
	}
	if !fixed.MatchedRawTotals || fixed.RawBytesDelta != 22 || fixed.RawAllocsDelta != 4 || fixed.Caution != "" {
		t.Fatalf("fixed paired totals lost: %+v", fixed)
	}
}
