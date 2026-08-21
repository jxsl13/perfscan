package checks

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"unsafe"
)

func TestEquivPS5121GuardedStringPieces(t *testing.T) {
	random := rand.New(rand.NewSource(5121))
	counts := []int{-9, -1, 2, 3, 19}
	for iteration := 0; iteration < 75_000; iteration++ {
		value := ps5121RandomString(random, random.Intn(128))
		separator := ps5121RandomString(random, 1+random.Intn(12))
		for _, count := range counts {
			indices := []int{0}
			if count == 2 {
				indices = append(indices, 1)
			}
			for _, index := range indices {
				beforePiece, beforeFound := ps5121BeforeString(value, separator, count, index)
				afterPiece, afterFound := ps5121AfterString(value, separator, index)
				if beforeFound != afterFound || beforePiece != afterPiece {
					t.Fatalf("string divergence: value=%q separator=%q count=%d index=%d before=(%q,%t) after=(%q,%t)", value, separator, count, index, beforePiece, beforeFound, afterPiece, afterFound)
				}
				if beforeFound && unsafe.StringData(beforePiece) != unsafe.StringData(afterPiece) {
					t.Fatalf("string storage divergence: value=%q separator=%q count=%d index=%d", value, separator, count, index)
				}
			}
		}
	}
}

func TestEquivPS5121GuardedBytePieces(t *testing.T) {
	random := rand.New(rand.NewSource(5121001))
	counts := []int{-9, -1, 2, 3, 19}
	for iteration := 0; iteration < 75_000; iteration++ {
		length := random.Intn(128)
		value := make([]byte, length, length+random.Intn(24))
		_, _ = random.Read(value)
		separator := make([]byte, 1+random.Intn(12))
		_, _ = random.Read(separator)
		for _, count := range counts {
			indices := []int{0}
			if count == 2 {
				indices = append(indices, 1)
			}
			for _, index := range indices {
				beforePiece, beforeFound := ps5121BeforeBytes(value, separator, count, index)
				afterPiece, afterFound := ps5121AfterBytes(value, separator, index)
				if beforeFound != afterFound || !bytes.Equal(beforePiece, afterPiece) ||
					(beforePiece == nil) != (afterPiece == nil) || len(beforePiece) != len(afterPiece) || cap(beforePiece) != cap(afterPiece) {
					t.Fatalf("byte divergence: value=%v separator=%v count=%d index=%d before=(%v,%d/%d,%t) after=(%v,%d/%d,%t)", value, separator, count, index, beforePiece, len(beforePiece), cap(beforePiece), beforeFound, afterPiece, len(afterPiece), cap(afterPiece), afterFound)
				}
				if beforeFound && unsafe.SliceData(beforePiece) != unsafe.SliceData(afterPiece) {
					t.Fatalf("byte storage divergence: value=%v separator=%v count=%d index=%d", value, separator, count, index)
				}
			}
		}
	}
}

func TestEquivPS5121ExcludedBoundaries(t *testing.T) {
	if !strings.Contains("abc", "") || strings.SplitN("abc", "", 2)[0] == "" {
		t.Fatal("string empty-separator counterexample changed")
	}
	if before, _, found := strings.Cut("abc", ""); !found || before != "" {
		t.Fatalf("string Cut empty-separator witness changed: before=%q found=%t", before, found)
	}
	if got := strings.SplitN("a:b", ":", 1)[0]; got != "a:b" {
		t.Fatalf("string count-one witness changed: got %q", got)
	}
	if before, _, _ := strings.Cut("a:b", ":"); before != "a" {
		t.Fatalf("string Cut count-one counterexample changed: got %q", before)
	}
	if got := strings.SplitN("a:b:c", ":", 3)[1]; got != "b" {
		t.Fatalf("string tail count-three witness changed: got %q", got)
	}
	if _, after, _ := strings.Cut("a:b:c", ":"); after != "b:c" {
		t.Fatalf("string Cut tail count-three counterexample changed: got %q", after)
	}
	if !bytes.Contains([]byte("abc"), nil) || bytes.Equal(bytes.SplitN([]byte("abc"), nil, 2)[0], nil) {
		t.Fatal("byte empty-separator counterexample changed")
	}
	if before, _, found := bytes.Cut([]byte("abc"), nil); !found || before == nil || len(before) != 0 {
		t.Fatalf("byte Cut empty-separator witness changed: before=%v found=%t", before, found)
	}
	if got := bytes.SplitN([]byte("a:b:c"), []byte(":"), -1)[1]; !bytes.Equal(got, []byte("b")) {
		t.Fatalf("byte tail negative-count witness changed: got %q", got)
	}
	if _, after, _ := bytes.Cut([]byte("a:b:c"), []byte(":")); !bytes.Equal(after, []byte("b:c")) {
		t.Fatalf("byte Cut tail negative-count counterexample changed: got %q", after)
	}
}

func ps5121BeforeString(value, separator string, count, index int) (string, bool) {
	if strings.Contains(value, separator) {
		return strings.SplitN(value, separator, count)[index], true
	}
	return "", false
}

func ps5121AfterString(value, separator string, index int) (string, bool) {
	before, after, found := strings.Cut(value, separator)
	if !found {
		return "", false
	}
	if index == 0 {
		return before, true
	}
	return after, true
}

func ps5121BeforeBytes(value, separator []byte, count, index int) ([]byte, bool) {
	if bytes.Contains(value, separator) {
		return bytes.SplitN(value, separator, count)[index], true
	}
	return nil, false
}

func ps5121AfterBytes(value, separator []byte, index int) ([]byte, bool) {
	before, after, found := bytes.Cut(value, separator)
	if !found {
		return nil, false
	}
	if index == 0 {
		return before[:len(before):len(before)], true
	}
	return after, true
}

func ps5121RandomString(random *rand.Rand, length int) string {
	value := make([]byte, length)
	_, _ = random.Read(value)
	return string(value)
}
