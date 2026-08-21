package checks

import (
	"bytes"
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS5074NestedClone(t *testing.T) {
	byteCases := [][]byte{nil, {}, make([]byte, 0, 32), []byte("payload"), make([]byte, 257)}
	for i, input := range byteCases {
		before := bytes.Clone(bytes.Clone(input))
		after := bytes.Clone(input)
		if (before == nil) != (after == nil) || len(before) != len(after) || cap(before) != cap(after) || !bytes.Equal(before, after) {
			t.Fatalf("bytes case %d differs: before nil/len/cap=%v/%d/%d after=%v/%d/%d", i, before == nil, len(before), cap(before), after == nil, len(after), cap(after))
		}
		if len(after) > 0 {
			original := input[0]
			before[0] ^= 1
			after[0] ^= 2
			if input[0] != original {
				t.Fatalf("bytes case %d aliases input", i)
			}
		}
	}

	sliceCases := [][]int{nil, {}, make([]int, 0, 32), {1, 2, 3}, make([]int, 257)}
	for i, input := range sliceCases {
		before := slices.Clone(slices.Clone(input))
		after := slices.Clone(input)
		if (before == nil) != (after == nil) || len(before) != len(after) || cap(before) != cap(after) || !slices.Equal(before, after) {
			t.Fatalf("slices case %d differs", i)
		}
		if len(after) > 0 {
			original := input[0]
			before[0]++
			after[0] += 2
			if input[0] != original {
				t.Fatalf("slices case %d aliases input", i)
			}
		}
	}

	for _, input := range []string{"", "a", "perfscan", strings.Repeat("x", 257)} {
		if before, after := strings.Clone(strings.Clone(input)), strings.Clone(input); before != after {
			t.Fatalf("strings differ for len %d", len(input))
		}
	}

	mapCases := []map[string]int{nil, {}, {"a": 1, "b": 2}}
	for i, input := range mapCases {
		before := maps.Clone(maps.Clone(input))
		after := maps.Clone(input)
		if (before == nil) != (after == nil) || !maps.Equal(before, after) {
			t.Fatalf("maps case %d differs", i)
		}
		if after != nil {
			after["new"] = 3
			if _, aliases := input["new"]; aliases {
				t.Fatalf("maps case %d aliases input", i)
			}
		}
	}
}
