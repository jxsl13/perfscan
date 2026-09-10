package checks

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// Unrelated declarations cannot change the callback graph, its reachable effects,
// or the amount of invocation provenance retained for that graph. This guards
// the former whole-package declaration-count recursion cutoff: three unrelated
// functions could multiply the number of retained recursive paths by eight.
func TestPS6080CallbackSummaryIgnoresUnrelatedDeclarations(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		source string
	}{
		{
			name: "conditional_recursive_may_call",
			source: `package probe
func walk(f func(), n int) {
 if n == 0 { f(); return }
 walk(f, n-1)
 walk(f, n-1)
}
`,
		},
		{
			name: "direct_call_before_recursive_forwarding",
			source: `package probe
func walk(f func(), n int) {
 f()
 walk(f, n-1)
 walk(f, n-1)
}
`,
		},
		{
			name: "mutually_recursive_forwarding",
			source: `package probe
func walk(f func(), n int) {
 f()
 step(f, n-1)
 step(f, n-1)
}
func step(f func(), n int) { walk(f, n) }
`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			baseline := ps6080CallbackSummarySize(t, test.source)
			if baseline.maySites == 0 {
				t.Fatal("reachable callback effect was erased")
			}
			for _, padding := range []int{3, 7} {
				var source strings.Builder
				source.WriteString(test.source)
				for index := range padding {
					fmt.Fprintf(&source, "func unrelated%d() {}\n", index)
				}
				got := ps6080CallbackSummarySize(t, source.String())
				if got != baseline {
					t.Errorf("%d unrelated declarations changed the callback summary: got %+v, baseline %+v", padding, got, baseline)
				}
			}
		})
	}
}

type ps6080CallbackSummaryCounts struct {
	maySites      int
	mayPositions  int
	mustCalls     int
	mustPositions int
}

func ps6080CallbackSummarySize(t *testing.T, source string) ps6080CallbackSummaryCounts {
	t.Helper()
	pass := ps6080CallbackGrowthPass(t, source)
	function := pass.Pkg.Scope().Lookup("walk").(*types.Func)
	var result ps6080CallbackSummaryCounts
	for _, sites := range ps6080MayNamedCallbackSites(pass)[function] {
		result.maySites += len(sites)
		for _, site := range sites {
			result.mayPositions += len(site.order)
		}
	}
	for _, invocations := range ps6080NamedCallbackInvocations(pass)[function] {
		result.mustCalls += len(invocations)
		for _, invocation := range invocations {
			result.mustPositions += len(invocation.order)
		}
	}
	return result
}

// Bounded summaries must preserve ordinary acyclic invocation order and the
// exact forwarded-argument mapping, not make every dispatcher unknown.
func TestPS6080CallbackSummaryPreservesAcyclicMapping(t *testing.T) {
	t.Parallel()
	pass := ps6080CallbackGrowthPass(t, `package probe
func leaf(f func(int), value int) { f(value) }
func walk(value int, f func(int)) { leaf(f, value) }
`)
	walk := pass.Pkg.Scope().Lookup("walk").(*types.Func)
	may := ps6080MayNamedCallbackSites(pass)[walk]
	if len(may) != 2 || len(may[1]) != 1 || len(may[1][0].order) != 2 {
		t.Fatalf("acyclic may-call provenance lost: %+v", may)
	}
	must := ps6080NamedCallbackInvocations(pass)[walk]
	if len(must) != 2 || len(must[1]) != 1 || len(must[1][0].order) != 2 {
		t.Fatalf("acyclic definite-call provenance lost: %+v", must)
	}
	arguments := must[1][0].arguments
	if len(arguments) != 1 || !ps6080HasIndex(arguments[0], 0) || ps6080HasIndex(arguments[0], 1) {
		t.Fatalf("expected callback argument 0 from walk parameter 0, got %+v", arguments)
	}
	if must[1][0].order[0] <= must[1][0].order[1] {
		t.Fatalf("outer call must precede leaf callback in invocation order: %v", must[1][0].order)
	}
}

func ps6080CallbackGrowthPass(t *testing.T, source string) *analysis.Pass {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "callbacks.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue), Defs: make(map[*ast.Ident]types.Object),
		Uses: make(map[*ast.Ident]types.Object), Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Instances: make(map[*ast.Ident]types.Instance),
	}
	pkg, err := (&types.Config{}).Check("callbackgrowth/probe", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	t.Cleanup(func() {
		ps6080MayCallbackCaches.Delete(pass)
		ps6080NamedOrderCaches.Delete(pass)
		ps6080NamedCallbackCaches.Delete(pass)
		ps6080InvokedLiteralCaches.Delete(pass)
	})
	return pass
}
