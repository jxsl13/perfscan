//go:build ps6099never

package ps6099ignoredshadow

import "ps6099ignoredshadow/simdops"

type scalarBackend struct{}

func (scalarBackend) ExpF64([]float64)  {}
func (scalarBackend) LogF64([]float64)  {}
func (scalarBackend) TanF64([]float64)  {}
func (scalarBackend) SinhF64([]float64) {}
func (scalarBackend) CoshF64([]float64) {}

func ApplyExpF64(dst []float64) {
	simdops := scalarBackend{}
	simdops.ExpF64(dst)
}

func ApplyLogF64(dst []float64) {
	{
		simdops := scalarBackend{}
		simdops.LogF64(dst)
	}
	simdops.LogF64(dst)
}

func ApplyLog10F64(dst []float64) {
	simdops.Log10F64(dst)
	simdops := scalarBackend{}
	_ = simdops
}

func SinSIMDF64([]float64) {}

func ApplySinF64(dst []float64) {
	SinSIMDF64 := func([]float64) {}
	SinSIMDF64(dst)
}

func ApplyTanF64(dst []float64, simdops scalarBackend) {
	simdops.TanF64(dst)
}

func ApplySinhF64(dst []float64) {
	if simdops := (scalarBackend{}); true {
		simdops.SinhF64(dst)
	}
}

func ApplyCoshF64(dst []float64) {
	backends := []scalarBackend{{}}
	for _, simdops := range backends {
		simdops.CoshF64(dst)
	}
}
