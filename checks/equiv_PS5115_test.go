package checks

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEquiv_PS5115ValidTerminalReplacementRandom(t *testing.T) {
	random := rand.New(rand.NewSource(5115))
	validReplacements := []string{"", "?", "�", "日本語", "\x00", string(utf8.RuneError)}
	outerReplacements := append([]string{"\xff", "\xfe\xff", "outer"}, validReplacements...)
	for iteration := range 50_000 {
		data := make([]byte, random.Intn(192))
		for index := range data {
			data[index] = byte(random.Intn(256))
		}
		input := string(data)
		innerReplacement := validReplacements[random.Intn(len(validReplacements))]
		outerReplacement := outerReplacements[random.Intn(len(outerReplacements))]
		inner := strings.ToValidUTF8(input, innerReplacement)
		if !utf8.ValidString(inner) {
			t.Fatalf("iteration %d: valid replacement %q produced invalid UTF-8 %q", iteration, innerReplacement, inner)
		}
		before := strings.ToValidUTF8(inner, outerReplacement)
		if before != inner {
			t.Fatalf("iteration %d: outer replacement %q changed proven-valid result: before=%q inner=%q", iteration, outerReplacement, before, inner)
		}
		deep := strings.ToValidUTF8(strings.ToValidUTF8(inner, "\xff"), "\xfe")
		if deep != inner {
			t.Fatalf("iteration %d: deep outer chain changed proven-valid result: deep=%q inner=%q", iteration, deep, inner)
		}
	}
}

func TestEquiv_PS5115InvalidTerminalCounterexample(t *testing.T) {
	input := "\xff"
	inner := strings.ToValidUTF8(input, "\xfe")
	before := strings.ToValidUTF8(inner, "?")
	if inner != "\xfe" || before != "?" || inner == before {
		t.Fatalf("invalid replacement guard counterexample changed: inner=%q outer=%q", inner, before)
	}
}
