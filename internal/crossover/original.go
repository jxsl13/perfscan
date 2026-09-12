package crossover

import (
	"errors"
	"math/big"
	"strconv"
	"strings"
)

type PrintedBenchmark struct {
	N           int               `json:"actualN"`
	NanosPerOp  string            `json:"printedNanosPerOp"`
	BytesPerOp  int64             `json:"printedRoundedBytesPerOp"`
	AllocsPerOp int64             `json:"printedRoundedAllocsPerOp"`
	Metrics     map[string]string `json:"printedCustomMetrics"`
}

type OriginalSample struct {
	Invocation Invocation       `json:"invocation"`
	Printed    PrintedBenchmark `json:"printedObservation"`
}

// ReadOriginal keeps the original production benchmark separate: its printed
// allocation quotients do NOT establish exact MemBytes/MemAllocs totals.
func ReadOriginal(stdout []byte, pattern string, size, procs, fixedN int) (PrintedBenchmark, error) {
	var result PrintedBenchmark
	if strings.Count(pattern, "{n}") != 1 {
		return result, errors.New("invalid original benchmark shape selector")
	}
	name := strings.Replace(pattern, "{n}", strconv.Itoa(size), 1) + "-" + strconv.Itoa(procs)
	rows, passes := 0, 0
	for line := range strings.SplitSeq(strings.TrimSpace(string(stdout)), "\n") {
		line = strings.TrimSpace(line)
		if line == "PASS" {
			passes++
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if strings.HasPrefix(fields[0], "Benchmark") {
			if fields[0] != name || len(fields) < 8 || len(fields)%2 != 0 {
				return result, errors.New("original benchmark emitted unexpected row or columns")
			}
			rows++
			n, err := strconv.Atoi(fields[1])
			if err != nil || n < 2 || fixedN > 0 && n != fixedN {
				return result, errors.New("original benchmark actualN invalid")
			}
			columns := make(map[string]string, len(fields)/2-1)
			for i := 2; i < len(fields); i += 2 {
				unit := fields[i+1]
				if _, ok := columns[unit]; ok {
					return result, errors.New("duplicated original benchmark metric")
				}
				if unit != "ns/op" && unit != "B/op" && unit != "allocs/op" && unit != "GB/s" && unit != "MB/s" {
					return result, errors.New("unsupported original benchmark metric")
				}
				columns[unit] = fields[i]
			}
			nanos, ok := new(big.Rat).SetString(columns["ns/op"])
			if !ok || nanos.Sign() <= 0 {
				return result, errors.New("original benchmark timing invalid")
			}
			bytes, err := strconv.ParseInt(columns["B/op"], 10, 64)
			if err != nil || bytes < 0 {
				return result, errors.New("original benchmark allocation quotient invalid")
			}
			allocs, err := strconv.ParseInt(columns["allocs/op"], 10, 64)
			if err != nil || allocs < 0 {
				return result, errors.New("original benchmark allocation count invalid")
			}
			metrics := make(map[string]string, 2)
			for _, unit := range []string{"GB/s", "MB/s"} {
				if value, ok := columns[unit]; ok {
					number, ok := new(big.Rat).SetString(value)
					if !ok || number.Sign() < 0 {
						return result, errors.New("invalid original benchmark throughput")
					}
					metrics[unit] = value
				}
			}
			result = PrintedBenchmark{N: n, NanosPerOp: columns["ns/op"], BytesPerOp: bytes, AllocsPerOp: allocs, Metrics: metrics}
			continue
		}
		if !strings.HasPrefix(line, "goos: ") && !strings.HasPrefix(line, "goarch: ") && !strings.HasPrefix(line, "pkg: ") && !strings.HasPrefix(line, "cpu: ") {
			return result, errors.New("original benchmark emitted failure or unexpected output")
		}
	}
	if rows != 1 || passes != 1 {
		return result, errors.New("original benchmark matrix cell is missing or duplicated")
	}
	return result, nil
}
