package checks

import (
	"bytes"
	"math/rand"
	"slices"
	"strings"
	"testing"
)

func TestEquivPS5123ContainsIndexFamilies(t *testing.T) {
	random := rand.New(rand.NewSource(5123))
	for iteration := 0; iteration < 75_000; iteration++ {
		value := ps5123RandomBytes(random, random.Intn(160))
		needle := ps5123RandomBytes(random, random.Intn(17))
		text, substring := string(value), string(needle)
		if before, after := ps5123BeforeString(text, substring), strings.Index(text, substring); before != after {
			t.Fatalf("strings divergence: value=%q needle=%q before=%d after=%d", text, substring, before, after)
		}
		if before, after := ps5123BeforeBytes(value, needle), bytes.Index(value, needle); before != after {
			t.Fatalf("bytes divergence: value=%v needle=%v before=%d after=%d", value, needle, before, after)
		}

		length := random.Intn(96)
		values := make([]int, length)
		for index := range values {
			values[index] = random.Intn(31) - 15
		}
		target := random.Intn(41) - 20
		if before, after := ps5123BeforeSlice(values, target), slices.Index(values, target); before != after {
			t.Fatalf("slices divergence: values=%v target=%d before=%d after=%d", values, target, before, after)
		}
	}
}

func TestEquivPS5123AnyAndRuneFamilies(t *testing.T) {
	random := rand.New(rand.NewSource(5123001))
	for iteration := 0; iteration < 75_000; iteration++ {
		value := ps5123RandomBytes(random, random.Intn(160))
		chars := ps5123RandomBytes(random, random.Intn(24))
		text, cutset := string(value), string(chars)
		if before, after := ps5123BeforeStringAny(text, cutset), strings.IndexAny(text, cutset); before != after {
			t.Fatalf("strings any divergence: value=%q chars=%q before=%d after=%d", text, cutset, before, after)
		}
		if before, after := ps5123BeforeBytesAny(value, cutset), bytes.IndexAny(value, cutset); before != after {
			t.Fatalf("bytes any divergence: value=%v chars=%q before=%d after=%d", value, cutset, before, after)
		}

		needle := rune(random.Uint32())
		if before, after := ps5123BeforeStringRune(text, needle), strings.IndexRune(text, needle); before != after {
			t.Fatalf("strings rune divergence: value=%q needle=%U before=%d after=%d", text, needle, before, after)
		}
		if before, after := ps5123BeforeBytesRune(value, needle), bytes.IndexRune(value, needle); before != after {
			t.Fatalf("bytes rune divergence: value=%v needle=%U before=%d after=%d", value, needle, before, after)
		}
	}
}

func TestEquivPS5123ExcludedBoundaries(t *testing.T) {
	calls := 0
	changingNeedle := func() string {
		calls++
		if calls == 1 {
			return ":"
		}
		return ";"
	}
	value := "a:b"
	before := -1
	if strings.Contains(value, changingNeedle()) {
		before = strings.Index(value, changingNeedle())
	}
	if before != -1 || calls != 2 {
		t.Fatalf("effectful guarded witness changed: result=%d calls=%d", before, calls)
	}
	calls = 0
	after := strings.Index(value, changingNeedle())
	if after != 1 || calls != 1 {
		t.Fatalf("effectful direct witness changed: result=%d calls=%d", after, calls)
	}

	data := []byte("a;b")
	guardNeedle, indexNeedle := []byte(":"), []byte(";")
	guarded := -1
	if bytes.Contains(data, guardNeedle) {
		guarded = bytes.Index(data, indexNeedle)
	}
	direct := bytes.Index(data, indexNeedle)
	if guarded != -1 || direct != 1 {
		t.Fatalf("different byte-needle counterexample changed: guarded=%d direct=%d", guarded, direct)
	}

	missing := "plain"
	wrongFallback := -2
	if strings.Contains(missing, ":") {
		wrongFallback = strings.Index(missing, ":")
	}
	if directIndex := strings.Index(missing, ":"); wrongFallback == directIndex {
		t.Fatalf("wrong fallback counterexample changed: guarded=%d direct=%d", wrongFallback, directIndex)
	}
}

func ps5123BeforeString(value, needle string) int {
	if strings.Contains(value, needle) {
		return strings.Index(value, needle)
	}
	return -1
}

func ps5123BeforeBytes(value, needle []byte) int {
	if bytes.Contains(value, needle) {
		return bytes.Index(value, needle)
	}
	return -1
}

func ps5123BeforeSlice(value []int, target int) int {
	if slices.Contains(value, target) {
		return slices.Index(value, target)
	}
	return -1
}

func ps5123BeforeStringAny(value, chars string) int {
	if strings.ContainsAny(value, chars) {
		return strings.IndexAny(value, chars)
	}
	return -1
}

func ps5123BeforeBytesAny(value []byte, chars string) int {
	if bytes.ContainsAny(value, chars) {
		return bytes.IndexAny(value, chars)
	}
	return -1
}

func ps5123BeforeStringRune(value string, needle rune) int {
	if strings.ContainsRune(value, needle) {
		return strings.IndexRune(value, needle)
	}
	return -1
}

func ps5123BeforeBytesRune(value []byte, needle rune) int {
	if bytes.ContainsRune(value, needle) {
		return bytes.IndexRune(value, needle)
	}
	return -1
}

func ps5123RandomBytes(random *rand.Rand, length int) []byte {
	value := make([]byte, length)
	_, _ = random.Read(value)
	return value
}
