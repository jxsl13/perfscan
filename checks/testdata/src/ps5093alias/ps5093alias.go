package ps5093alias

import (
	b "bytes"
	fmtalias "fmt"
	seq "slices"
	text "strings"
)

var _ = fmtalias.Sprintf

func sizes(data []byte, value string) (int, int64) {
	return b.NewReader(b.Clone(seq.Clone(data))).Len(), text.NewReader(text.Clone(value)).Size() // want "bytes.NewReader[(]...[)].Len constructs an ephemeral container only to recover its input length and carries 2 throwaway Clone layer" "strings.NewReader[(]...[)].Size constructs an ephemeral container only to recover its input length and carries 1 throwaway Clone layer"
}
