package ps4102

import bin "encoding/binary"

type nibble uint32

const (
	topNibbleShift = 28
	lowNibble      = 15
)

var effectCount int

func source() []byte {
	effectCount++
	return []byte{0, 1, 2, 3, 4, 5, 6, 7, 8}
}

func low() int {
	effectCount++
	return 1
}

func little16(data []byte) uint16 {
	return bin.LittleEndian.Uint16(data[1:]) >> 8 // want `final required byte`
}

func little32Owner(data []byte, group int) uint32 {
	return bin.LittleEndian.Uint32(data[66+group*4:]) >> topNibbleShift // want `final required byte`
}

func little64(data []byte) uint64 {
	return (bin.LittleEndian.Uint64(data[1:9:9]) >> 61) & 7 // want `final required byte`
}

func big16(data []byte) uint16 {
	return bin.BigEndian.Uint16(data[1:3]) & 0xff // want `final required byte`
}

func big32(data []byte) uint32 {
	return (bin.BigEndian.Uint32(data[1:]) >> 4) & lowNibble // want `final required byte`
}

func big64(data []byte) uint64 {
	return (bin.BigEndian.Uint64(data[1:9]) >> 7) & 1 // want `final required byte`
}

func effects() uint32 {
	return bin.LittleEndian.Uint32(source()[low():]) >> 28 // want `final required byte`
}

func namedResult(data []byte) nibble {
	return nibble(bin.LittleEndian.Uint32(data[1:]) >> 28) // want `final required byte`
}

func inferredResult(data []byte) any {
	value := bin.LittleEndian.Uint32(data[1:]) >> 28 // want `final required byte`
	return value
}

func localLittle(data []byte) uint32 {
	word := bin.LittleEndian.Uint32(data[1:]) // want `final required byte`
	return word >> 28
}

func localBig(data []byte) uint64 {
	word := bin.BigEndian.Uint64(data[1:9:9]) // want `final required byte`
	return (word >> 4) & 15
}

func commentsRemainVisible(data []byte) uint32 {
	return bin.LittleEndian.Uint32( /* retain */ data[1:]) >> 28 // want `final required byte`
}

func shadowedConversionName(data []byte) uint32 {
	uint32 := func(value byte) uint32 { return uint32(value) }
	_ = uint32
	return bin.LittleEndian.Uint32(data[1:]) >> 28 // want `final required byte`
}

func earlierLittleByte(data []byte) uint32 {
	return (bin.LittleEndian.Uint32(data[1:]) >> 16) & 0xff
}

func firstBigByte(data []byte) uint32 {
	return bin.BigEndian.Uint32(data[1:]) >> 24
}

func crossingBigField(data []byte) uint32 {
	return (bin.BigEndian.Uint32(data[1:]) >> 4) & 0x1f
}

func nonContiguousMask(data []byte) uint32 {
	return bin.BigEndian.Uint32(data[1:]) & 0xa
}

func nonContiguousLittleMask(data []byte) uint32 {
	return (bin.LittleEndian.Uint32(data[1:]) >> 28) & 0xa
}

func dynamicMask(data []byte, mask uint32) uint32 {
	return (bin.LittleEndian.Uint32(data[1:]) >> 28) & mask
}

func identity(value uint32) uint32 { return value }

// A rejected outer dynamic mask must not prune valid shifts nested inside an
// unrelated call or conversion.
func nestedCallArgument(data []byte, mask uint32) uint32 {
	return identity(bin.LittleEndian.Uint32(data[1:])>>28) & mask // want `final required byte`
}

func convertedArgument(data []byte, mask uint32) uint32 {
	return uint32(bin.LittleEndian.Uint32(data[1:])>>28) & mask // want `final required byte`
}

func dynamicShift(data []byte, shift uint) uint32 {
	return bin.LittleEndian.Uint32(data[1:]) >> shift
}

func extraUse(data []byte) uint32 {
	word := bin.LittleEndian.Uint32(data[1:])
	effectCount += int(word)
	return word >> 28
}

// The use-count walk must see uses inside matcher-pruned dynamic masks.
func extraUseInsideDynamicMask(data []byte, mask uint32) uint32 {
	word := bin.LittleEndian.Uint32(data[1:])
	effectCount += int((word >> 28) & mask)
	return word >> 28
}

func directBuffer(data []byte) uint32 {
	return bin.LittleEndian.Uint32(data) >> 28
}

func runtimeOrder(data []byte, order bin.ByteOrder) uint32 {
	return order.Uint32(data[1:]) >> 28
}

func nativeOrder(data []byte) uint32 {
	return bin.NativeEndian.Uint32(data[1:]) >> 28
}

type fakeOrder struct{}

func (fakeOrder) Uint32([]byte) uint32 { return 0 }

var fake = struct{ LittleEndian fakeOrder }{}

func fakeMethod(data []byte) uint32 {
	return fake.LittleEndian.Uint32(data[1:]) >> 28
}
