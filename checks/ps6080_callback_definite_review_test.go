package checks

import (
	"go/types"
	"testing"
)

func TestPS6080CallbackSummaryDoesNotPromotePossibleOrigin(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`func walk(f func(), b bool) { if b { f = func() {} }; f() }`,
		`func walk(f func(), g func(), b bool) { if b { f = g }; f() }`,
		`func leaf(f func()) { f() }; func walk(f func(), b bool) { if b { f = func() {} }; leaf(f) }`,
		`func walk(f func()) { func(){ f = func(){} }(); f() }`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			pass := ps6080CallbackGrowthPass(t, "package probe\n"+source)
			walk := pass.Pkg.Scope().Lookup("walk").(*types.Func)
			for _, calls := range ps6080NamedCallbackInvocations(pass)[walk] {
				if len(calls) != 0 {
					t.Fatalf("a possible callback origin is not a guaranteed invocation: %+v", calls)
				}
			}
		})
	}
}
