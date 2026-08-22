package checks

// Runtime differential for PS2048: fmt.Fprint(w, a, b, ..., z) over only
// plain-string operands vs the io.WriteString(w, a+b+...+z) form the fix
// emits. The safety claim is threefold:
//
//   - BYTES: fmt.Fprint inserts a separating space only "when neither
//     [neighbor] is a string"; with every operand a plain string that
//     condition never fires and the default %v formatting emits each
//     string verbatim, so the accumulated buffer is exactly the
//     concatenation — empty operands, invalid UTF-8, NUL bytes, digits
//     and spaces included.
//   - ONE WRITE, SAME (n, err): fmt.Fprint hands that buffer to exactly
//     ONE w.Write and returns its results; io.WriteString performs one
//     write of the identical bytes (w.WriteString when w is an
//     io.StringWriter — whose contract requires the same behavior as
//     Write([]byte(s)) — and w.Write otherwise), so (n, err) carry over
//     verbatim, error and short-write cases included.
//   - PERF PREMISE: the Before side allocates an interface box per
//     operand plus buffer traffic; the After side allocates only the
//     concatenated result.
//
// The suite crosses adversarial operand tuples (2..6 operands, empties,
// invalid UTF-8, surrogate-free multibyte runes, NULs, all-digit and
// all-space strings that would trigger the spacing rule if the string
// exemption were ever misread) with adversarial writers (StringWriter
// and not, failing after k bytes and not, Write-call counting).

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// ps2048CountWriter counts Write calls and captures bytes. It
// deliberately does NOT implement io.StringWriter, forcing
// io.WriteString down its Write fallback.
type ps2048CountWriter struct {
	buf          bytes.Buffer
	writes       int
	stringWrites int
	failAfter    int // -1: never fail; else accept at most failAfter bytes then error
}

var errPS2048Full = errors.New("ps2048: writer full")

func (w *ps2048CountWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.failAfter >= 0 && len(p) > w.failAfter {
		n, _ := w.buf.Write(p[:w.failAfter])
		return n, errPS2048Full
	}
	return w.buf.Write(p)
}

// ps2048StringWriter adds a contract-conforming WriteString (identical
// bytes and results as Write([]byte(s)) — the io.StringWriter contract
// PS2048, like PS2129, relies on).
type ps2048StringWriter struct{ ps2048CountWriter }

func (w *ps2048StringWriter) WriteString(s string) (int, error) {
	w.stringWrites++
	if w.failAfter >= 0 && len(s) > w.failAfter {
		n, _ := w.buf.WriteString(s[:w.failAfter])
		return n, errPS2048Full
	}
	return w.buf.WriteString(s)
}

// ps2048Concat mirrors the fix's + chain (variadic only to enumerate
// cases; the generated code is a literal a+b+...+z, which the compiler
// lowers to the same runtime concatenation).
func ps2048Concat(ops []string) string {
	switch len(ops) {
	case 2:
		return ops[0] + ops[1]
	case 3:
		return ops[0] + ops[1] + ops[2]
	case 4:
		return ops[0] + ops[1] + ops[2] + ops[3]
	case 5:
		return ops[0] + ops[1] + ops[2] + ops[3] + ops[4]
	case 6:
		return ops[0] + ops[1] + ops[2] + ops[3] + ops[4] + ops[5]
	}
	panic("ps2048Concat: unsupported arity")
}

func ps2048Fprint(w io.Writer, ops []string) (int, error) {
	switch len(ops) {
	case 2:
		return fmt.Fprint(w, ops[0], ops[1])
	case 3:
		return fmt.Fprint(w, ops[0], ops[1], ops[2])
	case 4:
		return fmt.Fprint(w, ops[0], ops[1], ops[2], ops[3])
	case 5:
		return fmt.Fprint(w, ops[0], ops[1], ops[2], ops[3], ops[4])
	case 6:
		return fmt.Fprint(w, ops[0], ops[1], ops[2], ops[3], ops[4], ops[5])
	}
	panic("ps2048Fprint: unsupported arity")
}

