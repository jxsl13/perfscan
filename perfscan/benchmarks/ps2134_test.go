package benchmarks

import (
	"io"
	"testing"
	"text/template"
)

// PS2134 — template.Must(template.New(name).Parse(constText)) built inline in a
// function tokenises and compiles the template's parse tree on EVERY call, then
// discards it. Hoisting it to a package-level var parses once at init and reuses
// the shared (read-only, concurrency-safe) template. Before pays the parse per
// call; After only executes.

const ps2134Tmpl = "Hello {{.Name}}, you have {{.Count}} messages{{if .Admin}} (admin){{end}}."

var ps2134Template = template.Must(template.New("t").Parse(ps2134Tmpl))

var ps2134Data = map[string]any{"Name": "Ann", "Count": 3, "Admin": true}

func BenchmarkPS2134_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		t := template.Must(template.New("t").Parse(ps2134Tmpl))
		_ = t.Execute(io.Discard, ps2134Data)
	}
}

func BenchmarkPS2134_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		_ = ps2134Template.Execute(io.Discard, ps2134Data)
	}
}
