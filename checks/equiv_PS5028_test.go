package checks

// Runtime differential for PS5028: fmt.Fprintf(w, "constant") with a
// verbless string literal and no operands vs the io.WriteString(w, s)
// form the fix emits. The safety argument is that with EXACTLY two
// arguments and NO '%' byte in the format, doPrintf finds no verb: fmt
// copies the format bytes verbatim into its pooled buffer and performs
// ONE w.Write of exactly those bytes, returning that Write's (n, err) —
// while io.WriteString performs the same single write of the same bytes
// (via w.WriteString when w is an io.StringWriter, whose contract
// requires writing the same bytes as Write([]byte(s))) and forwards the
// same (n, err). This suite pins:
//
//   - byte-identity of the written stream AND the (n, err) pair over
//     adversarial verbless formats — empty, single byte, long,
//     multi-byte UTF-8, NUL bytes, invalid UTF-8, escape-heavy — for a
//     StringWriter (bytes.Buffer / strings.Builder), a Write-only
//     writer (no WriteString method, exercising the []byte fallback),
//     and an erroring writer that truncates and fails mid-write, where
//     both sides must surface the identical (n, err);
//   - WHY the no-'%' guard is load-bearing, with concrete divergence
//     witnesses: "%%" collapses, a verb with no operand prints
//     %!d(MISSING), a lone trailing "%" prints %!(NOVERB);
//   - WHY the exactly-two-arguments guard is load-bearing: a verbless
//     format with an extra operand appends %!(EXTRA ...).

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// ps5028Fprintf is fmt.Fprintf reached through a function variable: the
// suite feeds it deliberately adversarial formats (non-constant strings,
// verbs with no operands, an extra operand) that `go vet`'s printf
// checker would reject at a direct call site — the indirection keeps vet
// out while still exercising the real fmt.Fprintf.
var ps5028Fprintf = fmt.Fprintf

// ps5028WriteOnly wraps a bytes.Buffer exposing ONLY Write, so
// io.WriteString must take its w.Write([]byte(s)) fallback — the same
// method fmt.Fprintf calls.
type ps5028WriteOnly struct{ buf bytes.Buffer }

func (w *ps5028WriteOnly) Write(p []byte) (int, error) { return w.buf.Write(p) }

// ps5028ErrWriter accepts at most limit bytes in total, then fails: a
// contract-conforming Write that returns a short count with an error.
// Both sides call Write exactly once with all the bytes, so both must
// see the identical truncated n and the identical error value.
type ps5028ErrWriter struct {
	buf   bytes.Buffer
	limit int
}

var errPS5028Full = errors.New("ps5028: writer full")

func (w *ps5028ErrWriter) Write(p []byte) (int, error) {
	room := w.limit - w.buf.Len()
	if room < 0 {
		room = 0
	}
	if len(p) <= room {
		return w.buf.Write(p)
	}
	n, _ := w.buf.Write(p[:room])
	return n, errPS5028Full
}

// ps5028Formats are adversarial VERBLESS literals — every shape the check
// may rewrite: empty, tiny, long (past any small-buffer path), multi-byte
// UTF-8, NUL bytes, invalid UTF-8, and escape-heavy runs. None contains a
// '%' byte; the guard witnesses live in TestEquivPS5028_PercentDiverges.
var ps5028Formats = []string{
	"",
	"a",
	"done\n",
	"shutdown: all shards flushed\n",
	strings.Repeat("0123456789abcdef", 65), // 1040 bytes, past pp.buf's initial cap
	"日本語テキスト\n",
	"a\x00b\x00c",
	"\xff\xfe invalid utf8 \x80",
	"tab\tcr\rlf\nbell\a backslash \\ quote \" done",
}

