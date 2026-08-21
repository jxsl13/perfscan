package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

var (
	ps2004ForeignView []byte
	ps2004Result      []string
	ps2004Values      = []string{
		"a long profile event label",
		"x",
		"",
		strings.Repeat("q", 120),
		"medium",
		"prefix\x00ignored",
	}
)

// ps2004ForeignFill uses a transient global view to model cgo escape
// conservatism: the pointer is not retained after the call, but escape
// analysis must place the passed backing array on the heap, as at a cgo ABI.
// The body fully defines all 96 bytes and preserves strncpy-style NUL behavior.
//
//go:noinline
func ps2004ForeignFill(dst []byte, value string) {
	ps2004ForeignView = dst
	clear(dst)
	if nul := strings.IndexByte(value, 0); nul >= 0 {
		value = value[:nul]
	}
	if len(value) >= len(dst) {
		value = value[:len(dst)-1]
	}
	copy(dst, value)
	dst[len(dst)-1] = 0
	ps2004ForeignView = nil
}

func ps2004Decode(dst []byte) string {
	end := bytes.IndexByte(dst, 0)
	return string(dst[:end])
}

func ps2004RecordsBefore(events int) []string {
	out := make([]string, 0, events)
	for i := range events {
		label := make([]byte, 96)
		ps2004ForeignFill(label, ps2004Values[i%len(ps2004Values)])
		out = append(out, ps2004Decode(label))
	}
	return out
}

func ps2004RecordsAfter(events int) []string {
	out := make([]string, 0, events)
	label := make([]byte, 96)
	for i := range events {
		ps2004ForeignFill(label, ps2004Values[i%len(ps2004Values)])
		out = append(out, ps2004Decode(label))
	}
	return out
}

func BenchmarkPS2004Cgo_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2004Result = ps2004RecordsBefore(128)
	}
}

func BenchmarkPS2004Cgo_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2004Result = ps2004RecordsAfter(128)
	}
}
