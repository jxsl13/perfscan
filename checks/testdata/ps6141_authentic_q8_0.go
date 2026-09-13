// Authentic producer/helpers copied without arithmetic changes from goai
// commit a10a6bff8f7cb0adf695742b6ec677b750b03c28, format/gguf/quant.go
// blob 7954b0e8f28b029d895b12a3891b3858842826bc. Package/import/constant
// scaffolding only is adapted for standalone typing. Real callers are model
// weight writers; this is NOT authentic activation-positive provenance.
package fixture
import("encoding/binary";"math")
const blockElems=32
// quantizeQ8_0: per 32-block, d = amax/127 (f16), qs[j] = roundf(x[j]/d) int8.
func quantizeQ8_0(x []float32) []byte {
	nb := len(x) / blockElems
	out := make([]byte, nb*34)
	for b := range nb {
		blk := x[b*blockElems : (b+1)*blockElems]
		var amax float32
		for _, v := range blk {
			// bit-exact abs via sign-bit clear (no f32->f64->f32 round-trip); |v| identical to math.Abs
			if a := math.Float32frombits(math.Float32bits(v) &^ (1 << 31)); a > amax {
				amax = a
			}
		}
		d := amax / 127
		var id float32
		if d != 0 {
			id = 1 / d
		}
		o := b * 34
		binary.LittleEndian.PutUint16(out[o:], f32ToF16(d))
		for j, v := range blk {
			out[o+2+j] = byte(int8(roundHalfAway(v * id)))
		}
	}
	return out
}

func roundHalfAway(v float32) float32 { return float32(math.Round(float64(v))) }

// f32ToF16 encodes an IEEE-754 float32 as binary16 with round-to-nearest-even
// (inverse of f16ToF32), handling overflow→inf, subnormals and NaN.
func f32ToF16(f float32) uint16 {
	b := math.Float32bits(f)
	sign := uint16(b >> 16 & 0x8000)
	exp := int32(b>>23&0xFF) - 127
	mant := b & 0x7FFFFF
	switch {
	case b&0x7FFFFFFF == 0: // ±0
		return sign
	case b>>23&0xFF == 0xFF: // inf / NaN
		if mant != 0 {
			return sign | 0x7E00 // NaN (quiet)
		}
		return sign | 0x7C00 // inf
	case exp > 15: // overflow → inf
		return sign | 0x7C00
	case exp < -24: // underflow → ±0
		return sign
	case exp < -14: // subnormal: drop (13 + (-14-exp)) low bits, round-to-nearest-even
		mant |= 0x800000 // implicit leading 1
		drop := uint32(13) + uint32(-14-exp)
		roundHalf := uint32(1) << (drop - 1)
		lower := mant & ((1 << drop) - 1)
		m := mant >> drop
		if lower > roundHalf || (lower == roundHalf && m&1 == 1) {
			m++ // may carry into the smallest normal exponent, which is correct
		}
		return sign | uint16(m)
	default: // normal
		e := uint16(exp+15) << 10
		roundHalf := uint32(1 << 12)
		lower := mant & 0x1FFF // 13 bits dropped (23→10)
		m := mant >> 13
		if lower > roundHalf || (lower == roundHalf && m&1 == 1) {
			m++
			if m == 0x400 { // mantissa overflow → bump exponent
				m = 0
				e += 1 << 10
				if e>>10 >= 0x1F {
					return sign | 0x7C00 // → inf
				}
			}
		}
		return sign | e | uint16(m)
	}
}

// Synthetic typing scaffold, not an observed activation boundary.
func owner(x []float32) []byte { return quantizeQ8_0(x) }
