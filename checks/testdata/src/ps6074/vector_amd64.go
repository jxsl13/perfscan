//go:build amd64

package ps6074

func rowMaxF32(values []float32) float32                  { return rowMaxF32AVX2(values) }
func expSumF32(values []float32, maximum float32) float32 { return expSumF32AVX2(values, maximum) }
func scaleRowF32(values []float32, scale float32)         { scaleRowF32AVX2(values, scale) }

func allMaxF32(values []float32) float32                  { return allMaxF32AVX2(values) }
func allExpF32(values []float32, maximum float32) float32 { return allExpF32AVX2(values, maximum) }
func allScaleF32(values []float32, scale float32)         { allScaleF32AVX2(values, scale) }

func tailMaxF32(values []float32) float32                  { return tailMaxF32AVX2(values) }
func tailExpF32(values []float32, maximum float32) float32 { return tailExpF32AVX2(values, maximum) }
func tailScaleF32(values []float32, scale float32)         { tailScaleF32AVX2(values, scale) }

func rowMaxF32AVX2([]float32) float32
func expSumF32AVX2([]float32, float32) float32
func scaleRowF32AVX2([]float32, float32)
func allMaxF32AVX2([]float32) float32
func allExpF32AVX2([]float32, float32) float32
func allScaleF32AVX2([]float32, float32)
func tailMaxF32AVX2([]float32) float32
func tailExpF32AVX2([]float32, float32) float32
func tailScaleF32AVX2([]float32, float32)
