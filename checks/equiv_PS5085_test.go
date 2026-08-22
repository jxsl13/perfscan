package checks

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS5085BufferWriteIncludingOverlap(t *testing.T) {
	inputs := [][]byte{nil, {}, []byte("plain"), {0xff, 0, 0xfe, 'x'}}
	for index, input := range inputs {
		var before, after bytes.Buffer
		before.Grow(len(input) + 16)
		after.Grow(len(input) + 16)
		beforeN, beforeErr := before.Write(bytes.Clone(slices.Clone(input)))
		afterN, afterErr := after.Write(input)
		if beforeN != afterN || beforeErr != afterErr || !bytes.Equal(before.Bytes(), after.Bytes()) || before.Cap() != after.Cap() {
			t.Fatalf("input %d differs: n=%d/%d err=%v/%v bytes=%v/%v cap=%d/%d", index, beforeN, afterN, beforeErr, afterErr, before.Bytes(), after.Bytes(), before.Cap(), after.Cap())
		}
	}

	before := bytes.NewBuffer(make([]byte, 0, 32))
	after := bytes.NewBuffer(make([]byte, 0, 32))
	_, _ = before.WriteString("overlap")
	_, _ = after.WriteString("overlap")
	beforeSource := before.Bytes()[1:]
	afterSource := after.Bytes()[1:]
	beforeN, beforeErr := before.Write(bytes.Clone(beforeSource))
	afterN, afterErr := after.Write(afterSource)
	if beforeN != afterN || beforeErr != afterErr || !bytes.Equal(before.Bytes(), after.Bytes()) || before.Cap() != after.Cap() {
		t.Fatalf("overlap differs: n=%d/%d err=%v/%v bytes=%q/%q cap=%d/%d", beforeN, afterN, beforeErr, afterErr, before.Bytes(), after.Bytes(), before.Cap(), after.Cap())
	}
}

func TestEquiv_PS5085BuilderAndStringWrites(t *testing.T) {
	inputs := []string{"", "plain", "世界", string([]byte{0xff, 0, 0xfe, 'x'})}
	for index, input := range inputs {
		var beforeBuffer, afterBuffer bytes.Buffer
		beforeBuffer.Grow(len(input) + 8)
		afterBuffer.Grow(len(input) + 8)
		beforeN, beforeErr := beforeBuffer.WriteString(strings.Clone(input))
		afterN, afterErr := afterBuffer.WriteString(input)
		if beforeN != afterN || beforeErr != afterErr || beforeBuffer.String() != afterBuffer.String() || beforeBuffer.Cap() != afterBuffer.Cap() {
			t.Fatalf("buffer string input %d differs", index)
		}

		var beforeBuilder, afterBuilder strings.Builder
		beforeBuilder.Grow(len(input) + 8)
		afterBuilder.Grow(len(input) + 8)
		beforeN, beforeErr = beforeBuilder.Write(bytes.Clone([]byte(input)))
		afterN, afterErr = afterBuilder.Write([]byte(input))
		if beforeN != afterN || beforeErr != afterErr || beforeBuilder.String() != afterBuilder.String() || beforeBuilder.Cap() != afterBuilder.Cap() {
			t.Fatalf("builder bytes input %d differs", index)
		}
	}

	var before, after strings.Builder
	_, _ = before.WriteString("self")
	_, _ = after.WriteString("self")
	beforeView, afterView := before.String(), after.String()
	beforeN, beforeErr := before.WriteString(strings.Clone(beforeView))
	afterN, afterErr := after.WriteString(afterView)
	if beforeN != afterN || beforeErr != afterErr || before.String() != after.String() || before.Cap() != after.Cap() {
		t.Fatalf("builder self-view differs: n=%d/%d err=%v/%v strings=%q/%q cap=%d/%d", beforeN, afterN, beforeErr, afterErr, before.String(), after.String(), before.Cap(), after.Cap())
	}
}
