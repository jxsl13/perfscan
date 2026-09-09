package ps6081silent

func parallelChunks(size int, body func(start, end int)) { body(0, size) }

func mapParallel(input, table []byte, size int) []byte {
	output := make([]byte, size)
	parallelChunks(size, func(start, end int) {
		for index := start; index < end; index++ {
			output[index] = table[input[index]]
		}
	})
	return output
}

func zip(left, right []byte) []byte { return left[:min(len(left), len(right))] }

func mapAndZip(input, firstTable, secondTable []byte, size int) []byte {
	first := mapParallel(input, firstTable, size)
	second := mapParallel(input, secondTable, size)
	return zip(first, second)
}
