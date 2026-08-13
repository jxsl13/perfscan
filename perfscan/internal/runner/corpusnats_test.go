package runner

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestFixNatsByteSprintfToAppendf pins a shape from corpus -fix validation
// (nats-io/nats.go, 21 bit-identical fixes; the main package's unit tests still
// passed): PS2109 rewrites []byte(fmt.Sprintf(f, args...)) -> fmt.Appendf(nil, f,
// args...), which formats straight into a fresh byte slice instead of building a
// throwaway string and copying it. Bit-identical bytes; one fewer allocation.
// fmt stays live (Appendf is fmt), so the import is retained.
func TestFixNatsByteSprintfToAppendf(t *testing.T) {
	const src = `package p

import "fmt"

func enc(name string, seq int) []byte {
	return []byte(fmt.Sprintf("%s.%d", name, seq))
}
`
	got := string(runFixMode(t, src))

	if !strings.Contains(got, `fmt.Appendf(nil, "%s.%d", name, seq)`) {
		t.Errorf("expected the []byte(Sprintf) rewritten to fmt.Appendf(nil, ...):\n%s", got)
	}
	if strings.Contains(got, "[]byte(fmt.Sprintf") {
		t.Errorf("the []byte(Sprintf) round-trip should be gone:\n%s", got)
	}
	// fmt is still used (by Appendf), so the import must be retained.
	if !strings.Contains(got, `"fmt"`) {
		t.Errorf("fmt is still live via Appendf and must be retained:\n%s", got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}
