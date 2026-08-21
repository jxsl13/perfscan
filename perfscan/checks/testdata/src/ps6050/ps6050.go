package ps6050

import "testing"

type unifiedMemoryBufferReport struct {
	HardwareFamily                 string
	MemoryArchitecture             string
	ResourceType                   string
	Producer                       string
	Mutability                     string
	AccessPattern                  string
	AccessFrequency                string
	CPUAccessAfterInitialization   bool
	ControlStorageMode             string
	CandidateStorageMode           string
	CandidateRequiresStaging       bool
	CandidateRequiresBlit          bool
	CandidateRequiresWait          bool
	PrivateOnlyFeatureDocumented   bool
	PayloadBytes                   int
	ControlTransientMetalBytes     int
	CandidateTransientMetalBytes   int
	ControlPersistentMetalBytes    int
	CandidatePersistentMetalBytes  int
	IncidentShapes                 []string
	IncidentRatios                 []float64
	Q4KIncidentRatios              []float64
	Q6KIncidentRatios              []float64
	OverallWarmGeomeanSpeedup      float64
	Q4KGeomeanSpeedup              float64
	Q6KGeomeanSpeedup              float64
	PrimaryControlLatencyNS        float64
	PrimaryCandidateLatencyNS      float64
	PrimaryControlCandidateRatio   float64
	SharedUploadLatencyNS          float64
	PrivateUploadLatencyNS         float64
	UploadControlCandidateRatio    float64
	UploadOverheadFraction         float64
	LeafControlAllocationBytes     int
	LeafCandidateAllocationBytes   int
	LeafControlAllocationCount     int
	LeafCandidateAllocationCount   int
	UploadControlAllocationBytes   int
	UploadCandidateAllocationBytes int
	UploadControlAllocationCount   int
	UploadCandidateAllocationCount int
	PrivateModeObserved            bool
	OddTailParity                  bool
	LocalBenchmarkEvidencePresent  bool
	CounterEvidencePresent         bool
	PrivateStorageRecommended      bool
	Classification                 string
	FinalDecision                  string
}

func runMetalPrivateUnifiedMemoryBufferResourceMode() {}

func TestMetalPrivateUnifiedMemoryBufferResourceModeMissing(t *testing.T) { // want `Apple unified-memory private-buffer campaign has no resource-mode manifest`
	runMetalPrivateUnifiedMemoryBufferResourceMode()
}

func TestMetalPrivateUnifiedMemoryBufferResourceModeIncomplete(t *testing.T) {
	evidence := unifiedMemoryBufferReport{ // want `Metal resource-mode evidence is incomplete; missing memory architecture, resource type, resource producer`
		HardwareFamily: "Apple M2 Pro",
	}
	_ = evidence
	runMetalPrivateUnifiedMemoryBufferResourceMode()
}

