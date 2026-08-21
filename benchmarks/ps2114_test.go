package benchmarks

import (
	"sync"
	"testing"
)

// PS2114 — sync.Pool storing a []byte by value boxes the 24-byte slice
// header into an interface on every Put (and in New); pooling a *[]byte
// keeps the round trip allocation-free.
var (
	ps2114ValuePool = sync.Pool{
		New: func() any { return make([]byte, 0, 256) },
	}
	ps2114PtrPool = sync.Pool{
		New: func() any {
			b := make([]byte, 0, 256)
			return &b
		},
	}
)

func BenchmarkPS2114_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		buf := ps2114ValuePool.Get().([]byte)
		buf = append(buf[:0], words[0]...)
		sinkI = len(buf)
		//lint:ignore SA6002 the Before side deliberately exhibits the boxing PS2114 reports
		ps2114ValuePool.Put(buf)
	}
}

func BenchmarkPS2114_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		p := ps2114PtrPool.Get().(*[]byte)
		*p = append((*p)[:0], words[0]...)
		sinkI = len(*p)
		ps2114PtrPool.Put(p)
	}
}
