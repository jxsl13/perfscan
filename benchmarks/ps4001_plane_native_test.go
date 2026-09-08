//go:build amd64 || arm64

package benchmarks

import "unsafe"

const ps4001HasNativeUint16View = true

func ps4001Uint16View(bytes []byte) []uint16 {
	return unsafe.Slice((*uint16)(unsafe.Pointer(unsafe.SliceData(bytes))), len(bytes)/2)
}
