package ps6106noconfig

import (
	"bytes"
	"slices"
)

// Ordinary slice utilities without PS6106's closed producer/consumer/observer
// sequence stay quiet even though the check runs without project vocabulary.
func compactAndCompare(input []byte) bool {
	copyOfInput := slices.Clone(input)
	copyOfInput = slices.Compact(copyOfInput)
	return bytes.Equal(copyOfInput, input)
}
