package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// TestEveryAutoFixHasGoldenTest is the CORRECTNESS companion to
// benchmarks.TestEveryAutoFixHasBenchmarkOrExemption (which guards the perf
// dimension). It enforces that every AutoFix check's fix is actually VERIFIED:
// the check must be exercised by analysistest.RunWithSuggestedFixes (which
// applies the SuggestedFixes and diffs the result against a .go.golden), NOT by
// the plain analysistest.Run (which only checks the diagnostics and never
// applies — nor verifies — the fix).
//
// Without this, an AutoFix check could ship with an advisory-style
// analysistest.Run test, leaving its rewrite completely unverified — the fix
// could corrupt code and every test would still pass. That is exactly the
// mistake an autonomous check-authoring agent can make, so the guard is
// machine-checked rather than left to review. The RunWithSuggestedFixes /
// Run call sites are PARSED from this package's *_test.go, so the set can never
// drift from the actual test wiring (same technique as benchmarkedIDs).
func TestEveryAutoFixHasGoldenTest(t *testing.T) {
	golden, advisory := analysistestWiring(t)

	for _, c := range All() {
		if !c.AutoFix {
			continue
		}
		if golden[c.ID] {
			continue
		}
		if advisory[c.ID] {
			t.Errorf("%s is AutoFix but is tested with analysistest.Run (diagnostics only) — its fix is NEVER applied or verified. Use analysistest.RunWithSuggestedFixes with a .go.golden fixture.", c.ID)
		} else {
			t.Errorf("%s is AutoFix but has no analysistest.RunWithSuggestedFixes test wiring in the checks package — add a TestPS%s with a .go.golden fixture that verifies the rewrite.", c.ID, c.ID[2:])
		}
	}
}

// analysistestWiring parses every *_test.go in the checks package and returns
// the sets of PS ids passed as the analyzer argument to
// analysistest.RunWithSuggestedFixes (golden) and analysistest.Run (advisory).
// The analyzer argument is always the selector `PS####.Analyzer`, so the id is
// the X ident of that selector.
func analysistestWiring(t *testing.T) (golden, advisory map[string]bool) {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	golden, advisory = map[string]bool{}, map[string]bool{}
	fset := token.NewFileSet()
	for _, f := range files {
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		ast.Inspect(af, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			var dst map[string]bool
			switch sel.Sel.Name {
			case "RunWithSuggestedFixes":
				dst = golden
			case "Run":
				dst = advisory
			default:
				return true
			}
			// Confirm the receiver is the analysistest package (not some other
			// Run/RunWithSuggestedFixes), then collect every `PS####.Analyzer`
			// argument.
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "analysistest" {
				return true
			}
			for _, arg := range call.Args {
				asel, ok := arg.(*ast.SelectorExpr)
				if !ok || asel.Sel.Name != "Analyzer" {
					continue
				}
				id, ok := asel.X.(*ast.Ident)
				if !ok {
					continue
				}
				dst[id.Name] = true
			}
			return true
		})
	}
	if len(golden) == 0 {
		t.Fatal("parsed no analysistest.RunWithSuggestedFixes call sites — glob or matcher is wrong")
	}
	return golden, advisory
}
