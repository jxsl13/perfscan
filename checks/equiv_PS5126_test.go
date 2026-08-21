package checks

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

func TestEquivPS5126ContainsLastIndexFamilies(t *testing.T) {
	random := rand.New(rand.NewSource(5126))
	for iteration := 0; iteration < 100_000; iteration++ {
		value := ps5126RandomBytes(random, random.Intn(192))
		needle := ps5126RandomBytes(random, random.Intn(19))
		text, substring := string(value), string(needle)
		if before, after := ps5126BeforeString(text, substring), strings.LastIndex(text, substring); before != after {
			t.Fatalf("strings divergence: value=%q needle=%q before=%d after=%d", text, substring, before, after)
		}
		if before, after := ps5126BeforeBytes(value, needle), bytes.LastIndex(value, needle); before != after {
			t.Fatalf("bytes divergence: value=%v needle=%v before=%d after=%d", value, needle, before, after)
		}
	}
}

func TestEquivPS5126ContainsAnyLastIndexAnyFamilies(t *testing.T) {
	random := rand.New(rand.NewSource(5126001))
	for iteration := 0; iteration < 100_000; iteration++ {
		value := ps5126RandomBytes(random, random.Intn(192))
		chars := ps5126RandomBytes(random, random.Intn(25))
		text, cutset := string(value), string(chars)
		if before, after := ps5126BeforeStringAny(text, cutset), strings.LastIndexAny(text, cutset); before != after {
			t.Fatalf("strings any divergence: value=%q chars=%q before=%d after=%d", text, cutset, before, after)
		}
		if before, after := ps5126BeforeBytesAny(value, cutset), bytes.LastIndexAny(value, cutset); before != after {
			t.Fatalf("bytes any divergence: value=%v chars=%q before=%d after=%d", value, cutset, before, after)
		}
	}
}

func TestEquivPS5126ExcludedBoundaries(t *testing.T) {
	calls := 0
	changingNeedle := func() string {
		calls++
		if calls == 1 {
			return ":"
		}
		return ";"
	}
	value := "a:b;b"
	before := -1
	if strings.Contains(value, changingNeedle()) {
		before = strings.LastIndex(value, changingNeedle())
	}
	if before != 3 || calls != 2 {
		t.Fatalf("effectful guarded witness changed: result=%d calls=%d", before, calls)
	}
	calls = 0
	after := strings.LastIndex(value, changingNeedle())
	if after != 1 || calls != 1 {
		t.Fatalf("effectful direct witness changed: result=%d calls=%d", after, calls)
	}

	data := []byte("a;b:b")
	guardNeedle, indexNeedle := []byte("|"), []byte(";")
	guarded := -1
	if bytes.Contains(data, guardNeedle) {
		guarded = bytes.LastIndex(data, indexNeedle)
	}
	direct := bytes.LastIndex(data, indexNeedle)
	if guarded != -1 || direct != 1 {
		t.Fatalf("different byte-needle witness changed: guarded=%d direct=%d", guarded, direct)
	}

	missing := "plain"
	wrongFallback := -2
	if strings.Contains(missing, ":") {
		wrongFallback = strings.LastIndex(missing, ":")
	}
	if directIndex := strings.LastIndex(missing, ":"); wrongFallback == directIndex {
		t.Fatalf("wrong fallback counterexample changed: guarded=%d direct=%d", wrongFallback, directIndex)
	}
}

func ps5126BeforeString(value, needle string) int {
	if strings.Contains(value, needle) {
		return strings.LastIndex(value, needle)
	}
	return -1
}

func ps5126BeforeBytes(value, needle []byte) int {
	if bytes.Contains(value, needle) {
		return bytes.LastIndex(value, needle)
	}
	return -1
}

func ps5126BeforeStringAny(value, chars string) int {
	if strings.ContainsAny(value, chars) {
		return strings.LastIndexAny(value, chars)
	}
	return -1
}

func ps5126BeforeBytesAny(value []byte, chars string) int {
	if bytes.ContainsAny(value, chars) {
		return bytes.LastIndexAny(value, chars)
	}
	return -1
}

func ps5126RandomBytes(random *rand.Rand, length int) []byte {
	value := make([]byte, length)
	_, _ = random.Read(value)
	return value
}
