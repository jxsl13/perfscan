package ps6083strings

// Even a loop without a candidate used to panic on a constant string.
func noCandidate() int {
	result := 0
	for index := range "éééé" {
		result += index
	}
	return result
}

func unicodeFour(output *[8]float32, packed uint32) {
	for index := range "éééé" {
		output[index] = float32(2*((packed>>(3*index))&7) + 1) // want `maps exactly 8 masked unsigned values`
	}
}

func asciiFour(output *[4]float32, packed uint32) {
	for index := range "abcd" {
		output[index] = float32(2*((packed>>(3*index))&7) + 1) // want `maps exactly 8 masked unsigned values`
	}
}

const invalidFour = "\xff\xff\xff\xff"

func invalidUTF8Four(output *[4]float32, packed uint32) {
	for index := range invalidFour {
		output[index] = float32(2*((packed>>(3*index))&7) + 1) // want `maps exactly 8 masked unsigned values`
	}
}

// Materiality counts range iterations, not bytes in the encoded string.
func onlyTwoRunes(output *[4]float32, packed uint32) {
	for index := range "éé" {
		output[index] = float32(2*((packed>>(3*index))&7) + 1)
	}
}

func emptyRange(output []float32, packed uint32) {
	for index := range "" {
		output[index] = float32(2*((packed>>(3*index))&7) + 1)
	}
}

func runtimeString(output []float32, text string, packed uint32) {
	for index := range text {
		output[index] = float32(2*((packed>>(3*index))&7) + 1) // want `maps exactly 8 masked unsigned values`
	}
}