func ps2048Tuples() [][]string {
	adversarial := []string{
		"",                          // empty operand: contributes nothing on both sides
		" ",                         // a literal space — must appear exactly once, never doubled
		"42",                        // digits: would gain a space if the string exemption were misread
		"a b",                       // interior spaces pass through verbatim
		"\xff\xfe\x80",              // invalid UTF-8 passes through verbatim (no replacement rune)
		"日本語",                       // multibyte runes
		"\x00zero\x00",              // NUL bytes
		"%v %s %d",                  // verbs are DATA to Fprint (no format string) and to + alike
		"\n\t",                      // control characters
		strings.Repeat("long-", 50), // forces buffer growth inside fmt's pp
	}
	var tuples [][]string
	// Every ordered pair of adversarial operands.
	for _, a := range adversarial {
		for _, b := range adversarial {
			tuples = append(tuples, []string{a, b})
		}
	}
	// Longer arities, empties interleaved at every position.
	tuples = append(tuples,
		[]string{"", "", ""},
		[]string{"host", ":", "8080"},
		[]string{"", "x", "", "y", ""},
		[]string{"a", " ", "1", " ", "2", "b"},
		[]string{"\xff", "", "日", "\x00", "42", " "},
		[]string{strings.Repeat("x", 1000), "", strings.Repeat("y", 1000)},
	)
	return tuples
}

// TestEquiv_PS2048Bytes pins byte identity and (n, err) parity on
// always-succeeding writers, StringWriter and not, plus the ONE-Write
// claim on the non-StringWriter path (fmt always uses w.Write; the
// rewrite's io.WriteString falls back to exactly one w.Write there).
func TestEquiv_PS2048Bytes(t *testing.T) {
	for _, ops := range ps2048Tuples() {
		want := ps2048Concat(ops)

		// Plain writer (no WriteString): both sides go through Write.
		bw := &ps2048CountWriter{failAfter: -1}
		aw := &ps2048CountWriter{failAfter: -1}
		bn, berr := ps2048Fprint(bw, ops)
		an, aerr := io.WriteString(aw, ps2048Concat(ops))
		if !bytes.Equal(bw.buf.Bytes(), aw.buf.Bytes()) || bw.buf.String() != want {
			t.Fatalf("ops %q: bytes diverged: fmt wrote %q, rewrite wrote %q, want %q",
				ops, bw.buf.Bytes(), aw.buf.Bytes(), want)
		}
		if bn != an || !errors.Is(berr, aerr) || berr != nil {
			t.Fatalf("ops %q: results diverged: fmt (%d, %v) vs rewrite (%d, %v)", ops, bn, berr, an, aerr)
		}
		if bw.writes != 1 || aw.writes != 1 {
			t.Fatalf("ops %q: write-call counts diverged from the one-Write claim: fmt %d, rewrite %d",
				ops, bw.writes, aw.writes)
		}

		// StringWriter: fmt still uses Write, the rewrite dispatches to
		// the contract-conforming WriteString — same bytes, same results.
		bsw := &ps2048StringWriter{ps2048CountWriter{failAfter: -1}}
		asw := &ps2048StringWriter{ps2048CountWriter{failAfter: -1}}
		bn, berr = ps2048Fprint(bsw, ops)
		an, aerr = io.WriteString(asw, ps2048Concat(ops))
		if bsw.buf.String() != asw.buf.String() || bsw.buf.String() != want {
			t.Fatalf("ops %q (StringWriter): bytes diverged: fmt %q, rewrite %q, want %q",
				ops, bsw.buf.String(), asw.buf.String(), want)
		}
		if bn != an || berr != nil || aerr != nil {
			t.Fatalf("ops %q (StringWriter): results diverged: fmt (%d, %v) vs rewrite (%d, %v)",
				ops, bn, berr, an, aerr)
		}
		if got := bsw.writes + bsw.stringWrites; got != 1 {
			t.Fatalf("ops %q: fmt made %d writes to the StringWriter, want 1", ops, got)
		}
		if got := asw.writes + asw.stringWrites; got != 1 || asw.stringWrites != 1 {
			t.Fatalf("ops %q: rewrite made %d writes (%d via WriteString), want exactly 1 via WriteString",
				ops, got, asw.stringWrites)
		}

		// stdlib StringWriters for good measure.
		var sb1, sb2 strings.Builder
		bn, _ = ps2048Fprint(&sb1, ops)
		an, _ = io.WriteString(&sb2, ps2048Concat(ops))
		if sb1.String() != sb2.String() || bn != an || sb1.String() != want {
			t.Fatalf("ops %q (strings.Builder): diverged: fmt (%q, %d) vs rewrite (%q, %d)",
				ops, sb1.String(), bn, sb2.String(), an)
		}
	}
}

