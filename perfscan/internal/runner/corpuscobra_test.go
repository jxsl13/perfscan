package runner

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestFixCobraWriteStringPartialFmtRetention pins a composition observed during
// corpus -fix validation (spf13/cobra command.go, 12 fixes applied, tests still
// passed): one file where PS2129 rewrites BOTH fmt.Fprint(w, s) AND
// fmt.Fprintf(w, "%s", s) to io.WriteString (the "%s" verb correctly stripped),
// while a surviving fmt.Sprintf keeps the fmt import LIVE. The runner must
// rewrite both write calls, keep the already-present io import, and — crucially
// — NOT prune fmt (its orphan detection must see the remaining Sprintf
// reference). io stays, fmt stays, the file compiles.
//
// This is the partial-retention counterpart to the fully-orphaned cases
// (TestFixPrunesCrossCheckOrphanImport): here fmt is used by exactly one
// non-rewritten call, so over-pruning would break the build. The surviving
// Sprintf uses a %d verb: the cobra original's "__%s_flag" splice is by now
// itself rewritten to a concatenation (and fmt then correctly pruned) by
// PS2034, so a %d over a plain int — which no non-loop check rewrites —
// keeps fmt live instead.
func TestFixCobraWriteStringPartialFmtRetention(t *testing.T) {
	const src = `package p

import (
	"fmt"
	"io"
)

func usage(w io.Writer, example string, n int) {
	fmt.Fprint(w, "Usage:")
	fmt.Fprintf(w, "%s", example)
	_ = fmt.Sprintf("__%d_flag", n)
}
`
	got := string(runFixMode(t, src))

	for _, want := range []string{`io.WriteString(w, "Usage:")`, "io.WriteString(w, example)"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in the fixed file:\n%s", want, got)
		}
	}
	if strings.Contains(got, "fmt.Fprint") {
		t.Errorf("both fmt.Fprint/Fprintf should have become io.WriteString:\n%s", got)
	}
	// fmt is still used by the untouched Sprintf, so it must NOT be pruned.
	if !strings.Contains(got, `"fmt"`) || !strings.Contains(got, "fmt.Sprintf") {
		t.Errorf("fmt is still live (Sprintf) and must be retained:\n%s", got)
	}
	if !strings.Contains(got, `"io"`) {
		t.Errorf("the already-present io import must remain:\n%s", got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}
