package checks

import (
	"go/types"
	"testing"
)

func TestPS6080CallbackAdapterIndependentReview(t *testing.T) {
	t.Parallel()
	t.Run("known terminal survives separate opaque route", func(t *testing.T) {
		t.Parallel()
		pass := ps6080CallbackGrowthPass(t, `package probe
var opaque func(func())
func walk(f func()) { f(); opaque(f) }
`)
		walk := pass.Pkg.Scope().Lookup("walk").(*types.Func)
		sites := ps6080MayNamedCallbackSites(pass)[walk][0]
		known, unknown := 0, 0
		for _, site := range sites {
			if site.unknown {
				unknown++
				if site.call != nil {
					t.Errorf("opaque sentinel retained a fabricated call: %v", site.call)
				}
			} else {
				known++
				if site.call == nil || len(site.order) != 1 {
					t.Errorf("known terminal lost call/order: %+v", site)
				}
			}
		}
		if known != 1 || unknown != 1 {
			t.Fatalf("known/opaque effects collapsed: known=%d unknown=%d sites=%+v", known, unknown, sites)
		}
		if got := ps6080NamedCallbackInvocations(pass)[walk][0]; len(got) != 1 {
			t.Fatalf("guaranteed direct callback erased by separate opaque route: %+v", got)
		}
	})

	t.Run("diamond remains one ambiguous MAY terminal", func(t *testing.T) {
		t.Parallel()
		pass := ps6080CallbackGrowthPass(t, `package probe
func leaf(f func()) { f() }
func left(f func()) { leaf(f) }
func right(f func()) { leaf(f) }
func walk(f func(), choose bool) { if choose { left(f) } else { right(f) } }
`)
		walk := pass.Pkg.Scope().Lookup("walk").(*types.Func)
		sites := ps6080MayNamedCallbackSites(pass)[walk][0]
		if len(sites) != 1 || !sites[0].unknown || sites[0].call == nil {
			t.Fatalf("diamond must collapse to one ambiguous source terminal: %+v", sites)
		}
		definite := ps6080NamedCallbackInvocations(pass)[walk]
		if len(definite) > 0 && len(definite[0]) != 0 {
			got := definite[0]
			t.Fatalf("ambiguous path manufactured definite invocation: %+v", got)
		}
	})

	t.Run("unique path preserves argument mapping", func(t *testing.T) {
		t.Parallel()
		pass := ps6080CallbackGrowthPass(t, `package probe
func leaf(f func(int), value int) { f(value) }
func walk(value int, f func(int)) { leaf(f, value) }
`)
		walk := pass.Pkg.Scope().Lookup("walk").(*types.Func)
		sites := ps6080MayNamedCallbackSites(pass)[walk][1]
		if len(sites) != 1 || sites[0].unknown || len(sites[0].order) != 2 {
			t.Fatalf("unique MAY provenance lost: %+v", sites)
		}
		invocations := ps6080NamedCallbackInvocations(pass)[walk][1]
		if len(invocations) != 1 || len(invocations[0].arguments) != 1 || !ps6080HasIndex(invocations[0].arguments[0], 0) {
			t.Fatalf("unique definite argument mapping lost: %+v", invocations)
		}
	})
}
