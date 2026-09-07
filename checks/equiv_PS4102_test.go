package checks

import (
	"encoding/binary"
	"testing"
)

func TestEquiv_PS4102LastRequiredByte(t *testing.T) {
	t.Parallel()
	for _, width := range []int{2, 4, 8} {
		for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
			for last := 0; last < 256; last++ {
				data := make([]byte, width)
				for index := range data {
					data[index] = byte(index*37 + last*13)
				}
				data[width-1] = byte(last)
				var decoded uint64
				switch width {
				case 2:
					decoded = uint64(order.Uint16(data))
				case 4:
					decoded = uint64(order.Uint32(data))
				case 8:
					decoded = order.Uint64(data)
				}
				for byteShift := uint(0); byteShift < 8; byteShift++ {
					wideShift := byteShift
					if order == binary.LittleEndian {
						wideShift += uint((width - 1) * 8)
					}
					for mask := uint64(1); mask < 1<<(8-byteShift); mask = mask<<1 | 1 {
						before := decoded >> wideShift & mask
						after := uint64(data[width-1]) >> byteShift & mask
						if before != after {
							t.Fatalf("width=%d order=%s byte=%#x shift=%d mask=%#x: before=%#x after=%#x", width, order, last, byteShift, mask, before, after)
						}
					}
				}
			}
		}
	}
}

func TestEquiv_PS4102PanicAndEffects(t *testing.T) {
	t.Parallel()
	for _, width := range []int{2, 4, 8} {
		for length := 0; length <= width+1; length++ {
			data := make([]byte, length)
			for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
				widePanic := ps4102DidPanic(func() {
					switch width {
					case 2:
						_ = order.Uint16(data)
					case 4:
						_ = order.Uint32(data)
					case 8:
						_ = order.Uint64(data)
					}
				})
				narrowPanic := ps4102DidPanic(func() { _ = data[width-1] })
				if widePanic != narrowPanic || widePanic != (length < width) {
					t.Fatalf("width=%d len=%d order=%s: wide panic=%v narrow panic=%v", width, length, order, widePanic, narrowPanic)
				}
			}
		}
	}

	beforeTrace := make([]int, 0, 2)
	beforeSource := func() []byte {
		beforeTrace = append(beforeTrace, 1)
		return []byte{0, 0, 0, 0xa0}
	}
	beforeLow := func() int {
		beforeTrace = append(beforeTrace, 2)
		return 0
	}
	before := binary.LittleEndian.Uint32(beforeSource()[beforeLow():]) >> 28

	afterTrace := make([]int, 0, 2)
	afterSource := func() []byte {
		afterTrace = append(afterTrace, 1)
		return []byte{0, 0, 0, 0xa0}
	}
	afterLow := func() int {
		afterTrace = append(afterTrace, 2)
		return 0
	}
	after := uint32((afterSource()[afterLow():])[3]) >> 4
	if before != after || len(beforeTrace) != 2 || len(afterTrace) != 2 ||
		beforeTrace[0] != 1 || beforeTrace[1] != 2 ||
		afterTrace[0] != 1 || afterTrace[1] != 2 {
		t.Fatalf("effect/order mismatch: before=%d trace=%v after=%d trace=%v", before, beforeTrace, after, afterTrace)
	}
}

func ps4102DidPanic(fn func()) (panicked bool) {
	defer func() { panicked = recover() != nil }()
	fn()
	return false
}
