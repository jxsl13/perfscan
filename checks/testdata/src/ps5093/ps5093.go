package ps5093

import (
	"bytes"
	"slices"
	"strings"
)

func stringLen(text string) int {
	return strings.NewReader(strings.Clone(strings.Clone(text))).Len() // want "strings.NewReader[(]...[)].Len constructs an ephemeral container only to recover its input length and carries 2 throwaway Clone layer"
}

func stringSize(text string) int64 {
	return strings.NewReader(strings.Clone(text)).Size() // want "strings.NewReader[(]...[)].Size constructs an ephemeral container only to recover its input length and carries 1 throwaway Clone layer"
}

func byteReaderLen(data []byte) int {
	return bytes.NewReader(bytes.Clone(slices.Clone(bytes.Clone(data)))).Len() // want "bytes.NewReader[(]...[)].Len constructs an ephemeral container only to recover its input length and carries 3 throwaway Clone layer"
}

func byteReaderSize(data []byte) int64 {
	return bytes.NewReader(slices.Clone(data)).Size() // want "bytes.NewReader[(]...[)].Size constructs an ephemeral container only to recover its input length and carries 1 throwaway Clone layer"
}

func bufferLen(data []byte) int {
	return bytes.NewBuffer(bytes.Clone(data)).Len() // want "bytes.NewBuffer[(]...[)].Len constructs an ephemeral container only to recover its input length and carries 1 throwaway Clone layer"
}

func bufferStringLen(text string) int {
	return bytes.NewBufferString(strings.Clone(text)).Len() // want "bytes.NewBufferString[(]...[)].Len constructs an ephemeral container only to recover its input length and carries 1 throwaway Clone layer"
}

func conversionSize(text string) int64 {
	return bytes.NewReader(bytes.Clone([]byte(strings.Clone(strings.Clone(text))))).Size() // want "bytes.NewReader[(]...[)].Size constructs an ephemeral container only to recover its input length and carries 3 throwaway Clone layer"
}

func commentPreserved(text string) int {
	return strings.NewReader( /* constructor rationale */ strings.Clone(text)).Len() // want "strings.NewReader[(]...[)].Len constructs an ephemeral container only to recover its input length and carries 1 throwaway Clone layer"
}

// Constant string inputs are excluded to preserve runtime-expression
// constantness in every source context.
func constantString() int {
	return strings.NewReader("constant").Len()
}

func untypedNil() int {
	return bytes.NewReader(nil).Len()
}

func readerUsedBeforeLen(text string) int {
	reader := strings.NewReader(strings.Clone(text))
	_, _ = reader.ReadByte()
	return reader.Len()
}

type reader struct{}

func (reader) Len() int { return 0 }

func NewReader(string) reader { return reader{} }

func userConstructor(text string) int {
	return NewReader(strings.Clone(text)).Len()
}

func functionValue(text string) int {
	constructor := strings.NewReader
	return constructor(strings.Clone(text)).Len()
}
