package ps6009

import "testing"

func metalTileKernel() {}

func BenchmarkMetalTilePromotionGate(b *testing.B) { // want `tiled accelerator promotion benchmark has no reuse-axis gate manifest; missing reuse axis, tile extent`
	for range b.N {
		metalTileKernel()
	}
}

type reuseAxisGate struct {
	ReuseAxis                    string
	TileExtent                   int
	SampleShapes                 []int
	LargestRoutedProductionShape int
	Comparator                   string
	CandidatePerTileSetup        string
	BaselineMaterialization      string
	FirstTileInterceptUS         float64
	PerTileSlopeUS               float64
}

func BenchmarkCUDATileCandidatePartial(b *testing.B) {
	gate := reuseAxisGate{ // want `reuse-axis gate manifest is incomplete; missing boundary sample shapes`
		ReuseAxis:  "M",
		TileExtent: 32,
	}
	_ = gate
	for range b.N {
		metalTileKernel()
	}
}

func BenchmarkMetalTilePromotionBadGrid(b *testing.B) {
	gate := reuseAxisGate{ // want `reuse-axis gate evidence fails boundary audit: shape grid omits required reuse-axis point\(s\) 33, 64, 128; candidate declares per-tile staging/dequantization against persistent baseline materialization but comparator is an uncached/fallback path, not production`
		ReuseAxis:                    "M/prompt rows",
		TileExtent:                   32,
		SampleShapes:                 []int{32},
		LargestRoutedProductionShape: 128,
		Comparator:                   "uncached f32 dequant+GEMM fallback",
		CandidatePerTileSetup:        "dequantize and stage each tile",
		BaselineMaterialization:      "persistent cached dense f16",
		FirstTileInterceptUS:         380,
		PerTileSlopeUS:               290,
	}
	_ = gate
	for range b.N {
		metalTileKernel()
	}
}

func BenchmarkMetalTilePromotionComplete(b *testing.B) {
	gate := reuseAxisGate{
		ReuseAxis:                    "M/prompt rows",
		TileExtent:                   32,
		SampleShapes:                 []int{32, 33, 64, 128},
		LargestRoutedProductionShape: 128,
		Comparator:                   "cached-f16 production incumbent",
		CandidatePerTileSetup:        "dequantize and stage each tile",
		BaselineMaterialization:      "persistent cached dense f16",
		FirstTileInterceptUS:         380,
		PerTileSlopeUS:               290,
	}
	_ = gate
	for range b.N {
		metalTileKernel()
	}
}

// A generic tiled-kernel microbenchmark without a promotion/gate claim stays
// outside this evidence audit.
func BenchmarkMetalTileKernel(b *testing.B) {
	for range b.N {
		metalTileKernel()
	}
}
