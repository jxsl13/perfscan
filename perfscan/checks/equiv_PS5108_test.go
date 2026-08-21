package checks

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS5108StringsRepeat(t *testing.T) {
	seeds := []string{"", "a", "ab", "-", "é\x00", string([]byte{0xff, 'x'})}
	for _, seed := range seeds {
		before := strings.Repeat(strings.Repeat(strings.Repeat(seed, 2), 3), 4)
		after := strings.Repeat(seed, 24)
		if before != after {
			t.Fatalf("string repeat differs for %q: before=%q after=%q", seed, before, after)
		}
	}
}

func TestEquiv_PS5108BytesRepeat(t *testing.T) {
	inputs := [][]byte{nil, {}, {'a'}, {'a', 0, 0xff}}
	for index, input := range inputs {
		original := bytes.Clone(input)
		before := bytes.Repeat(bytes.Repeat(input, 3), 2)
		after := bytes.Repeat(input, 6)
		if !bytes.Equal(before, after) || (before == nil) != (after == nil) || len(before) != len(after) || cap(before) != cap(after) {
			t.Fatalf("case %d slice shape differs: before=(%v,len=%d,cap=%d,nil=%v) after=(%v,len=%d,cap=%d,nil=%v)",
				index, before, len(before), cap(before), before == nil, after, len(after), cap(after), after == nil)
		}
		if len(before) > 0 {
			before[0] ^= 0xff
			after[0] ^= 0xff
		}
		if !bytes.Equal(before, after) || !bytes.Equal(input, original) {
			t.Fatalf("case %d mutation/independence differs: input=%v original=%v before=%v after=%v", index, input, original, before, after)
		}
	}
}

type ps5108NamedInts []int

func TestEquiv_PS5108SlicesRepeat(t *testing.T) {
	inputs := []ps5108NamedInts{nil, {}, {1}, {1, 2, 3}}
	for index, input := range inputs {
		original := slices.Clone(input)
		before := slices.Repeat[ps5108NamedInts](slices.Repeat[ps5108NamedInts](input, 2), 3)
		after := slices.Repeat[ps5108NamedInts](input, 6)
		if !slices.Equal(before, after) || (before == nil) != (after == nil) || len(before) != len(after) || cap(before) != cap(after) {
			t.Fatalf("case %d named slice shape differs: before=(%v,len=%d,cap=%d,nil=%v) after=(%v,len=%d,cap=%d,nil=%v)",
				index, before, len(before), cap(before), before == nil, after, len(after), cap(after), after == nil)
		}
		if len(before) > 0 {
			before[0]++
			after[0]++
		}
		if !slices.Equal(before, after) || !slices.Equal(input, original) {
			t.Fatalf("case %d named slice mutation/independence differs: input=%v original=%v before=%v after=%v", index, input, original, before, after)
		}
	}
}

func TestEquiv_PS5108SlicesRepeatIsShallow(t *testing.T) {
	value := 7
	input := []*int{&value}
	before := slices.Repeat(slices.Repeat(input, 2), 3)
	after := slices.Repeat(input, 6)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("shallow element aliases differ: before=%v after=%v", before, after)
	}
	for index := range before {
		if before[index] != &value || after[index] != &value {
			t.Fatalf("element %d no longer aliases the original pointer", index)
		}
	}
}

func TestEquiv_PS5108MixedGenericResultTypeIsDeliberatelyExcluded(t *testing.T) {
	input := ps5108NamedInts{1, 2}
	var nested any = slices.Repeat[[]int](slices.Repeat[ps5108NamedInts](input, 2), 3)
	var collapsedInner any = slices.Repeat[ps5108NamedInts](input, 6)
	if reflect.TypeOf(nested) == reflect.TypeOf(collapsedInner) {
		t.Fatalf("mixed generic exclusion witness lost: nested=%T collapsed=%T", nested, collapsedInner)
	}
}

func TestEquiv_PS5108SeedEvaluation(t *testing.T) {
	run := func(flat bool) (string, int) {
		calls := 0
		seed := func() string {
			calls++
			return "ab"
		}
		if flat {
			return strings.Repeat(seed(), 6), calls
		}
		return strings.Repeat(strings.Repeat(seed(), 2), 3), calls
	}
	before, beforeCalls := run(false)
	after, afterCalls := run(true)
	if before != after || beforeCalls != afterCalls || afterCalls != 1 {
		t.Fatalf("seed evaluation differs: before=(%q,%d) after=(%q,%d)", before, beforeCalls, after, afterCalls)
	}
}
