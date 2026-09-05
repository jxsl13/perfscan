package ps6077round2shapeamd64

import . "ps6077left"

func DotGammaF64(values []Value) float64 {
	var sum Value
	for index := 0; index+1 < len(values); index += 2 {
		sum += values[index] + values[index+1]
	}
	return float64(sum)
}
