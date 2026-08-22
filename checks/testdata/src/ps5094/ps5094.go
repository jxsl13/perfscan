package ps5094

import (
	"bytes"
	"slices"
	"strings"
)

type Data []byte
type Text string

func clonedString(data []byte) string {
	return bytes.NewBuffer(bytes.Clone(slices.Clone(bytes.Clone(data)))).String() // want "bytes.NewBuffer[(]...[)].String constructs an ephemeral Buffer only to extract its initial value and carries 3 throwaway Clone layer"
}

func copiedBytes(text string) []byte {
	return bytes.NewBufferString(strings.Clone(strings.Clone(text))).Bytes() // want "bytes.NewBufferString[(]...[)].Bytes constructs an ephemeral Buffer only to extract its initial value and carries 2 throwaway Clone layer"
}

func aliasedBytes(data Data) []byte {
	return bytes.NewBuffer([]byte(data)).Bytes() // want "bytes.NewBuffer[(]...[)].Bytes constructs an ephemeral Buffer only to extract its initial value and carries 0 throwaway Clone layer"
}

func aliasedText(text Text) []byte {
	return bytes.NewBufferString(string(text)).Bytes() // want "bytes.NewBufferString[(]...[)].Bytes constructs an ephemeral Buffer only to extract its initial value and carries 0 throwaway Clone layer"
}

func nilBytes() []byte {
	return bytes.NewBuffer(nil).Bytes() // want "bytes.NewBuffer[(]...[)].Bytes constructs an ephemeral Buffer only to extract its initial value and carries 0 throwaway Clone layer"
}

func nilString() string {
	return bytes.NewBuffer(nil).String() // want "bytes.NewBuffer[(]...[)].String constructs an ephemeral Buffer only to extract its initial value and carries 0 throwaway Clone layer"
}

func constantBytes() []byte {
	return bytes.NewBufferString("constant").Bytes() // want "bytes.NewBufferString[(]...[)].Bytes constructs an ephemeral Buffer only to extract its initial value and carries 0 throwaway Clone layer"
}

func byteLength(data []byte) int {
	return len(bytes.NewBuffer(bytes.Clone(slices.Clone(data))).Bytes()) // want "bytes.NewBuffer[(]...[)].Bytes constructs an ephemeral Buffer only to extract its initial value and carries 2 throwaway Clone layer"
}

func textLength(text string) int {
	return len((bytes.NewBufferString(strings.Clone(text)).Bytes())) // want "bytes.NewBufferString[(]...[)].Bytes constructs an ephemeral Buffer only to extract its initial value and carries 1 throwaway Clone layer"
}

func constantTextLength() int {
	return len(bytes.NewBufferString("constant").Bytes()) // want "bytes.NewBufferString[(]...[)].Bytes constructs an ephemeral Buffer only to extract its initial value and carries 0 throwaway Clone layer"
}

func duplicateConstantCase(value int) string {
	switch value {
	case len(bytes.NewBufferString("x").Bytes()): // want "bytes.NewBufferString[(]...[)].Bytes constructs an ephemeral Buffer only to extract its initial value and carries 0 throwaway Clone layer"
		return "runtime"
	case 1:
		return "constant"
	}
	return ""
}

var duplicateConstantKey = map[int]string{
	len(bytes.NewBufferString("x").Bytes()): "runtime", // want "bytes.NewBufferString[(]...[)].Bytes constructs an ephemeral Buffer only to extract its initial value and carries 0 throwaway Clone layer"
	1:                                       "constant",
}

func preserveSnapshot(data []byte) []byte {
	return bytes.NewBuffer(slices.Clone(data)).Bytes() // want "bytes.NewBuffer[(]...[)].Bytes constructs an ephemeral Buffer only to extract its initial value and carries 0 throwaway Clone layer"
}

// NewBufferString(...).String is a possible retention/detachment boundary.
func detachedString(text string) string {
	return bytes.NewBufferString(text).String()
}

// A specialized outer observer owns this shape.
func comparedString(data []byte) bool {
	return bytes.NewBuffer(data).String() == "payload"
}

// A conversion cannot replace a method call in a bare call-only statement.
func bareCall(data []byte) {
	bytes.NewBuffer(data).Bytes() // want "bytes.NewBuffer[(]...[)].Bytes constructs an ephemeral Buffer only to extract its initial value and carries 0 throwaway Clone layer"
}

func functionValue(data []byte) []byte {
	constructor := bytes.NewBuffer
	return constructor(data).Bytes()
}

type buffer struct{ data []byte }

func (b *buffer) Bytes() []byte     { return b.data }
func NewBuffer(data []byte) *buffer { return &buffer{data: data} }

func userConstructor(data []byte) []byte {
	return NewBuffer(data).Bytes()
}
