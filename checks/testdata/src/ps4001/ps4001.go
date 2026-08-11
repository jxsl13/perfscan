package ps4001

import (
	"encoding/binary"
	"math"
)

func rawCopyLE(dst []float64, src []byte) {}

func perElement(out []float64, buf []byte) {
	for i := range out {
		out[i] = math.Float64frombits(binary.LittleEndian.Uint64(buf[i*8:])) // want `binary\.LittleEndian\.Uint64 decodes one scalar per iteration; on a little-endian host a same-layout bulk copy moves the whole buffer once — keep this loop as the big-endian/strided fallback`
	}
}

// The bulk-copy helper is present: the loop is the intended fallback.
func withBulkPath(out []float64, buf []byte, hostLE bool) {
	if hostLE {
		rawCopyLE(out, buf)
		return
	}
	for i := range out {
		out[i] = math.Float64frombits(binary.LittleEndian.Uint64(buf[i*8:]))
	}
}

// One decode outside a loop: silent.
func header(buf []byte) uint32 {
	return binary.LittleEndian.Uint32(buf)
}
