package ps6083fixed

func fixed(output, input []float32, packed uint32) {
	lookup := [8]float32{1, 3, 5, 7, 9, 11, 13, 15}
	for index := range output {
		output[index] = input[index] * lookup[(packed>>(3*index))&7]
	}
}
