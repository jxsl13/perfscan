package ps5108alias

import sequence "bytes"

func repeat(data []byte) []byte {
	return sequence.Repeat(sequence.Repeat(data, 6), 7) // want "bytes.Repeat is nested 2 times with positive constant counts; combine them to 42"
}
