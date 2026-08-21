package checks

import (
	"bytes"
	"strings"
	"testing"
)

func TestEquiv_PS5080NoopReplacementChain(t *testing.T) {
	stringInputs := []string{
		"",
		"payload",
		"aaaabaaa",
		"日本語",
		string([]byte{0xff, 'a', 0xfe, 'b'}),
	}
	for _, input := range stringInputs {
		if got := strings.ReplaceAll(input, "x", "x"); got != input {
			t.Fatalf("ReplaceAll equal old/new changed %q to %q", input, got)
		}
		if got := ps5080StringReplace(input, "x", "different", 0); got != input {
			t.Fatalf("Replace n=0 changed %q to %q", input, got)
		}
		before := strings.ReplaceAll(
			ps5080StringReplace(
				strings.ReplaceAll(input, "a", "a"),
				"old", "new", 0,
			),
			"", "",
		)
		if before != input {
			t.Fatalf("deep strings no-op chain changed %q to %q", input, before)
		}
	}

	inputs := [][]byte{
		nil,
		make([]byte, 0, 11),
		ps5080Spare([]byte("payload")),
		ps5080Spare([]byte("aaaabaaa")),
		ps5080Spare([]byte{0xff, 'a', 0xfe, 'b'}),
	}
	patterns := [][]byte{nil, {}, []byte("a"), []byte("ab"), {0xff}}
	for inputIndex, input := range inputs {
		for patternIndex, pattern := range patterns {
			beforeInput := ps5080DuplicateHeader(input)
			afterInput := ps5080DuplicateHeader(input)
			before := bytes.ReplaceAll(
				ps5080BytesReplace(
					bytes.ReplaceAll(beforeInput, []byte("q"), []byte("q")),
					[]byte("old"), []byte("new"), 0,
				),
				pattern, pattern,
			)
			after := bytes.ReplaceAll(afterInput, pattern, pattern)
			ps5080CheckByteContract(t, inputIndex, patternIndex, beforeInput, afterInput, before, after)

			beforeInput = ps5080DuplicateHeader(input)
			afterInput = ps5080DuplicateHeader(input)
			before = ps5080BytesReplace(
				bytes.ReplaceAll(
					ps5080BytesReplace(beforeInput, []byte("x"), []byte("x"), -1),
					[]byte("y"), []byte("y"),
				),
				pattern, []byte("different"), 0,
			)
			after = ps5080BytesReplace(afterInput, pattern, []byte("different"), 0)
			ps5080CheckByteContract(t, inputIndex, patternIndex, beforeInput, afterInput, before, after)
		}
	}
}

func ps5080StringReplace(input, old, new string, n int) string {
	return strings.Replace(input, old, new, n)
}

func ps5080BytesReplace(input, old, new []byte, n int) []byte {
	return bytes.Replace(input, old, new, n)
}

func ps5080Spare(value []byte) []byte {
	result := make([]byte, len(value), len(value)+13)
	copy(result, value)
	return result
}

func ps5080DuplicateHeader(value []byte) []byte {
	if value == nil {
		return nil
	}
	result := make([]byte, len(value), cap(value))
	copy(result, value)
	return result
}

func ps5080CheckByteContract(t *testing.T, inputIndex, patternIndex int, beforeInput, afterInput, before, after []byte) {
	t.Helper()
	if (before == nil) != (after == nil) || len(before) != len(after) || cap(before) != cap(after) || !bytes.Equal(before, after) {
		t.Fatalf("input %d pattern %d: chain header/content = nil:%v len/cap:%d/%d %q; outer-only = nil:%v len/cap:%d/%d %q",
			inputIndex, patternIndex, before == nil, len(before), cap(before), before,
			after == nil, len(after), cap(after), after)
	}
	if len(before) > 0 {
		beforeAliases := &before[0] == &beforeInput[0]
		afterAliases := &after[0] == &afterInput[0]
		if beforeAliases != afterAliases || beforeAliases {
			t.Fatalf("input %d pattern %d: copy/alias contract differs (chain=%v outer-only=%v)", inputIndex, patternIndex, beforeAliases, afterAliases)
		}
	}
}
