package checks

// Runtime differential for PS2042: fmt.Fprintf(w, "%s", b) with b an unnamed
// []byte versus w.Write(b). The fix's safety argument: %s on a []byte writes
// the bytes VERBATIM (invalid UTF-8 preserved, fmt metacharacters inside b
// copied as data, nil/empty writing zero bytes), and fmt.Fprintf's final step
// is exactly `n, err = w.Write(p.buf)` with p.buf holding precisely b's
// bytes — so both forms call w.Write exactly once with the same bytes and
// return that call's (n, err) pair unchanged, short writes and sticky errors
// included. This suite pins:
//
//   - byte identity of everything reaching the writer, plus (n, err) parity,
//     over adversarial operands (nil, empty, NUL, every flavor of invalid
//     UTF-8, fmt metacharacters that must pass through UNinterpreted, a lone
//     surrogate, 4 KiB, random byte strings with a fixed seed);
//   - exact (n, err) passthrough and single-Write-call parity on a writer
//     that short-writes with a custom error;
//   - evaluation order and count parity for the writer and operand
//     expressions (both may be calls; both forms evaluate w then b, once);
//   - the divergences the fix's gates exclude, reproduced positively: "%v"
//     (the decimal slice form), a NAMED []byte whose String() %s honors,
//     fmt.Fprint's two-argument form (also the decimal form), a longer
//     format ("%s\n"), and the nil-[]byte-under-%s parity that needs NO
//     gate;
//   - the perf premise: the After shape is allocation-free while fmt.Fprintf
//     boxes the slice header.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"testing"
)

func ps2042Before(w io.Writer, b []byte) (int, error) { return fmt.Fprintf(w, "%s", b) }
func ps2042After(w io.Writer, b []byte) (int, error)  { return w.Write(b) }

// ps2042Rec records every Write call's bytes and call count.
type ps2042Rec struct {
	got   []byte
	calls int
}

func (r *ps2042Rec) Write(p []byte) (int, error) {
	r.calls++
	r.got = append(r.got, p...)
	return len(p), nil
}

func TestEquiv_PS2042_ByteIdentity(t *testing.T) {
	ops := [][]byte{
		nil,
		{},
		[]byte("a"),
		[]byte("hello, world"),
		[]byte("\x00"), []byte("a\x00b"),
		[]byte("\xff\xfe\x80"),            // invalid UTF-8 — %s copies raw bytes
		[]byte(string(rune(0xD800))),      // lone surrogate literal encodes as RuneError
		[]byte("%s %v %% %d %w"),          // fmt metacharacters are DATA in the operand
		[]byte("héllo, 世界"),               //
		[]byte(strings.Repeat("q", 4096)), //
		bytes.Repeat([]byte{0xC0}, 33),    // overlong-encoding lead bytes
	}
	rng := rand.New(rand.NewSource(0x2042))
	for range 500 {
		b := make([]byte, rng.Intn(64))
		rng.Read(b)
		ops = append(ops, b)
	}
	for _, b := range ops {
		var w1, w2 ps2042Rec
		n1, e1 := ps2042Before(&w1, b)
		n2, e2 := ps2042After(&w2, b)
		if !bytes.Equal(w1.got, w2.got) {
			t.Fatalf("bytes diverge for %q: before wrote %q, after wrote %q", b, w1.got, w2.got)
		}
		if n1 != n2 || e1 != e2 {
			t.Fatalf("(n, err) diverge for %q: before (%d, %v), after (%d, %v)", b, n1, e1, n2, e2)
		}
		if w1.calls != 1 || w2.calls != 1 {
			t.Fatalf("Write call count diverges for %q: before %d, after %d (both must be exactly 1)", b, w1.calls, w2.calls)
		}
	}
}

// ps2042Short short-writes: it reports 2 bytes and a custom error for any
// input longer than 2 bytes. fmt.Fprintf must pass that (n, err) through
// unchanged — its last step is literally `n, err = w.Write(p.buf)`.
type ps2042Short struct{ calls int }

var errPS2042Short = errors.New("ps2042: short write")

func (s *ps2042Short) Write(p []byte) (int, error) {
	s.calls++
	if len(p) > 2 {
		return 2, errPS2042Short
	}
	return len(p), nil
}

func TestEquiv_PS2042_ShortWritePassthrough(t *testing.T) {
	b := []byte("hello")
	s1, s2 := &ps2042Short{}, &ps2042Short{}
	n1, e1 := ps2042Before(s1, b)
	n2, e2 := ps2042After(s2, b)
	if n1 != 2 || !errors.Is(e1, errPS2042Short) {
		t.Fatalf("fmt.Fprintf did not pass the writer's (n, err) through: got (%d, %v) — PS2042's premise is broken", n1, e1)
	}
	if n1 != n2 || e1 != e2 {
		t.Fatalf("short-write results diverge: before (%d, %v), after (%d, %v)", n1, e1, n2, e2)
	}
	if s1.calls != 1 || s2.calls != 1 {
		t.Fatalf("short-write call counts diverge: before %d, after %d", s1.calls, s2.calls)
	}
}

