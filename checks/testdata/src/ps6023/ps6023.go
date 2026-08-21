package ps6023

import "testing"

type hostDeviceFloorEvidence struct {
	Hardware               string
	Shape                  string
	ControlStorage         string
	CandidateStorage       string
	Warmups                int
	Interleaved            bool
	SamplesPerCampaign     int
	Campaigns              int
	ContextSizes           []int
	WorkingSetBytes        []int
	DeviceSpeedups         []float64
	HostBoundarySpeedups   []float64
	UnchangedControlRatios []float64
	NoiseBand              float64
	PromotionRatio         float64
	ExactParityPassed      bool
	FiniteOutput           bool
}

func recordSubmitWait() {}

func BenchmarkMetalAsyncAttentionDeviceHostCacheSweepMissing(b *testing.B) { // want "asynchronous accelerator host/device cache benchmark has no host-floor evidence manifest; missing hardware, workload geometry"
	for range b.N {
		recordSubmitWait()
	}
}

func BenchmarkMetalAsyncAttentionDeviceHostCacheSweepMasked(b *testing.B) {
	evidence := hostDeviceFloorEvidence{ // want "host/device cache-threshold audit: host floor masks device gain at sweep index 1 \\(device 1.24x, host 1.02x inside ±3.00% noise\\); device sweep crosses 1.03x promotion ratio.*host boundary sweep crosses 1.03x promotion ratio"
		Hardware:               "Apple M2 Pro",
		Shape:                  "sq=1,h=32,kv=4,dk=64",
		ControlStorage:         "f32 KV",
		CandidateStorage:       "IEEE f16 KV",
		Warmups:                8,
		Interleaved:            true,
		SamplesPerCampaign:     31,
		Campaigns:              5,
		ContextSizes:           []int{128, 512, 1024, 2048},
		WorkingSetBytes:        []int{1 << 20, 4 << 20, 8 << 20, 16 << 20},
		DeviceSpeedups:         []float64{1.013, 1.24, 1.50, 1.63},
		HostBoundarySpeedups:   []float64{1.002, 1.02, 1.10, 1.20},
		UnchangedControlRatios: []float64{1.00, 1.00, 1.00, 1.00},
		NoiseBand:              0.03,
		PromotionRatio:         1.03,
		ExactParityPassed:      true,
		FiniteOutput:           true,
	}
	_ = evidence
	for range b.N {
		recordSubmitWait()
	}
}

func BenchmarkMetalAsyncAttentionDeviceHostCacheSweepStable(b *testing.B) {
	evidence := hostDeviceFloorEvidence{
		Hardware:               "Apple M2 Pro",
		Shape:                  "sq=1,h=32,kv=4,dk=64",
		ControlStorage:         "f32 KV",
		CandidateStorage:       "IEEE f16 KV",
		Warmups:                8,
		Interleaved:            true,
		SamplesPerCampaign:     31,
		Campaigns:              5,
		ContextSizes:           []int{512, 1024},
		WorkingSetBytes:        []int{4 << 20, 8 << 20},
		DeviceSpeedups:         []float64{1.20, 1.30},
		HostBoundarySpeedups:   []float64{1.10, 1.15},
		UnchangedControlRatios: []float64{1.00, 1.00},
		NoiseBand:              0.03,
		PromotionRatio:         1.03,
		ExactParityPassed:      true,
		FiniteOutput:           true,
	}
	_ = evidence
	for range b.N {
		recordSubmitWait()
	}
}
