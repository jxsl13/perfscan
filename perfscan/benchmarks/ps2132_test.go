package benchmarks

import (
	"strings"
	"testing"
)

// PS2132 — a strings.NewReplacer built inline with constant pairs analyses the
// pairs and builds a fresh lookup structure on EVERY call, then discards it.
// Hoisting it to a package-level var builds that structure once at init and
// reuses the shared (immutable, concurrency-safe) replacer. Before pays the
// build + allocation per call; After does not.

var ps2132Replacer = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;",
)

const ps2132Input = "a <b> & \"c\" 'd' — plain text with a few specials"

func BenchmarkPS2132_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.NewReplacer(
			"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;",
		).Replace(ps2132Input)
	}
}

func BenchmarkPS2132_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = ps2132Replacer.Replace(ps2132Input)
	}
}
