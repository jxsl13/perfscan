package checks

// Runtime differential for PS2049: fmt.Fprintln(w, a, b, ...) with two
// or more plain-string operands vs the io.WriteString(w, a+" "+b+...+"\n")
// form the fix emits. The safety argument is that doPrintln is
// UNCONDITIONAL — it writes exactly one ' ' between every adjacent pair
// of operands and exactly one trailing '\n', with no format
// interpretation — and the default %v formatting of a PLAIN string is
// the verbatim bytes, so fmt assembles byte-for-byte a + " " + b + ...
// + "\n" in its pooled buffer and performs ONE w.Write of it, returning
// that Write's (n, err), while io.WriteString performs the same single
// write of the same bytes (via w.WriteString when w is an
// io.StringWriter, whose contract requires writing the same bytes as
// Write([]byte(s))) and forwards the same (n, err). This suite pins:
//
//   - byte-identity of the written stream AND the (n, err) pair over
//     every pair and triple drawn from adversarial operands — empty
//     (empty operands still get their separators), newline-bearing,
//     long (past fmt's small-buffer path), multi-byte UTF-8, NUL
//     bytes, invalid UTF-8, escape-heavy, and '%'-bearing strings
//     (Fprintln never interprets verbs, so '%' is data on both
//     sides) — for a StringWriter (bytes.Buffer / strings.Builder), a
//     Write-only writer (no WriteString method, exercising the []byte
//     fallback), and an erroring writer that truncates and fails
//     mid-write, where both sides must surface the identical (n, err);
//   - WHY the all-plain-strings guard is load-bearing: a named string
//     type's String()/Format() methods are honored by Fprintln's %v
//     formatting and ignored by +;
//   - WHY the space join is exactly one ' ' per adjacent pair: a
//     space-free join MUST diverge, otherwise the join text would be
//     wrong (the same divergence PS5038 pins from the other side).

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// ps2049WriteOnly wraps a bytes.Buffer exposing ONLY Write, so
// io.WriteString must take its w.Write([]byte(s)) fallback — the same
// method fmt.Fprintln calls.
type ps2049WriteOnly struct{ buf bytes.Buffer }

func (w *ps2049WriteOnly) Write(p []byte) (int, error) { return w.buf.Write(p) }

// ps2049ErrWriter accepts at most limit bytes in total, then fails: a
// contract-conforming Write that returns a short count with an error.
// Both sides call Write exactly once with all the bytes (the joined
// operands plus the trailing '\n'), so both must see the identical
// truncated n and the identical error value.
type ps2049ErrWriter struct {
	buf   bytes.Buffer
	limit int
}

var errPS2049Full = errors.New("ps2049: writer full")

func (w *ps2049ErrWriter) Write(p []byte) (int, error) {
	room := w.limit - w.buf.Len()
	if room < 0 {
		room = 0
	}
	if len(p) <= room {
		return w.buf.Write(p)
	}
	n, _ := w.buf.Write(p[:room])
	return n, errPS2049Full
}

// ps2049Inputs are adversarial operands — every shape the check may
// rewrite: empty (a plain string is never nil; empty operands still get
// their separators), tiny, newline-bearing (Fprintln appends its own
// '\n' anyway), long (past any small-buffer path), multi-byte UTF-8,
// NUL bytes, invalid UTF-8, escape-heavy runs, and '%'-bearing
// strings — Fprintln never interprets verbs, so unlike the Fprintf
// checks NO '%' guard is needed and identity must HOLD for them.
var ps2049Inputs = []string{
	"",
	"a",
	"host port",
	"already\nsplit\n",
	strings.Repeat("0123456789abcdef", 65), // 1040 bytes, past pp.buf's initial cap
	"日本語テキスト",
	"a\x00b\x00c",
	"\xff\xfe invalid utf8 \x80",
	"tab\tcr\rlf\nbell\a backslash \\ quote \" done",
	"100%% and %d and a trailing %",
	"%v %s %q",
}

