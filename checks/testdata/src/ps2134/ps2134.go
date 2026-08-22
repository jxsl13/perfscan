package ps2134

import (
	htmltmpl "html/template"
	"io"
	"text/template"
)

const pageTmpl = "Hello {{.Name}}"

// Inline text/template New().Parse(const) inside a function: flagged.
func render(w io.Writer, data any) {
	t := template.Must(template.New("page").Parse(pageTmpl)) // want `text/template: New\(\.\.\.\)\.Parse of a constant template inline re-parses it on every call`
	_ = t.Execute(w, data)
}

// Inline html/template New().Parse(const) inside a function: flagged (html pkg).
func renderHTML(w io.Writer, data any) {
	t := htmltmpl.Must(htmltmpl.New("page").Parse(pageTmpl)) // want `html/template: New\(\.\.\.\)\.Parse of a constant template inline re-parses it on every call`
	_ = t.Execute(w, data)
}

// ADVISORY (no fix): the template is mutated after creation (t.Parse adds to it),
// so a shared package instance is NOT equivalent — reported, not auto-hoisted.
func mutated(w io.Writer, data any) {
	t := template.Must(template.New("m").Parse(pageTmpl)) // want `text/template: New\(\.\.\.\)\.Parse of a constant template inline re-parses it on every call`
	_, _ = t.Parse("{{define \"x\"}}x{{end}}")
	_ = t.Execute(w, data)
}

// ADVISORY (no fix): the template escapes (passed to another function), where it
// could be mutated through the shared instance — reported, not auto-hoisted.
func escapes(w io.Writer, data any) {
	t := template.Must(template.New("e").Parse(pageTmpl)) // want `text/template: New\(\.\.\.\)\.Parse of a constant template inline re-parses it on every call`
	useTmpl(t)
	_ = t.Execute(w, data)
}

func useTmpl(*template.Template) {}

// ADVISORY (no fix): no template.Must wrapper — the (*Template, error) tuple has
// no single value to hoist, so this stays advisory.
func noMust(w io.Writer, data any) {
	t, err := template.New("n").Parse(pageTmpl) // want `text/template: New\(\.\.\.\)\.Parse of a constant template inline re-parses it on every call`
	if err != nil {
		return
	}
	_ = t.Execute(w, data)
}

// NEGATIVE: package-level var initializer parses once at init: out of scope.
var pageTemplate = template.Must(template.New("page").Parse(pageTmpl))

func good(w io.Writer, data any) { _ = pageTemplate.Execute(w, data) }

// NEGATIVE: a runtime (variable) template string genuinely varies: silent.
func dynamic(w io.Writer, src string, data any) {
	t := template.Must(template.New("x").Parse(src))
	_ = t.Execute(w, data)
}

// NEGATIVE: Parse on an existing (non-New) template — adding to a base, not the
// inline create-and-parse chain: silent.
var base = template.New("base")

func addTo(data any) {
	_, _ = base.Parse(pageTmpl)
}
