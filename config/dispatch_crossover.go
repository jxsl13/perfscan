package config

import (
	"encoding/hex"
	"slices"
	"strings"
)

// DispatchCrossoverContract selects one measured, operation-specific dispatch.
// Names select source objects; they do not attest hotness or measurement facts.
// PS6131 independently proves the threshold, route and implementation closure.
type DispatchCrossoverContract struct {
	DispatchFunction  string `json:"dispatchFunction" yaml:"dispatchFunction"`
	ThresholdConstant string `json:"thresholdConstant" yaml:"thresholdConstant"`
	SerialPolicy      string `json:"serialPolicy" yaml:"serialPolicy"`
	ParallelPolicy    string `json:"parallelPolicy" yaml:"parallelPolicy"`
	// Optional direct-slice policy wrappers. The allocated Tensor/context route
	// is observed directly and does not require invented wrapper declarations.
	SerialOperation       string   `json:"serialOperation" yaml:"serialOperation"`
	ParallelOperation     string   `json:"parallelOperation" yaml:"parallelOperation"`
	WorkerRunner          string   `json:"workerRunner" yaml:"workerRunner"`
	LeafKernels           []string `json:"leafKernels" yaml:"leafKernels"`
	DiagnosticFunction    string   `json:"diagnosticFunction" yaml:"diagnosticFunction"`
	OperationInputFactory string   `json:"operationInputFactory" yaml:"operationInputFactory"`
	// ProductionBenchmark is the original unmodified full-operation benchmark
	// path, with exactly one {n} shape placeholder (for example BenchCPU/n{n}).
	// Its raw printed evidence is separate from forced-route instrumentation.
	ProductionBenchmark string `json:"productionBenchmark" yaml:"productionBenchmark"`
	// Campaign selects retained controlled-build and raw execution artifacts.
	Campaign         string `json:"campaign" yaml:"campaign"`
	PlanSHA256       string `json:"planSHA256" yaml:"planSHA256"`
	RecordsSHA256    string `json:"recordsSHA256" yaml:"recordsSHA256"`
	EvidenceGoBinary string `json:"evidenceGoBinary" yaml:"evidenceGoBinary"`
}

func cloneDispatchCrossoverContracts(values []DispatchCrossoverContract) []DispatchCrossoverContract {
	if values == nil {
		return nil
	}
	result := make([]DispatchCrossoverContract, len(values))
	copy(result, values)
	for i := range result {
		result[i].LeafKernels = slices.Clone(values[i].LeafKernels)
	}
	return result
}

func UsableDispatchCrossoverContractCount(values []DispatchCrossoverContract) int {
	count := 0
	names := make(map[string]int, len(values))
	for i := range values {
		names[values[i].DispatchFunction]++
	}
	for i := range values {
		c := &values[i]
		_, planErr := hex.DecodeString(c.PlanSHA256)
		_, recordsErr := hex.DecodeString(c.RecordsSHA256)
		if c.Valid() && names[c.DispatchFunction] == 1 && len(c.PlanSHA256) == 64 && len(c.RecordsSHA256) == 64 && planErr == nil && recordsErr == nil {
			count++
		}
	}
	return count
}

// Valid rejects ambiguous or incomplete selectors, not unmeasured runtime facts.
func (c DispatchCrossoverContract) Valid() bool { //perfscan:ignore PS3106 preserve the public config value API
	for _, name := range []string{c.DispatchFunction, c.ThresholdConstant, c.SerialPolicy, c.ParallelPolicy, c.WorkerRunner, c.OperationInputFactory, c.DiagnosticFunction} {
		if !psTopKFunctionIDValid(name) {
			return false
		}
	}
	for _, name := range []string{c.SerialOperation, c.ParallelOperation} {
		if name != "" && !psTopKFunctionIDValid(name) {
			return false
		}
	}
	if !strings.HasPrefix(c.ProductionBenchmark, "Benchmark") || strings.Count(c.ProductionBenchmark, "{n}") != 1 {
		return false
	}
	if len(c.LeafKernels) == 0 || c.Campaign == "" || strings.TrimSpace(c.Campaign) != c.Campaign {
		return false
	}
	for _, name := range c.LeafKernels {
		if !psTopKFunctionIDValid(name) {
			return false
		}
	}
	return true
}