// ps2049Join is the reference spelling of the rewrite's + chain:
// operands joined by single spaces plus one trailing newline.
func ps2049Join(ops []string) string {
	joined := ops[0]
	for _, s := range ops[1:] {
		joined += " " + s
	}
	return joined + "\n"
}

// ps2049Tuples yields every pair and triple over ps2049Inputs plus a
// couple of wide tuples — the arities the check rewrites (the
// single-operand form is PS5038's).
func ps2049Tuples() [][]string {
	var tuples [][]string
	for _, a := range ps2049Inputs {
		for _, b := range ps2049Inputs {
			tuples = append(tuples, []string{a, b})
		}
	}
	for i, a := range ps2049Inputs {
		b := ps2049Inputs[(i+3)%len(ps2049Inputs)]
		c := ps2049Inputs[(i+7)%len(ps2049Inputs)]
		tuples = append(tuples, []string{a, b, c})
	}
	tuples = append(tuples, ps2049Inputs, []string{"", "", "", ""})
	return tuples
}

// ps2049Box boxes a tuple for Fprintln's variadic ...any.
func ps2049Box(ops []string) []any {
	args := make([]any, len(ops))
	for i, s := range ops {
		args[i] = s
	}
	return args
}

// TestEquivPS2049_Identity pins that fmt.Fprintln(w, a, b, ...) and
// io.WriteString(w, a+" "+b+...+"\n") write the identical byte stream
// and return the identical (n, err) for every tuple, across the
// StringWriter path, the Write-only fallback path, and a
// strings.Builder.
func TestEquivPS2049_Identity(t *testing.T) {
	for _, tp := range ps2049Tuples() {
		args := ps2049Box(tp)
		joined := ps2049Join(tp)

		// StringWriter path: io.WriteString dispatches to WriteString.
		var a, b bytes.Buffer
		nA, eA := fmt.Fprintln(&a, args...)
		nB, eB := io.WriteString(&b, joined)
		if a.String() != b.String() || nA != nB || (eA == nil) != (eB == nil) {
			t.Errorf("bytes.Buffer: Fprintln(%q) vs io.WriteString: bytes %q/%q n %d/%d err %v/%v", tp, a.String(), b.String(), nA, nB, eA, eB)
		}
		if nB != len(joined) || eB != nil {
			t.Errorf("bytes.Buffer: io.WriteString(%q) = (%d, %v), want (%d, nil)", joined, nB, eB, len(joined))
		}

		// Write-only path: io.WriteString falls back to Write([]byte(s)) —
		// the very method fmt calls.
		var wo1, wo2 ps2049WriteOnly
		nC, eC := fmt.Fprintln(&wo1, args...)
		nD, eD := io.WriteString(&wo2, joined)
		if wo1.buf.String() != wo2.buf.String() || nC != nD || (eC == nil) != (eD == nil) {
			t.Errorf("write-only: Fprintln(%q) vs io.WriteString: bytes %q/%q n %d/%d err %v/%v", tp, wo1.buf.String(), wo2.buf.String(), nC, nD, eC, eD)
		}

		// strings.Builder is a second StringWriter with a different
		// WriteString implementation.
		var sb1, sb2 strings.Builder
		nE, eE := fmt.Fprintln(&sb1, args...)
		nF, eF := io.WriteString(&sb2, joined)
		if sb1.String() != sb2.String() || nE != nF || (eE == nil) != (eF == nil) {
			t.Errorf("strings.Builder: Fprintln(%q) vs io.WriteString: bytes %q/%q n %d/%d", tp, sb1.String(), sb2.String(), nE, nF)
		}
	}
}

