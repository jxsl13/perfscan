package checks

import (
	"maps"
	"strings"
	"testing"
)

func TestEquiv_PS5091StringIndexAndMapLookup(t *testing.T) {
	inputs := []string{"a", "payload", "世界", string([]byte{0xff, 'x', 0xfe})}
	for inputIndex, input := range inputs {
		for index := range len(input) {
			beforeValue, beforePanic := ps5091IndexResult(strings.Clone(strings.Clone(input)), index)
			afterValue, afterPanic := ps5091IndexResult(input, index)
			if beforeValue != afterValue || beforePanic != afterPanic {
				t.Fatalf("string index %d/%d differs: value=%d/%d panic=%v/%v", inputIndex, index, beforeValue, afterValue, beforePanic, afterPanic)
			}
		}
		_, beforePanic := ps5091IndexResult(strings.Clone(input), len(input))
		_, afterPanic := ps5091IndexResult(input, len(input))
		if beforePanic != afterPanic || !afterPanic {
			t.Fatalf("bounds panic %d differs: clone=%v direct=%v", inputIndex, beforePanic, afterPanic)
		}
	}

	keys := []string{"", "payload", "世界", string([]byte{0xff, 'k'})}
	values := map[string]int{"": 1, "payload": 2, "世界": 3, string([]byte{0xff, 'k'}): 4}
	interfaceValues := map[any]int{"payload": 9}
	for index, key := range keys {
		beforeValue, beforeOK := values[strings.Clone(strings.Clone(key))]
		afterValue, afterOK := values[key]
		if beforeValue != afterValue || beforeOK != afterOK {
			t.Fatalf("map lookup %d differs: clone=%d,%v direct=%d,%v", index, beforeValue, beforeOK, afterValue, afterOK)
		}
		if before, after := interfaceValues[strings.Clone(key)], interfaceValues[key]; before != after {
			t.Fatalf("interface-key map lookup %d differs: clone=%d direct=%d", index, before, after)
		}
	}
	var nilMap map[string]int
	if before, after := nilMap[strings.Clone("missing")], nilMap["missing"]; before != after {
		t.Fatalf("nil map lookup differs: clone=%d direct=%d", before, after)
	}
}

func TestEquiv_PS5091DeleteAndNestedLookup(t *testing.T) {
	type record struct{ count int }
	beforeRecords := map[string]*record{"key": {count: 1}}
	afterRecords := map[string]*record{"key": {count: 1}}
	beforeRecords[strings.Clone("key")].count++
	afterRecords["key"].count++
	if beforeRecords["key"].count != afterRecords["key"].count {
		t.Fatalf("nested lookup differs: clone=%d direct=%d", beforeRecords["key"].count, afterRecords["key"].count)
	}

	before := map[string]int{"keep": 1, "remove": 2}
	after := maps.Clone(before)
	delete(before, strings.Clone(strings.Clone("remove")))
	delete(after, "remove")
	if !maps.Equal(before, after) {
		t.Fatalf("delete differs: clone=%v direct=%v", before, after)
	}
}

func ps5091IndexResult(value string, index int) (result byte, panicked bool) {
	defer func() {
		panicked = recover() != nil
	}()
	return value[index], false
}
