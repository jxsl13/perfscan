package checks

import (
	"bytes"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

func TestEquivPS5128StringSplitFamilies(t *testing.T) {
	random := rand.New(rand.NewSource(5128))
	counts := []int{-5, -2, -1, 1}
	for iteration := 0; iteration < 75_000; iteration++ {
		value := string(ps5128RandomBytes(random, random.Intn(192)))
		separator := string(ps5128RandomBytes(random, random.Intn(13)))
		count := counts[random.Intn(len(counts))]
		checks := []struct {
			name   string
			before []string
			after  []string
		}{
			{name: "Split", before: ps5128BeforeString(value, separator, strings.Split), after: strings.Split(value, separator)},
			{name: "SplitAfter", before: ps5128BeforeString(value, separator, strings.SplitAfter), after: strings.SplitAfter(value, separator)},
			{name: "SplitN", before: ps5128BeforeStringN(value, separator, count, strings.SplitN), after: strings.SplitN(value, separator, count)},
			{name: "SplitAfterN", before: ps5128BeforeStringN(value, separator, count, strings.SplitAfterN), after: strings.SplitAfterN(value, separator, count)},
		}
		for _, check := range checks {
			if !reflect.DeepEqual(check.before, check.after) || len(check.before) != len(check.after) || cap(check.before) != cap(check.after) {
				t.Fatalf("%s divergence: value=%q separator=%q count=%d before=%#v len/cap=%d/%d after=%#v len/cap=%d/%d",
					check.name, value, separator, count, check.before, len(check.before), cap(check.before), check.after, len(check.after), cap(check.after))
			}
		}
	}
}

func TestEquivPS5128ByteSplitFamiliesAndAliases(t *testing.T) {
	random := rand.New(rand.NewSource(5128001))
	counts := []int{-5, -2, -1, 1}
	for iteration := 0; iteration < 75_000; iteration++ {
		length := random.Intn(192)
		value := make([]byte, length, length+random.Intn(24))
		_, _ = random.Read(value)
		separator := ps5128RandomBytes(random, random.Intn(13))
		count := counts[random.Intn(len(counts))]
		checks := []struct {
			name   string
			before [][]byte
			after  [][]byte
		}{
			{name: "Split", before: ps5128BeforeBytes(value, separator, bytes.Split), after: bytes.Split(value, separator)},
			{name: "SplitAfter", before: ps5128BeforeBytes(value, separator, bytes.SplitAfter), after: bytes.SplitAfter(value, separator)},
			{name: "SplitN", before: ps5128BeforeBytesN(value, separator, count, bytes.SplitN), after: bytes.SplitN(value, separator, count)},
			{name: "SplitAfterN", before: ps5128BeforeBytesN(value, separator, count, bytes.SplitAfterN), after: bytes.SplitAfterN(value, separator, count)},
		}
		for _, check := range checks {
			ps5128AssertByteResult(t, check.name, value, separator, count, check.before, check.after)
		}
	}

	var nilValue []byte
	for _, separator := range [][]byte{nil, []byte(":"), []byte{}} {
		ps5128AssertByteResult(t, "nil Split", nilValue, separator, -1,
			ps5128BeforeBytes(nilValue, separator, bytes.Split), bytes.Split(nilValue, separator))
	}
}

func TestEquivPS5128ExcludedBoundaries(t *testing.T) {
	value := "plain"
	guarded := []string{value}
	if strings.Contains(value, ":") {
		guarded = strings.SplitN(value, ":", 0)
	}
	direct := strings.SplitN(value, ":", 0)
	if len(guarded) != 1 || direct != nil {
		t.Fatalf("zero-count counterexample changed: guarded=%#v direct=%#v", guarded, direct)
	}
	guarded = []string{value}
	if strings.Contains(value, ":") {
		guarded = strings.SplitN(value, ":", 3)
	}
	direct = strings.SplitN(value, ":", 3)
	if !reflect.DeepEqual(guarded, direct) || cap(guarded) != 1 || cap(direct) != 3 {
		t.Fatalf("positive-count capacity counterexample changed: guarded len/cap=%d/%d direct=%d/%d", len(guarded), cap(guarded), len(direct), cap(direct))
	}

	calls := 0
	separator := func() string {
		calls++
		if calls == 1 {
			return ":"
		}
		return ";"
	}
	value = "a:b;c"
	var before []string
	if strings.Contains(value, separator()) {
		before = strings.Split(value, separator())
	} else {
		before = []string{value}
	}
	if !reflect.DeepEqual(before, []string{"a:b", "c"}) || calls != 2 {
		t.Fatalf("effectful guarded separator witness changed: result=%#v calls=%d", before, calls)
	}
	calls = 0
	after := strings.Split(value, separator())
	if !reflect.DeepEqual(after, []string{"a", "b;c"}) || calls != 1 {
		t.Fatalf("effectful direct separator witness changed: result=%#v calls=%d", after, calls)
	}
}

func ps5128BeforeString(value, separator string, split func(string, string) []string) []string {
	if strings.Contains(value, separator) {
		return split(value, separator)
	}
	return []string{value}
}

func ps5128BeforeStringN(value, separator string, count int, split func(string, string, int) []string) []string {
	if strings.Contains(value, separator) {
		return split(value, separator, count)
	}
	return []string{value}
}

func ps5128BeforeBytes(value, separator []byte, split func([]byte, []byte) [][]byte) [][]byte {
	if bytes.Contains(value, separator) {
		return split(value, separator)
	}
	return [][]byte{value}
}

func ps5128BeforeBytesN(value, separator []byte, count int, split func([]byte, []byte, int) [][]byte) [][]byte {
	if bytes.Contains(value, separator) {
		return split(value, separator, count)
	}
	return [][]byte{value}
}

func ps5128AssertByteResult(t *testing.T, name string, input, separator []byte, count int, before, after [][]byte) {
	t.Helper()
	if !reflect.DeepEqual(before, after) || (before == nil) != (after == nil) || len(before) != len(after) || cap(before) != cap(after) {
		t.Fatalf("%s divergence: input=%v separator=%v count=%d before=%#v len/cap=%d/%d after=%#v len/cap=%d/%d",
			name, input, separator, count, before, len(before), cap(before), after, len(after), cap(after))
	}
	for index := range before {
		beforeOffset, beforeAliases := ps5128AliasOffset(input, before[index])
		afterOffset, afterAliases := ps5128AliasOffset(input, after[index])
		if len(before[index]) != len(after[index]) || cap(before[index]) != cap(after[index]) ||
			beforeAliases != afterAliases || beforeOffset != afterOffset {
			t.Fatalf("%s piece %d shape divergence: before len/cap/alias/offset=%d/%d/%v/%d after=%d/%d/%v/%d",
				name, index, len(before[index]), cap(before[index]), beforeAliases, beforeOffset,
				len(after[index]), cap(after[index]), afterAliases, afterOffset)
		}
	}
}

func ps5128AliasOffset(input, piece []byte) (uintptr, bool) {
	inputPointer := uintptr(unsafe.Pointer(unsafe.SliceData(input)))
	piecePointer := uintptr(unsafe.Pointer(unsafe.SliceData(piece)))
	if inputPointer == 0 || piecePointer < inputPointer || piecePointer > inputPointer+uintptr(cap(input)) {
		return 0, inputPointer == 0 && piecePointer == 0
	}
	return piecePointer - inputPointer, true
}

func ps5128RandomBytes(random *rand.Rand, length int) []byte {
	value := make([]byte, length)
	_, _ = random.Read(value)
	return value
}
