package ps6048

import "testing"

type metalNativeArtifactEvidence struct {
	Hardware                               string
	OSIdentity                             string
	ToolchainComponentBuild                string
	CompilerVersion                        string
	BinaryVersions                         []string
	MountedToolchainPath                   string
	ToolchainLocatorSource                 string
	ArchitectureResolutionCommand          string
	ResolvedDeviceArchitecture             string
	SDKVersion                             string
	DeploymentTarget                       string
	SourceIdentityHash                     string
	FunctionIdentity                       string
	PipelineDescriptorHash                 string
	TranslationCommands                    []string
	ToolHashes                             []string
	MSLToAIRPassed                         bool
	MetalLibPassed                         bool
	PipelineScriptPassed                   bool
	NativeTranslationPassed                bool
	NativeExtractionSaveTempsPassed        bool
	NativeObjectHash                       string
	NativeTextBytes                        int
	NativeMetadataBytes                    int
	NativeStatisticsBytes                  int
	NativeSpillScratchBytes                int
	SpillScratchSectionPresent             bool
	StatisticsSectionSemanticallyValidated bool
	MetadataDecodedWithStatisticsSchema    bool
	RegisterCountCapability                string
	SpillStatisticCapability               string
	DisassemblyCapability                  string
	PerformanceStatisticsRequested         bool
	PerformanceStatisticsEffective         bool
	MaxRegistersRequested                  int
	MaxRegistersEffective                  bool
	OptionArtifactChanged                  bool
	Classification                         string
	FinalDecision                          string
}

func runMetalNativeCompilerArtifactFailClosedOfflineTranslation() {}

func TestMetalNativeCompilerArtifactFailClosedOfflineTranslationMissing(t *testing.T) { // want `Metal native artifact analysis has no fail-closed provenance manifest`
	runMetalNativeCompilerArtifactFailClosedOfflineTranslation()
}

