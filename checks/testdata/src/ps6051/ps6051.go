package ps6051

import "testing"

type metadataBroadcastExperiment struct {
	Hardware                           string
	ToolchainIdentity                  string
	CandidateDefaultOff                bool
	WarmupCount                        int
	TimedIterationCount                int
	UnchangedLaunchGeometry            bool
	UnchangedQuantLoads                bool
	UnchangedActivationLoads           bool
	UnchangedAccumulation              bool
	UnchangedOutputMapping             bool
	SubgroupWidth                      int
	PrimaryLeaderFanout                int
	SecondaryLeaderFanout              int
	SharedPayloadBytes                 int
	BytesSavedPerSubgroupIteration     int
	ShuffleCountPerLane                int
	BandwidthModel                     string
	SourceAddressUniformity            string
	OriginalLoadsCoalesced             bool
	EstimatedCacheResident             bool
	ReuseDistanceBytes                 int
	DivergenceAdded                    bool
	NumericalOrderContractionSensitive bool
	IncidentShapes                     []string
	IncidentControlLatenciesNS         []float64
	IncidentCandidateLatenciesNS       []float64
	IncidentSpeedups                   []float64
	GeometricMeanSpeedup               float64
	RegressionCount                    int
	ControlAllocationBytes             int
	CandidateAllocationBytes           int
	ControlAllocationCount             int
	CandidateAllocationCount           int
	ExactOddTailRequired               bool
	OddTailExactParity                 bool
	OddTailMismatchCount               int
	OddTailULPClass                    int
	PerformanceMinimum                 float64
	PerformanceVerdict                 string
	CorrectnessVerdict                 string
	TransformationRecommended          bool
	Classification                     string
	FinalDecision                      string
}

func runMetalGPUCrossLaneShuffleMetadataBandwidth() {}

func TestMetalGPUCrossLaneShuffleMetadataBandwidthMissing(t *testing.T) { // want `GPU cross-lane metadata broadcast has no bandwidth manifest`
	runMetalGPUCrossLaneShuffleMetadataBandwidth()
}

