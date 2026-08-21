package ps5084multi

import (
	b "bytes"
	s "slices"
)

func adjacentCloneImports(dst, src []byte) int {
	return copy(dst, b.Clone(s.Clone(src))) // want "copy source consumes 2 throwaway standard-library Clone layer"
}