func TestMetalNativeCompilerArtifactFailClosedOfflineTranslationIncomplete(t *testing.T) {
	evidence := metalNativeArtifactEvidence{ // want `Metal native artifact evidence is incomplete; missing OS identity, toolchain component build, compiler version`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalNativeCompilerArtifactFailClosedOfflineTranslation()
}

func TestMetalNativeCompilerArtifactFailClosedOfflineTranslationUnsafeClaims(t *testing.T) {
	evidence := metalNativeArtifactEvidence{ // want `native metadata is decoded with the statistics schema; register-count capability "known-zero" claims a value without nonempty validated statistics; spill-statistic capability "reported-zero" claims a value without nonempty validated statistics; performance-statistics option is marked effective without changed artifact or emitted statistics; max-register option is marked effective without changed artifact or emitted statistics`
		Hardware:                               "Apple M2 Pro",
		OSIdentity:                             "macOS 26.5.1",
		ToolchainComponentBuild:                "Metal Toolchain 17F109",
		CompilerVersion:                        "Apple metal 32023.883",
		BinaryVersions:                         []string{"metal 32023.883", "air-arch 17F109", "air-nt 17F109"},
		MountedToolchainPath:                   "/Library/Developer/Toolchains/Metal.xctoolchain",
		ToolchainLocatorSource:                 "xcodebuild -showComponent MetalToolchain -json",
		ArchitectureResolutionCommand:          "air-arch -default",
		ResolvedDeviceArchitecture:             "applegpu_g14s",
		SDKVersion:                             "macOS 26.5.1 SDK",
		DeploymentTarget:                       "macOS 26.5",
		SourceIdentityHash:                     "sha256:source",
		FunctionIdentity:                       "q6_k_matvec",
		PipelineDescriptorHash:                 "sha256:pipeline",
		TranslationCommands:                    []string{"metal -c kernel.metal", "metallib kernel.air", "swift pipeline-script", "air-nt -arch applegpu_g14s -save-temps kernel.metallib"},
		ToolHashes:                             []string{"sha256:metal", "sha256:metallib", "sha256:air-nt"},
		MSLToAIRPassed:                         true,
		MetalLibPassed:                         true,
		PipelineScriptPassed:                   true,
		NativeTranslationPassed:                true,
		NativeExtractionSaveTempsPassed:        true,
		NativeObjectHash:                       "sha256:native",
		NativeTextBytes:                        2738,
		NativeMetadataBytes:                    512,
		NativeStatisticsBytes:                  0,
		NativeSpillScratchBytes:                0,
		SpillScratchSectionPresent:             false,
		StatisticsSectionSemanticallyValidated: false,
		MetadataDecodedWithStatisticsSchema:    true,
		RegisterCountCapability:                "known-zero",
		SpillStatisticCapability:               "reported-zero",
		DisassemblyCapability:                  "unsupported-agx2",
		PerformanceStatisticsRequested:         true,
		PerformanceStatisticsEffective:         true,
		MaxRegistersRequested:                  32,
		MaxRegistersEffective:                  true,
		OptionArtifactChanged:                  false,
		Classification:                         "unsafe-inferred-statistics",
		FinalDecision:                          "rejected",
	}
	_ = evidence
	runMetalNativeCompilerArtifactFailClosedOfflineTranslation()
}

func TestMetalNativeCompilerArtifactFailClosedOfflineTranslationStable(t *testing.T) {
	evidence := metalNativeArtifactEvidence{
		Hardware:                               "Apple M2 Pro",
		OSIdentity:                             "macOS 26.5.1",
		ToolchainComponentBuild:                "Metal Toolchain 17F109",
		CompilerVersion:                        "Apple metal 32023.883",
		BinaryVersions:                         []string{"metal 32023.883", "air-arch 17F109", "air-nt 17F109"},
		MountedToolchainPath:                   "/Library/Developer/Toolchains/Metal.xctoolchain",
		ToolchainLocatorSource:                 "xcodebuild -showComponent MetalToolchain -json",
		ArchitectureResolutionCommand:          "air-arch -default",
		ResolvedDeviceArchitecture:             "applegpu_g14s",
		SDKVersion:                             "macOS 26.5.1 SDK",
		DeploymentTarget:                       "macOS 26.5",
		SourceIdentityHash:                     "sha256:source",
		FunctionIdentity:                       "q6_k_matvec",
		PipelineDescriptorHash:                 "sha256:pipeline",
		TranslationCommands:                    []string{"metal -c kernel.metal", "metallib kernel.air", "swift pipeline-script", "air-nt -arch applegpu_g14s -save-temps kernel.metallib"},
		ToolHashes:                             []string{"sha256:metal", "sha256:metallib", "sha256:air-nt"},
		MSLToAIRPassed:                         true,
		MetalLibPassed:                         true,
		PipelineScriptPassed:                   true,
		NativeTranslationPassed:                true,
		NativeExtractionSaveTempsPassed:        true,
		NativeObjectHash:                       "sha256:native",
		NativeTextBytes:                        2738,
		NativeMetadataBytes:                    512,
		NativeStatisticsBytes:                  0,
		NativeSpillScratchBytes:                0,
		SpillScratchSectionPresent:             false,
		StatisticsSectionSemanticallyValidated: false,
		MetadataDecodedWithStatisticsSchema:    false,
		RegisterCountCapability:                "unknown-missing-statistics",
		SpillStatisticCapability:               "unknown-missing-statistics",
		DisassemblyCapability:                  "unsupported-agx2",
		PerformanceStatisticsRequested:         true,
		PerformanceStatisticsEffective:         false,
		MaxRegistersRequested:                  32,
		MaxRegistersEffective:                  false,
		OptionArtifactChanged:                  false,
		Classification:                         "native-sections-known-registers-unknown",
		FinalDecision:                          "static-gate-only",
	}
	_ = evidence
	runMetalNativeCompilerArtifactFailClosedOfflineTranslation()
}