// TestEquivPS5028_Identity pins that fmt.Fprintf(w, s) and
// io.WriteString(w, s) write the identical byte stream and return the
// identical (n, err) for every verbless format, across the StringWriter
// path, the Write-only fallback path, and a strings.Builder.
func TestEquivPS5028_Identity(t *testing.T) {
	for _, s := range ps5028Formats {
		// StringWriter path: io.WriteString dispatches to WriteString.
		var a, b bytes.Buffer
		nA, eA := ps5028Fprintf(&a, s)
		nB, eB := io.WriteString(&b, s)
		if a.String() != b.String() || nA != nB || !errors.Is(eA, eB) || (eA == nil) != (eB == nil) {
			t.Errorf("bytes.Buffer: Fprintf(%q) vs io.WriteString: bytes %q/%q n %d/%d err %v/%v", s, a.String(), b.String(), nA, nB, eA, eB)
		}
		if nB != len(s) || eB != nil {
			t.Errorf("bytes.Buffer: io.WriteString(%q) = (%d, %v), want (%d, nil)", s, nB, eB, len(s))
		}

		// Write-only path: io.WriteString falls back to Write([]byte(s)) —
		// the very method fmt calls.
		var wo1, wo2 ps5028WriteOnly
		nC, eC := ps5028Fprintf(&wo1, s)
		nD, eD := io.WriteString(&wo2, s)
		if wo1.buf.String() != wo2.buf.String() || nC != nD || (eC == nil) != (eD == nil) {
			t.Errorf("write-only: Fprintf(%q) vs io.WriteString: bytes %q/%q n %d/%d err %v/%v", s, wo1.buf.String(), wo2.buf.String(), nC, nD, eC, eD)
		}

		// strings.Builder is a second StringWriter with a different
		// WriteString implementation.
		var sb1, sb2 strings.Builder
		nE, eE := ps5028Fprintf(&sb1, s)
		nF, eF := io.WriteString(&sb2, s)
		if sb1.String() != sb2.String() || nE != nF || (eE == nil) != (eF == nil) {
			t.Errorf("strings.Builder: Fprintf(%q) vs io.WriteString: bytes %q/%q n %d/%d", s, sb1.String(), sb2.String(), nE, nF)
		}
	}
}

// TestEquivPS5028_ErrorWriter pins that a failing writer surfaces the
// IDENTICAL truncated n, error value and partial byte stream through both
// forms: each performs exactly one Write call carrying all the bytes.
func TestEquivPS5028_ErrorWriter(t *testing.T) {
	for _, s := range ps5028Formats {
		for _, limit := range []int{0, 1, 3, len(s), len(s) + 4} {
			w1 := &ps5028ErrWriter{limit: limit}
			w2 := &ps5028ErrWriter{limit: limit}
			n1, e1 := ps5028Fprintf(w1, s)
			n2, e2 := io.WriteString(w2, s)
			if n1 != n2 || !errors.Is(e1, e2) || (e1 == nil) != (e2 == nil) {
				t.Errorf("limit %d: Fprintf(%q) = (%d, %v), io.WriteString = (%d, %v) — must be identical", limit, s, n1, e1, n2, e2)
			}
			if w1.buf.String() != w2.buf.String() {
				t.Errorf("limit %d: partial streams diverge: %q vs %q", limit, w1.buf.String(), w2.buf.String())
			}
		}
	}
}

// TestEquivPS5028_PercentDiverges pins WHY the no-'%' guard is
// load-bearing: for every format containing a '%', fmt interprets it
// while io.WriteString would copy it verbatim — the outputs MUST differ,
// otherwise the guard would be needlessly conservative.
func TestEquivPS5028_PercentDiverges(t *testing.T) {
	for _, s := range []string{
		"100%%\n",    // %% collapses to a single %
		"%d\n",       // a verb with no operand prints %!d(MISSING)
		"trailing %", // a lone trailing % prints %!(NOVERB)
		"%",
		"a\x25b", // a '%' smuggled through an escape is still a %
	} {
		var a, b bytes.Buffer
		_, _ = ps5028Fprintf(&a, s)
		_, _ = io.WriteString(&b, s)
		if a.String() == b.String() {
			t.Errorf("Fprintf(%q) == io.WriteString(%q) == %q — expected divergence; the no-'%%' guard would be needlessly conservative", s, s, a.String())
		}
	}
}

// TestEquivPS5028_ExtraArgDiverges pins WHY the exactly-two-arguments
// guard is load-bearing: a verbless format with an operand appends
// %!(EXTRA ...) — not the literal bytes.
func TestEquivPS5028_ExtraArgDiverges(t *testing.T) {
	var a, b bytes.Buffer
	_, _ = ps5028Fprintf(&a, "done\n", 42)
	_, _ = io.WriteString(&b, "done\n")
	if a.String() == b.String() {
		t.Errorf("Fprintf with an EXTRA operand matched io.WriteString (%q) — the two-argument guard would be needlessly conservative", a.String())
	}
	if want := "done\n%!(EXTRA int=42)"; a.String() != want {
		t.Errorf("Fprintf(\"done\\n\", 42) = %q, want %q", a.String(), want)
	}
}
