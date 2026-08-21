package ps5081multi

import (
	b "bytes"
	sl "slices"
	u "unicode/utf8"
)

func adjacentCloneImports(data []byte) bool {
	return u.Valid(b.Clone(sl.Clone(data))) // want "unicode/utf8.Valid read-only observation consumes 2 throwaway clone layer"
}
