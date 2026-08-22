package ps6011alias

import tt "testing"

func explicitCastVendorMatmul() {}
func graphFusedMatmul()         {}
func reportSpeedRatio()         {}

func BenchmarkGraphMatmulPromotionAlias(b *tt.B) { // want `graph/compiler-fused matmul timing gate has no numerical-parity manifest`
	for range b.N {
		explicitCastVendorMatmul()
		graphFusedMatmul()
		reportSpeedRatio()
	}
}
