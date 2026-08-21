package checks

import (
	"math/rand"
	"strings"
	"testing"
	"unsafe"
)

func TestEquivPS5120SplitHeads(t *testing.T) {
	random := rand.New(rand.NewSource(5120))
	counts := []int{-7, -1, 2, 3, 17}
	for iteration := 0; iteration < 75_000; iteration++ {
		value := ps5120RandomBytes(random, random.Intn(128))
		separator := ps5120RandomBytes(random, 1+random.Intn(12))
		cutHead, _, _ := strings.Cut(value, separator)
		for _, count := range counts {
			splitHead := strings.SplitN(value, separator, count)[0]
			if splitHead != cutHead {
				t.Fatalf("head divergence: value=%q separator=%q count=%d split=%q cut=%q", value, separator, count, splitHead, cutHead)
			}
			if unsafe.StringData(splitHead) != unsafe.StringData(cutHead) {
				t.Fatalf("head storage divergence: value=%q separator=%q count=%d", value, separator, count)
			}
		}
		allHead := strings.Split(value, separator)[0]
		if allHead != cutHead || unsafe.StringData(allHead) != unsafe.StringData(cutHead) {
			t.Fatalf("Split head divergence: value=%q separator=%q split=%q cut=%q", value, separator, allHead, cutHead)
		}
	}
}

func TestEquivPS5120ExcludedBoundaries(t *testing.T) {
	if got, want := strings.SplitN("abc", "", 2)[0], "a"; got != want {
		t.Fatalf("empty-separator witness changed: got %q want %q", got, want)
	}
	if before, _, _ := strings.Cut("abc", ""); before != "" {
		t.Fatalf("Cut empty-separator witness changed: got %q want empty", before)
	}
	if got := strings.SplitN("a:b", ":", 1)[0]; got != "a:b" {
		t.Fatalf("count-one witness changed: got %q", got)
	}
	if before, _, _ := strings.Cut("a:b", ":"); before != "a" {
		t.Fatalf("Cut count-one counterexample changed: got %q", before)
	}
}

func ps5120RandomBytes(random *rand.Rand, length int) string {
	value := make([]byte, length)
	for index := range value {
		value[index] = byte(random.Intn(256))
	}
	return string(value)
}
