package ps5085alias

import (
	b "bytes"
	sl "slices"
)

func lastCloneAliasImport(buffer *b.Buffer, data []byte) (int, error) {
	return buffer.Write(sl.Clone(data)) // want `bytes.Buffer.Write copies the argument before returning but receives 1 throwaway standard-library Clone layer`
}
