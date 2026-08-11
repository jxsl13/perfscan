package ps2120

import (
	"bytes"
	"fmt"
	stdfmt "fmt"
	"io"
	"strings"
)

func basic(sb *strings.Builder, name string, id int) {
	sb.WriteString(fmt.Sprintf("user=%s id=%d", name, id)) // want `WriteString\(fmt\.Sprintf\(\.\.\.\)\) builds a throwaway string; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

func sprint(sb *strings.Builder, a, b string) {
	sb.WriteString(fmt.Sprint(a, " ", b)) // want `WriteString\(fmt\.Sprint\(\.\.\.\)\) builds a throwaway string; fmt\.Fprint\(w, \.\.\.\) writes the formatted bytes directly to w`
}

func sprintln(sb *strings.Builder, v int) {
	sb.WriteString(fmt.Sprintln("v:", v)) // want `WriteString\(fmt\.Sprintln\(\.\.\.\)\) builds a throwaway string; fmt\.Fprintln\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// Fprintf returns (n, err) just as WriteString did, so the results carry
// over.
func results(buf *bytes.Buffer, v int) (int, error) {
	return buf.WriteString(fmt.Sprintf("%d", v)) // want `WriteString\(fmt\.Sprintf\(\.\.\.\)\) builds a throwaway string; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// An aliased fmt import keeps its qualifier.
func aliased(sb *strings.Builder, v int) {
	sb.WriteString(stdfmt.Sprintf("%d", v)) // want `WriteString\(stdfmt\.Sprintf\(\.\.\.\)\) builds a throwaway string; stdfmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// A spread argument list is kept verbatim.
func spread(sb *strings.Builder, format string, args []any) {
	sb.WriteString(fmt.Sprintf(format, args...)) // want `WriteString\(fmt\.Sprintf\(\.\.\.\)\) builds a throwaway string; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// A concrete *bytes.Buffer is assignable to io.Writer.
func pointerBuffer(buf *bytes.Buffer, v int) {
	buf.WriteString(fmt.Sprintf("%d", v)) // want `WriteString\(fmt\.Sprintf\(\.\.\.\)\) builds a throwaway string; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// An interface receiver that bundles Write and WriteString is assignable
// to io.Writer.
type stringWriter interface {
	io.Writer
	io.StringWriter
}

func ifaceWriter(w stringWriter, v int) {
	w.WriteString(fmt.Sprintf("%d", v)) // want `WriteString\(fmt\.Sprintf\(\.\.\.\)\) builds a throwaway string; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// Parentheses around the fmt call are absorbed by the range edits.
func parenthesized(sb *strings.Builder, v int) {
	sb.WriteString((fmt.Sprintf("%d", v))) // want `WriteString\(fmt\.Sprintf\(\.\.\.\)\) builds a throwaway string; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// fmt.Sprint() with no operands: the writer becomes the only argument.
func emptySprint(sb *strings.Builder) {
	sb.WriteString(fmt.Sprint()) // want `WriteString\(fmt\.Sprint\(\.\.\.\)\) builds a throwaway string; fmt\.Fprint\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// --- advisory only: reported, but never rewritten ---

// strings.Builder's methods have pointer receivers: sb.WriteString
// compiles via auto-address, but the VALUE sb is not assignable to
// io.Writer, so fmt.Fprintf(sb, ...) would not compile — no fix.
func valueBuilder(v int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d", v)) // want `WriteString\(fmt\.Sprintf\(\.\.\.\)\) builds a throwaway string; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
	return sb.String()
}

// A comment inside the rewritten scaffolding would be swallowed — no fix.
func commented(sb *strings.Builder, v int) {
	sb.WriteString( /* keep me */ fmt.Sprintf("%d", v)) // want `WriteString\(fmt\.Sprintf\(\.\.\.\)\) builds a throwaway string; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// --- guards: none of the following may be reported ---

// A plain string payload or literal is not a formatting call.
func plain(sb *strings.Builder, s string) {
	sb.WriteString(s)
	sb.WriteString("hi")
}

// A non-fmt string source stays: only fmt.Sprintf/Sprint/Sprintln match.
func notFmt(sb *strings.Builder, s string) {
	sb.WriteString(strings.ToUpper(s))
}

// A type with WriteString but no Write method cannot be handed to
// fmt.Fprintf in any form — the recommendation itself would be wrong,
// so nothing is reported.
type onlyString struct{}

func (onlyString) WriteString(s string) (n int, err error) { return len(s), nil }

func noWrite(v int) {
	var os onlyString
	os.WriteString(fmt.Sprintf("%d", v))
}

// io.StringWriter promises WriteString only; the dynamic value may lack
// Write entirely, so fmt.Fprintf cannot be offered.
func stringWriterOnly(w io.StringWriter, v int) {
	w.WriteString(fmt.Sprintf("%d", v))
}

// io.WriteString is a package FUNCTION, not a WriteString method.
func packageFunc(w io.Writer, v int) {
	io.WriteString(w, fmt.Sprintf("%d", v))
}

// A WriteString method with a different signature is not
// io.StringWriter's WriteString.
type countWriter struct{}

func (countWriter) Write(p []byte) (int, error) { return len(p), nil }
func (countWriter) WriteString(s string) int    { return len(s) }

func oneResult(v int) int {
	var cw countWriter
	return cw.WriteString(fmt.Sprintf("%d", v))
}

// A local object named fmt shadows the package: not stdlib fmt.Sprintf.
type fakeFmt struct{}

func (fakeFmt) Sprintf(format string, args ...any) string { return format }

func shadowed(sb *strings.Builder, v int) {
	var fmt fakeFmt
	sb.WriteString(fmt.Sprintf("%d", v))
}

// fmt.Sprintf used for anything but an immediate WriteString is out of
// scope.
func kept(sb *strings.Builder, v int) string {
	s := fmt.Sprintf("%d", v)
	sb.WriteString(s)
	return s
}
