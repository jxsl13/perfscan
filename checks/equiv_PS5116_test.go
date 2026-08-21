package checks

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEquiv_PS5116SanitizedOutputAlwaysValidRandom(t *testing.T) {
	random := rand.New(rand.NewSource(5116))
	stringReplacements := []string{"", "?", "�", "日本語", "\x00"}
	byteReplacements := [][]byte{nil, {}, []byte("?"), []byte("�"), []byte("日本語"), {0}}
	for iteration := range 50_000 {
		input := make([]byte, random.Intn(192))
		for index := range input {
			input[index] = byte(random.Intn(256))
		}
		stringReplacement := stringReplacements[random.Intn(len(stringReplacements))]
		if got := utf8.ValidString(strings.ToValidUTF8(string(input), stringReplacement)); !got {
			t.Fatalf("iteration %d: strings sanitizer with valid replacement %q produced invalid UTF-8 from %q", iteration, stringReplacement, input)
		}
		byteReplacement := byteReplacements[random.Intn(len(byteReplacements))]
		if got := utf8.Valid(bytes.ToValidUTF8(input, byteReplacement)); !got {
			t.Fatalf("iteration %d: bytes sanitizer with valid replacement %q produced invalid UTF-8 from %q", iteration, byteReplacement, input)
		}
	}
}

func TestEquiv_PS5116InvalidReplacementCounterexamples(t *testing.T) {
	input := "\xff"
	if got := utf8.ValidString(strings.ToValidUTF8(input, "\xfe")); got {
		t.Fatal("invalid string replacement unexpectedly produced valid UTF-8")
	}
	if got := utf8.Valid(bytes.ToValidUTF8([]byte(input), []byte("\xfe"))); got {
		t.Fatal("invalid byte replacement unexpectedly produced valid UTF-8")
	}
}
