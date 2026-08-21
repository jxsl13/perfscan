package benchmarks

import (
	"strings"
	"testing"
)

const ps5077Cutset = "αβγδεζηθικλμνξοπρστυφχψωΑΒΓΔΕΖΗΘΙΚΛΜΝΞΟΠΡΣΤΥΦΧΨΩ"

var (
	ps5077Input = "payload"
	ps5077Sink  string
)

func BenchmarkPS5077_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5077Sink = strings.Trim(strings.Trim(ps5077Input, ps5077Cutset), ps5077Cutset)
	}
}

func BenchmarkPS5077_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5077Sink = strings.Trim(ps5077Input, ps5077Cutset)
	}
}
