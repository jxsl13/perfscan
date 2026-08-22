package checks

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

func TestEquivPS5124ContainsCountFamilies(t *testing.T) {
	random := rand.New(rand.NewSource(5124))
	values := [][]byte{nil, {}, {0}, []byte("a:b:b"), {0xff, 0, 0xc3, 0x28}}
	needles := [][]byte{nil, {}, {0}, []byte(":"), {0xff}}
	for iteration := 0; iteration < 100_000; iteration++ {
		value := ps5124RandomBytes(random, random.Intn(192))
		needle := ps5124RandomBytes(random, random.Intn(20))
		values = append(values[:5], value)
		needles = append(needles[:5], needle)
		for _, candidate := range values {
			for _, separator := range needles {
				beforeBytes := ps5124BeforeBytes(candidate, separator)
				afterBytes := bytes.Count(candidate, separator)
				if beforeBytes != afterBytes {
					t.Fatalf("bytes divergence: value=%v needle=%v before=%d after=%d", candidate, separator, beforeBytes, afterBytes)
				}
				text, substring := string(candidate), string(separator)
				beforeString := ps5124BeforeString(text, substring)
				afterString := strings.Count(text, substring)
				if beforeString != afterString {
					t.Fatalf("strings divergence: value=%q needle=%q before=%d after=%d", text, substring, beforeString, afterString)
				}
			}
		}
	}
}

func TestEquivPS5124ExcludedBoundaries(t *testing.T) {
	calls := 0
	changingNeedle := func() string {
		calls++
		if calls == 1 {
			return ":"
		}
		return ";"
	}
	value := ":;;"
	before := 0
	if strings.Contains(value, changingNeedle()) {
		before = strings.Count(value, changingNeedle())
	}
	if before != 2 || calls != 2 {
		t.Fatalf("effectful guarded witness changed: result=%d calls=%d", before, calls)
	}
	calls = 0
	after := strings.Count(value, changingNeedle())
	if after != 1 || calls != 1 {
		t.Fatalf("effectful direct witness changed: result=%d calls=%d", after, calls)
	}

	data := []byte(";;")
	guardNeedle, countNeedle := []byte(":"), []byte(";")
	guarded := 0
	if bytes.Contains(data, guardNeedle) {
		guarded = bytes.Count(data, countNeedle)
	}
	direct := bytes.Count(data, countNeedle)
	if guarded != 0 || direct != 2 {
		t.Fatalf("different byte-needle counterexample changed: guarded=%d direct=%d", guarded, direct)
	}

	missing := "plain"
	wrongFallback := -1
	if strings.Contains(missing, ":") {
		wrongFallback = strings.Count(missing, ":")
	}
	if directCount := strings.Count(missing, ":"); wrongFallback == directCount {
		t.Fatalf("wrong fallback counterexample changed: guarded=%d direct=%d", wrongFallback, directCount)
	}
}

func ps5124BeforeString(value, needle string) int {
	if strings.Contains(value, needle) {
		return strings.Count(value, needle)
	}
	return 0
}

func ps5124BeforeBytes(value, needle []byte) int {
	if bytes.Contains(value, needle) {
		return bytes.Count(value, needle)
	}
	return 0
}

func ps5124RandomBytes(random *rand.Rand, length int) []byte {
	value := make([]byte, length)
	_, _ = random.Read(value)
	return value
}
