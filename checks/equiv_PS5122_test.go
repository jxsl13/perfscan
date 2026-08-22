package checks

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"unsafe"
)

func TestEquivPS5122GuardedReplaceAll(t *testing.T) {
	random := rand.New(rand.NewSource(5122))
	for iteration := 0; iteration < 75_000; iteration++ {
		value := ps5122RandomString(random, random.Intn(128))
		old := ps5122RandomString(random, random.Intn(13))
		replacement := ps5122RandomString(random, random.Intn(13))

		before := ps5122BeforeString(value, old, replacement)
		after := strings.ReplaceAll(value, old, replacement)
		if before != after {
			t.Fatalf("value divergence: value=%q old=%q replacement=%q before=%q after=%q", value, old, replacement, before, after)
		}
		beforeRetained := unsafe.StringData(before) == unsafe.StringData(value)
		afterRetained := unsafe.StringData(after) == unsafe.StringData(value)
		if beforeRetained != afterRetained {
			t.Fatalf("original-storage class divergence: value=%q old=%q replacement=%q before=%t after=%t", value, old, replacement, beforeRetained, afterRetained)
		}
	}
}

func TestEquivPS5122ExcludedBoundaries(t *testing.T) {
	calls := 0
	replacement := func() string {
		calls++
		return "-"
	}
	value := "plain"
	before := value
	if strings.Contains(value, ":") {
		before = strings.ReplaceAll(value, ":", replacement())
	}
	if calls != 0 || before != value {
		t.Fatalf("guarded effect witness changed: calls=%d value=%q", calls, before)
	}
	after := strings.ReplaceAll(value, ":", replacement())
	if calls != 1 || after != value {
		t.Fatalf("unguarded effect witness changed: calls=%d value=%q", calls, after)
	}

	data := make([]byte, 3, 32)
	copy(data, "abc")
	guarded := data
	if bytes.Contains(data, []byte(":")) {
		guarded = bytes.ReplaceAll(data, []byte(":"), []byte("-"))
	}
	unguarded := bytes.ReplaceAll(data, []byte(":"), []byte("-"))
	if unsafe.SliceData(guarded) != unsafe.SliceData(data) || unsafe.SliceData(unguarded) == unsafe.SliceData(data) {
		t.Fatalf("bytes no-match storage witness changed: source=%p guarded=%p unguarded=%p", unsafe.SliceData(data), unsafe.SliceData(guarded), unsafe.SliceData(unguarded))
	}
	unguarded[0] = 'z'
	if data[0] != 'a' {
		t.Fatalf("bytes.ReplaceAll no-match result unexpectedly aliases its input: data=%q result=%q", data, unguarded)
	}

	input, output := "plain", "existing"
	guardedOutput := output
	if strings.Contains(input, ":") {
		guardedOutput = strings.ReplaceAll(input, ":", "-")
	}
	unguardedOutput := strings.ReplaceAll(input, ":", "-")
	if guardedOutput == unguardedOutput {
		t.Fatalf("different-target no-match counterexample changed: guarded=%q unguarded=%q", guardedOutput, unguardedOutput)
	}
}

func ps5122BeforeString(value, old, replacement string) string {
	result := value
	if strings.Contains(result, old) {
		result = strings.ReplaceAll(result, old, replacement)
	}
	return result
}

func ps5122RandomString(random *rand.Rand, length int) string {
	value := make([]byte, length)
	_, _ = random.Read(value)
	return string(value)
}
