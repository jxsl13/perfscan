package ps6015

import "testing"

func BenchmarkVulkanOpLayerNormF32RouteHardwareShapeCrossover(b *testing.B) {
	hardware := "test GPU"
	shapes := [][2]int{{1, 512}, {256, 2048}}
	buildMode := "GOEXPERIMENT=simd"
	controlImplementation := "typed arm64 NEON host kernel"
	candidateImplementation := "native Vulkan device kernel"
	invalidateOnCapabilityChange := true
	_, _, _, _, _, _ = hardware, shapes, buildMode, controlImplementation, candidateImplementation, invalidateOnCapabilityChange
	for range b.N {
	}
}

// This historically correct GPU-loses result has hardware and shape evidence,
// but no build/capability dependency or invalidation trigger.
func BenchmarkMetalOpGELUF32RouteHardwareShapeCrossover(b *testing.B) { // want "route evidence for .*gelu.*f32 is not capability-stable; missing build-mode dependency, control implementation capability, candidate implementation capability, implementation-change invalidation.*either gains a SIMD/native kernel"
	hardware := "Apple M2"
	shapes := [][2]int{{1, 512}, {256, 2048}}
	_, _ = hardware, shapes
	for range b.N {
	}
}
