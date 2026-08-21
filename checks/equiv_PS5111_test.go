package checks

import (
	"math/rand"
	"path"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEquiv_PS5111CanonicalProducersRandom(t *testing.T) {
	t.Parallel()
	random := rand.New(rand.NewSource(5111))
	alphabet := []byte("./abc\\:\x00")
	word := func() string {
		data := make([]byte, random.Intn(64))
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
			{"path.Dir", path.Clean(path.Dir(name)), path.Dir(name)},
			{"path.Base", path.Clean(path.Base(name)), path.Base(name)},
			{"path.Join", path.Clean(path.Join(name, "fixed", tail)), path.Join(name, "fixed", tail)},
			{"deep path.Dir", path.Clean(path.Clean(path.Clean(path.Dir(name)))), path.Dir(name)},
		}
		for _, check := range checks {
			if check.before != check.after {
				t.Fatalf("iteration %d %s differs for name=%q tail=%q: before=%q after=%q", iteration, check.label, name, tail, check.before, check.after)
			}
		}
	}
}

func TestEquiv_PS5111JoinEmptyCounterexample(t *testing.T) {
	t.Parallel()
	joined := path.Join("", "")
	cleaned := path.Clean(joined)
	if joined != "" || cleaned != "." {
		t.Fatalf("empty-Join guard counterexample changed: Join=%q Clean(Join)=%q", joined, cleaned)
	}
}

func TestEquiv_PS5111WindowsFilepathCounterexamples(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("Windows filepath volume grammar")
	}
	checks := []struct {
		label  string
		before string
		after  string
	}{
		{"filepath.Dir", filepath.Clean(filepath.Dir("a:/\\b/c")), filepath.Dir("a:/\\b/c")},
		{"filepath.Base", filepath.Clean(filepath.Base("x/a:")), filepath.Base("x/a:")},
		{"filepath.Join", filepath.Clean(filepath.Join("/:")), filepath.Join("/:")},
	}
	for _, check := range checks {
		if check.before == check.after {
			t.Fatalf("%s counterexample no longer differs: %q", check.label, check.before)
		}
	}
}
