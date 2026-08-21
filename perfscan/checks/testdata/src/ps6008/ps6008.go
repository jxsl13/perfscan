package ps6008

import "testing"

func metalKernel() {}

func BenchmarkMetalIncumbentKernelPort(b *testing.B) { // want `incumbent-justified accelerator kernel port has no selector-regime manifest; missing source input state, source storage state`
	for range b.N {
		metalKernel()
	}
}

type kernelPortContext struct {
	SourceInputState  string
	TargetInputState  string
	TargetIncumbent   string
	OmittedTileLayout string
}

func BenchmarkCUDAReferenceKernelPartial(b *testing.B) {
	ctx := kernelPortContext{ // want `selector-regime manifest for an incumbent-justified accelerator kernel port is incomplete; missing source storage state`
		SourceInputState:  "half",
		TargetInputState:  "half",
		TargetIncumbent:   "library GEMM",
		OmittedTileLayout: "none",
	}
	_ = ctx
	for range b.N {
		metalKernel()
	}
}

type selectorRegimeContext struct {
	SourceInputState              string
	SourceStorageState            string
	TargetInputState              string
	TargetStorageState            string
	SourceFallback                string
	SourceSelectionReason         string
	TargetIncumbent               string
	CachedBytesOutsideTimedKernel int
	OmittedTileLayout             string
	OmittedStagingLayout          string
	OmittedDispatchGeometry       string
}

type portEvidence struct {
	SourceInputState               string
	SourceStorageState             string
	TargetInputState               string
	TargetStorageState             string
	SourceFallback                 string
	SourceSelectionReason          string
	TargetIncumbent                string
	MaterializedBytesOutsideKernel int
	OmittedCoupledFeatures         string
}

func BenchmarkMetalIncumbentKernelMismatch(b *testing.B) {
	ctx := selectorRegimeContext{ // want `selector-regime manifest explicitly describes different direct-vs-cached source/target memory policies and non-none omitted coupled features \(dispatch geometry, staging, tile/layout\)`
		SourceInputState:              "half operands",
		SourceStorageState:            "direct Q4_K quantized weights, no cache",
		TargetInputState:              "f16 activations",
		TargetStorageState:            "persistent cached dense f16 weights",
		SourceFallback:                "scalar direct-quant kernel",
		SourceSelectionReason:         "expanded cache unavailable",
		TargetIncumbent:               "MPS GEMM over cached f16",
		CachedBytesOutsideTimedKernel: 23 << 20,
		OmittedTileLayout:             "source tile shape retained",
		OmittedStagingLayout:          "threadgroup staging differs",
		OmittedDispatchGeometry:       "source grid retained",
	}
	_ = ctx
	for range b.N {
		metalKernel()
	}
}

// A complete same-policy table with no omissions is sufficient source
// evidence; the benchmark still decides performance.
func BenchmarkMetalIncumbentKernelComplete(b *testing.B) {
	ctx := selectorRegimeContext{
		SourceInputState:              "f16 activations",
		SourceStorageState:            "persistent cached dense f16 weights",
		TargetInputState:              "f16 activations",
		TargetStorageState:            "persistent cached dense f16 weights",
		SourceFallback:                "direct kernel",
		SourceSelectionReason:         "cache miss only",
		TargetIncumbent:               "MPS GEMM",
		CachedBytesOutsideTimedKernel: 23 << 20,
		OmittedTileLayout:             "none",
		OmittedStagingLayout:          "none",
		OmittedDispatchGeometry:       "none",
	}
	_ = ctx
	for range b.N {
		metalKernel()
	}
}

// An explicit aggregate "none" covers all coupled-feature omission axes.
func BenchmarkGPUReferenceKernelAggregate(b *testing.B) {
	ctx := portEvidence{
		SourceInputState:               "f16 activations",
		SourceStorageState:             "persistent cached dense f16 weights",
		TargetInputState:               "f16 activations",
		TargetStorageState:             "persistent cached dense f16 weights",
		SourceFallback:                 "direct kernel",
		SourceSelectionReason:          "cache miss only",
		TargetIncumbent:                "library GEMM",
		MaterializedBytesOutsideKernel: 23 << 20,
		OmittedCoupledFeatures:         "none",
	}
	_ = ctx
	for range b.N {
		metalKernel()
	}
}

// Ordinary benchmarks without a port/incumbent claim are outside this audit.
func BenchmarkMetalKernelBaseline(b *testing.B) {
	for range b.N {
		metalKernel()
	}
}

// "support" contains the bytes "port" but is not a porting claim.
func BenchmarkGPUKernelSupport(b *testing.B) {
	for range b.N {
		metalKernel()
	}
}

// Non-benchmark helpers are outside this source evidence boundary.
func metalIncumbentKernelPort() { metalKernel() }