// TestEquiv_PS2042_EvaluationOrder pins that both forms evaluate the writer
// expression, then the operand expression, exactly once each, before the
// single Write.
func TestEquiv_PS2042_EvaluationOrder(t *testing.T) {
	var trace []string
	rec := &ps2042Rec{}
	wFn := func() io.Writer { trace = append(trace, "w"); return rec }
	bFn := func() []byte { trace = append(trace, "b"); return []byte("x") }

	trace = nil
	fmt.Fprintf(wFn(), "%s", bFn())
	beforeTrace := strings.Join(trace, ",")

	trace = nil
	wFn().Write(bFn())
	afterTrace := strings.Join(trace, ",")

	if beforeTrace != afterTrace {
		t.Fatalf("evaluation order diverges: before=%s after=%s", beforeTrace, afterTrace)
	}
	if beforeTrace != "w,b" {
		t.Fatalf("unexpected evaluation trace %q, want w,b", beforeTrace)
	}
	if string(rec.got) != "xx" || rec.calls != 2 {
		t.Fatalf("expected both forms to write %q once each, recorder saw %q in %d calls", "x", rec.got, rec.calls)
	}
}

// ps2042NamedB is the divergence witness for the fix's unnamed-[]byte gate:
// %s honors its String() method, Write copies its raw bytes.
type ps2042NamedB []byte

func (ps2042NamedB) String() string { return "STRINGER" }

// TestEquiv_PS2042_GuardWitnesses pins that every guard excludes a shape
// that ACTUALLY diverges — the guards are load-bearing.
func TestEquiv_PS2042_GuardWitnesses(t *testing.T) {
	// "%v" of a []byte is the decimal slice form, nothing like the bytes.
	if got := fmt.Sprintf("%v", []byte("hi")); got != "[104 105]" {
		t.Fatalf("%%v of []byte = %q, want %q — re-audit the %%s-only gate", got, "[104 105]")
	}
	// A NAMED []byte with String(): %s honors the method, Write the bytes.
	var n ps2042NamedB = []byte("raw")
	//lint:ignore S1025 the %s-over-a-Stringer detour IS the divergence under test
	if got := fmt.Sprintf("%s", n); got != "STRINGER" {
		t.Fatalf("%%s of a Stringer-carrying named []byte = %q, want %q — re-audit the unnamed-slice gate", got, "STRINGER")
	}
	var w ps2042Rec
	w.Write(n)
	if string(w.got) != "raw" {
		t.Fatalf("Write of a named []byte wrote %q, want the raw bytes %q", w.got, "raw")
	}
	// fmt.Fprint's default verb is also the decimal form — no []byte arm
	// exists for the two-argument form.
	if got := fmt.Sprint([]byte("hi")); got != "[104 105]" {
		t.Fatalf("fmt.Sprint of []byte = %q, want %q — re-audit the Fprint exclusion", got, "[104 105]")
	}
	// Any format longer than the one verb writes different bytes.
	if got := fmt.Sprintf("%s\n", []byte("hi")); got != "hi\n" {
		t.Fatalf("%%s\\n of []byte = %q, want %q", got, "hi\n")
	}
	// nil under %s needs NO gate: both forms hand the writer zero bytes.
	//lint:ignore S1025 the %s-over-a-[]byte detour IS the shape under test
	if got := fmt.Sprintf("%s", []byte(nil)); got != "" {
		t.Fatalf("%%s of nil []byte = %q, want the empty string", got)
	}
	// nil the LITERAL is excluded (not statically []byte): it formats as an
	// error annotation, not zero bytes.
	var nilArg any
	if got := fmt.Sprintf("%s", nilArg); got != "%!s(<nil>)" {
		t.Fatalf("%%s of a nil argument = %q, want %q", got, "%!s(<nil>)")
	}
}

// TestEquiv_PS2042_AfterAllocProfile pins the perf premise: w.Write(b)
// allocates nothing, while fmt.Fprintf boxes b's slice header into an
// interface. The operand lives at package scope so the compiler cannot prove
// the box does not escape.
var (
	ps2042AllocB   = []byte("hello, world")
	ps2042AllocOut int
)

func TestEquiv_PS2042_AfterAllocProfile(t *testing.T) {
	if avg := testing.AllocsPerRun(200, func() {
		n, _ := io.Discard.Write(ps2042AllocB)
		ps2042AllocOut = n
	}); avg != 0 {
		t.Errorf("w.Write(b) allocates %v times per run, want 0", avg)
	}
	if avg := testing.AllocsPerRun(200, func() {
		n, _ := fmt.Fprintf(io.Discard, "%s", ps2042AllocB)
		ps2042AllocOut = n
	}); avg < 1 {
		t.Logf("fmt.Fprintf allocates %v times per run — the compiler learned to elide the box; the win claim may need re-framing", avg)
	}
}
