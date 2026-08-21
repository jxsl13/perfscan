package checks

// Runtime differential for PS5038: fmt.Fprintln(w, s) with a single
// plain-string operand vs the io.WriteString(w, s+"\n") form the fix
// emits. The safety argument is that with exactly ONE operand doPrintln
// applies no between-operand spaces and appends exactly one trailing
// '\n', and the default %v formatting of a PLAIN string is the verbatim
// bytes — so fmt assembles byte-for-byte s + "\n" in its pooled buffer
// and performs ONE w.Write of it, returning that Write's (n, err),
// while io.WriteString performs the same single write of the same bytes
// (via w.WriteString when w is an io.StringWriter, whose contract
// requires writing the same bytes as Write([]byte(s))) and forwards the
// same (n, err). This suite pins:
//
//   - byte-identity of the written stream AND the (n, err) pair over
//     adversarial operands — empty, single byte, newline-bearing,
//     long (past fmt's small-buffer path), multi-byte UTF-8, NUL
//     bytes, invalid UTF-8, escape-heavy, and '%'-bearing strings
//     (Fprintln never interprets verbs, so '%' is data on both
//     sides) — for a StringWriter (bytes.Buffer / strings.Builder), a
//     Write-only writer (no WriteString method, exercising the []byte
//     fallback), and an erroring writer that truncates and fails
//     mid-write, where both sides must surface the identical (n, err);
//   - WHY the exactly-one-operand guard is load-bearing: a second
//     operand gains Fprintln's separating space, which a bare
//     a+b+"\n" join would drop;
//   - WHY the EXACT-predeclared-string guard is load-bearing: a named
//     string type's String()/Format() methods are honored by
//     Fprintln's %v formatting and ignored by +.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// ps5038WriteOnly wraps a bytes.Buffer exposing ONLY Write, so
// io.WriteString must take its w.Write([]byte(s)) fallback — the same
// method fmt.Fprintln calls.
type ps5038WriteOnly struct{ buf bytes.Buffer }

func (w *ps5038WriteOnly) Write(p []byte) (int, error) { return w.buf.Write(p) }

// ps5038ErrWriter accepts at most limit bytes in total, then fails: a
// contract-conforming Write that returns a short count with an error.
// Both sides call Write exactly once with all the bytes (s plus the
// trailing '\n'), so both must see the identical truncated n and the
// identical error value.
type ps5038ErrWriter struct {
	buf   bytes.Buffer
	limit int
}

var errPS5038Full = errors.New("ps5038: writer full")

func (w *ps5038ErrWriter) Write(p []byte) (int, error) {
	room := w.limit - w.buf.Len()
	if room < 0 {
		room = 0
	}
	if len(p) <= room {
		return w.buf.Write(p)
	}
	n, _ := w.buf.Write(p[:room])
	return n, errPS5038Full
}

// ps5038Inputs are adversarial single operands — every shape the check
// may rewrite: empty (a plain string is never nil; empty writes just
// "\n"), tiny, newline-bearing (Fprintln appends its own '\n' anyway),
// long (past any small-buffer path), multi-byte UTF-8, NUL bytes,
// invalid UTF-8, escape-heavy runs, and '%'-bearing strings — Fprintln
// never interprets verbs, so unlike the Fprintf checks NO '%' guard is
// needed and identity must HOLD for them.
var ps5038Inputs = []string{
	"",
	"a",
	"done",
	"already\nsplit\nacross\nlines\n",
	strings.Repeat("0123456789abcdef", 65), // 1040 bytes, past pp.buf's initial cap
	"日本語テキスト",
	"a\x00b\x00c",
	"\xff\xfe invalid utf8 \x80",
	"tab\tcr\rlf\nbell\a backslash \\ quote \" done",
	"100%% and %d and a trailing %",
	"%v %s %q",
}

// TestEquivPS5038_Identity pins that fmt.Fprintln(w, s) and
// io.WriteString(w, s+"\n") write the identical byte stream and return
// the identical (n, err) for every operand, across the StringWriter
// path, the Write-only fallback path, and a strings.Builder.
func TestEquivPS5038_Identity(t *testing.T) {
	for _, s := range ps5038Inputs {
		// StringWriter path: io.WriteString dispatches to WriteString.
		var a, b bytes.Buffer
		nA, eA := fmt.Fprintln(&a, s)
		nB, eB := io.WriteString(&b, s+"\n")
		if a.String() != b.String() || nA != nB || !errors.Is(eA, eB) || (eA == nil) != (eB == nil) {
			t.Errorf("bytes.Buffer: Fprintln(%q) vs io.WriteString: bytes %q/%q n %d/%d err %v/%v", s, a.String(), b.String(), nA, nB, eA, eB)
		}
		if nB != len(s)+1 || eB != nil {
			t.Errorf("bytes.Buffer: io.WriteString(%q+\"\\n\") = (%d, %v), want (%d, nil)", s, nB, eB, len(s)+1)
		}
		if want := s + "\n"; b.String() != want {
			t.Errorf("bytes.Buffer: stream %q, want %q", b.String(), want)
		}

		// Write-only path: io.WriteString falls back to Write([]byte(s)) —
		// the very method fmt calls.
		var wo1, wo2 ps5038WriteOnly
		nC, eC := fmt.Fprintln(&wo1, s)
		nD, eD := io.WriteString(&wo2, s+"\n")
		if wo1.buf.String() != wo2.buf.String() || nC != nD || (eC == nil) != (eD == nil) {
			t.Errorf("write-only: Fprintln(%q) vs io.WriteString: bytes %q/%q n %d/%d err %v/%v", s, wo1.buf.String(), wo2.buf.String(), nC, nD, eC, eD)
		}

		// strings.Builder is a second StringWriter with a different
		// WriteString implementation.
		var sb1, sb2 strings.Builder
		nE, eE := fmt.Fprintln(&sb1, s)
		nF, eF := io.WriteString(&sb2, s+"\n")
		if sb1.String() != sb2.String() || nE != nF || (eE == nil) != (eF == nil) {
			t.Errorf("strings.Builder: Fprintln(%q) vs io.WriteString: bytes %q/%q n %d/%d", s, sb1.String(), sb2.String(), nE, nF)
		}
	}
}

