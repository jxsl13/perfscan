package checks

import (
	"bytes"
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS5092StringComparisons(t *testing.T) {
	inputs := []string{"", "a", "payload", "世界", string([]byte{0xff, 'x', 0xfe})}
	for leftIndex, left := range inputs {
		for rightIndex, right := range inputs {
			beforeLeft := strings.Clone(strings.Clone(left))
			beforeRight := strings.Clone(right)
			before := [6]bool{
				beforeLeft == beforeRight,
				beforeLeft != beforeRight,
				beforeLeft < beforeRight,
				beforeLeft <= beforeRight,
				beforeLeft > beforeRight,
				beforeLeft >= beforeRight,
			}
			after := [6]bool{left == right, left != right, left < right, left <= right, left > right, left >= right}
			if before != after {
				t.Fatalf("string comparison %d/%d differs: clone=%v direct=%v", leftIndex, rightIndex, before, after)
			}
		}
	}
}

func TestEquiv_PS5092ContainerNilness(t *testing.T) {
	byteInputs := [][]byte{nil, {}, []byte("payload")}
	for index, input := range byteInputs {
		before := bytes.Clone(slices.Clone(bytes.Clone(input))) == nil
		after := input == nil
		if before != after {
			t.Fatalf("byte nilness %d differs: clone=%v direct=%v", index, before, after)
		}
	}
	sliceInputs := [][]int{nil, {}, {1, 2, 3}}
	for index, input := range sliceInputs {
		if before, after := slices.Clone(input) != nil, input != nil; before != after {
			t.Fatalf("slice nilness %d differs: clone=%v direct=%v", index, before, after)
		}
	}
	mapInputs := []map[string]int{nil, {}, {"key": 1}}
	for index, input := range mapInputs {
		if before, after := maps.Clone(maps.Clone(input)) == nil, input == nil; before != after {
			t.Fatalf("map nilness %d differs: clone=%v direct=%v", index, before, after)
		}
	}
}
