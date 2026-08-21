package checks

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"unsafe"
)

func TestEquivPS5119Strings(t *testing.T) {
	random := rand.New(rand.NewSource(5119))
	for iteration := 0; iteration < 50_000; iteration++ {
		value := ps5119RandomBytes(random, random.Intn(96))
		boundary := ps5119RandomBytes(random, random.Intn(24))
		for _, suffix := range []bool{false, true} {
			before, beforeFound := ps5119BeforeString(value, boundary, suffix)
			after, afterFound := ps5119AfterString(value, boundary, suffix)
			if before != after || beforeFound != afterFound {
				t.Fatalf("string divergence: value=%q boundary=%q suffix=%v before=(%q,%v) after=(%q,%v)", value, boundary, suffix, before, beforeFound, after, afterFound)
			}
			if unsafe.StringData(before) != unsafe.StringData(after) {
				t.Fatalf("string storage divergence: value=%q boundary=%q suffix=%v", value, boundary, suffix)
			}
		}
	}
}

func TestEquivPS5119Bytes(t *testing.T) {
	random := rand.New(rand.NewSource(15_119))
	values := [][]byte{nil, {}, {0}, []byte("prefix/value/suffix"), {0xff, 0, 0xc3, 0x28}}
	for iteration := 0; iteration < 50_000; iteration++ {
		value := []byte(ps5119RandomBytes(random, random.Intn(96)))
		boundary := []byte(ps5119RandomBytes(random, random.Intn(24)))
		values = append(values[:5], value)
		for _, candidate := range values {
			for _, suffix := range []bool{false, true} {
				before, beforeFound := ps5119BeforeBytes(candidate, boundary, suffix)
				after, afterFound := ps5119AfterBytes(candidate, boundary, suffix)
				if !bytes.Equal(before, after) || beforeFound != afterFound ||
					(before == nil) != (after == nil) || len(before) != len(after) || cap(before) != cap(after) ||
					unsafe.SliceData(before) != unsafe.SliceData(after) {
					t.Fatalf("byte-slice divergence: value=%v boundary=%v suffix=%v before=(%v,len=%d,cap=%d,nil=%v,%v) after=(%v,len=%d,cap=%d,nil=%v,%v)", candidate, boundary, suffix, before, len(before), cap(before), before == nil, beforeFound, after, len(after), cap(after), after == nil, afterFound)
				}
			}
		}
	}
}

func ps5119BeforeString(value, boundary string, suffix bool) (string, bool) {
	if suffix {
		if strings.HasSuffix(value, boundary) {
			return strings.TrimSuffix(value, boundary), true
		}
		return value, false
	}
	if strings.HasPrefix(value, boundary) {
		return strings.TrimPrefix(value, boundary), true
	}
	return value, false
}

func ps5119AfterString(value, boundary string, suffix bool) (string, bool) {
	if suffix {
		return strings.CutSuffix(value, boundary)
	}
	return strings.CutPrefix(value, boundary)
}

func ps5119BeforeBytes(value, boundary []byte, suffix bool) ([]byte, bool) {
	if suffix {
		if bytes.HasSuffix(value, boundary) {
			return bytes.TrimSuffix(value, boundary), true
		}
		return value, false
	}
	if bytes.HasPrefix(value, boundary) {
		return bytes.TrimPrefix(value, boundary), true
	}
	return value, false
}

func ps5119AfterBytes(value, boundary []byte, suffix bool) ([]byte, bool) {
	if suffix {
		return bytes.CutSuffix(value, boundary)
	}
	return bytes.CutPrefix(value, boundary)
}

func ps5119RandomBytes(random *rand.Rand, length int) string {
	value := make([]byte, length)
	for index := range value {
		value[index] = byte(random.Intn(256))
	}
	return string(value)
}
