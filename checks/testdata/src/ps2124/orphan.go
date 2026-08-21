package ps2124

// The only strings reference in this FILE is the fixable Join itself:
// the concatenation rewrite would orphan the import, so advisory only.

import (
	"strings"
)

func orphanJoin(a, b string) string {
	return strings.Join([]string{a, b}, "/") // want `strings\.Join over an inline \[\]string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved \+ concatenation builds the identical string`
}
