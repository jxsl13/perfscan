package checks

import (
	"math/rand"
	"strings"
	"testing"
)

func TestPS5112InverseIdentitiesRandomized(t *testing.T) {
	random := rand.New(rand.NewSource(5112))
	randomString := func(limit int) string {
		data := make([]byte, random.Intn(limit+1))
		for index := range data {
			data[index] = byte(random.Intn(256))
		}
		return string(data)
	}

	for iteration := 0; iteration < 50_000; iteration++ {
		input := randomString(96)
		separator := randomString(8)
		other := randomString(8)
		checks := []struct {
			name string
			got  string
		}{
			{name: "Split", got: strings.Join(strings.Split(input, separator), separator)},
			{name: "SplitAfter", got: strings.Join(strings.SplitAfter(input, separator), "")},
			{name: "SplitN(-1)", got: strings.Join(strings.SplitN(input, separator, -1), separator)},
			{name: "SplitN(-2)", got: strings.Join(strings.SplitN(input, separator, -2), separator)},
			{name: "SplitN(1)", got: strings.Join(strings.SplitN(input, separator, 1), other)},
			{name: "SplitAfterN(-1)", got: strings.Join(strings.SplitAfterN(input, separator, -1), "")},
			{name: "SplitAfterN(-2)", got: strings.Join(strings.SplitAfterN(input, separator, -2), "")},
			{name: "SplitAfterN(1)", got: strings.Join(strings.SplitAfterN(input, separator, 1), other)},
		}
		for _, check := range checks {
			if check.got != input {
				t.Fatalf("iteration %d %s diverged for input=%q separator=%q other=%q: got %q", iteration, check.name, input, separator, other, check.got)
			}
		}
	}
}

func TestPS5112MalformedUTF8EmptySeparator(t *testing.T) {
	input := string([]byte{0xff, 'a', 0xc0, 0x80, 0xfe})
	if got := strings.Join(strings.Split(input, ""), ""); got != input {
		t.Fatalf("Split/Join changed malformed UTF-8 bytes: got %x, want %x", got, input)
	}
	if got := strings.Join(strings.SplitAfter(input, ""), ""); got != input {
		t.Fatalf("SplitAfter/Join changed malformed UTF-8 bytes: got %x, want %x", got, input)
	}
}
