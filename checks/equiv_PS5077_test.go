package checks

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS5077ConstantTrim(t *testing.T) {
	stringFns := []struct {
		name string
		fn   func(string, string) string
	}{
		{"Trim", strings.Trim},
		{"TrimLeft", strings.TrimLeft},
		{"TrimRight", strings.TrimRight},
	}
	inputs := []string{
		"", "payload", "xxpayloadyy", "xxxxx", " y payload x ",
		"ββpayloadβ", string([]byte{0xff, 'x', 'p', 0xfe, 'y'}),
	}
	cutsets := []string{"", "xy", " \t\r\n", "β\u00a0", string([]byte{0xff}), "\uFFFD"}
	for _, fn := range stringFns {
		for _, input := range inputs {
			for _, cutset := range cutsets {
				before := fn.fn(fn.fn(input, cutset), cutset)
				after := fn.fn(input, cutset)
				if before != after {
					t.Fatalf("strings.%s differs for input %q cutset %q: %q != %q", fn.name, input, cutset, before, after)
				}
			}
		}
	}

	byteFns := []struct {
		name string
		fn   func([]byte, string) []byte
	}{
		{"Trim", bytes.Trim},
		{"TrimLeft", bytes.TrimLeft},
		{"TrimRight", bytes.TrimRight},
	}
	byteInputs := [][]byte{
		nil, {}, []byte("payload"), []byte("xxpayloadyy"), []byte("xxxxx"),
		[]byte("ββpayloadβ"), {0xff, 'x', 'p', 0xfe, 'y'},
	}
	for _, fn := range byteFns {
		for i, input := range byteInputs {
			for _, cutset := range cutsets {
				beforeBacking := ps5077Backing(input)
				afterBacking := ps5077Backing(input)
				before := fn.fn(fn.fn(beforeBacking, cutset), cutset)
				after := fn.fn(afterBacking, cutset)
				if (before == nil) != (after == nil) || len(before) != len(after) || cap(before) != cap(after) ||
					!bytes.Equal(before, after) || !slices.Equal(beforeBacking, afterBacking) {
					t.Fatalf("bytes.%s case %d cutset %q differs: before=%q len/cap=%d/%d after=%q len/cap=%d/%d", fn.name, i, cutset, before, len(before), cap(before), after, len(after), cap(after))
				}
				if before != nil && cap(beforeBacking)-cap(before) != cap(afterBacking)-cap(after) {
					t.Fatalf("bytes.%s case %d cutset %q changed subslice start", fn.name, i, cutset)
				}
			}
		}
	}
}

func ps5077Backing(input []byte) []byte {
	if input == nil {
		return nil
	}
	backing := make([]byte, len(input), len(input)+7)
	copy(backing, input)
	return backing
}
