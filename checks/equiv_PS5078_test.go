package checks

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS5078WhitespaceTrimComposition(t *testing.T) {
	stringFns := []struct {
		name string
		fn   func(string, string) string
	}{
		{"Trim", strings.Trim},
		{"TrimLeft", strings.TrimLeft},
		{"TrimRight", strings.TrimRight},
	}
	inputs := []string{
		"", "payload", " \tpayload\r\n", "\u00a0\u2003payload\u3000", " \t\u00a0 ",
		string([]byte{' ', 0xff, 'p', 0xfe, '\t'}),
	}
	cutsets := []string{"", " ", "\t\r\n", "\u00a0\u2003", " \t\r\n\u0085\u00a0\u1680\u2000\u2028\u2029\u205f\u3000"}
	for _, fn := range stringFns {
		for _, input := range inputs {
			for _, cutset := range cutsets {
				if before, after := strings.TrimSpace(fn.fn(input, cutset)), strings.TrimSpace(input); before != after {
					t.Fatalf("strings.TrimSpace(strings.%s) differs for input %q cutset %q: %q != %q", fn.name, input, cutset, before, after)
				}
				if before, after := fn.fn(strings.TrimSpace(input), cutset), strings.TrimSpace(input); before != after {
					t.Fatalf("strings.%s(strings.TrimSpace) differs for input %q cutset %q: %q != %q", fn.name, input, cutset, before, after)
				}
			}
		}
	}

	for _, input := range inputs {
		before := strings.TrimSpace(strings.Trim(strings.TrimLeft(strings.TrimRight(input, "\n"), "\t"), " \u00a0"))
		after := strings.TrimSpace(input)
		if before != after {
			t.Fatalf("deep strings chain differs for %q: %q != %q", input, before, after)
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
		nil, {}, []byte("payload"), []byte(" \tpayload\r\n"), []byte("\u00a0\u2003payload\u3000"),
		{' ', 0xff, 'p', 0xfe, '\t'},
	}
	for _, fn := range byteFns {
		for i, input := range byteInputs {
			for _, cutset := range cutsets {
				beforeBacking := ps5078Backing(input)
				afterBacking := ps5078Backing(input)
				before := bytes.TrimSpace(fn.fn(beforeBacking, cutset))
				after := bytes.TrimSpace(afterBacking)
				ps5078CheckBytes(t, fn.name+" before TrimSpace", i, cutset, beforeBacking, afterBacking, before, after)

				beforeBacking = ps5078Backing(input)
				afterBacking = ps5078Backing(input)
				before = fn.fn(bytes.TrimSpace(beforeBacking), cutset)
				after = bytes.TrimSpace(afterBacking)
				ps5078CheckBytes(t, fn.name+" after TrimSpace", i, cutset, beforeBacking, afterBacking, before, after)
			}
		}
	}
}

func ps5078Backing(input []byte) []byte {
	if input == nil {
		return nil
	}
	backing := make([]byte, len(input), len(input)+9)
	copy(backing, input)
	return backing
}

func ps5078CheckBytes(t *testing.T, name string, inputCase int, cutset string, beforeBacking, afterBacking, before, after []byte) {
	t.Helper()
	if (before == nil) != (after == nil) || len(before) != len(after) || cap(before) != cap(after) ||
		!bytes.Equal(before, after) || !slices.Equal(beforeBacking, afterBacking) {
		t.Fatalf("bytes.%s case %d cutset %q differs: before=%q len/cap=%d/%d after=%q len/cap=%d/%d", name, inputCase, cutset, before, len(before), cap(before), after, len(after), cap(after))
	}
	if before != nil && cap(beforeBacking)-cap(before) != cap(afterBacking)-cap(after) {
		t.Fatalf("bytes.%s case %d cutset %q changed subslice start", name, inputCase, cutset)
	}
}
