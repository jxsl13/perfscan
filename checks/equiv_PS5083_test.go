package checks

import (
	"bytes"
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS5083LenOfClone(t *testing.T) {
	byteInputs := [][]byte{nil, {}, []byte("payload"), {0xff, 'a', 0xfe}}
	for i, input := range byteInputs {
		before := len(bytes.Clone(slices.Clone(bytes.Clone(input))))
		if after := len(input); before != after {
			t.Fatalf("byte input %d: clone length=%d direct=%d", i, before, after)
		}
	}

	sliceInputs := [][]int{nil, {}, {1}, {1, 2, 3}}
	for i, input := range sliceInputs {
		if before, after := len(slices.Clone(input)), len(input); before != after {
			t.Fatalf("slice input %d: clone length=%d direct=%d", i, before, after)
		}
	}

	mapInputs := []map[string]int{nil, {}, {"a": 1}, {"a": 1, "b": 2}}
	for i, input := range mapInputs {
		if before, after := len(maps.Clone(input)), len(input); before != after {
			t.Fatalf("map input %d: clone length=%d direct=%d", i, before, after)
		}
	}

	stringInputs := []string{"", "payload", "日本語", string([]byte{0xff, 'a', 0xfe})}
	for i, input := range stringInputs {
		if before, after := len(strings.Clone(input)), len(input); before != after {
			t.Fatalf("string input %d: clone length=%d direct=%d", i, before, after)
		}
	}
}

func TestEquiv_PS5083StringByteFixedPoint(t *testing.T) {
	inputs := []string{"", "payload", "日本語", string([]byte{0xff, 'a', 0xfe})}
	for index, input := range inputs {
		before := len(bytes.Clone([]byte(strings.Clone(strings.Clone(input)))))
		if after := len(input); before != after {
			t.Fatalf("input %d: clone/conversion length=%d direct=%d", index, before, after)
		}
	}
}
