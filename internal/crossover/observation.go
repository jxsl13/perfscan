package crossover

import (
	"bytes"
	"errors"
	"math/big"

	"github.com/jxsl13/perfscan/benchmarkevidence"
	"github.com/jxsl13/perfscan/internal/allocationcampaign"
)

// ReadObservation rejects failed/noisy wrapper output and rounded aggregate
// records. Native timing and exact allocations are distinct retained fields.
func ReadObservation(stdout []byte, fixedN int) (benchmarkevidence.Observation, error) {
	var result benchmarkevidence.Observation
	lines := bytes.Split(bytes.TrimSpace(stdout), []byte{'\n'})
	if len(lines) != 2 || string(bytes.TrimSpace(lines[1])) != "PASS" {
		return result, errors.New("diagnostic must emit exactly one native record and PASS")
	}
	if err := allocationcampaign.Decode(lines[0], &result); err != nil {
		return result, err
	}
	if result.Allocation.N < 2 || result.ElapsedNanos <= 0 || fixedN < 0 || fixedN == 1 || fixedN > 0 && result.Allocation.N != fixedN {
		return result, errors.New("invalid native iteration count or elapsed timing")
	}
	if _, err := benchmarkevidence.PairedDelta(result.Allocation, result.Allocation); err != nil {
		return result, err
	}
	return result, nil
}

type Comparison struct {
	NanosPerOpDifference  string `json:"nanosPerOpDifference"`
	BytesPerOpDifference  string `json:"bytesPerOpDifference"`
	AllocsPerOpDifference string `json:"allocsPerOpDifference"`
	MatchedRawTotals      bool   `json:"matchedRawTotals"`
	RawBytesDelta         int64  `json:"rawBytesDelta"`
	RawAllocsDelta        int64  `json:"rawAllocsDelta"`
	Caution               string `json:"caution"`
}

// Compare preserves adaptive rawN and compares exact rational normalized
// quantities. Unequal-N adaptive totals are never labelled matched paired
// deltas: allocation-heavy/GC-cadence comparisons should be rerun fixed-work.
func Compare(a, b benchmarkevidence.Observation) (Comparison, error) {
	for _, sample := range []benchmarkevidence.Observation{a, b} {
		if sample.Allocation.N < 2 || sample.ElapsedNanos <= 0 {
			return Comparison{}, errors.New("invalid native observation")
		}
		if _, err := benchmarkevidence.PairedDelta(sample.Allocation, sample.Allocation); err != nil {
			return Comparison{}, err
		}
	}
	difference := func(at int64, an int, bt int64, bn int) string {
		left := new(big.Rat).SetFrac(big.NewInt(at), big.NewInt(int64(an)))
		right := new(big.Rat).SetFrac(big.NewInt(bt), big.NewInt(int64(bn)))
		return right.Sub(right, left).RatString()
	}
	result := Comparison{NanosPerOpDifference: difference(a.ElapsedNanos, a.Allocation.N, b.ElapsedNanos, b.Allocation.N), BytesPerOpDifference: difference(a.Allocation.MemBytes, a.Allocation.N, b.Allocation.MemBytes, b.Allocation.N), AllocsPerOpDifference: difference(a.Allocation.MemAllocs, a.Allocation.N, b.Allocation.MemAllocs, b.Allocation.N)}
	if a.Allocation.N == b.Allocation.N {
		delta, err := benchmarkevidence.PairedDelta(a.Allocation, b.Allocation)
		if err != nil {
			return result, err
		}
		result.MatchedRawTotals = true
		result.RawBytesDelta = delta.MemBytes
		result.RawAllocsDelta = delta.MemAllocs
	} else {
		result.Caution = "adaptive unequal-N totals are not paired fixed-work deltas; rerun allocation-heavy operations with fixedN to check GC cadence"
	}
	return result, nil
}
