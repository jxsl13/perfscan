package runner

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestFixReachesFixedPointInOnePass pins that the -fix pipeline is IDEMPOTENT:
// running the full check set over already-fixed code must change nothing. A
// second pass that still rewrites something would mean a fix re-triggers on its
// own output (the self-dispatch class the PS2111/PS2118/PS2129 corpus bug fell
// into) or that two checks oscillate — either way an infinite -fix churn that
// the per-check golden tests, which only look at a single pass, cannot catch.
//
// The composite deliberately mixes checks that PRUNE a stdlib import
// (PS2107 fmt->strconv, PS3104 sort->slices) with checks that leave imports
// intact, so a non-idempotent import edit (a re-pruned or re-added spec) would
// surface here too, exercising the real analyzers + patchedFiles + import
// cleanup + format.Source pipeline end to end.
func TestFixReachesFixedPointInOnePass(t *testing.T) {
	const src = `package p

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

func compose(n int, s string, a, b []byte, xs []int, m map[string]int, zs []int) {
	_ = fmt.Sprintf("%d", n)
	sort.Ints(xs)
	_ = bytes.Compare(a, b) == 0
	_ = string([]byte(s))
	_ = strings.Count(s, "x") > 0
	for k := range m {
		delete(m, k)
	}
	for i := range zs {
		zs[i] = 0
	}
}
`
	pass1 := string(runFixMode(t, src))
	if pass1 == src {
		t.Fatalf("expected the first -fix pass to change the file, but it was unchanged:\n%s", pass1)
	}
	pass2 := string(runFixMode(t, pass1))
	if pass2 != pass1 {
		t.Errorf("-fix is NOT idempotent: a second pass changed the already-fixed file (infinite-churn risk).\n--- pass1 ---\n%s\n--- pass2 ---\n%s", pass1, pass2)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", pass2, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, pass2)
	}
	// Sanity that the fixes actually landed (so the idempotence above is over a
	// genuinely rewritten file, not a no-op run).
	for _, want := range []string{"strconv.Itoa(n)", "slices.Sort(xs)", "bytes.Equal(a, b)", "strings.Contains(s,", "clear(m)", "clear(zs)"} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected %q in the fixed output:\n%s", want, pass1)
		}
	}
	// The two import-pruning checks must have removed their now-orphaned stdlib
	// imports and added the replacements — and pass 2 must not have re-touched
	// that block.
	if strings.Contains(pass1, `"fmt"`) || strings.Contains(pass1, `"sort"`) {
		t.Errorf("fmt/sort should have been pruned as orphans:\n%s", pass1)
	}
	for _, want := range []string{`"slices"`, `"strconv"`} {
		if !strings.Contains(pass1, want) {
			t.Errorf("expected added import %q:\n%s", want, pass1)
		}
	}
}
