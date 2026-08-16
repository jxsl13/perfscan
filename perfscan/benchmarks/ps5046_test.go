package benchmarks

import (
	"regexp"
	"strings"
	"testing"
)

var rePS5046 = regexp.MustCompile("ab+c")
var subjPS5046 = strings.Repeat("abbbc ", 64)
var sinkPS5046 bool

// BenchmarkPS5046Before is re.FindStringIndex(s) != nil: allocates a []int.
func BenchmarkPS5046Before(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkPS5046 = rePS5046.FindStringIndex(subjPS5046) != nil
	}
}

// BenchmarkPS5046After is re.MatchString(s): same match, no allocation.
func BenchmarkPS5046After(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkPS5046 = rePS5046.MatchString(subjPS5046)
	}
}
