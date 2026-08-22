package ps6007alias

import bench "testing"

func metalMatmul() {}

const (
	removableTrafficFraction = 0.03
	minimumSpeedup           = 1.08
)

func BenchmarkAlias(b *bench.B) {
	for range b.N {
		metalMatmul()
	}
	_ = removableTrafficFraction
	_ = minimumSpeedup // want `declared removable-pass share 3\.0000% has a zero-cost whole-chain ceiling of 1\.0309x, below the 1\.0800x promotion gate`
}
