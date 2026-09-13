// Authentic decode/reference/native-wrapper source from GoAI commit
// a10a6bff8f7cb0adf695742b6ec677b750b03c28. gguf.go blob
// f0606eb238b5888be5d20d4511999cfa34cc7379; scalar reference blob
// 39c74e29aac3a0ceb68e37c4532ffd02f7027613; arm64 wrapper blob
// 74768f5b8b775f11d50bcd526f313b2533a494de. Package/import/constants
// and table declarations adapted only for standalone typing. These kernels
// fuse FLOAT activation with packed WEIGHT: no activation-positive provenance.
package fixture

import (
	"encoding/binary"
	"math"
)

const blockElems = 32

var f16Table [65536]float32
var qKByteToF32Indexes [256]byte

func f16ToF32(h uint16) float32 { return f16Table[h] }

// f16ToF32bits converts an IEEE-754 binary16 to float32 (handles subnormals,
// inf, NaN). Reference implementation; runtime conversions use f16Table.
func f16ToF32bits(h uint16) float32 {
	sign := uint32(h>>15) << 31
	exp := uint32(h>>10) & 0x1F
	frac := uint32(h) & 0x3FF
	switch exp {
	case 0:
		if frac == 0 {
			return math.Float32frombits(sign) // ±0
		}
		// subnormal: normalize
		e := uint32(127 - 15 + 1)
		for frac&0x400 == 0 {
			frac <<= 1
			e--
		}
		frac &= 0x3FF
		return math.Float32frombits(sign | e<<23 | frac<<13)
	case 0x1F:
		return math.Float32frombits(sign | 0xFF<<23 | frac<<13) // inf/NaN
	default:
		return math.Float32frombits(sign | (exp+127-15)<<23 | frac<<13)
	}
}
func scalarQ8RowReference(x []float32, raw []byte) float32 {
	var acc float64
	for b := 0; b*blockElems < len(x); b++ {
		blk := raw[b*34 : b*34+34]
		//perfscan:ignore PS4001 scalar oracle intentionally decodes one scale per quant block
		d := f16ToF32(binary.LittleEndian.Uint16(blk))
		for i, q := range blk[2:] {
			acc += float64(x[b*blockElems+i]) * float64(d*float32(int8(q)))
		}
	}
	return float32(acc)
}

//
//go:noescape
func dotQ8RowNeon(x *float32, raw *byte, f16 *float32, indexes *byte, blocks int) float32

func q8FusedDecodeM1Neon(row []float32, weight []byte, n, k, rowBytes int, outf []float32) {
	blocks := k / blockElems
	if blocks == 0 {
		return
	}
	for ni := range n {
		outf[ni] = dotQ8RowNeon(
			&row[0], &weight[ni*rowBytes], &f16Table[0], &qKByteToF32Indexes[0], blocks,
		)
	}
}

// Synthetic typing scaffold only.
func owner(x []float32, raw []byte) float32 { return scalarQ8RowReference(x, raw) }
