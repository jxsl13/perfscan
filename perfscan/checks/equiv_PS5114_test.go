package checks

import (
	"math/rand"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

func TestEquiv_PS5114NativeFilepathProducersRandom(t *testing.T) {
	random := rand.New(rand.NewSource(5114))
	alphabet := []byte("./abc\\:\x00\xff")
	word := func() string {
		data := make([]byte, random.Intn(96))
		for index := range data {
			data[index] = alphabet[random.Intn(len(alphabet))]
		}
		return string(data)
	}
	for iteration := range 50_000 {
		name, tail := word(), word()
		checks := []struct {
			label  string
			before string
			after  string
		}{
			{"Clean", filepath.FromSlash(filepath.Clean(name)), filepath.Clean(name)},
			{"Join", filepath.FromSlash(filepath.Join(name, tail)), filepath.Join(name, tail)},
			{"empty Join", filepath.FromSlash(filepath.Join()), filepath.Join()},
			{"Dir", filepath.FromSlash(filepath.Dir(name)), filepath.Dir(name)},
			{"Base", filepath.FromSlash(filepath.Base(name)), filepath.Base(name)},
			{"Ext", filepath.FromSlash(filepath.Ext(name)), filepath.Ext(name)},
			{"VolumeName", filepath.FromSlash(filepath.VolumeName(name)), filepath.VolumeName(name)},
			{"deep Clean", filepath.FromSlash(filepath.FromSlash(filepath.FromSlash(filepath.Clean(name)))), filepath.Clean(name)},
		}
		for _, check := range checks {
			if check.before != check.after {
				t.Fatalf("iteration %d %s differs for name=%q tail=%q: before=%q after=%q", iteration, check.label, name, tail, check.before, check.after)
			}
		}
	}
}

// Windows filepath producers accepted by PS5114 have one shared property:
// their output contains no '/'. Model that postcondition independently of the
// host GOOS and exercise arbitrary bytes, native separators, volume-like
// prefixes, roots, empty strings, and path-element outputs against Go's exact
// FromSlash byte mapping.
func TestEquiv_PS5114WindowsNativeOutputInvariant(t *testing.T) {
	random := rand.New(rand.NewSource(0x5114))
	inputs := []string{"", `.`, `\\`, `C:`, `C:\\`, `\\host\\share`, `.ext`, string([]byte{0xff, '\\', 0})}
	for range 50_000 {
		data := make([]byte, random.Intn(128))
		for index := range data {
			data[index] = byte(random.Intn(256))
		}
		// A Windows Clean/Join/Dir result uses native separators. Base and
		// Ext results are separator-free except that Base may return "\\".
		nativePath := strings.ReplaceAll(path.Clean(strings.ReplaceAll(string(data), `\\`, `/`)), `/`, `\\`)
		inputs = append(inputs, nativePath, strings.ReplaceAll(path.Base(string(data)), `/`, `\\`), path.Ext(string(data)))
	}
	for index, output := range inputs {
		if strings.Contains(output, "/") {
			t.Fatalf("modeled producer output %d still contains slash: %q", index, output)
		}
		if got := ps5113FromSlash(output, '\\'); got != output {
			t.Fatalf("Windows FromSlash changed native producer output %d: got %q, want %q", index, got, output)
		}
	}
}
