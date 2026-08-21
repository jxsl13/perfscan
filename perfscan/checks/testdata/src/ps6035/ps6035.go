package ps6035

import "testing"

type persistentEncoderEvidence struct {
	Hardware                           string
	CandidateTopology                  string
	SyntheticDepthRatio                float64
	ParentWorkload                     string
	ProfilerAttributionScope           string
	ParentScreenPairCount              int
	ParentControlTimesNS               []float64
	ParentCandidateTimesNS             []float64
	ArithmeticMeanRatio                float64
	ParentKillGate                     float64
	AlternatingFreshProcessOrderPassed bool
	ExactDigestPassed                  bool
	ComputeBlitComputePassed           bool
	ComputeMPSComputePassed            bool
	BufferBarriersInserted             bool
	NoiseEquivalentClassified          bool
	FullCampaignLaunched               bool
	CounterCaptureLaunched             bool
	CandidatePathDeleted               bool
	FinalDecision                      string
}

func runMetalPersistentEncoderExactParentKillScreen() {}

func TestMetalPersistentEncoderExactParentKillScreenMissing(t *testing.T) { // want `persistent encoder campaign has no exact-parent kill-screen manifest`
	runMetalPersistentEncoderExactParentKillScreen()
}

func TestMetalPersistentEncoderExactParentKillScreenIncomplete(t *testing.T) {
	evidence := persistentEncoderEvidence{ // want `persistent encoder kill-screen evidence is incomplete; missing candidate topology, synthetic depth ratio, production parent workload`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalPersistentEncoderExactParentKillScreen()
}

func TestMetalPersistentEncoderExactParentKillScreenIgnored(t *testing.T) {
	evidence := persistentEncoderEvidence{ // want `1.0015x parent result misses 1.1x gate but is not classified noise-equivalent; full campaign is launched after the parent kill screen fails; counter capture is launched after the parent kill screen fails; candidate runtime/API path remains installed after the parent kill screen fails; final decision "retained" retains a candidate below the parent kill gate`
		Hardware:                           "Apple M2 Pro",
		CandidateTopology:                  "persistent adjacent custom compute dispatches",
		SyntheticDepthRatio:                1.037,
		ParentWorkload:                     "TinyLlama-1.1B Q4_K_M 200-token decode",
		ProfilerAttributionScope:           "revalidation-only",
		ParentScreenPairCount:              2,
		ParentControlTimesNS:               []float64{7_364_441_583, 7_355_263_625},
		ParentCandidateTimesNS:             []float64{7_347_287_458, 7_350_303_000},
		ArithmeticMeanRatio:                (7_364_441_583.0 + 7_355_263_625) / (7_347_287_458 + 7_350_303_000),
		ParentKillGate:                     1.10,
		AlternatingFreshProcessOrderPassed: true,
		ExactDigestPassed:                  true,
		ComputeBlitComputePassed:           true,
		ComputeMPSComputePassed:            true,
		BufferBarriersInserted:             true,
		NoiseEquivalentClassified:          false,
		FullCampaignLaunched:               true,
		CounterCaptureLaunched:             true,
		CandidatePathDeleted:               false,
		FinalDecision:                      "retained",
	}
	_ = evidence
	runMetalPersistentEncoderExactParentKillScreen()
}

func TestMetalPersistentEncoderExactParentKillScreenRatioMismatch(t *testing.T) {
	evidence := persistentEncoderEvidence{ // want `arithmetic-mean ratio 1.2x disagrees with control/candidate means 1.0015x`
		Hardware:                           "Apple M2 Pro",
		CandidateTopology:                  "persistent adjacent custom compute dispatches",
		SyntheticDepthRatio:                1.037,
		ParentWorkload:                     "TinyLlama-1.1B Q4_K_M 200-token decode",
		ProfilerAttributionScope:           "revalidation-only",
		ParentScreenPairCount:              2,
		ParentControlTimesNS:               []float64{7_364_441_583, 7_355_263_625},
		ParentCandidateTimesNS:             []float64{7_347_287_458, 7_350_303_000},
		ArithmeticMeanRatio:                1.2,
		ParentKillGate:                     1.10,
		AlternatingFreshProcessOrderPassed: true,
		ExactDigestPassed:                  true,
		ComputeBlitComputePassed:           true,
		ComputeMPSComputePassed:            true,
		BufferBarriersInserted:             true,
		NoiseEquivalentClassified:          true,
		FullCampaignLaunched:               false,
		CounterCaptureLaunched:             false,
		CandidatePathDeleted:               true,
		FinalDecision:                      "removed",
	}
	_ = evidence
	runMetalPersistentEncoderExactParentKillScreen()
}

func TestMetalPersistentEncoderExactParentKillScreenProfilerOverclaim(t *testing.T) {
	evidence := persistentEncoderEvidence{ // want `profiler attribution scope "promotion-evidence" is not revalidation-only`
		Hardware:                           "Apple M2 Pro",
		CandidateTopology:                  "persistent adjacent custom compute dispatches",
		SyntheticDepthRatio:                1.037,
		ParentWorkload:                     "TinyLlama-1.1B Q4_K_M 200-token decode",
		ProfilerAttributionScope:           "promotion-evidence",
		ParentScreenPairCount:              2,
		ParentControlTimesNS:               []float64{7_364_441_583, 7_355_263_625},
		ParentCandidateTimesNS:             []float64{7_347_287_458, 7_350_303_000},
		ArithmeticMeanRatio:                (7_364_441_583.0 + 7_355_263_625) / (7_347_287_458 + 7_350_303_000),
		ParentKillGate:                     1.10,
		AlternatingFreshProcessOrderPassed: true,
		ExactDigestPassed:                  true,
		ComputeBlitComputePassed:           true,
		ComputeMPSComputePassed:            true,
		BufferBarriersInserted:             true,
		NoiseEquivalentClassified:          true,
		FullCampaignLaunched:               false,
		CounterCaptureLaunched:             false,
		CandidatePathDeleted:               true,
		FinalDecision:                      "removed",
	}
	_ = evidence
	runMetalPersistentEncoderExactParentKillScreen()
}

func TestMetalPersistentEncoderExactParentKillScreenStable(t *testing.T) {
	evidence := persistentEncoderEvidence{
		Hardware:                           "Apple M2 Pro",
		CandidateTopology:                  "persistent adjacent custom compute dispatches",
		SyntheticDepthRatio:                1.037,
		ParentWorkload:                     "TinyLlama-1.1B Q4_K_M 200-token decode",
		ProfilerAttributionScope:           "revalidation-only",
		ParentScreenPairCount:              2,
		ParentControlTimesNS:               []float64{7_364_441_583, 7_355_263_625},
		ParentCandidateTimesNS:             []float64{7_347_287_458, 7_350_303_000},
		ArithmeticMeanRatio:                (7_364_441_583.0 + 7_355_263_625) / (7_347_287_458 + 7_350_303_000),
		ParentKillGate:                     1.10,
		AlternatingFreshProcessOrderPassed: true,
		ExactDigestPassed:                  true,
		ComputeBlitComputePassed:           true,
		ComputeMPSComputePassed:            true,
		BufferBarriersInserted:             true,
		NoiseEquivalentClassified:          true,
		FullCampaignLaunched:               false,
		CounterCaptureLaunched:             false,
		CandidatePathDeleted:               true,
		FinalDecision:                      "removed",
	}
	_ = evidence
	runMetalPersistentEncoderExactParentKillScreen()
}
