package ps6077round2shapearm64

type shadowFloat = float64

func ReceiverCoshF64(values []shadowFloat) shadowFloat {
	var sum shadowFloat
	for index := 0; index+1 < len(values); index += 2 {
		sum += values[index] + values[index+1]
	}
	return sum
}
