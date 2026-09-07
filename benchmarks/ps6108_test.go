package benchmarks

import (
	"math"
	"os"
	"os/exec"
	"testing"
)

var (
	ps6108Headers = [8][12]byte{
		{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb},
		{0xff, 0xee, 0xdd, 0xcc, 0xbb, 0xaa, 0x99, 0x88, 0x77, 0x66, 0x55, 0x44},
		{0x3f, 0x40, 0x7f, 0x80, 0xbf, 0xc0, 0x01, 0xfe, 0x0f, 0xf0, 0x5a, 0xa5},
		{0x91, 0x27, 0xd3, 0x6e, 0x48, 0xbc, 0x05, 0xfa, 0x39, 0xc6, 0x7d, 0x82},
		{0x14, 0x28, 0x42, 0x84, 0x18, 0x81, 0x24, 0x42, 0xbd, 0xdb, 0xe7, 0x7e},
		{0x6d, 0xb6, 0xdb, 0xed, 0xf6, 0x7b, 0x3d, 0x9e, 0xcf, 0x67, 0x33, 0x19},
		{0x01, 0x03, 0x07, 0x0f, 0x1f, 0x3f, 0x7f, 0xff, 0xfe, 0xfc, 0xf8, 0xf0},
		{0x52, 0xa4, 0x49, 0x92, 0x25, 0x4a, 0x94, 0x29, 0x53, 0xa6, 0x4d, 0x9a},
	}
	ps6108Scales  = [8]float32{0.5, -0.75, 1.25, -1.5, 2.25, -3.5, 0.03125, 7.75}
	ps6108Minimum = [8]float32{-0.25, 0.625, -1.125, 1.75, -2.5, 4.25, -0.015625, 8.5}
	ps6108Sink    uint64
)

func ps6108FieldPair(index int, packed []byte) (scale, minimum byte) {
	if index < 4 {
		return packed[index] & 63, packed[index+4] & 63
	}
	scale = (packed[index+4] & 0x0f) | ((packed[index-4] >> 6) << 4)
	minimum = (packed[index+4] >> 4) | ((packed[index] >> 6) << 4)
	return
}

//go:noinline
func ps6108BeforeWork(packed *[12]byte, scale, minimum float32) [16]float32 {
	var coefficients [16]float32
	header := packed[0:12:12]
	for pair := range 4 {
		logical := pair * 2
		s0, m0 := ps6108FieldPair(logical, header)
		s1, m1 := ps6108FieldPair(logical+1, header)
		coefficient := pair * 4
		coefficients[coefficient+0] = scale * float32(s0)
		coefficients[coefficient+1] = minimum * float32(m0)
		coefficients[coefficient+2] = scale * float32(s1)
		coefficients[coefficient+3] = minimum * float32(m1)
	}
	return coefficients
}

//go:noinline
func ps6108AfterWork(packed *[12]byte, scale, minimum float32) [16]float32 {
	var coefficients [16]float32
	for field := range 4 {
		low, middle, high := packed[field], packed[field+4], packed[field+8]
		lowCoefficient, highCoefficient := field*2, (field+4)*2
		coefficients[lowCoefficient+0] = scale * float32(low&63)
		coefficients[lowCoefficient+1] = minimum * float32(middle&63)
		coefficients[highCoefficient+0] = scale * float32((high&0x0f)|((low>>6)<<4))
		coefficients[highCoefficient+1] = minimum * float32((high>>4)|((middle>>6)<<4))
	}
	return coefficients
}

func ps6108Retain(coefficients [16]float32) uint64 {
	hash := uint64(0xcbf29ce484222325)
	for _, coefficient := range coefficients {
		hash ^= uint64(math.Float32bits(coefficient))
		hash *= 0x100000001b3
	}
	return hash
}

func BenchmarkPS6108_Before(b *testing.B) {
	b.ReportAllocs()
	for index := range b.N {
		bank := index & (len(ps6108Headers) - 1)
		ps6108Sink = ps6108Retain(ps6108BeforeWork(&ps6108Headers[bank], ps6108Scales[bank], ps6108Minimum[bank]))
	}
}

func BenchmarkPS6108_After(b *testing.B) {
	b.ReportAllocs()
	for index := range b.N {
		bank := index & (len(ps6108Headers) - 1)
		ps6108Sink = ps6108Retain(ps6108AfterWork(&ps6108Headers[bank], ps6108Scales[bank], ps6108Minimum[bank]))
	}
}

