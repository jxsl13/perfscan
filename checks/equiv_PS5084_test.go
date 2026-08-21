package checks

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS5084CopyIncludingOverlap(t *testing.T) {
	tests := []struct {
		name                string
		initial             []byte
		dstStart, dstEnd    int
		sourceStart, srcEnd int
	}{
		{name: "disjoint", initial: []byte("abcdefgh"), dstStart: 0, dstEnd: 3, sourceStart: 4, srcEnd: 8},
		{name: "overlap forward", initial: []byte("abcdefgh"), dstStart: 0, dstEnd: 6, sourceStart: 2, srcEnd: 8},
		{name: "overlap backward", initial: []byte("abcdefgh"), dstStart: 2, dstEnd: 8, sourceStart: 0, srcEnd: 6},
		{name: "same slice", initial: []byte("abcdefgh"), dstStart: 0, dstEnd: 8, sourceStart: 0, srcEnd: 8},
		{name: "empty", initial: []byte("abcdefgh"), dstStart: 1, dstEnd: 1, sourceStart: 3, srcEnd: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := bytes.Clone(test.initial)
			after := bytes.Clone(test.initial)
			beforeN := copy(before[test.dstStart:test.dstEnd], bytes.Clone(before[test.sourceStart:test.srcEnd]))
			afterN := copy(after[test.dstStart:test.dstEnd], after[test.sourceStart:test.srcEnd])
			if beforeN != afterN || !bytes.Equal(before, after) {
				t.Fatalf("copy differs: n=%d/%d data=%q/%q", beforeN, afterN, before, after)
			}
		})
	}
}

func TestEquiv_PS5084AppendIncludingOverlap(t *testing.T) {
	tests := []struct {
		name                     string
		initial                  []int
		dstStart, dstEnd, dstCap int
		sourceStart, sourceEnd   int
	}{
		{name: "spec overlap", initial: []int{0, 0, 2, 3, 5, 7, 0, 0, 0, 0}, dstStart: 3, dstEnd: 6, dstCap: 10, sourceStart: 2, sourceEnd: 8},
		{name: "append self", initial: []int{1, 2, 3, 0, 0, 0}, dstStart: 0, dstEnd: 3, dstCap: 6, sourceStart: 0, sourceEnd: 3},
		{name: "growth allocation", initial: []int{1, 2, 3, 4}, dstStart: 0, dstEnd: 4, dstCap: 4, sourceStart: 1, sourceEnd: 4},
		{name: "empty", initial: []int{1, 2, 3}, dstStart: 0, dstEnd: 2, dstCap: 3, sourceStart: 1, sourceEnd: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beforeBacking := slices.Clone(test.initial)
			afterBacking := slices.Clone(test.initial)
			before := append(beforeBacking[test.dstStart:test.dstEnd:test.dstCap], slices.Clone(beforeBacking[test.sourceStart:test.sourceEnd])...)
			after := append(afterBacking[test.dstStart:test.dstEnd:test.dstCap], afterBacking[test.sourceStart:test.sourceEnd]...)
			if !slices.Equal(before, after) || !slices.Equal(beforeBacking, afterBacking) {
				t.Fatalf("append differs: result=%v/%v backing=%v/%v", before, after, beforeBacking, afterBacking)
			}
		})
	}
}

func TestEquiv_PS5084CopyingConversions(t *testing.T) {
	byteInputs := [][]byte{nil, {}, []byte("plain"), {0xff, 0, 0xfe, 'x'}}
	for index, input := range byteInputs {
		before := string(bytes.Clone(input))
		after := string(input)
		if before != after {
			t.Fatalf("byte-to-string input %d differs: %q/%q", index, before, after)
		}
	}
	runeInputs := [][]rune{nil, {}, {'a', '世'}, {0xd800, -1, 0x10ffff, 0x110000}}
	for index, input := range runeInputs {
		before := string(slices.Clone(input))
		after := string(input)
		if before != after {
			t.Fatalf("rune-to-string input %d differs: %q/%q", index, before, after)
		}
	}
	stringInputs := []string{"", "plain", "世", string([]byte{0xff, 0, 0xfe, 'x'})}
	for index, input := range stringInputs {
		beforeBytes, afterBytes := []byte(strings.Clone(input)), []byte(input)
		if !bytes.Equal(beforeBytes, afterBytes) || (beforeBytes == nil) != (afterBytes == nil) || cap(beforeBytes) != cap(afterBytes) {
			t.Fatalf("string-to-bytes input %d differs: %v/%v", index, beforeBytes, afterBytes)
		}
		beforeRunes, afterRunes := []rune(strings.Clone(input)), []rune(input)
		if !slices.Equal(beforeRunes, afterRunes) || (beforeRunes == nil) != (afterRunes == nil) || cap(beforeRunes) != cap(afterRunes) {
			t.Fatalf("string-to-runes input %d differs: %v/%v", index, beforeRunes, afterRunes)
		}
	}
}

type ps5084NamedBytes []byte
type ps5084NamedString string

func TestEquiv_PS5084NamedConversionTargetsCompile(t *testing.T) {
	sourceBytes := []byte{0xff, 'x', 0}
	beforeString := ps5084NamedString(bytes.Clone(sourceBytes))
	afterString := ps5084NamedString(sourceBytes)
	if beforeString != afterString {
		t.Fatalf("named string differs: %q/%q", beforeString, afterString)
	}
	sourceString := string(sourceBytes)
	beforeBytes := ps5084NamedBytes(strings.Clone(sourceString))
	afterBytes := ps5084NamedBytes(sourceString)
	if !bytes.Equal(beforeBytes, afterBytes) {
		t.Fatalf("named bytes differ: %v/%v", beforeBytes, afterBytes)
	}
}

func TestEquiv_PS5084StringConversionFixedPoints(t *testing.T) {
	inputs := []string{"", "plain", "世界", string([]byte{0xff, 0, 0xfe, 'x'})}
	for index, input := range inputs {
		beforeDestination := make([]byte, len(input)+2)
		afterDestination := make([]byte, len(input)+2)
		beforeN := copy(beforeDestination, bytes.Clone([]byte(strings.Clone(strings.Clone(input)))))
		afterN := copy(afterDestination, input)
		if beforeN != afterN || !bytes.Equal(beforeDestination, afterDestination) {
			t.Fatalf("copy input %d differs: n=%d/%d data=%v/%v", index, beforeN, afterN, beforeDestination, afterDestination)
		}

		beforeAppend := append([]byte("prefix:"), bytes.Clone([]byte(strings.Clone(input)))...)
		afterAppend := append([]byte("prefix:"), input...)
		if !bytes.Equal(beforeAppend, afterAppend) || cap(beforeAppend) != cap(afterAppend) {
			t.Fatalf("append input %d differs: %v/%v cap=%d/%d", index, beforeAppend, afterAppend, cap(beforeAppend), cap(afterAppend))
		}

		beforeString := string(bytes.Clone([]byte(strings.Clone(input))))
		if beforeString != input {
			t.Fatalf("string round-trip input %d differs: %q/%q", index, beforeString, input)
		}
	}
}
