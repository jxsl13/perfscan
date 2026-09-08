//go:build !amd64 && !arm64

package benchmarks

const ps4001HasNativeUint16View = false

func ps4001Uint16View([]byte) []uint16 {
	panic("native little-endian uint16 view is unavailable")
}
