package ps5094alias

import (
	b "bytes"
	fmtalias "fmt"
	seq "slices"
	textpkg "strings"
)

var _ = fmtalias.Sprintf

func extract(data []byte, value string) (string, []byte, int) {
	return b.NewBuffer(b.Clone(seq.Clone(data))).String(), b.NewBufferString(textpkg.Clone(value)).Bytes(), len(b.NewBuffer(b.Clone(data)).Bytes()) // want "bytes.NewBuffer[(]...[)].String constructs an ephemeral Buffer only to extract its initial value and carries 2 throwaway Clone layer" "bytes.NewBufferString[(]...[)].Bytes constructs an ephemeral Buffer only to extract its initial value and carries 1 throwaway Clone layer" "bytes.NewBuffer[(]...[)].Bytes constructs an ephemeral Buffer only to extract its initial value and carries 1 throwaway Clone layer"
}
