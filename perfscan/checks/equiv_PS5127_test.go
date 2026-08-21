package checks

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEquivPS5127ValidationGuardedSanitizer(t *testing.T) {
	random := rand.New(rand.NewSource(5127))
	for iteration := 0; iteration < 100_000; iteration++ {
		value := string(ps5127RandomBytes(random, random.Intn(224)))
		replacement := string(ps5127RandomBytes(random, random.Intn(17)))
		before := ps5127Before(value, replacement)
		after := strings.ToValidUTF8(value, replacement)
		if before != after {
			t.Fatalf("divergence: value=%q replacement=%q before=%q after=%q", value, replacement, before, after)
		}
	}
}

func TestEquivPS5127ValidAndInvalidBoundaries(t *testing.T) {
	values := []string{
		"", "plain ASCII", "λ世界", "\x00\x7f", "\xff", "a\xffb", "\xc0\x80", "\xed\xa0\x80", "\xf4\x90\x80\x80",
	}
	replacements := []string{"", "?", "�", "\xff", "\xc0\x80"}
	for _, value := range values {
		for _, replacement := range replacements {
			if before, after := ps5127Before(value, replacement), strings.ToValidUTF8(value, replacement); before != after {
				t.Fatalf("boundary divergence: value=%q replacement=%q before=%q after=%q", value, replacement, before, after)
			}
		}
	}
}

func TestEquivPS5127ExcludedEvaluationsAndBytesIdentity(t *testing.T) {
	calls := 0
	replacement := func() string {
		calls++
		return "?"
	}
	value := "already valid"
	before := value
	if !utf8.ValidString(value) {
		before = strings.ToValidUTF8(value, replacement())
	}
	if before != value || calls != 0 {
		t.Fatalf("guarded valid path changed: result=%q calls=%d", before, calls)
	}
	after := strings.ToValidUTF8(value, replacement())
	if after != value || calls != 1 {
		t.Fatalf("unguarded effectful replacement witness changed: result=%q calls=%d", after, calls)
	}

	data := []byte("already valid")
	guarded := data
	if !utf8.Valid(data) {
		guarded = bytes.ToValidUTF8(data, []byte("?"))
	}
	direct := bytes.ToValidUTF8(data, []byte("?"))
	if &guarded[0] != &data[0] || &direct[0] == &data[0] {
		t.Fatalf("bytes alias witness changed: guarded aliases input=%v direct aliases input=%v", &guarded[0] == &data[0], &direct[0] == &data[0])
	}
	direct[0] = 'A'
	if data[0] != 'a' {
		t.Fatal("bytes.ToValidUTF8 valid-input result unexpectedly aliases its input")
	}
}

func ps5127Before(value, replacement string) string {
	if !utf8.ValidString(value) {
		return strings.ToValidUTF8(value, replacement)
	}
	return value
}

func ps5127RandomBytes(random *rand.Rand, length int) []byte {
	value := make([]byte, length)
	_, _ = random.Read(value)
	return value
}
