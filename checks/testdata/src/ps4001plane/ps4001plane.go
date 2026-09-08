package ps4001plane

import "encoding/binary"

const qhCount = 8

var globalBytes []byte

type planeHolder struct {
	bytes  []byte
	fields [8]uint16
}

func uint16Plane(src []byte, records int, out [][qhCount]uint16) {
	for record := 0; record < records; record++ {
		base := record*50 + 4
		for field := 0; field < qhCount; field++ {
			out[record][field] = binary.LittleEndian.Uint16(src[base+field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func localUint16Plane(src []byte, records int) uint64 {
	var sum uint64
	for record := 0; record < records; record++ {
		base := record*50 + 4
		var fields [qhCount]uint16
		for field := 0; field < qhCount; field++ {
			fields[field] = binary.LittleEndian.Uint16(src[base+field*2:]) // want "fixed 8-field/16-byte contiguous binary.LittleEndian.Uint16 plane"
		}
		for _, raw := range fields {
			sum += uint64(raw)
		}
	}
	return sum
}

func invariantSourceAlias(src []byte, records int) uint64 {
	var sum uint64
	for record := 0; record < records; record++ {
		block := src[record*50+4:]
		var fields [8]uint16
		for field := 0; field < 8; field++ {
			fields[field] = binary.LittleEndian.Uint16(block[field*2:]) // want "fixed 8-field/16-byte contiguous binary.LittleEndian.Uint16 plane"
		}
		for _, raw := range fields {
			sum += uint64(raw)
		}
	}
	return sum
}

func perFieldBacking(chunks [][]byte, records int) uint64 {
	var sum uint64
	for record := 0; record < records; record++ {
		var fields [8]uint16
		for field := 0; field < 8; field++ {
			fields[field] = binary.LittleEndian.Uint16(chunks[field][record*50+field*2:]) // want "decodes one scalar per iteration"
		}
		for _, raw := range fields {
			sum += uint64(raw)
		}
	}
	return sum
}

func stridedRecordOffset(src []byte, consume func([8]uint16)) {
	for base := 0; base+50 <= len(src); base += 50 {
		var fields [8]uint16
		for field := 0; field < 8; field++ {
			fields[field] = binary.LittleEndian.Uint16(src[base+4+field*2:]) // want "decodes one scalar per iteration"
		}
		consume(fields)
	}
}

func minimumPlane(records [][]byte) uint64 {
	var sum uint64
	for _, block := range records {
		var fields [2]uint16
		for field := 0; field < 2; field++ {
			fields[field] = binary.LittleEndian.Uint16(block[field*2:]) // want "fixed 2-field/4-byte contiguous binary.LittleEndian.Uint16 plane"
		}
		for _, raw := range fields {
			sum += uint64(raw)
		}
	}
	return sum
}

func maximumPlane(records [][]byte) uint64 {
	var sum uint64
	for _, block := range records {
		var fields [64]uint16
		for field := 0; field < 64; field++ {
			fields[field] = binary.LittleEndian.Uint16(block[field*2:]) // want "fixed 64-field/128-byte contiguous binary.LittleEndian.Uint16 plane"
		}
		for _, raw := range fields {
			sum += uint64(raw)
		}
	}
	return sum
}

func multiInductionPlane(src []byte, records int) uint64 {
	var sum uint64
	for record, base := 0, 4; record < records && base+16 <= len(src); record, base = record+1, base+50 {
		var fields [8]uint16
		for field := 0; field < 8; field++ {
			fields[field] = binary.LittleEndian.Uint16(src[base+field*2:]) // want "fixed 8-field/16-byte contiguous binary.LittleEndian.Uint16 plane"
		}
		for _, raw := range fields {
			sum += uint64(raw)
		}
	}
	return sum
}

func invariantOuterInitializer(src []byte, records int) uint64 {
	var sum uint64
	for record, invariantBase := 0, 4; record < records; record++ {
		var fields [8]uint16
		for field := 0; field < 8; field++ {
			fields[field] = binary.LittleEndian.Uint16(src[invariantBase+field*2:]) // want "decodes one scalar per iteration"
		}
		for _, raw := range fields {
			sum += uint64(raw)
		}
	}
	return sum
}

func constantDestination(src []byte, records int) uint64 {
	var sum uint64
	for record := 0; record < records; record++ {
		base := record*50 + 4
		var fields [8]uint16
		for field := 0; field < 8; field++ {
			fields[0] = binary.LittleEndian.Uint16(src[base+field*2:]) // want "decodes one scalar per iteration"
		}
		for _, raw := range fields {
			sum += uint64(raw)
		}
	}
	return sum
}

func reversedDestination(src []byte, records int) uint64 {
	var sum uint64
	for record := 0; record < records; record++ {
		base := record*50 + 4
		var fields [8]uint16
		for field := 0; field < 8; field++ {
			fields[7-field] = binary.LittleEndian.Uint16(src[base+field*2:]) // want "decodes one scalar per iteration"
		}
		for _, raw := range fields {
			sum += uint64(raw)
		}
	}
	return sum
}

func singleScalarDestination(src []byte, records int) uint16 {
	var last uint16
	for record := 0; record < records; record++ {
		base := record*50 + 4
		for field := 0; field < 8; field++ {
			last = binary.LittleEndian.Uint16(src[base+field*2:]) // want "decodes one scalar per iteration"
		}
	}
	return last
}

func sourceMutationBeforeConsumer(src []byte, records int) uint64 {
	var sum uint64
	for record := 0; record < records; record++ {
		base := record*50 + 4
		var fields [8]uint16
		for field := 0; field < 8; field++ {
			fields[field] = binary.LittleEndian.Uint16(src[base+field*2:]) // want "decodes one scalar per iteration"
		}
		src[base] = 0
		for _, raw := range fields {
			sum += uint64(raw)
		}
	}
	return sum
}

func destinationMutationBeforeConsumer(src []byte, records int) uint64 {
	var sum uint64
	for record := 0; record < records; record++ {
		base := record*50 + 4
		var fields [8]uint16
		for field := 0; field < 8; field++ {
			fields[field] = binary.LittleEndian.Uint16(src[base+field*2:]) // want "decodes one scalar per iteration"
		}
		fields[0] = 0
		for _, raw := range fields {
			sum += uint64(raw)
		}
	}
	return sum
}

func callbackBeforeConsumer(src []byte, records int, mutate func([]byte)) uint64 {
	var sum uint64
	for record := 0; record < records; record++ {
		base := record*50 + 4
		var fields [8]uint16
		for field := 0; field < 8; field++ {
			fields[field] = binary.LittleEndian.Uint16(src[base+field*2:]) // want "decodes one scalar per iteration"
		}
		mutate(src[base : base+16])
		for _, raw := range fields {
			sum += uint64(raw)
		}
	}
	return sum
}

func partialConsumer(src []byte, records int) uint64 {
	var sum uint64
	for record := 0; record < records; record++ {
		base := record*50 + 4
		var fields [8]uint16
		for field := 0; field < 8; field++ {
			fields[field] = binary.LittleEndian.Uint16(src[base+field*2:]) // want "decodes one scalar per iteration"
		}
		sum += uint64(fields[0])
	}
	return sum
}

func stridedScalarField(src []byte, records int) uint16 {
	var sum uint16
	for record := 0; record < records; record++ {
		sum += binary.LittleEndian.Uint16(src[record*50:]) // want "decodes one scalar per iteration"
	}
	return sum
}

func uint32ArrayRange(records [][]byte, out [][4]uint32) {
	for record := range records {
		block := records[record]
		for field := range [4]struct{}{} {
			out[record][field] = binary.LittleEndian.Uint32(block[3+4*field:]) // want "decodes one scalar per iteration"
		}
	}
}

func uint64IntegerRange(records [][]byte, out [][2]uint64) {
	for record, block := range records {
		for field := range 2 {
			low := 8*field + record - record
			out[record][field] = binary.LittleEndian.Uint64(block[low : low+8]) // want "decodes one scalar per iteration"
		}
	}
}

func bigEndianIsNotNativePlane(records [][]byte, out [][8]uint16) {
	for record, block := range records {
		for field := 0; field < 8; field++ {
			out[record][field] = binary.BigEndian.Uint16(block[field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func putUintRemainsGeneric(records [][]byte) {
	for _, block := range records {
		for field := 0; field < 8; field++ {
			binary.LittleEndian.PutUint16(block[field*2:], uint16(field)) // want "decodes one scalar per iteration"
		}
	}
}

func dynamicCount(records [][]byte, count int, out [][]uint16) {
	for record, block := range records {
		for field := 0; field < count; field++ {
			out[record][field] = binary.LittleEndian.Uint16(block[field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func tooManyFields(records [][]byte, out [][65]uint16) {
	for record, block := range records {
		for field := 0; field < 65; field++ {
			out[record][field] = binary.LittleEndian.Uint16(block[field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func wrongStride(records [][]byte, out [][8]uint16) {
	for record, block := range records {
		for field := 0; field < 8; field++ {
			out[record][field] = binary.LittleEndian.Uint16(block[field*4:]) // want "decodes one scalar per iteration"
		}
	}
}

func conditionalPlane(records [][]byte, out [][8]uint16) {
	for record, block := range records {
		for field := 0; field < 8; field++ {
			if field&1 == 0 {
				out[record][field] = binary.LittleEndian.Uint16(block[field*2:]) // want "decodes one scalar per iteration"
			}
		}
	}
}

func convertedWrappingIndex(records [][]byte, out [][64]uint64) {
	for record, block := range records {
		for field := 0; field < 64; field++ {
			out[record][field] = binary.LittleEndian.Uint64(block[int8(field*8):]) // want "decodes one scalar per iteration"
		}
	}
}

func narrowWrappingIndex(records [][]byte, out [][64]uint64) {
	for record, block := range records {
		for field := uint8(0); field < 64; field++ {
			out[record][field] = binary.LittleEndian.Uint64(block[field*8:]) // want "decodes one scalar per iteration"
		}
	}
}

func branchCanShortenPlane(records [][]byte, out [][8]uint16) {
	for record, block := range records {
		for field := 0; field < 8; field++ {
			if len(block) == 0 {
				break
			}
			out[record][field] = binary.LittleEndian.Uint16(block[field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func standaloneWholeBuffer(block []byte, out [8]uint16) {
	for field := 0; field < 8; field++ {
		out[field] = binary.LittleEndian.Uint16(block[field*2:]) // want "decodes one scalar per iteration"
	}
}

func unrelatedOuterLoop(block []byte, out [8]uint16) {
	for repeat := 0; repeat < 3; repeat++ {
		_ = repeat
		for field := 0; field < 8; field++ {
			out[field] = binary.LittleEndian.Uint16(block[field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func cancelledOuterDependency(block []byte, out [8]uint16) {
	for record := 0; record < 3; record++ {
		for field := 0; field < 8; field++ {
			out[field] = binary.LittleEndian.Uint16(block[(record-record)+field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func outerRecordValueMutates(records [][]byte, replacement []byte, out [][8]uint16) {
	for record, block := range records {
		for field := 0; field < 8; field++ {
			block = replacement
			out[record][field] = binary.LittleEndian.Uint16(block[field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func modifiedIndex(records [][]byte, out [][8]uint16) {
	for record, block := range records {
		for field := 0; field < 8; field++ {
			field++
			out[record][field] = binary.LittleEndian.Uint16(block[field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func rangeAssignedOffset(records [][]byte, out [][8]uint16) {
	for record, block := range records {
		for field := 0; field < 8; field++ {
			low := field * 2
			for low = range [1]struct{}{} {
			}
			out[record][field] = binary.LittleEndian.Uint16(block[low:]) // want "decodes one scalar per iteration"
		}
	}
}

func addressTakenOffset(records [][]byte, out [][8]uint16) {
	for record, block := range records {
		for field := 0; field < 8; field++ {
			low := field * 2
			_ = &low
			out[record][field] = binary.LittleEndian.Uint16(block[low:]) // want "decodes one scalar per iteration"
		}
	}
}

func externallyDeclaredIndex(records [][]byte, out [][8]uint16) {
	var field int
	for record, block := range records {
		for field = 0; field < 8; field++ {
			out[record][field] = binary.LittleEndian.Uint16(block[field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func mismatchedBoundedHigh(records [][]byte, out [][8]uint16) {
	for record, block := range records {
		base := record * 50
		for field := 0; field < 8; field++ {
			low := base + field*2
			out[record][field] = binary.LittleEndian.Uint16(block[low : (base-field)*2+2]) // want "decodes one scalar per iteration"
		}
	}
}

func sideEffectingSource(records func() [][]byte, count int, out [][8]uint16) {
	for record := 0; record < count; record++ {
		for field := 0; field < 8; field++ {
			out[record][field] = binary.LittleEndian.Uint16(records()[record][field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func sourceElementRebound(records [][]byte, replacement []byte, out [][8]uint16) {
	for record := range records {
		for field := 0; field < 8; field++ {
			out[record][field] = binary.LittleEndian.Uint16(records[record][field*2:]) // want "decodes one scalar per iteration"
			records[record] = replacement
		}
	}
}

func sourceSelectorRebound(records []planeHolder, replacement []byte) {
	for record := range records {
		for field := 0; field < 8; field++ {
			records[record].fields[field] = binary.LittleEndian.Uint16(records[record].bytes[field*2:]) // want "decodes one scalar per iteration"
			records[record].bytes = replacement
		}
	}
}

func siblingCallback(records [][]byte, mutate func([]byte), out [][8]uint16) {
	for record, block := range records {
		for field := 0; field < 8; field++ {
			out[record][field] = binary.LittleEndian.Uint16(block[field*2:]) // want "decodes one scalar per iteration"
			mutate(block)
		}
	}
}

func destinationCallback(records [][]byte, output func() [][8]uint16) {
	for record, block := range records {
		for field := 0; field < 8; field++ {
			output()[record][field] = binary.LittleEndian.Uint16(block[field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func sameAggregateRoot(records []planeHolder) {
	for record := range records {
		for field := 0; field < 8; field++ {
			records[record].fields[field] = binary.LittleEndian.Uint16(records[record].bytes[field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func refreshedSourceAlias(loader func(int) []byte, count int, out [][8]uint16) {
	for record := 0; record < count; record++ {
		for field := 0; field < 8; field++ {
			block := loader(record)
			out[record][field] = binary.LittleEndian.Uint16(block[field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func reassignedSourceParameter(src, replacement []byte, records int, out [][8]uint16) {
	for record := 0; record < records; record++ {
		for field := 0; field < 8; field++ {
			src = replacement
			out[record][field] = binary.LittleEndian.Uint16(src[record*50+field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func packageSource(records int, out [][8]uint16) {
	for record := 0; record < records; record++ {
		for field := 0; field < 8; field++ {
			out[record][field] = binary.LittleEndian.Uint16(globalBytes[record*50+field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func allocatingSourceConversion(text string, records int, out [][8]uint16) {
	for record := 0; record < records; record++ {
		for field := 0; field < 8; field++ {
			out[record][field] = binary.LittleEndian.Uint16([]byte(text)[record*50+field*2:]) // want "decodes one scalar per iteration"
		}
	}
}

func storedClosure(records [][]byte, out [][8]uint16) {
	for record, block := range records {
		_ = func() {
			for field := 0; field < 8; field++ {
				out[record][field] = binary.LittleEndian.Uint16(block[field*2:]) // want "decodes one scalar per iteration"
			}
		}
	}
}
