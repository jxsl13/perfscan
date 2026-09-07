//go:build amd64

package ps6077

import f "ps6077left"

func ImportedLog10F64(values []f.Value) float64 {
	var sum float64
	for index := 0; index+1 < len(values); index += 2 {
		sum += float64(values[index] + values[index+1])
	}
	return sum
}
