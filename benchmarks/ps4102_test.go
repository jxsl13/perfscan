package benchmarks

import (
	"encoding/binary"
	"testing"
)

// PS4102 — decode a full little-endian uint32 to select its high nibble
// versus loading only the final required byte. Both forms retain the same
// minimum four-byte input requirement.
var (
	ps4102Input    = []byte{0x12, 0x34, 0x56, 0xa7}
	ps4102BigInput = []byte{0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 0}
	ps4102Sink     uint32
)

func BenchmarkPS4102_Before(b *testing.B) {
	for range b.N {
		ps4102Sink = binary.LittleEndian.Uint32(ps4102Input[:]) >> 28
	}
}

func BenchmarkPS4102_After(b *testing.B) {
	for range b.N {
		ps4102Sink = uint32((ps4102Input[:])[3]) >> 4
	}
}

// The low nibble selects the final physical byte of a big-endian uint32. The
// result becomes the next iteration's slice offset, keeping both arms' loads
// and bounds checks data-dependent while visiting offsets 0 through 15.
func BenchmarkPS4102BigEndian_Before(b *testing.B) {
	index := uint32(0)
	for range b.N {
		index = binary.BigEndian.Uint32(ps4102BigInput[index:]) & 15
	}
	ps4102Sink = index
}

func BenchmarkPS4102BigEndian_After(b *testing.B) {
	index := uint32(0)
	for range b.N {
		index = uint32((ps4102BigInput[index:])[3]) & 15
	}
	ps4102Sink = index
}
