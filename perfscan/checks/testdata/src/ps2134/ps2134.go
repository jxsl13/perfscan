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
