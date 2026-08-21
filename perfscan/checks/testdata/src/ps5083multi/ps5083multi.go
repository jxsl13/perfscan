package ps5083multi

import (
	b "bytes"
	s "slices"
)

func adjacentCloneImports(data []byte) int {
	return len(b.Clone(s.Clone(data))) // want "len consumes 2 throwaway standard-library Clone layer"
}
