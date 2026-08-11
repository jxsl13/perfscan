package ps3071

import (
	"encoding/binary"
	"io"
)

type reader struct {
	src     io.Reader
	scratch [8]byte
}

func (r *reader) u32() uint32 {
	var buf [4]byte
	_, _ = io.ReadFull(r.src, buf[:]) // want `local array buf sliced into an interface parameter escapes and is heap-allocated on every call; hang the buffer on the receiver \(check the type is not used concurrently\)`
	return binary.LittleEndian.Uint32(buf[:])
}

func (r *reader) u64OnReceiver() uint64 {
	_, _ = io.ReadFull(r.src, r.scratch[:])
	return binary.LittleEndian.Uint64(r.scratch[:])
}

// Slicing into a concrete []byte parameter does not box: silent.
func fill(dst []byte) {}

func (r *reader) concrete() {
	var buf [4]byte
	fill(buf[:])
}

// A plain function has no receiver to hang the buffer on: silent (a
// different remedy applies).
func standalone(src io.Reader) uint32 {
	var buf [4]byte
	_, _ = io.ReadFull(src, buf[:])
	return binary.LittleEndian.Uint32(buf[:])
}
