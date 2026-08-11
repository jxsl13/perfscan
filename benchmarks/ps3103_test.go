package benchmarks

import "testing"

// PS3103 — ranging by value over large elements vs iterating by index and
// reading through a pointer. The After is the documented MANUAL remedy
// (the check is advisory; the alias change is not auto-applied).

// ps3103Row is 2056 bytes: well above the 128-byte reporting threshold.
type ps3103Row struct {
	Payload [2048]byte
	Sum     int64
}

var ps3103Rows = func() []ps3103Row {
	rows := make([]ps3103Row, 256)
	for i := range rows {
		rows[i].Sum = int64(i)
		rows[i].Payload[0] = byte(i)
		rows[i].Payload[2047] = byte(i * 3)
	}
	return rows
}()

func BenchmarkPS3103_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		total := 0
		for _, v := range ps3103Rows { // copies 2056 bytes per iteration
			total += int(v.Sum) + int(v.Payload[0]) + int(v.Payload[2047])
		}
		sinkI = total
	}
}

func BenchmarkPS3103_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		total := 0
		for i := range ps3103Rows {
			v := &ps3103Rows[i] // aliases the element; reads only
			total += int(v.Sum) + int(v.Payload[0]) + int(v.Payload[2047])
		}
		sinkI = total
	}
}