func TestMetalPrivateUnifiedMemoryBufferResourceModeOverrecommend(t *testing.T) {
	evidence := unifiedMemoryBufferReport{ // want `private storage is recommended for a CPU-populated unified-memory buffer without supporting local or private-only evidence`
		HardwareFamily:                 "Apple M2 Pro",
		MemoryArchitecture:             "Apple unified memory",
		ResourceType:                   "buffer",
		Producer:                       "CPU-populated",
		Mutability:                     "immutable after initialization",
		AccessPattern:                  "linear GPU reads",
		AccessFrequency:                "every decode token",
		CPUAccessAfterInitialization:   false,
		ControlStorageMode:             "MTLStorageModeShared",
		CandidateStorageMode:           "MTLStorageModePrivate",
		CandidateRequiresStaging:       true,
		CandidateRequiresBlit:          true,
		CandidateRequiresWait:          true,
		PrivateOnlyFeatureDocumented:   false,
		PayloadBytes:                   6_488_064,
		ControlTransientMetalBytes:     6_488_064,
		CandidateTransientMetalBytes:   2 * 6_488_064,
		ControlPersistentMetalBytes:    6_488_064,
		CandidatePersistentMetalBytes:  6_488_064,
		IncidentShapes:                 []string{"Q4-1", "Q4-2", "Q4-3", "Q4-4", "Q4-5", "Q4-6", "Q6-1", "Q6-2", "Q6-3", "Q6-4", "Q6-5"},
		IncidentRatios:                 []float64{0.916, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317, 1.007, 1.007, 1.007, 1.007, 1.007},
		Q4KIncidentRatios:              []float64{0.916, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317},
		Q6KIncidentRatios:              []float64{1.007, 1.007, 1.007, 1.007, 1.007},
		OverallWarmGeomeanSpeedup:      1.0053625268134967,
		Q4KGeomeanSpeedup:              1.004,
		Q6KGeomeanSpeedup:              1.007,
		PrimaryControlLatencyNS:        180_659,
		PrimaryCandidateLatencyNS:      197_212,
		PrimaryControlCandidateRatio:   180_659.0 / 197_212,
		SharedUploadLatencyNS:          668_883,
		PrivateUploadLatencyNS:         1_184_567,
		UploadControlCandidateRatio:    668_883.0 / 1_184_567,
		UploadOverheadFraction:         1_184_567.0/668_883 - 1,
		LeafControlAllocationBytes:     8,
		LeafCandidateAllocationBytes:   8,
		LeafControlAllocationCount:     1,
		LeafCandidateAllocationCount:   1,
		UploadControlAllocationBytes:   72,
		UploadCandidateAllocationBytes: 72,
		UploadControlAllocationCount:   3,
		UploadCandidateAllocationCount: 3,
		PrivateModeObserved:            true,
		OddTailParity:                  true,
		LocalBenchmarkEvidencePresent:  true,
		CounterEvidencePresent:         false,
		PrivateStorageRecommended:      true,
		Classification:                 "generic-private-resource-win",
		FinalDecision:                  "retained",
	}
	_ = evidence
	runMetalPrivateUnifiedMemoryBufferResourceMode()
}

func TestMetalPrivateUnifiedMemoryBufferResourceModeStaleGeomean(t *testing.T) {
	evidence := unifiedMemoryBufferReport{ // want `overall geometric mean 2x disagrees with 1.00536x`
		HardwareFamily:                 "Apple M2 Pro",
		MemoryArchitecture:             "Apple unified memory",
		ResourceType:                   "buffer",
		Producer:                       "CPU-populated",
		Mutability:                     "immutable after initialization",
		AccessPattern:                  "linear GPU reads",
		AccessFrequency:                "every decode token",
		CPUAccessAfterInitialization:   false,
		ControlStorageMode:             "MTLStorageModeShared",
		CandidateStorageMode:           "MTLStorageModePrivate",
		CandidateRequiresStaging:       true,
		CandidateRequiresBlit:          true,
		CandidateRequiresWait:          true,
		PrivateOnlyFeatureDocumented:   false,
		PayloadBytes:                   6_488_064,
		ControlTransientMetalBytes:     6_488_064,
		CandidateTransientMetalBytes:   2 * 6_488_064,
		ControlPersistentMetalBytes:    6_488_064,
		CandidatePersistentMetalBytes:  6_488_064,
		IncidentShapes:                 []string{"Q4-1", "Q4-2", "Q4-3", "Q4-4", "Q4-5", "Q4-6", "Q6-1", "Q6-2", "Q6-3", "Q6-4", "Q6-5"},
		IncidentRatios:                 []float64{0.916, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317, 1.007, 1.007, 1.007, 1.007, 1.007},
		Q4KIncidentRatios:              []float64{0.916, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317},
		Q6KIncidentRatios:              []float64{1.007, 1.007, 1.007, 1.007, 1.007},
		OverallWarmGeomeanSpeedup:      2,
		Q4KGeomeanSpeedup:              1.004,
		Q6KGeomeanSpeedup:              1.007,
		PrimaryControlLatencyNS:        180_659,
		PrimaryCandidateLatencyNS:      197_212,
		PrimaryControlCandidateRatio:   180_659.0 / 197_212,
		SharedUploadLatencyNS:          668_883,
		PrivateUploadLatencyNS:         1_184_567,
		UploadControlCandidateRatio:    668_883.0 / 1_184_567,
		UploadOverheadFraction:         1_184_567.0/668_883 - 1,
		LeafControlAllocationBytes:     8,
		LeafCandidateAllocationBytes:   8,
		LeafControlAllocationCount:     1,
		LeafCandidateAllocationCount:   1,
		UploadControlAllocationBytes:   72,
		UploadCandidateAllocationBytes: 72,
		UploadControlAllocationCount:   3,
		UploadCandidateAllocationCount: 3,
		PrivateModeObserved:            true,
		OddTailParity:                  true,
		LocalBenchmarkEvidencePresent:  true,
		CounterEvidencePresent:         false,
		PrivateStorageRecommended:      false,
		Classification:                 "warm-neutral-upload-regression",
		FinalDecision:                  "removed",
	}
	_ = evidence
	runMetalPrivateUnifiedMemoryBufferResourceMode()
}

