package checks

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS5094EphemeralBufferExtraction(t *testing.T) {
	byteInputs := [][]byte{nil, {}, make([]byte, 0, 9), []byte("payload"), {0xff, 0, 0xfe}}
	for index, input := range byteInputs {
		beforeBytes := bytes.NewBuffer(input).Bytes()
		afterBytes := []byte(input)
		if (beforeBytes == nil) != (afterBytes == nil) || len(beforeBytes) != len(afterBytes) || cap(beforeBytes) != cap(afterBytes) || !bytes.Equal(beforeBytes, afterBytes) {
			t.Fatalf("Bytes input %d differs: before nil/len/cap=%v/%d/%d after=%v/%d/%d", index, beforeBytes == nil, len(beforeBytes), cap(beforeBytes), afterBytes == nil, len(afterBytes), cap(afterBytes))
		}
		if before, after := bytes.NewBuffer(bytes.Clone(slices.Clone(input))).String(), string(input); before != after {
			t.Fatalf("String input %d differs: before=%q after=%q", index, before, after)
		}
		if before, after := len(bytes.NewBuffer(bytes.Clone(input)).Bytes()), len(input); before != after {
			t.Fatalf("len(Bytes) input %d differs: before=%d after=%d", index, before, after)
		}
	}

	stringInputs := []string{"", "payload", "世界", string([]byte{0xff, 'x', 0xfe})}
	for index, input := range stringInputs {
		before := bytes.NewBufferString(strings.Clone(strings.Clone(input))).Bytes()
		after := []byte(input)
		if (before == nil) != (after == nil) || len(before) != len(after) || cap(before) != cap(after) || !bytes.Equal(before, after) {
			t.Fatalf("NewBufferString.Bytes input %d differs: before nil/len/cap=%v/%d/%d after=%v/%d/%d", index, before == nil, len(before), cap(before), after == nil, len(after), cap(after))
		}
	}
}

func TestEquiv_PS5094EvaluationCountAndStringCopy(t *testing.T) {
	calls := 0
	produce := func() []byte {
		calls++
		return []byte("mutable")
	}
	before := bytes.NewBuffer(produce()).String()
	if calls != 1 {
		t.Fatalf("constructor chain evaluated input %d times", calls)
	}
	calls = 0
	after := string(produce())
	if calls != 1 || before != after {
		t.Fatalf("conversion evaluated input %d times or changed result: before=%q after=%q", calls, before, after)
	}

	input := []byte("copy boundary")
	before = bytes.NewBuffer(input).String()
	after = string(input)
	for index := range input {
		input[index] = 'x'
	}
	if before != "copy boundary" || after != "copy boundary" {
		t.Fatalf("byte-to-string independence changed: before=%q after=%q", before, after)
	}
}
