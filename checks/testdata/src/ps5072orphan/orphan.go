package ps5072orphan

import (
	"encoding/binary"
	"hash/maphash"
)

func sum(h *maphash.Hash) uint64 {
	return binary.LittleEndian.Uint64(h.Sum(nil)) // want `binary\.LittleEndian\.Uint64\(h\.Sum\(nil\)\) makes and decodes an 8-byte representation`
}