// TestEquivPS2049_Separators pins the exact separator rule the join
// relies on: one ' ' between EVERY adjacent pair — empty operands
// included — and one trailing '\n', and that a space-free join MUST
// diverge (otherwise the emitted join text would be wrong).
func TestEquivPS2049_Separators(t *testing.T) {
	var a bytes.Buffer
	_, _ = fmt.Fprintln(&a, "", "")
	if want := " \n"; a.String() != want {
		t.Errorf("Fprintln(w, \"\", \"\") = %q, want %q — empty operands must keep their separator", a.String(), want)
	}

	var c, d bytes.Buffer
	_, _ = fmt.Fprintln(&c, "host", "port")
	_, _ = io.WriteString(&d, "host"+"port"+"\n")
	if c.String() == d.String() {
		t.Errorf("Fprintln with two operands matched the SPACE-FREE join (%q) — the +\" \"+ separator would be wrong", c.String())
	}
	if want := "host port\n"; c.String() != want {
		t.Errorf("Fprintln(\"host\", \"port\") = %q, want %q", c.String(), want)
	}
}

// TestEquivPS2049_ErrorWriter pins that a failing writer surfaces the
// IDENTICAL truncated n, error value and partial byte stream through
// both forms: each performs exactly one Write call carrying all the
// bytes (the joined operands plus the trailing '\n').
func TestEquivPS2049_ErrorWriter(t *testing.T) {
	for _, tp := range ps2049Tuples() {
		args := ps2049Box(tp)
		joined := ps2049Join(tp)
		for _, limit := range []int{0, 1, 3, len(joined) - 1, len(joined), len(joined) + 4} {
			if limit < 0 {
				continue
			}
			w1 := &ps2049ErrWriter{limit: limit}
			w2 := &ps2049ErrWriter{limit: limit}
			n1, e1 := fmt.Fprintln(w1, args...)
			n2, e2 := io.WriteString(w2, joined)
			if n1 != n2 || !errors.Is(e1, e2) || (e1 == nil) != (e2 == nil) {
				t.Errorf("limit %d: Fprintln(%q) = (%d, %v), io.WriteString = (%d, %v) — must be identical", limit, tp, n1, e1, n2, e2)
			}
			if w1.buf.String() != w2.buf.String() {
				t.Errorf("limit %d: partial streams diverge: %q vs %q", limit, w1.buf.String(), w2.buf.String())
			}
		}
	}
}

// ps2049Loud is a NAMED string type whose String() method reports
// something other than its underlying bytes.
type ps2049Loud string

func (ps2049Loud) String() string { return "SURPRISE" }

// ps2049Fmt is a NAMED string type implementing fmt.Formatter.
type ps2049Fmt string

func (ps2049Fmt) Format(f fmt.State, verb rune) { io.WriteString(f, "FORMATTED") }

// TestEquivPS2049_NamedTypeDiverges pins WHY the all-plain-strings
// guard is load-bearing: Fprintln's %v formatting honors a named string
// type's String()/Format() methods, while + on the underlying string
// would emit the raw bytes — the outputs MUST differ even when only ONE
// operand is named.
func TestEquivPS2049_NamedTypeDiverges(t *testing.T) {
	var a, b bytes.Buffer
	_, _ = fmt.Fprintln(&a, "id:", ps2049Loud("quiet"))
	_, _ = io.WriteString(&b, "id:"+" "+string(ps2049Loud("quiet"))+"\n")
	if a.String() == b.String() {
		t.Errorf("Fprintln of a Stringer-bearing named string matched the raw-bytes join (%q) — the plain-string guard would be needlessly conservative", a.String())
	}
	if want := "id: SURPRISE\n"; a.String() != want {
		t.Errorf("Fprintln(\"id:\", ps2049Loud) = %q, want %q", a.String(), want)
	}

	var c, d bytes.Buffer
	_, _ = fmt.Fprintln(&c, ps2049Fmt("plain"), "tail")
	_, _ = io.WriteString(&d, string(ps2049Fmt("plain"))+" "+"tail"+"\n")
	if c.String() == d.String() {
		t.Errorf("Fprintln of a Formatter-bearing named string matched the raw-bytes join (%q)", c.String())
	}
	if want := "FORMATTED tail\n"; c.String() != want {
		t.Errorf("Fprintln(ps2049Fmt, \"tail\") = %q, want %q", c.String(), want)
	}
}
