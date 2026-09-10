package checks

import (
	"fmt"
	"go/types"
	"strings"
	"testing"
)

func TestPS6080CallbackSummaryDiamondSourceBound(t *testing.T) {
	t.Parallel()
	const depth = 48
	var source strings.Builder
	source.WriteString("package probe\nfunc walk(f func()) { level0(f) }\n")
	for index := range depth {
		fmt.Fprintf(&source, "func level%d(f func()) { level%d(f); level%d(f) }\n", index, index+1, index+1)
	}
	fmt.Fprintf(&source, "func level%d(f func()) { f() }\n", depth)
	pass := ps6080CallbackGrowthPass(t, source.String())
	walk := pass.Pkg.Scope().Lookup("walk").(*types.Func)
	may := ps6080MayNamedCallbackSites(pass)
	sites := may[walk][0]
	if len(sites) != 1 || !sites[0].unknown || len(sites[0].order) != 0 {
		t.Fatalf("2^48 invocation paths must remain one uncertain source terminal: %+v", sites)
	}
	references := 0
	for _, parameters := range may {
		for _, sites := range parameters {
			references += len(sites)
		}
	}
	if references != depth+2 {
		t.Fatalf("summary size must be bounded by source nodes: got %d, want %d", references, depth+2)
	}
	for _, calls := range ps6080NamedCallbackInvocations(pass)[walk] {
		if len(calls) != 0 {
			t.Fatal("ambiguous diamond must not manufacture linear ordering evidence")
		}
	}
}
