package benchmarks

import (
	"regexp"
	"strings"
	"testing"
)

var rePS2051 = regexp.MustCompile("ab+c")
var subjPS2051 = strings.Repeat("abbbx ", 512)
var sinkPS2051 bool

// BenchmarkPS2051Before is re.Match([]byte(s)): a full copy of s before matching.
func BenchmarkPS2051Before(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkPS2051 = rePS2051.Match([]byte(subjPS2051))
	}
}

// BenchmarkPS2051After is re.MatchString(s): scans s in place, no copy.
func BenchmarkPS2051After(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkPS2051 = rePS2051.MatchString(subjPS2051)
	}
}
