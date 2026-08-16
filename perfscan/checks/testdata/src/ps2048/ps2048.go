package ps2048

import (
	"fmt"
	"io"
	"os"
)

// fmt stays imported: this reference keeps the import alive after every
// fix in this file is applied.
func keepFmtAlive(w io.Writer) {
	fmt.Fprintln(w, "kept")
}

func basic(w io.Writer, a, b string) {
	fmt.Fprint(w, a, b) // want `fmt\.Fprint over only plain strings inserts no separators, boxes every operand and walks fmt's reflection printer just to write their concatenation; io\.WriteString\(w, a\+b\+\.\.\.\) writes the identical bytes with the same \(n, err\)`
}

func three(w io.Writer, host, port string) {
	fmt.Fprint(w, host, ":", port) // want `fmt\.Fprint over only plain strings inserts no separators, boxes every operand and walks fmt's reflection printer just to write their concatenation; io\.WriteString\(w, a\+b\+\.\.\.\) writes the identical bytes with the same \(n, err\)`
}

// Both return (n int, err error) from the one underlying write: the
// results carry over verbatim.
func results(w io.Writer, a, b string) (int, error) {
	return fmt.Fprint(w, a, b) // want `fmt\.Fprint over only plain strings inserts no separators, boxes every operand and walks fmt's reflection printer just to write their concatenation; io\.WriteString\(w, a\+b\+\.\.\.\) writes the identical bytes with the same \(n, err\)`
}

// Untyped string constants default to the predeclared string.
func literals(w io.Writer) {
	fmt.Fprint(w, "hello, ", "world") // want `fmt\.Fprint over only plain strings inserts no separators, boxes every operand and walks fmt's reflection printer just to write their concatenation; io\.WriteString\(w, a\+b\+\.\.\.\) writes the identical bytes with the same \(n, err\)`
}

// Operands are kept byte-verbatim: index, slice, call and + operands all
// splice into the chain unchanged (a string-typed operand is a primary
// or a + chain, so no operand ever needs parentheses).
func exprs(w io.Writer, m map[string]string, xs []string, s string, f func() string) {
	fmt.Fprint(w, m["k"], xs[0], s[1:], f(), s+"!") // want `fmt\.Fprint over only plain strings inserts no separators, boxes every operand and walks fmt's reflection printer just to write their concatenation; io\.WriteString\(w, a\+b\+\.\.\.\) writes the identical bytes with the same \(n, err\)`
}

// The writer expression stays verbatim too.
type holder struct{ w io.Writer }

func (h holder) field(a, b string) {
	fmt.Fprint(h.w, a, b) // want `fmt\.Fprint over only plain strings inserts no separators, boxes every operand and walks fmt's reflection printer just to write their concatenation; io\.WriteString\(w, a\+b\+\.\.\.\) writes the identical bytes with the same \(n, err\)`
}

// --- advisory only: reported, but never rewritten ---

// A local named io shadows the package at the call site: the emitted
// io.WriteString qualifier would not resolve — no fix.
func shadowedIo(w io.Writer, a, b string) {
	io := 1
	_ = io
	fmt.Fprint(w, a, b) // want `fmt\.Fprint over only plain strings inserts no separators, boxes every operand and walks fmt's reflection printer just to write their concatenation; io\.WriteString\(w, a\+b\+\.\.\.\) writes the identical bytes with the same \(n, err\)`
}

// A comment inside the rewritten scaffolding would be swallowed — no fix.
func commented(w io.Writer, a, b string) {
	fmt.Fprint(w, a /* keep me */, b) // want `fmt\.Fprint over only plain strings inserts no separators, boxes every operand and walks fmt's reflection printer just to write their concatenation; io\.WriteString\(w, a\+b\+\.\.\.\) writes the identical bytes with the same \(n, err\)`
}

// --- guards: none of the following may be reported ---

type myStr string

type stamp struct{}

func (stamp) String() string { return "stamp" }

// The single-operand form is PS2129's territory, not PS2048's.
func oneOperand(w io.Writer, s string) {
	fmt.Fprint(w, s)
}

// One non-string operand re-engages Fprint's spacing rule against its
// neighbors AND formats through fmt — never matched, wherever it sits.
func mixed(w io.Writer, a string, n int, err error, ok bool) {
	fmt.Fprint(w, a, n)
	fmt.Fprint(w, n, a)
	fmt.Fprint(w, a, err, a)
	fmt.Fprint(w, a, ok)
	fmt.Fprint(w, a, nil)
}

// A NAMED string type could implement fmt.Stringer/fmt.Formatter, which
// Fprint's %v formatting honors and + would not; []byte prints its bytes
// via fmt but would not compile in a + chain of strings anyway; a
// Stringer prints String(). None match — bit-identity needs the EXACT
// predeclared string.
func notPlain(w io.Writer, a string, m myStr, st stamp, b []byte) {
	fmt.Fprint(w, a, m)
	fmt.Fprint(w, a, st)
	fmt.Fprint(w, a, b)
}

// No variadic spread: the operands (and their types) are unknown here.
func spread(w io.Writer, args []any) {
	fmt.Fprint(w, args...)
}

// Fprintln is ALWAYS space-separated and appends a newline; Fprintf
// formats. Neither matches.
func otherFuncs(w io.Writer, a, b string) {
	fmt.Fprintln(w, a, b)
	fmt.Fprintf(w, "%s%s", a, b)
}

// A local object named fmt shadows the package: not stdlib fmt.Fprint.
type fakeFmt struct{}

func (fakeFmt) Fprint(w io.Writer, a, b string) (int, error) { return 0, nil }

func shadowedFmt(w io.Writer, a, b string) {
	var fmt fakeFmt
	fmt.Fprint(w, a, b)
}

// A WriteString that delegates through fmt: the rewrite
// io.WriteString(f, a+b) would dispatch to f.WriteString — the enclosing
// method itself: unbounded recursion that still compiles. Nothing is
// reported.
type selfF struct{ b []byte }

func (f *selfF) Write(p []byte) (int, error) {
	f.b = append(f.b, p...)
	return len(p), nil
}

func (f *selfF) WriteString(s string) (int, error) {
	return fmt.Fprint(f, s, "!")
}

// A writer WITHOUT WriteString: io.WriteString(g, ...) would fall back
// to g.Write — inside Write that is the enclosing method itself:
// recursion. Nothing is reported.
type selfF2 struct{ b []byte }

func (g *selfF2) Write(p []byte) (int, error) {
	return fmt.Fprint(g, string(p), "!")
}

// The same call in any OTHER method of the receiver, or on a different
// writer, is still reported.
func (f *selfF) dump(a, b string) {
	fmt.Fprint(f, a, b) // want `fmt\.Fprint over only plain strings inserts no separators, boxes every operand and walks fmt's reflection printer just to write their concatenation; io\.WriteString\(w, a\+b\+\.\.\.\) writes the identical bytes with the same \(n, err\)`
}

func (f *selfF) mirror(a, b string) (int, error) {
	return fmt.Fprint(os.Stdout, a, b) // want `fmt\.Fprint over only plain strings inserts no separators, boxes every operand and walks fmt's reflection printer just to write their concatenation; io\.WriteString\(w, a\+b\+\.\.\.\) writes the identical bytes with the same \(n, err\)`
}
