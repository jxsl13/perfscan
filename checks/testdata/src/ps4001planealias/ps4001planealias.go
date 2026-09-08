package ps4001planealias

import bin "encoding/binary"

func aliasedImport(records [][]byte, out [][4]uint32) {
	for record, block := range records {
		for field := 0; field < 4; field++ {
			out[record][field] = bin.LittleEndian.Uint32(block[field*4:]) // want "decodes one scalar per iteration"
		}
	}
}

func aliasedUint16(records [][]byte) uint64 {
	var sum uint64
	for _, block := range records {
		var fields [8]uint16
		for field := 0; field < 8; field++ {
			fields[field] = bin.LittleEndian.Uint16(block[field*2:]) // want "fixed 8-field/16-byte contiguous binary.LittleEndian.Uint16 plane"
		}
		for _, raw := range fields {
			sum += uint64(raw)
		}
	}
	return sum
}