func TestMetalPrivateUnifiedMemoryBufferResourceModeStable(t *testing.T) {
	evidence := unifiedMemoryBufferReport{
		HardwareFamily:                 "Apple M2 Pro",
		MemoryArchitecture:             "Apple unified memory",
		ResourceType:                   "buffer",
		Producer:                       "CPU-populated",
		Mutability:                     "immutable after initialization",
		AccessPattern:                  "linear GPU reads",
		AccessFrequency:                "every decode token",
		CPUAccessAfterInitialization:   false,
		ControlStorageMode:             "MTLStorageModeShared",
		CandidateStorageMode:           "MTLStorageModePrivate",
		CandidateRequiresStaging:       true,
		CandidateRequiresBlit:          true,
		CandidateRequiresWait:          true,
		PrivateOnlyFeatureDocumented:   false,
		PayloadBytes:                   6_488_064,
		ControlTransientMetalBytes:     6_488_064,
		CandidateTransientMetalBytes:   2 * 6_488_064,
		ControlPersistentMetalBytes:    6_488_064,
		CandidatePersistentMetalBytes:  6_488_064,
		IncidentShapes:                 []string{"Q4-1", "Q4-2", "Q4-3", "Q4-4", "Q4-5", "Q4-6", "Q6-1", "Q6-2", "Q6-3", "Q6-4", "Q6-5"},
		IncidentRatios:                 []float64{0.916, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317, 1.007, 1.007, 1.007, 1.007, 1.007},
		Q4KIncidentRatios:              []float64{0.916, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317, 1.0225895743611317},
		Q6KIncidentRatios:              []float64{1.007, 1.007, 1.007, 1.007, 1.007},
		OverallWarmGeomeanSpeedup:      1.0053625268134967,
		Q4KGeomeanSpeedup:              1.004,
		Q6KGeomeanSpeedup:              1.007,
		PrimaryControlLatencyNS:        180_659,
		PrimaryCandidateLatencyNS:      197_212,
		PrimaryControlCandidateRatio:   180_659.0 / 197_212,
		SharedUploadLatencyNS:          668_883,
		PrivateUploadLatencyNS:         1_184_567,
		UploadControlCandidateRatio:    668_883.0 / 1_184_567,
		UploadOverheadFraction:         1_184_567.0/668_883 - 1,
		LeafControlAllocationBytes:     8,
		LeafCandidateAllocationBytes:   8,
		LeafControlAllocationCount:     1,
		LeafCandidateAllocationCount:   1,
		UploadControlAllocationBytes:   72,
		UploadCandidateAllocationBytes: 72,
		UploadControlAllocationCount:   3,
		UploadCandidateAllocationCount: 3,
		PrivateModeObserved:            true,
		OddTailParity:                  true,
		LocalBenchmarkEvidencePresent:  true,
		CounterEvidencePresent:         false,
		PrivateStorageRecommended:      false,
		Classification:                 "warm-neutral-upload-regression",
		FinalDecision:                  "removed",
	}
	_ = evidence
	runMetalPrivateUnifiedMemoryBufferResourceMode()
}
