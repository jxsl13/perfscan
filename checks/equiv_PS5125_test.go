package checks

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"unsafe"
)

func TestEquivPS5125ContainsGuardedReplace(t *testing.T) {
	random := rand.New(rand.NewSource(5125))
	for iteration := 0; iteration < 100_000; iteration++ {
		value := string(ps5125RandomBytes(random, random.Intn(192)))
		old := string(ps5125RandomBytes(random, random.Intn(18)))
		replacement := string(ps5125RandomBytes(random, random.Intn(18)))
		limit := random.Intn(14) - 4
		before := ps5125Before(value, old, replacement, limit)
		after := strings.Replace(value, old, replacement, limit)
		if before != after {
			t.Fatalf("result divergence: value=%q old=%q replacement=%q limit=%d before=%q after=%q", value, old, replacement, limit, before, after)
		}
		beforeAliasesInput := unsafe.StringData(before) == unsafe.StringData(value)
		afterAliasesInput := unsafe.StringData(after) == unsafe.StringData(value)
		if beforeAliasesInput != afterAliasesInput {
			t.Fatalf("input-alias divergence: value=%q old=%q replacement=%q limit=%d before=%v after=%v", value, old, replacement, limit, beforeAliasesInput, afterAliasesInput)
		}
	}
}

func TestEquivPS5125ExcludedBoundaries(t *testing.T) {
	replacementCalls := 0
	replacement := func() string {
		replacementCalls++
		return "-"
	}
	missing := "plain"
	before := missing
	if strings.Contains(missing, ":") {
		before = strings.Replace(missing, ":", replacement(), 1)
	}
	if before != missing || replacementCalls != 0 {
		t.Fatalf("guarded replacement witness changed: result=%q calls=%d", before, replacementCalls)
	}
	after := strings.Replace(missing, ":", replacement(), 1)
	if after != missing || replacementCalls != 1 {
		t.Fatalf("direct replacement witness changed: result=%q calls=%d", after, replacementCalls)
	}

	limitCalls := 0
	limit := func() int {
		limitCalls++
		return 1
	}
	before = missing
	if strings.Contains(missing, ":") {
		before = strings.Replace(missing, ":", "-", limit())
	}
	after = strings.Replace(missing, ":", "-", limit())
	if before != after || limitCalls != 1 {
		t.Fatalf("effectful limit counterexample changed: before=%q after=%q calls=%d", before, after, limitCalls)
	}

	data := []byte("plain")
	guardedBytes := data
	if bytes.Contains(data, []byte(":")) {
		guardedBytes = bytes.Replace(data, []byte(":"), []byte("-"), 1)
	}
	directBytes := bytes.Replace(data, []byte(":"), []byte("-"), 1)
	if !bytes.Equal(guardedBytes, directBytes) || unsafe.SliceData(guardedBytes) == unsafe.SliceData(directBytes) {
		t.Fatalf("bytes allocation counterexample changed: guarded=%v direct=%v", guardedBytes, directBytes)
	}

	differentOld := "a;b"
	guarded := differentOld
	if strings.Contains(differentOld, ":") {
		guarded = strings.Replace(differentOld, ";", "-", 1)
	}
	directDifferent := strings.Replace(differentOld, ";", "-", 1)
	if guarded == directDifferent {
		t.Fatalf("different-old counterexample changed: guarded=%q direct=%q", guarded, directDifferent)
	}
}

func ps5125Before(value, old, replacement string, limit int) string {
	if strings.Contains(value, old) {
		return strings.Replace(value, old, replacement, limit)
	}
	return value
}

func ps5125RandomBytes(random *rand.Rand, length int) []byte {
	value := make([]byte, length)
	_, _ = random.Read(value)
	return value
}
