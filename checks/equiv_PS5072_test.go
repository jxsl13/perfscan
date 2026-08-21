package checks

import (
	"encoding/binary"
	"hash/maphash"
	"testing"
)

// TestEquiv_PS5072MaphashSum64 proves the exact stdlib composition behind the
// fix across multiple seeds and write histories. Sum must also leave the hash
// state unchanged, so a second direct Sum64 call must still agree.
func TestEquiv_PS5072MaphashSum64(t *testing.T) {
	payloads := [][]byte{
		nil,
		{},
		[]byte("a"),
		[]byte("perfscan"),
		make([]byte, 257),
	}
	for i, payload := range payloads {
		var h maphash.Hash
		h.SetSeed(maphash.MakeSeed())
		if _, err := h.Write(payload); err != nil {
			t.Fatalf("case %d: Write: %v", i, err)
		}
		before := binary.LittleEndian.Uint64(h.Sum(nil))
		after := h.Sum64()
		if before != after {
			t.Fatalf("case %d: LittleEndian.Uint64(Sum(nil))=%#x, Sum64=%#x", i, before, after)
		}
		if again := h.Sum64(); again != after {
			t.Fatalf("case %d: Sum mutated hash state: first=%#x second=%#x", i, after, again)
		}
	}
}