func TestPS6108WorkPairBitEquality(t *testing.T) {
	t.Parallel()
	backgrounds := ps6108Headers
	for backgroundIndex, background := range backgrounds {
		for position := range background {
			for value := 0; value < 256; value++ {
				packed := background
				packed[position] = byte(value)
				original := packed
				before := ps6108BeforeWork(&packed, ps6108Scales[backgroundIndex], ps6108Minimum[backgroundIndex])
				after := ps6108AfterWork(&packed, ps6108Scales[backgroundIndex], ps6108Minimum[backgroundIndex])
				for coefficient := range before {
					if math.Float32bits(before[coefficient]) != math.Float32bits(after[coefficient]) {
						t.Fatalf("background %d byte %d value %#02x coefficient %d: before %#08x after %#08x", backgroundIndex, position, value, coefficient, math.Float32bits(before[coefficient]), math.Float32bits(after[coefficient]))
					}
				}
				if packed != original {
					t.Fatalf("work pair changed source: background %d byte %d value %#02x", backgroundIndex, position, value)
				}
			}
		}
	}
}

func TestPS6108WorkPairHostileScalars(t *testing.T) {
	t.Parallel()
	scalars := [][2]float32{
		{0, float32(math.Copysign(0, -1))},
		{float32(math.Inf(1)), float32(math.Inf(-1))},
		{math.Float32frombits(0x7fc00001), math.Float32frombits(0xffc00001)},
	}
	for index, pair := range scalars {
		before := ps6108BeforeWork(&ps6108Headers[index], pair[0], pair[1])
		after := ps6108AfterWork(&ps6108Headers[index], pair[0], pair[1])
		for coefficient := range before {
			if math.Float32bits(before[coefficient]) != math.Float32bits(after[coefficient]) {
				t.Fatalf("scalar pair %d coefficient %d: before %#08x after %#08x", index, coefficient, math.Float32bits(before[coefficient]), math.Float32bits(after[coefficient]))
			}
		}
	}
}

func TestPS6108WorkPairSeededMultiByteHeaders(t *testing.T) {
	t.Parallel()
	boundaries := [][12]byte{
		{}, // every extracted field is the minimum value
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, // maximum
	}
	for index, packed := range boundaries {
		ps6108AssertWorkPair(t, packed, ps6108Scales[index], ps6108Minimum[index], "boundary", index)
	}

	// A fixed xorshift64 sequence changes all twelve bytes together. This
	// complements the exhaustive one-byte perturbation with deterministic
	// interactions among every packed region.
	state := uint64(0x6a09e667f3bcc909)
	for sample := 0; sample < 2048; sample++ {
		var packed [12]byte
		for position := range packed {
			state ^= state << 13
			state ^= state >> 7
			state ^= state << 17
			packed[position] = byte(state >> 29)
		}
		bank := sample & (len(ps6108Scales) - 1)
		ps6108AssertWorkPair(t, packed, ps6108Scales[bank], ps6108Minimum[bank], "seeded", sample)
	}
}

func ps6108AssertWorkPair(t *testing.T, packed [12]byte, scale, minimum float32, corpus string, sample int) {
	t.Helper()
	original := packed
	before := ps6108BeforeWork(&packed, scale, minimum)
	after := ps6108AfterWork(&packed, scale, minimum)
	for coefficient := range before {
		if math.Float32bits(before[coefficient]) != math.Float32bits(after[coefficient]) {
			t.Fatalf("%s sample %d coefficient %d: before %#08x after %#08x", corpus, sample, coefficient, math.Float32bits(before[coefficient]), math.Float32bits(after[coefficient]))
		}
	}
	if packed != original {
		t.Fatalf("%s sample %d changed source", corpus, sample)
	}
}

func TestPS6108WorkPairAllocations(t *testing.T) {
	if os.Getenv("PERFSCAN_PS6108_ALLOC_CHILD") != "1" {
		t.Parallel()
		command := exec.Command(os.Args[0], "-test.run=^TestPS6108WorkPairAllocations$", "-test.count=1")
		command.Env = append(os.Environ(), "PERFSCAN_PS6108_ALLOC_CHILD=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("isolated allocation child failed: %v\n%s", err, output)
		}
		return
	}
	if allocations := testing.AllocsPerRun(100, func() {
		ps6108Sink = ps6108Retain(ps6108BeforeWork(&ps6108Headers[3], ps6108Scales[3], ps6108Minimum[3]))
	}); allocations != 0 {
		t.Fatalf("before allocations/run = %v, want 0", allocations)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		ps6108Sink = ps6108Retain(ps6108AfterWork(&ps6108Headers[3], ps6108Scales[3], ps6108Minimum[3]))
	}); allocations != 0 {
		t.Fatalf("after allocations/run = %v, want 0", allocations)
	}
}
