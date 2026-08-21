package checks

import (
	"math/rand"
	"strings"
	"testing"
	"unsafe"
)

func TestEquiv_PS5118SingleByteEliminationRandom(t *testing.T) {
	random := rand.New(rand.NewSource(5118))
	for iteration := range 50_000 {
		data := make([]byte, random.Intn(256))
		for index := range data {
			data[index] = byte(random.Intn(256))
		}
		oldByte := byte(random.Intn(256))
		old := string([]byte{oldByte})
		replacementBytes := make([]byte, random.Intn(24))
		for index := range replacementBytes {
			candidate := byte(random.Intn(255))
			if candidate >= oldByte {
				candidate++
			}
			replacementBytes[index] = candidate
		}
		replacement := string(replacementBytes)
		terminal := strings.ReplaceAll(string(data), old, replacement)
		if strings.Contains(terminal, old) {
			t.Fatalf("iteration %d: terminal replacement %q -> %q retained byte %q", iteration, data, terminal, old)
		}

		outerReplacement := string([]byte{byte(random.Intn(256)), oldByte, byte(random.Intn(256))})
		before := strings.Replace(strings.ReplaceAll(terminal, old, outerReplacement), old, "unused", random.Intn(11)-5)
		if before != terminal {
			t.Fatalf("iteration %d differs: terminal=%q before=%q old=%q", iteration, terminal, before, old)
		}
		if terminal != "" && unsafe.StringData(before) != unsafe.StringData(terminal) {
			t.Fatalf("iteration %d: no-match outer replacements did not return the terminal string storage", iteration)
		}

		negativeCountTerminal := strings.Replace(string(data), old, replacement, -2)
		if negativeCountTerminal != terminal {
			t.Fatalf("iteration %d: negative-count Replace differs from ReplaceAll: %q/%q", iteration, negativeCountTerminal, terminal)
		}
	}
}

func TestEquiv_PS5118RejectedCounterexamples(t *testing.T) {
	multiOnce := strings.ReplaceAll("aabb", "ab", "")
	multiTwice := strings.ReplaceAll(multiOnce, "ab", "")
	if multiOnce != "ab" || multiTwice != "" || multiOnce == multiTwice {
		t.Fatalf("multi-byte boundary counterexample changed: once=%q twice=%q", multiOnce, multiTwice)
	}

	reintroducedOnce := strings.ReplaceAll("x", "x", "xx")
	reintroducedTwice := strings.ReplaceAll(reintroducedOnce, "x", "xx")
	if reintroducedOnce == reintroducedTwice {
		t.Fatalf("replacement-containing-old counterexample changed: once=%q twice=%q", reintroducedOnce, reintroducedTwice)
	}

	limited := strings.Replace("xxx", "x", "", 1)
	completed := strings.ReplaceAll(limited, "x", "")
	if limited == completed {
		t.Fatalf("limited terminal unexpectedly eliminates every match: limited=%q completed=%q", limited, completed)
	}
}

func TestEquiv_PS5118DynamicOuterArgumentsRemainObservable(t *testing.T) {
	terminal := strings.ReplaceAll("x payload", "x", "")
	calls := 0
	replacement := func() string {
		calls++
		return "unused"
	}
	before := strings.ReplaceAll(terminal, "x", replacement())
	if before != terminal || calls != 1 {
		t.Fatalf("dynamic outer evaluation changed: before=%q terminal=%q calls=%d", before, terminal, calls)
	}
}
