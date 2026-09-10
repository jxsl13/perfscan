package checks

import (
	"go/ast"
	"testing"
)

func TestPS6080CallbackPossibleInvocationCannotKillCapturedState(t *testing.T) {
	t.Parallel()
	for _, dispatcher := range []string{
		`func dispatch(f func(), b bool) { if b { f() } }`,
		`func dispatch(f func(), b bool) { if b { f = func(){} }; f() }`,
	} {
		t.Run(dispatcher, func(t *testing.T) {
			t.Parallel()
			pass := ps6080CallbackGrowthPass(t, "package probe\n"+dispatcher+`
func caller(b bool) { value := 1; dispatch(func(){ value = 0 }, b); _ = value }
`)
			function := pass.Files[0].Decls[1].(*ast.FuncDecl)
			invoked := ps6080ComputeInvokedFunctionLiterals(pass, function.Body)
			if len(invoked.literals) != 1 {
				t.Fatalf("possible callback body must remain reachable, got %d", len(invoked.literals))
			}
			for literal := range invoked.literals {
				if roots := ps6080LiteralRootInvocationPositions(invoked, literal, nil); len(roots) != 0 {
					t.Fatalf("possible-only callback became a definite overwrite barrier: %v", roots)
				}
			}
		})
	}
}
