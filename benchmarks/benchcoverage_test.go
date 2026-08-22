package benchmarks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/jxsl13/perfscan/checks"
)

// benchExemptions are the AutoFix checks intentionally WITHOUT a Before/After
// micro-benchmark, each with its reason. This is the ONLY place the exemption
// list lives (the README describes the policy in prose; this makes it
// machine-checked). Keep it minimal: a check with a real gc-visible win must
// ship a bench pair — an exemption is only for parity-by-construction rewrites.
var benchExemptions = map[string]string{
	"PS4008": "documented exemption: gc does not auto-vectorize, so the matmul ikj/axpy reorder is ~parity in Go — the rewrite is a structural pointer toward asm/cgo/BLAS, not a gc win (see benchmarks/README.md). The other AutoFix checks — including the gc-parity ones PS2119/PS2125 — carry benchmarks that demonstrate the parity empirically.",
}

// TestEveryAutoFixHasBenchmarkOrExemption enforces the README policy
// mechanically: every AutoFix check has a Before/After micro-benchmark, or a
// documented exemption above. The benchmarked set is PARSED from this package's
// source (any func named BenchmarkPS<digits>...), so it can never drift from the
// actual benchmark functions. It also keeps the exemption list honest — a stale
// exemption (for a check that is benchmarked, or is not AutoFix) fails too.
func TestEveryAutoFixHasBenchmarkOrExemption(t *testing.T) {
	benched := benchmarkedIDs(t)

	autofix := map[string]bool{}
	for _, c := range checks.All() {
		if !c.AutoFix {
			continue
		}
		autofix[c.ID] = true
		if benched[c.ID] {
			continue
		}
		if _, ok := benchExemptions[c.ID]; ok {
			continue
		}
		t.Errorf("%s is AutoFix but has no Before/After benchmark and no exemption — add a Benchmark%s_Before/_After pair, or a benchExemptions entry with the reason", c.ID, c.ID)
	}

	for id := range benchExemptions {
		if !autofix[id] {
			t.Errorf("benchExemptions lists %s, which is not an AutoFix check — remove the stale exemption", id)
		} else if benched[id] {
			t.Errorf("benchExemptions lists %s, but it IS benchmarked — remove the stale exemption", id)
		}
	}
}

// benchmarkedIDs parses every *_test.go in this package and returns the set of
// PS ids that have a benchmark function (Benchmark PS<digits><anything>).
func benchmarkedIDs(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`^BenchmarkPS(\d+)`)
	out := map[string]bool{}
	fset := token.NewFileSet()
	for _, f := range files {
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, d := range af.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if m := re.FindStringSubmatch(fn.Name.Name); m != nil {
				out["PS"+m[1]] = true
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("parsed no benchmark functions — glob or regex is wrong")
	}
	return out
}