// TestEquivPS5038_ErrorWriter pins that a failing writer surfaces the
// IDENTICAL truncated n, error value and partial byte stream through
// both forms: each performs exactly one Write call carrying all the
// bytes (s plus the trailing '\n').
func TestEquivPS5038_ErrorWriter(t *testing.T) {
	for _, s := range ps5038Inputs {
		for _, limit := range []int{0, 1, 3, len(s), len(s) + 1, len(s) + 4} {
			w1 := &ps5038ErrWriter{limit: limit}
			w2 := &ps5038ErrWriter{limit: limit}
			n1, e1 := fmt.Fprintln(w1, s)
			n2, e2 := io.WriteString(w2, s+"\n")
			if n1 != n2 || !errors.Is(e1, e2) || (e1 == nil) != (e2 == nil) {
				t.Errorf("limit %d: Fprintln(%q) = (%d, %v), io.WriteString = (%d, %v) — must be identical", limit, s, n1, e1, n2, e2)
			}
			if w1.buf.String() != w2.buf.String() {
				t.Errorf("limit %d: partial streams diverge: %q vs %q", limit, w1.buf.String(), w2.buf.String())
			}
		}
	}
}

// TestEquivPS5038_TwoOperandsDiverge pins WHY the exactly-one-operand
// guard is load-bearing: with two operands Fprintln writes exactly one
// separating space, which a bare a+b+"\n" join drops — the outputs MUST
// differ, otherwise the guard would be needlessly conservative.
func TestEquivPS5038_TwoOperandsDiverge(t *testing.T) {
	var a, b bytes.Buffer
	_, _ = fmt.Fprintln(&a, "host", "port")
	_, _ = io.WriteString(&b, "host"+"port"+"\n")
	if a.String() == b.String() {
		t.Errorf("Fprintln with TWO operands matched the space-free join (%q) — the one-operand guard would be needlessly conservative", a.String())
	}
	if want := "host port\n"; a.String() != want {
		t.Errorf("Fprintln(\"host\", \"port\") = %q, want %q", a.String(), want)
	}
}

// ps5038Loud is a NAMED string type whose String() method reports
// something other than its underlying bytes.
type ps5038Loud string

func (ps5038Loud) String() string { return "SURPRISE" }

// ps5038Fmt is a NAMED string type implementing fmt.Formatter.
type ps5038Fmt string

func (ps5038Fmt) Format(f fmt.State, verb rune) { io.WriteString(f, "FORMATTED") }

// TestEquivPS5038_NamedTypeDiverges pins WHY the EXACT-predeclared-string
// guard is load-bearing: Fprintln's %v formatting honors a named string
// type's String()/Format() methods, while + on the underlying string
// would emit the raw bytes — the outputs MUST differ.
func TestEquivPS5038_NamedTypeDiverges(t *testing.T) {
	var a, b bytes.Buffer
	_, _ = fmt.Fprintln(&a, ps5038Loud("quiet"))
	_, _ = io.WriteString(&b, string(ps5038Loud("quiet"))+"\n")
	if a.String() == b.String() {
		t.Errorf("Fprintln of a Stringer-bearing named string matched the raw-bytes join (%q) — the plain-string guard would be needlessly conservative", a.String())
	}
	if want := "SURPRISE\n"; a.String() != want {
		t.Errorf("Fprintln(ps5038Loud) = %q, want %q", a.String(), want)
	}

	var c, d bytes.Buffer
	_, _ = fmt.Fprintln(&c, ps5038Fmt("plain"))
	_, _ = io.WriteString(&d, string(ps5038Fmt("plain"))+"\n")
	if c.String() == d.String() {
		t.Errorf("Fprintln of a Formatter-bearing named string matched the raw-bytes join (%q)", c.String())
	}
	if want := "FORMATTED\n"; c.String() != want {
		t.Errorf("Fprintln(ps5038Fmt) = %q, want %q", c.String(), want)
	}
}
