package ps5094comment

import (
	"bytes"
	"strings"
)

func extract(text string) []byte {
	return bytes.NewBufferString( /* preserve rationale */ strings.Clone(text)).Bytes() // want "bytes.NewBufferString[(]...[)].Bytes constructs an ephemeral Buffer only to extract its initial value and carries 1 throwaway Clone layer"
}
