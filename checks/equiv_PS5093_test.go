package checks

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS5093EphemeralReaderBufferSizes(t *testing.T) {
	stringInputs := []string{"", "payload", "世界", string([]byte{0xff, 'x', 0xfe})}
	for index, input := range stringInputs {
		if before, after := strings.NewReader(strings.Clone(strings.Clone(input))).Len(), len(input); before != after {
			t.Fatalf("strings.Reader.Len input %d differs: chain=%d len=%d", index, before, after)
		}
		if before, after := strings.NewReader(strings.Clone(input)).Size(), int64(len(input)); before != after {
			t.Fatalf("strings.Reader.Size input %d differs: chain=%d len=%d", index, before, after)
		}
		if before, after := bytes.NewBufferString(strings.Clone(input)).Len(), len(input); before != after {
			t.Fatalf("bytes.BufferString.Len input %d differs: chain=%d len=%d", index, before, after)
		}
		if before, after := bytes.NewReader(bytes.Clone([]byte(strings.Clone(input)))).Size(), int64(len(input)); before != after {
			t.Fatalf("bytes.Reader conversion Size input %d differs: chain=%d len=%d", index, before, after)
		}
	}

	byteInputs := [][]byte{nil, {}, []byte("payload"), {0xff, 0, 0xfe}}
	for index, input := range byteInputs {
		if before, after := bytes.NewReader(bytes.Clone(slices.Clone(input))).Len(), len(input); before != after {
			t.Fatalf("bytes.Reader.Len input %d differs: chain=%d len=%d", index, before, after)
		}
		if before, after := bytes.NewReader(slices.Clone(input)).Size(), int64(len(input)); before != after {
			t.Fatalf("bytes.Reader.Size input %d differs: chain=%d len=%d", index, before, after)
		}
		if before, after := bytes.NewBuffer(bytes.Clone(input)).Len(), len(input); before != after {
			t.Fatalf("bytes.Buffer.Len input %d differs: chain=%d len=%d", index, before, after)
		}
	}
}
