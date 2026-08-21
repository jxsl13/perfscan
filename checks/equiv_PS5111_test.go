package checks

import (
	"math/rand"
	"path"
	"path/filepath"
	"testing"
)

func TestEquiv_PS5111CanonicalProducersRandom(t *testing.T) {
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
			{"filepath.Dir", filepath.Clean(filepath.Dir(name)), filepath.Dir(name)},
			{"filepath.Base", filepath.Clean(filepath.Base(name)), filepath.Base(name)},
			{"path.Join", path.Clean(path.Join(name, "fixed", tail)), path.Join(name, "fixed", tail)},
			{"filepath.Join", filepath.Clean(filepath.Join(name, "fixed", tail)), filepath.Join(name, "fixed", tail)},
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
	joined := path.Join("", "")
	cleaned := path.Clean(joined)
	if joined != "" || cleaned != "." {
		t.Fatalf("empty-Join guard counterexample changed: Join=%q Clean(Join)=%q", joined, cleaned)
	}
}
