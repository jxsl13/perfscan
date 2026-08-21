package ps2113

import (
	"bytes"
	"fmt"
	stdfmt "fmt"
	"io"
	"strings"
)

func basic(w io.Writer, name string, id int) {
	w.Write([]byte(fmt.Sprintf("user=%s id=%d", name, id))) // want `Write\(\[\]byte\(fmt\.Sprintf\(\.\.\.\)\)\) builds a throwaway string and \[\]byte; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

func sprint(w io.Writer, a, b string) {
	w.Write([]byte(fmt.Sprint(a, " ", b))) // want `Write\(\[\]byte\(fmt\.Sprint\(\.\.\.\)\)\) builds a throwaway string and \[\]byte; fmt\.Fprint\(w, \.\.\.\) writes the formatted bytes directly to w`
}

func sprintln(w io.Writer, v int) {
	w.Write([]byte(fmt.Sprintln("v:", v))) // want `Write\(\[\]byte\(fmt\.Sprintln\(\.\.\.\)\)\) builds a throwaway string and \[\]byte; fmt\.Fprintln\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// Fprintf returns exactly what the single w.Write call returned, so the
// results carry over.
func results(w io.Writer, v int) (int, error) {
	return w.Write([]byte(fmt.Sprintf("%d", v))) // want `Write\(\[\]byte\(fmt\.Sprintf\(\.\.\.\)\)\) builds a throwaway string and \[\]byte; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// An aliased fmt import keeps its qualifier.
func aliased(w io.Writer, v int) {
	w.Write([]byte(stdfmt.Sprintf("%d", v))) // want `Write\(\[\]byte\(stdfmt\.Sprintf\(\.\.\.\)\)\) builds a throwaway string and \[\]byte; stdfmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// A spread argument list is kept verbatim.
func spread(w io.Writer, format string, args []any) {
	w.Write([]byte(fmt.Sprintf(format, args...))) // want `Write\(\[\]byte\(fmt\.Sprintf\(\.\.\.\)\)\) builds a throwaway string and \[\]byte; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// A concrete *bytes.Buffer is assignable to io.Writer.
func pointerBuffer(buf *bytes.Buffer, v int) {
	buf.Write([]byte(fmt.Sprintf("%d", v))) // want `Write\(\[\]byte\(fmt\.Sprintf\(\.\.\.\)\)\) builds a throwaway string and \[\]byte; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// Parentheses around the fmt call are absorbed by the range edits.
func parenthesized(w io.Writer, v int) {
	w.Write([]byte((fmt.Sprintf("%d", v)))) // want `Write\(\[\]byte\(fmt\.Sprintf\(\.\.\.\)\)\) builds a throwaway string and \[\]byte; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// fmt.Sprint() with no operands: the writer becomes the only argument.
func emptySprint(w io.Writer) {
	w.Write([]byte(fmt.Sprint())) // want `Write\(\[\]byte\(fmt\.Sprint\(\.\.\.\)\)\) builds a throwaway string and \[\]byte; fmt\.Fprint\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// --- advisory only: reported, but never rewritten ---

// bytes.Buffer's Write has a pointer receiver: buf.Write compiles via
// auto-address, but the VALUE buf is not assignable to io.Writer, so
// fmt.Fprintf(buf, ...) would not compile — no fix.
func valueBuffer(v int) string {
	var buf bytes.Buffer
	buf.Write([]byte(fmt.Sprintf("%d", v))) // want `Write\(\[\]byte\(fmt\.Sprintf\(\.\.\.\)\)\) builds a throwaway string and \[\]byte; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
	return buf.String()
}

// A comment inside the rewritten scaffolding would be swallowed — no fix.
func commented(w io.Writer, v int) {
	w.Write( /* keep me */ []byte(fmt.Sprintf("%d", v))) // want `Write\(\[\]byte\(fmt\.Sprintf\(\.\.\.\)\)\) builds a throwaway string and \[\]byte; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// --- guards: none of the following may be reported ---

// A plain []byte payload or literal is not a formatting call.
func plain(w io.Writer, p []byte) {
	w.Write(p)
	w.Write([]byte("hi"))
}

// A non-fmt string source stays: only fmt.Sprintf/Sprint/Sprintln match.
func notFmt(w io.Writer, s string) {
	w.Write([]byte(strings.ToUpper(s)))
}

// A NAMED byte-slice conversion is not the predeclared []byte conversion.
type payload []byte

func namedConv(w io.Writer, v int) {
	w.Write(payload(fmt.Sprintf("%d", v)))
}

// A Write method with a different signature is not io.Writer's Write.
type countWriter struct{}

func (countWriter) Write(p []byte) int { return len(p) }

func oneResult(v int) int {
	var cw countWriter
	return cw.Write([]byte(fmt.Sprintf("%d", v)))
}

// A local object named fmt shadows the package: not stdlib fmt.Sprintf.
type fakeFmt struct{}

func (fakeFmt) Sprintf(format string, args ...any) string { return format }

func shadowed(w io.Writer, v int) {
	var fmt fakeFmt
	w.Write([]byte(fmt.Sprintf("%d", v)))
}

// fmt.Sprintf used for anything but an immediate []byte-convert-and-write
// is out of scope.
func kept(w io.Writer, v int) string {
	s := fmt.Sprintf("%d", v)
	w.Write([]byte(s))
	return s
}

// Inside the receiver's own Write method the rewrite fmt.Fprintf(g, ...)
// would write through g.Write — the enclosing method itself: unbounded
// recursion that still compiles. Nothing is reported.
type selfG struct{ b []byte }

func (g *selfG) Write(p []byte) (int, error) {
	return g.Write([]byte(fmt.Sprintf("x=%d", len(p))))
}

// The same call in any OTHER method of the receiver is still reported:
// the rewrite dispatches to Write, not to the enclosing method.
func (g *selfG) dump(v int) {
	g.Write([]byte(fmt.Sprintf("%d", v))) // want `Write\(\[\]byte\(fmt\.Sprintf\(\.\.\.\)\)\) builds a throwaway string and \[\]byte; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}

// A DIFFERENT writer inside Write is still reported: w is not the
// receiver.
func (g *selfG) mirror(w io.Writer, p []byte) (int, error) {
	return w.Write([]byte(fmt.Sprintf("%x", p))) // want `Write\(\[\]byte\(fmt\.Sprintf\(\.\.\.\)\)\) builds a throwaway string and \[\]byte; fmt\.Fprintf\(w, \.\.\.\) writes the formatted bytes directly to w`
}