func TestMetalGPUCrossLaneShuffleMetadataBandwidthIncomplete(t *testing.T) {
	evidence := metadataBroadcastExperiment{ // want `GPU shuffle bandwidth evidence is incomplete; missing toolchain identity, candidate default-off status, warmup count`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalGPUCrossLaneShuffleMetadataBandwidth()
}

func TestMetalGPUCrossLaneShuffleMetadataBandwidthOverrecommend(t *testing.T) {
	evidence := metadataBroadcastExperiment{ // want `performance verdict "pass" disagrees with computed gate; correctness verdict "pass" disagrees with computed gate; cross-lane shuffle transformation is recommended despite cache-local payload or failed performance/correctness gates; final decision "retained" retains a shuffle candidate that failed a gate`
		Hardware:                           "Apple M2 Pro",
		ToolchainIdentity:                  "Xcode 26.6 Metal",
		CandidateDefaultOff:                true,
		WarmupCount:                        20,
		TimedIterationCount:                200,
		UnchangedLaunchGeometry:            true,
		UnchangedQuantLoads:                true,
		UnchangedActivationLoads:           true,
		UnchangedAccumulation:              true,
		UnchangedOutputMapping:             true,
		SubgroupWidth:                      32,
		PrimaryLeaderFanout:                8,
		SecondaryLeaderFanout:              4,
		SharedPayloadBytes:                 8,
		BytesSavedPerSubgroupIteration:     48,
		ShuffleCountPerLane:                6,
		BandwidthModel:                     "tiny cache-local metadata versus six shuffles",
		SourceAddressUniformity:            "uniform within lane group",
		OriginalLoadsCoalesced:             true,
		EstimatedCacheResident:             true,
		ReuseDistanceBytes:                 32,
		DivergenceAdded:                    true,
		NumericalOrderContractionSensitive: true,
		IncidentShapes:                     []string{"K2048,N2048", "K2048,N2560", "K2048,N5632", "K5632,N2048", "K2048,N11008", "K2048,N16384", "K2048,N32000"},
		IncidentControlLatenciesNS:         []float64{223422, 179328, 200208, 198504, 234836, 270058, 368778},
		IncidentCandidateLatenciesNS:       []float64{265786, 174067, 204836, 222494, 274267, 325810, 480710},
		IncidentSpeedups:                   []float64{223422.0 / 265786, 179328.0 / 174067, 200208.0 / 204836, 198504.0 / 222494, 234836.0 / 274267, 270058.0 / 325810, 368778.0 / 480710},
		GeometricMeanSpeedup:               0.8807640902008231,
		RegressionCount:                    6,
		ControlAllocationBytes:             8,
		CandidateAllocationBytes:           8,
		ControlAllocationCount:             1,
		CandidateAllocationCount:           1,
		ExactOddTailRequired:               true,
		OddTailExactParity:                 false,
		OddTailMismatchCount:               1,
		OddTailULPClass:                    1,
		PerformanceMinimum:                 1,
		PerformanceVerdict:                 "pass",
		CorrectnessVerdict:                 "pass",
		TransformationRecommended:          true,
		Classification:                     "metadata-load-optimization",
		FinalDecision:                      "retained",
	}
	_ = evidence
	runMetalGPUCrossLaneShuffleMetadataBandwidth()
}

func TestMetalGPUCrossLaneShuffleMetadataBandwidthStale(t *testing.T) {
	evidence := metadataBroadcastExperiment{ // want `incident speedup for "K2048,N2048" is 2x, want 0.840609x`
		Hardware:                           "Apple M2 Pro",
		ToolchainIdentity:                  "Xcode 26.6 Metal",
		CandidateDefaultOff:                true,
		WarmupCount:                        20,
		TimedIterationCount:                200,
		UnchangedLaunchGeometry:            true,
		UnchangedQuantLoads:                true,
		UnchangedActivationLoads:           true,
		UnchangedAccumulation:              true,
		UnchangedOutputMapping:             true,
		SubgroupWidth:                      32,
		PrimaryLeaderFanout:                8,
		SecondaryLeaderFanout:              4,
		SharedPayloadBytes:                 8,
		BytesSavedPerSubgroupIteration:     48,
		ShuffleCountPerLane:                6,
		BandwidthModel:                     "tiny cache-local metadata versus six shuffles",
		SourceAddressUniformity:            "uniform within lane group",
		OriginalLoadsCoalesced:             true,
		EstimatedCacheResident:             true,
		ReuseDistanceBytes:                 32,
		DivergenceAdded:                    true,
		NumericalOrderContractionSensitive: true,
		IncidentShapes:                     []string{"K2048,N2048", "K2048,N2560", "K2048,N5632", "K5632,N2048", "K2048,N11008", "K2048,N16384", "K2048,N32000"},
		IncidentControlLatenciesNS:         []float64{223422, 179328, 200208, 198504, 234836, 270058, 368778},
		IncidentCandidateLatenciesNS:       []float64{265786, 174067, 204836, 222494, 274267, 325810, 480710},
		IncidentSpeedups:                   []float64{2, 2, 2, 2, 2, 2, 2},
		GeometricMeanSpeedup:               2,
		RegressionCount:                    0,
		ControlAllocationBytes:             8,
		CandidateAllocationBytes:           8,
		ControlAllocationCount:             1,
		CandidateAllocationCount:           1,
		ExactOddTailRequired:               true,
		OddTailExactParity:                 false,
		OddTailMismatchCount:               1,
		OddTailULPClass:                    1,
		PerformanceMinimum:                 1,
		PerformanceVerdict:                 "pass",
		CorrectnessVerdict:                 "fail",
		TransformationRecommended:          false,
		Classification:                     "stale",
		FinalDecision:                      "removed",
	}
	_ = evidence
	runMetalGPUCrossLaneShuffleMetadataBandwidth()
}

func TestMetalGPUCrossLaneShuffleMetadataBandwidthStable(t *testing.T) {
	evidence := metadataBroadcastExperiment{
		Hardware:                           "Apple M2 Pro",
		ToolchainIdentity:                  "Xcode 26.6 Metal",
		CandidateDefaultOff:                true,
		WarmupCount:                        20,
		TimedIterationCount:                200,
		UnchangedLaunchGeometry:            true,
		UnchangedQuantLoads:                true,
		UnchangedActivationLoads:           true,
		UnchangedAccumulation:              true,
		UnchangedOutputMapping:             true,
		SubgroupWidth:                      32,
		PrimaryLeaderFanout:                8,
		SecondaryLeaderFanout:              4,
		SharedPayloadBytes:                 8,
		BytesSavedPerSubgroupIteration:     48,
		ShuffleCountPerLane:                6,
		BandwidthModel:                     "tiny cache-local metadata versus six shuffles",
		SourceAddressUniformity:            "uniform within lane group",
		OriginalLoadsCoalesced:             true,
		EstimatedCacheResident:             true,
		ReuseDistanceBytes:                 32,
		DivergenceAdded:                    true,
		NumericalOrderContractionSensitive: true,
		IncidentShapes:                     []string{"K2048,N2048", "K2048,N2560", "K2048,N5632", "K5632,N2048", "K2048,N11008", "K2048,N16384", "K2048,N32000"},
		IncidentControlLatenciesNS:         []float64{223422, 179328, 200208, 198504, 234836, 270058, 368778},
		IncidentCandidateLatenciesNS:       []float64{265786, 174067, 204836, 222494, 274267, 325810, 480710},
		IncidentSpeedups:                   []float64{223422.0 / 265786, 179328.0 / 174067, 200208.0 / 204836, 198504.0 / 222494, 234836.0 / 274267, 270058.0 / 325810, 368778.0 / 480710},
		GeometricMeanSpeedup:               0.8807640902008231,
		RegressionCount:                    6,
		ControlAllocationBytes:             8,
		CandidateAllocationBytes:           8,
		ControlAllocationCount:             1,
		CandidateAllocationCount:           1,
		ExactOddTailRequired:               true,
		OddTailExactParity:                 false,
		OddTailMismatchCount:               1,
		OddTailULPClass:                    1,
		PerformanceMinimum:                 1,
		PerformanceVerdict:                 "fail",
		CorrectnessVerdict:                 "fail",
		TransformationRecommended:          false,
		Classification:                     "performance-and-correctness-regression",
		FinalDecision:                      "removed",
	}
	_ = evidence
	runMetalGPUCrossLaneShuffleMetadataBandwidth()
}
