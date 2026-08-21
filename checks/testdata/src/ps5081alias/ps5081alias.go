package ps5081alias

import (
	sl "slices"
	u "unicode/utf8"
)

func lastCloneImport(data []byte) bool {
	return u.Valid(sl.Clone(data)) // want "unicode/utf8.Valid read-only observation consumes 1 throwaway clone layer"
}
