package ps3009

import (
	"fmt"
	"slices"
	"strings"
)

// strings.Compare passed DIRECTLY as the comparator is the same
// anti-pattern minus the closure layer; it rewrites identically and the
// deletions drop this file's only strings references, so the
// spec-in-group import is pruned. Both the unstable and the stable
// callee match.
func sortFuncValue(ys, zs []string) {
	slices.SortFunc(ys, strings.Compare)       // want `slices\.Sort sorts the string elements in the identical byte-lexicographic order with the comparison inlined`
	slices.SortStableFunc(zs, strings.Compare) // want `slices\.Sort sorts the string elements in the identical byte-lexicographic order, the comparison inlined and no stability cost`
	fmt.Println(ys, zs)
}
