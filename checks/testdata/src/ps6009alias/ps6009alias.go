package ps6009alias

import bench "testing"

func BenchmarkGPUTileSpeedup(b *bench.B) { // want `tiled accelerator promotion benchmark has no reuse-axis gate manifest`
	for range b.N {
	}
}