// TestEquiv_PS2048ErrParity pins (n, err) parity on failing and
// short-writing writers: both sides perform ONE write of the identical
// bytes, so a writer accepting only k of them returns the same k and
// the same error either way — through Write and through a
// contract-conforming WriteString alike.
func TestEquiv_PS2048ErrParity(t *testing.T) {
	ops := []string{"hello ", "", "wor", "ld\xff"}
	full := len(ps2048Concat(ops))
	for _, failAfter := range []int{0, 1, 3, full - 1, full, full + 1} {
		bw := &ps2048CountWriter{failAfter: failAfter}
		aw := &ps2048CountWriter{failAfter: failAfter}
		bn, berr := ps2048Fprint(bw, ops)
		an, aerr := io.WriteString(aw, ps2048Concat(ops))
		if bn != an || berr != aerr {
			t.Fatalf("failAfter %d: results diverged: fmt (%d, %v) vs rewrite (%d, %v)",
				failAfter, bn, berr, an, aerr)
		}
		if !bytes.Equal(bw.buf.Bytes(), aw.buf.Bytes()) {
			t.Fatalf("failAfter %d: partial bytes diverged: fmt %q vs rewrite %q",
				failAfter, bw.buf.Bytes(), aw.buf.Bytes())
		}

		bsw := &ps2048StringWriter{ps2048CountWriter{failAfter: failAfter}}
		asw := &ps2048StringWriter{ps2048CountWriter{failAfter: failAfter}}
		bn, berr = ps2048Fprint(bsw, ops)
		an, aerr = io.WriteString(asw, ps2048Concat(ops))
		if bn != an || berr != aerr {
			t.Fatalf("failAfter %d (StringWriter): results diverged: fmt (%d, %v) vs rewrite (%d, %v)",
				failAfter, bn, berr, an, aerr)
		}
		if bsw.buf.String() != asw.buf.String() {
			t.Fatalf("failAfter %d (StringWriter): partial bytes diverged: fmt %q vs rewrite %q",
				failAfter, bsw.buf.String(), asw.buf.String())
		}
	}
}

// TestEquiv_PS2048Allocs pins the perf premise: writing three plain
// strings to io.Discard, the rewrite allocates only the concatenated
// result, strictly fewer allocations than fmt.Fprint's per-operand
// interface boxes. The operands are built at runtime — the gc compiler
// staticizes the interface box of a compile-time-constant string, which
// would hide the boxing cost this test pins.
func TestEquiv_PS2048Allocs(t *testing.T) {
	a, b, c := strings.Repeat("the quick brown fox jump", 1), strings.Repeat(":", 1), strings.Repeat("s over the lazy dog", 1)
	var sinkN int
	after := testing.AllocsPerRun(200, func() {
		n, _ := io.WriteString(io.Discard, a+b+c)
		sinkN = n
	})
	before := testing.AllocsPerRun(200, func() {
		n, _ := fmt.Fprint(io.Discard, a, b, c)
		sinkN = n
	})
	_ = sinkN
	if after > 1 {
		t.Errorf("io.WriteString(w, a+b+c) allocates %v/op, want <= 1 (the result string)", after)
	}
	if before <= after {
		t.Errorf("perf premise broken: fmt.Fprint %v allocs/op vs rewrite %v allocs/op — re-measure PS2048's win", before, after)
	}
}
