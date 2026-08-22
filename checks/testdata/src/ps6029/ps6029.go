package ps6029

import "testing"

type splitPassProfilerEvidence struct {
	Hardware                          string
	WorkloadDigest                    string
	GraphIdentity                     string
	BuildIdentity                     string
	ThermalProcessContract            string
	ProtocolMode                      string
	StrictOnePassAvailable            bool
	ClaimSignal                       string
	WitnessSignal                     string
	ClaimPassRequiredCount            int
	ClaimPassAttemptedCount           int
	ClaimPassAcceptedCount            int
	ClaimPassRejectedCount            int
	ClaimPassIDs                      []string
	ClaimRejectionReasons             []string
	WitnessPassRequiredCount          int
	WitnessPassAttemptedCount         int
	WitnessPassAcceptedCount          int
	WitnessPassRejectedCount          int
	WitnessPassIDs                    []string
	WitnessRejectionReasons           []string
	JoinWorkloadDigestMatched         bool
	JoinGraphIdentityMatched          bool
	JoinHardwareMatched               bool
	JoinBuildMatched                  bool
	JoinThermalProcessContractMatched bool
	ClaimPassesIndependent            bool
	WitnessPassesIndependent          bool
	WitnessAggregationIndependent     bool
	MissingWitnessImputedZero         bool
	ExactOutputDigestPassed           bool
	ExactEventTopologyPassed          bool
	MediansPublished                  bool
	CandidateSelected                 bool
}

func runMetalGPUProfilerSplitPassClaimWitnessCampaign() {}

func TestMetalGPUProfilerSplitPassClaimWitnessMissing(t *testing.T) { // want `profiler claim/witness campaign has no split-pass join manifest`
	runMetalGPUProfilerSplitPassClaimWitnessCampaign()
}

