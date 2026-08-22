package benchmarks

import (
	"strings"
	"testing"
)

const ps5078Cutset = "\t\n\v\f\r \u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000"

var (
	ps5078Input = "payload"
	ps5078Sink  string
)

func BenchmarkPS5078_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5078Sink = strings.TrimSpace(strings.Trim(ps5078Input, ps5078Cutset))
	}
}

func BenchmarkPS5078_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5078Sink = strings.TrimSpace(ps5078Input)
	}
}
