package benchmarks

import (
	"encoding/binary"
	"testing"
)

const (
	ps4001PlaneRecordBytes = 50
	ps4001PlaneOffset      = 4
	ps4001PlaneFields      = 8
	ps4001PlaneRecords     = 256
)

var (
	ps4001PlaneInput = func() []byte {
		input := make([]byte, ps4001PlaneRecordBytes*ps4001PlaneRecords)
		state := uint64(0x6a09e667f3bcc909)
		for index := range input {
			state ^= state << 13
			state ^= state >> 7
			state ^= state << 17
			input[index] = byte(state >> 29)
		}
		return input
	}()
	ps4001PlaneSink uint64
)

//go:noinline
func ps4001PlaneConsume(scale uint16, plane []uint16) uint64 {
	hash := uint64(scale) ^ 0xcbf29ce484222325
	for field, raw := range plane {
		// Retain representative packed-field work as well as every raw word.
		low := raw & 0x1ff
		high := raw >> 9
		hash ^= uint64(raw) | uint64(low)<<16 | uint64(high)<<32 | uint64(field)<<48
		hash *= 0x100000001b3
	}
	return hash
}

//go:noinline
func ps4001PlaneBeforeWork(input []byte) uint64 {
	hash := uint64(0)
	for base := 0; base+ps4001PlaneRecordBytes <= len(input); base += ps4001PlaneRecordBytes {
		record := input[base : base+ps4001PlaneRecordBytes]
		scale := binary.LittleEndian.Uint16(record[:2]) // strided scalar field
		var plane [ps4001PlaneFields]uint16
		for field := range plane {
			low := ps4001PlaneOffset + field*2
			plane[field] = binary.LittleEndian.Uint16(record[low : low+2])
		}
		hash ^= ps4001PlaneConsume(scale, plane[:])
	}
	return hash
}

//go:noinline
func ps4001PlaneAfterWork(input []byte) uint64 {
	if ps4001HasNativeUint16View { // architecture capability hoisted above the record loop
		hash := uint64(0)
		for base := 0; base+ps4001PlaneRecordBytes <= len(input); base += ps4001PlaneRecordBytes {
			record := input[base : base+ps4001PlaneRecordBytes]
			scale := binary.LittleEndian.Uint16(record[:2]) // remains strided
			bytes := record[ps4001PlaneOffset : ps4001PlaneOffset+ps4001PlaneFields*2]
			hash ^= ps4001PlaneConsume(scale, ps4001Uint16View(bytes))
		}
		return hash
	}
	return ps4001PlaneBeforeWork(input)
}

func BenchmarkPS4001Plane_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps4001PlaneSink = ps4001PlaneBeforeWork(ps4001PlaneInput)
	}
}

func BenchmarkPS4001Plane_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps4001PlaneSink = ps4001PlaneAfterWork(ps4001PlaneInput)
	}
}

func TestPS4001PlaneCrossPathBitExact(t *testing.T) {
	t.Parallel()
	before := ps4001PlaneBeforeWork(ps4001PlaneInput)
	after := ps4001PlaneAfterWork(ps4001PlaneInput)
	if before != after {
		t.Fatalf("complete consumer differs: before %#x after %#x", before, after)
	}

	input := append([]byte(nil), ps4001PlaneInput[:ps4001PlaneRecordBytes*3]...)
	for position := range input {
		original := input[position]
		for value := 0; value < 256; value++ {
			input[position] = byte(value)
			before = ps4001PlaneBeforeWork(input)
			after = ps4001PlaneAfterWork(input)
			if before != after {
				t.Fatalf("byte %d value %#02x: before %#x after %#x", position, value, before, after)
			}
			if ps4001HasNativeUint16View {
				ps4001AssertRawPlaneMatch(t, input, position, byte(value))
			}
		}
		input[position] = original
	}
}

func ps4001AssertRawPlaneMatch(t *testing.T, input []byte, changedPosition int, changedValue byte) {
	t.Helper()
	for base := 0; base+ps4001PlaneRecordBytes <= len(input); base += ps4001PlaneRecordBytes {
		record := input[base : base+ps4001PlaneRecordBytes]
		bytes := record[ps4001PlaneOffset : ps4001PlaneOffset+ps4001PlaneFields*2]
		view := ps4001Uint16View(bytes)
		for field := range ps4001PlaneFields {
			low := field * 2
			want := binary.LittleEndian.Uint16(bytes[low : low+2])
			if view[field] != want {
				t.Fatalf("byte %d value %#02x record %d field %d: view %#04x binary.LittleEndian %#04x", changedPosition, changedValue, base/ps4001PlaneRecordBytes, field, view[field], want)
			}
		}
	}
}

func TestPS4001PlaneViewIsZeroCopyOnSupportedTargets(t *testing.T) {
	t.Parallel()
	if !ps4001HasNativeUint16View {
		t.Skip("architecture-keyed uint16 view is not enabled on this target")
	}
	bytes := []byte{0x34, 0x12, 0xcd, 0xab}
	view := ps4001Uint16View(bytes)
	if len(view) != 2 || view[0] != 0x1234 || view[1] != 0xabcd {
		t.Fatalf("initial view = %#v", view)
	}
	bytes[0], bytes[1] = 0x78, 0x56
	if view[0] != 0x5678 {
		t.Fatalf("view does not alias bytes: got %#04x", view[0])
	}
}