func TestMetalGPUProfilerSplitPassClaimWitnessIncomplete(t *testing.T) {
	evidence := splitPassProfilerEvidence{ // want `profiler split-pass evidence is incomplete; missing workload digest, graph identity, build identity`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalGPUProfilerSplitPassClaimWitnessCampaign()
}

func TestMetalGPUProfilerSplitPassClaimWitnessPrematurePublication(t *testing.T) {
	evidence := splitPassProfilerEvidence{ // want `medians are published before both predeclared accepted-pass counts are satisfied; candidate is selected before both predeclared accepted-pass counts are satisfied`
		Hardware:                          "Apple M2 Pro",
		WorkloadDigest:                    "tinyllama-q4-k-m",
		GraphIdentity:                     "296-event-graph",
		BuildIdentity:                     "goai-t1029",
		ThermalProcessContract:            "fresh process, thermal steady",
		ProtocolMode:                      "split-pass",
		StrictOnePassAvailable:            true,
		ClaimSignal:                       "GPU intervals and topology",
		WitnessSignal:                     "contamination counters",
		ClaimPassRequiredCount:            5,
		ClaimPassAttemptedCount:           5,
		ClaimPassAcceptedCount:            3,
		ClaimPassRejectedCount:            2,
		ClaimPassIDs:                      []string{"claim-1", "claim-2", "claim-3", "claim-4", "claim-5"},
		ClaimRejectionReasons:             []string{"identity mismatch", "timing parse failure"},
		WitnessPassRequiredCount:          5,
		WitnessPassAttemptedCount:         5,
		WitnessPassAcceptedCount:          3,
		WitnessPassRejectedCount:          2,
		WitnessPassIDs:                    []string{"witness-1", "witness-2", "witness-3", "witness-4", "witness-5"},
		WitnessRejectionReasons:           []string{"Fragment Occupancy missing", "Texture Sample Limiter missing"},
		JoinWorkloadDigestMatched:         true,
		JoinGraphIdentityMatched:          true,
		JoinHardwareMatched:               true,
		JoinBuildMatched:                  true,
		JoinThermalProcessContractMatched: true,
		ClaimPassesIndependent:            true,
		WitnessPassesIndependent:          true,
		WitnessAggregationIndependent:     true,
		MissingWitnessImputedZero:         false,
		ExactOutputDigestPassed:           true,
		ExactEventTopologyPassed:          true,
		MediansPublished:                  true,
		CandidateSelected:                 true,
	}
	_ = evidence
	runMetalGPUProfilerSplitPassClaimWitnessCampaign()
}

func TestMetalGPUProfilerSplitPassClaimWitnessJoinAndImputationFailure(t *testing.T) {
	evidence := splitPassProfilerEvidence{ // want `workload-digest join is explicitly false; missing contamination witness is imputed as zero`
		Hardware:                          "Apple M2 Pro",
		WorkloadDigest:                    "tinyllama-q4-k-m",
		GraphIdentity:                     "296-event-graph",
		BuildIdentity:                     "goai-t1029",
		ThermalProcessContract:            "fresh process, thermal steady",
		ProtocolMode:                      "split-pass",
		StrictOnePassAvailable:            true,
		ClaimSignal:                       "GPU intervals and topology",
		WitnessSignal:                     "contamination counters",
		ClaimPassRequiredCount:            3,
		ClaimPassAttemptedCount:           3,
		ClaimPassAcceptedCount:            3,
		ClaimPassRejectedCount:            0,
		ClaimPassIDs:                      []string{"claim-1", "claim-2", "claim-3"},
		ClaimRejectionReasons:             []string{},
		WitnessPassRequiredCount:          3,
		WitnessPassAttemptedCount:         3,
		WitnessPassAcceptedCount:          3,
		WitnessPassRejectedCount:          0,
		WitnessPassIDs:                    []string{"witness-1", "witness-2", "witness-3"},
		WitnessRejectionReasons:           []string{},
		JoinWorkloadDigestMatched:         false,
		JoinGraphIdentityMatched:          true,
		JoinHardwareMatched:               true,
		JoinBuildMatched:                  true,
		JoinThermalProcessContractMatched: true,
		ClaimPassesIndependent:            true,
		WitnessPassesIndependent:          true,
		WitnessAggregationIndependent:     true,
		MissingWitnessImputedZero:         true,
		ExactOutputDigestPassed:           true,
		ExactEventTopologyPassed:          true,
		MediansPublished:                  false,
		CandidateSelected:                 false,
	}
	_ = evidence
	runMetalGPUProfilerSplitPassClaimWitnessCampaign()
}

func TestMetalGPUProfilerSplitPassClaimWitnessSharedPass(t *testing.T) {
	evidence := splitPassProfilerEvidence{ // want `split-pass claim and witness pass identities overlap`
		Hardware:                          "Apple M2 Pro",
		WorkloadDigest:                    "tinyllama-q4-k-m",
		GraphIdentity:                     "296-event-graph",
		BuildIdentity:                     "goai-t1029",
		ThermalProcessContract:            "fresh process, thermal steady",
		ProtocolMode:                      "split-pass",
		StrictOnePassAvailable:            true,
		ClaimSignal:                       "GPU intervals and topology",
		WitnessSignal:                     "contamination counters",
		ClaimPassRequiredCount:            3,
		ClaimPassAttemptedCount:           3,
		ClaimPassAcceptedCount:            3,
		ClaimPassRejectedCount:            0,
		ClaimPassIDs:                      []string{"claim-1", "shared-2", "claim-3"},
		ClaimRejectionReasons:             []string{},
		WitnessPassRequiredCount:          3,
		WitnessPassAttemptedCount:         3,
		WitnessPassAcceptedCount:          3,
		WitnessPassRejectedCount:          0,
		WitnessPassIDs:                    []string{"witness-1", "shared-2", "witness-3"},
		WitnessRejectionReasons:           []string{},
		JoinWorkloadDigestMatched:         true,
		JoinGraphIdentityMatched:          true,
		JoinHardwareMatched:               true,
		JoinBuildMatched:                  true,
		JoinThermalProcessContractMatched: true,
		ClaimPassesIndependent:            true,
		WitnessPassesIndependent:          true,
		WitnessAggregationIndependent:     true,
		MissingWitnessImputedZero:         false,
		ExactOutputDigestPassed:           true,
		ExactEventTopologyPassed:          true,
		MediansPublished:                  true,
		CandidateSelected:                 true,
	}
	_ = evidence
	runMetalGPUProfilerSplitPassClaimWitnessCampaign()
}

func TestMetalGPUProfilerSplitPassClaimWitnessAccountingFailure(t *testing.T) {
	evidence := splitPassProfilerEvidence{ // want `claim pass accounting disagrees \(accepted 3 \+ rejected 1 != attempted 5\); rejected claim passes have no disclosed reasons`
		Hardware:                          "Apple M2 Pro",
		WorkloadDigest:                    "tinyllama-q4-k-m",
		GraphIdentity:                     "296-event-graph",
		BuildIdentity:                     "goai-t1029",
		ThermalProcessContract:            "fresh process, thermal steady",
		ProtocolMode:                      "split-pass",
		StrictOnePassAvailable:            true,
		ClaimSignal:                       "GPU intervals and topology",
		WitnessSignal:                     "contamination counters",
		ClaimPassRequiredCount:            3,
		ClaimPassAttemptedCount:           5,
		ClaimPassAcceptedCount:            3,
		ClaimPassRejectedCount:            1,
		ClaimPassIDs:                      []string{"claim-1", "claim-2", "claim-3", "claim-4", "claim-5"},
		ClaimRejectionReasons:             []string{},
		WitnessPassRequiredCount:          3,
		WitnessPassAttemptedCount:         3,
		WitnessPassAcceptedCount:          3,
		WitnessPassRejectedCount:          0,
		WitnessPassIDs:                    []string{"witness-1", "witness-2", "witness-3"},
		WitnessRejectionReasons:           []string{},
		JoinWorkloadDigestMatched:         true,
		JoinGraphIdentityMatched:          true,
		JoinHardwareMatched:               true,
		JoinBuildMatched:                  true,
		JoinThermalProcessContractMatched: true,
		ClaimPassesIndependent:            true,
		WitnessPassesIndependent:          true,
		WitnessAggregationIndependent:     true,
		MissingWitnessImputedZero:         false,
		ExactOutputDigestPassed:           true,
		ExactEventTopologyPassed:          true,
		MediansPublished:                  true,
		CandidateSelected:                 true,
	}
	_ = evidence
	runMetalGPUProfilerSplitPassClaimWitnessCampaign()
}

func TestMetalGPUProfilerSplitPassClaimWitnessStable(t *testing.T) {
	evidence := splitPassProfilerEvidence{
		Hardware:                          "Apple M2 Pro",
		WorkloadDigest:                    "tinyllama-q4-k-m",
		GraphIdentity:                     "296-event-graph",
		BuildIdentity:                     "goai-t1029",
		ThermalProcessContract:            "fresh process, thermal steady",
		ProtocolMode:                      "split-pass",
		StrictOnePassAvailable:            true,
		ClaimSignal:                       "GPU intervals and topology",
		WitnessSignal:                     "contamination counters",
		ClaimPassRequiredCount:            3,
		ClaimPassAttemptedCount:           5,
		ClaimPassAcceptedCount:            3,
		ClaimPassRejectedCount:            2,
		ClaimPassIDs:                      []string{"claim-1", "claim-2", "claim-3", "claim-4", "claim-5"},
		ClaimRejectionReasons:             []string{"identity mismatch", "timing parse failure"},
		WitnessPassRequiredCount:          3,
		WitnessPassAttemptedCount:         5,
		WitnessPassAcceptedCount:          3,
		WitnessPassRejectedCount:          2,
		WitnessPassIDs:                    []string{"witness-1", "witness-2", "witness-3", "witness-4", "witness-5"},
		WitnessRejectionReasons:           []string{"Fragment Occupancy missing", "Texture Sample Limiter missing"},
		JoinWorkloadDigestMatched:         true,
		JoinGraphIdentityMatched:          true,
		JoinHardwareMatched:               true,
		JoinBuildMatched:                  true,
		JoinThermalProcessContractMatched: true,
		ClaimPassesIndependent:            true,
		WitnessPassesIndependent:          true,
		WitnessAggregationIndependent:     true,
		MissingWitnessImputedZero:         false,
		ExactOutputDigestPassed:           true,
		ExactEventTopologyPassed:          true,
		MediansPublished:                  true,
		CandidateSelected:                 true,
	}
	_ = evidence
	runMetalGPUProfilerSplitPassClaimWitnessCampaign()
}
