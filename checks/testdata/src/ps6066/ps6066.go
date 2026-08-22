package ps6066

type bytesAlias []byte

func decodeAligned4(src []byte, blocks, lanes int, out []uint32) {
	for block := 0; block < blocks; block++ {
		for lane := 0; lane < lanes; lane++ {
			base := block*16 + lane*4
			word := uint32(src[base]) | // want `4 adjacent byte loads from src in nested decode loops start at an offset provably divisible by 4 relative to element zero`
				uint32(src[base+1])<<8 |
				uint32(src[base+2])<<16 |
				uint32(src[base+3])<<24
			out[block*lanes+lane] = word
		}
	}
}

func unpackAligned8(src bytesAlias, blocks, lanes int, out []uint64) {
	for block := 0; block < blocks; block++ {
		for lane := 0; lane < lanes; lane++ {
			base := block*64 + lane*8
			word := uint64(src[base]) | // want `8 adjacent byte loads from src in nested decode loops start at an offset provably divisible by 8 relative to element zero`
				uint64(src[base+1])<<8 |
				uint64(src[base+2])<<16 |
				uint64(src[base+3])<<24 |
				uint64(src[base+4])<<32 |
				uint64(src[base+5])<<40 |
				uint64(src[base+6])<<48 |
				uint64(src[base+7])<<56
			out[block*lanes+lane] = word
		}
	}
}

func decodeHeader16(src []byte, blocks, lanes int, out [][16]byte) {
	for block := 0; block < blocks; block++ {
		for lane := 0; lane < lanes; lane++ {
			base := block*32 + lane*16
			header := [16]byte{ // want `16 adjacent byte loads from src in nested decode loops start at an offset provably divisible by 16 relative to element zero`
				src[base], src[base+1], src[base+2], src[base+3],
				src[base+4], src[base+5], src[base+6], src[base+7],
				src[base+8], src[base+9], src[base+10], src[base+11],
				src[base+12], src[base+13], src[base+14], src[base+15],
			}
			out[block*lanes+lane] = header
		}
	}
}

func decodeDirectStore(src []byte, blocks, lanes int, out []uint32) {
	for block := range blocks {
		for lane := range lanes {
			out[block*lanes+lane] = uint32(src[block*16+lane*4]) | // want `4 adjacent byte loads from src in nested decode loops start at an offset provably divisible by 4 relative to element zero`
				uint32(src[block*16+lane*4+1])<<8 |
				uint32(src[block*16+lane*4+2])<<16 |
				uint32(src[block*16+lane*4+3])<<24
		}
	}
}

// One loop is not enough to establish the intended nested hot-loop shape.
func decodeSingleLoop(src []byte, blocks int, out []uint32) {
	for block := 0; block < blocks; block++ {
		base := block * 4
		out[block] = uint32(src[base]) | uint32(src[base+1])<<8 |
			uint32(src[base+2])<<16 | uint32(src[base+3])<<24
	}
}

// The block stride destroys four-byte alignment for odd blocks.
func decodeUnalignedStride(src []byte, blocks, lanes int, out []uint32) {
	for block := range blocks {
		for lane := range lanes {
			base := block*10 + lane*4
			out[block*lanes+lane] = uint32(src[base]) | uint32(src[base+1])<<8 |
				uint32(src[base+2])<<16 | uint32(src[base+3])<<24
		}
	}
}

func decodeGap(src []byte, blocks, lanes int, out []uint32) {
	for block := range blocks {
		for lane := range lanes {
			base := block*16 + lane*4
			out[block*lanes+lane] = uint32(src[base]) | uint32(src[base+1])<<8 |
				uint32(src[base+2])<<16 | uint32(src[base+4])<<24
		}
	}
}

func decodeMixedBuffers(left, right []byte, blocks, lanes int, out []uint32) {
	for block := range blocks {
		for lane := range lanes {
			base := block*16 + lane*4
			out[block*lanes+lane] = uint32(left[base]) | uint32(left[base+1])<<8 |
				uint32(right[base+2])<<16 | uint32(right[base+3])<<24
		}
	}
}

func decodeMutableBase(src []byte, blocks, lanes int, out []uint32) {
	for block := range blocks {
		for lane := range lanes {
			base := block*16 + lane*4
			base++
			out[block*lanes+lane] = uint32(src[base]) | uint32(src[base+1])<<8 |
				uint32(src[base+2])<<16 | uint32(src[base+3])<<24
		}
	}
}

// A decoded value that controls a branch is deliberately outside the rule.
func decodeControlsBranch(src []byte, blocks, lanes int, out []uint32) {
	for block := range blocks {
		for lane := range lanes {
			base := block*16 + lane*4
			word := uint32(src[base]) | uint32(src[base+1])<<8 |
				uint32(src[base+2])<<16 | uint32(src[base+3])<<24
			if word != 0 {
				out[block*lanes+lane] = word
			}
		}
	}
}

// Function names outside decode/unpack/dequant vocabulary stay silent.
func checksum(src []byte, blocks, lanes int, out []uint32) {
	for block := range blocks {
		for lane := range lanes {
			base := block*16 + lane*4
			out[block*lanes+lane] = uint32(src[base]) | uint32(src[base+1])<<8 |
				uint32(src[base+2])<<16 | uint32(src[base+3])<<24
		}
	}
}

func mix(values ...uint32) uint32 {
	var result uint32
	for _, value := range values {
		result |= value
	}
	return result
}

func decodeRuntimeCall(src []byte, blocks, lanes int, out []uint32) {
	for block := range blocks {
		for lane := range lanes {
			base := block*16 + lane*4
			out[block*lanes+lane] = mix(uint32(src[base]), uint32(src[base+1])<<8,
				uint32(src[base+2])<<16, uint32(src[base+3])<<24)
		}
	}
}

//perfscan:packed-decode-load-validated native alignment and shape threshold recorded externally.
func decodeValidated(src []byte, blocks, lanes int, out []uint32) {
	for block := range blocks {
		for lane := range lanes {
			base := block*16 + lane*4
			out[block*lanes+lane] = uint32(src[base]) | uint32(src[base+1])<<8 |
				uint32(src[base+2])<<16 | uint32(src[base+3])<<24
		}
	}
}
