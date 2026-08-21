package checks

import (
	"math/rand"
	"path/filepath"
	"testing"
)

func ps5113ToSlash(path string, separator byte) string {
	if separator == '/' {
		return path
	}
	return ps5113ReplaceByte(path, separator, '/')
}

func ps5113FromSlash(path string, separator byte) string {
	if separator == '/' {
		return path
	}
	return ps5113ReplaceByte(path, '/', separator)
}

func ps5113ReplaceByte(value string, old, replacement byte) string {
	data := []byte(value)
	for index := range data {
		if data[index] == old {
			data[index] = replacement
		}
	}
	return string(data)
}

func TestPS5113SlashNormalizerAlgebra(t *testing.T) {
	random := rand.New(rand.NewSource(5113))
	for _, separator := range []byte{'/', '\\'} {
		for iteration := 0; iteration < 50_000; iteration++ {
			data := make([]byte, random.Intn(128))
			for index := range data {
				data[index] = byte(random.Intn(256))
			}
			input := string(data)
			to := func(value string) string { return ps5113ToSlash(value, separator) }
			from := func(value string) string { return ps5113FromSlash(value, separator) }
			checks := []struct {
				name        string
				composition string
				outer       string
			}{
				{name: "To(To)", composition: to(to(input)), outer: to(input)},
				{name: "To(From)", composition: to(from(input)), outer: to(input)},
				{name: "From(To)", composition: from(to(input)), outer: from(input)},
				{name: "From(From)", composition: from(from(input)), outer: from(input)},
				{name: "deep mixed", composition: to(from(to(from(input)))), outer: to(input)},
			}
			for _, check := range checks {
				if check.composition != check.outer {
					t.Fatalf("separator %q iteration %d %s: composition=%q outer=%q input=%q", separator, iteration, check.name, check.composition, check.outer, input)
				}
			}
		}
	}
}

func TestPS5113HostFilepathIdentities(t *testing.T) {
	inputs := []string{"", "/", `\\`, `a/b\\c`, `C:\\mixed/path`, string([]byte{0xff, '/', '\\'})}
	for _, input := range inputs {
		if got, want := filepath.ToSlash(filepath.FromSlash(input)), filepath.ToSlash(input); got != want {
			t.Errorf("ToSlash(FromSlash(%q))=%q, want %q", input, got, want)
		}
		if got, want := filepath.FromSlash(filepath.ToSlash(input)), filepath.FromSlash(input); got != want {
			t.Errorf("FromSlash(ToSlash(%q))=%q, want %q", input, got, want)
		}
	}
}

func TestPS5113MixedChainRestoresNativeProducer(t *testing.T) {
	random := rand.New(rand.NewSource(0x51135114))
	for _, separator := range []byte{'/', '\\'} {
		for iteration := range 50_000 {
			data := make([]byte, random.Intn(128))
			for index := range data {
				data[index] = byte(random.Intn(256))
			}
			// Model the postcondition of a filepath producer: the result
			// uses the platform separator and therefore contains no other '/'.
			native := ps5113FromSlash(string(data), separator)
			before := ps5113FromSlash(ps5113ToSlash(ps5113FromSlash(native, separator), separator), separator)
			if before != native {
				t.Fatalf("separator %q iteration %d: mixed chain=%q native producer=%q", separator, iteration, before, native)
			}
		}
	}
}
