package runner

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestFixViperMapPrealloc pins a shape observed during corpus -fix validation
// (spf13/viper viper.go, 8 fixes applied, tests still passed): an unsized map
// literal filled in a range over another collection is pre-sized by PS2104 to
// make(map[K]V, len(SRC)) — where SRC is the RANGED collection, not the map. The
// size is a capacity HINT only (map contents and iteration are unchanged), which
// is why viper's tests pass after the rewrite; this locks that PS2104 picks the
// ranged-collection length as the hint and the result compiles.
func TestFixViperMapPrealloc(t *testing.T) {
	const src = `package p

func flatten(src map[string]any) map[string]any {
	tgt := map[string]any{}
	for k, v := range src {
		tgt[k] = v
	}
	return tgt
}
`
	got := string(runFixMode(t, src))

	if !strings.Contains(got, "make(map[string]any, len(src))") {
		t.Errorf("expected the map pre-sized to len(src) (the ranged collection):\n%s", got)
	}
	if strings.Contains(got, "map[string]any{}") {
		t.Errorf("the unsized map literal should have been rewritten:\n%s", got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}
