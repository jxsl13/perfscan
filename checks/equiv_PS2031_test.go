package checks

//lint:file-ignore ST1017 the Yoda-ordered comparisons are the exact swapped-operand Before shapes the differential pins

// Runtime differential for PS2031: buf.String() == "lit" / != "lit"
// (either operand order) versus the bytes.Equal(buf.Bytes(),
// []byte("lit")) rewrites. The fix's safety argument is that for a
// non-nil receiver, String() returns string(b.buf[b.off:]) and Bytes()
// returns b.buf[b.off:] — the same raw bytes — and both a string
// comparison and bytes.Equal are pure byte-for-byte equality with no
// UTF-8 or case interpretation, so the two forms agree for every
// buffer state and every constant. This suite pins that claim over:
//
//   - the exact literal shapes of the check's Before/After for "OK"
//     and the adversarial "<nil>" sentinel, all four operator/order
//     combinations;
//   - a generic model over an adversarial constant set — exact match,
//     prefix, suffix, embedded NUL, invalid UTF-8, a single byte, a
//     4 KiB constant — crossed with targeted buffer states: the zero
//     value, exact-equal contents, contents that share only a prefix
//     or length, non-UTF-8 contents, written-then-fully-drained
//     (off > 0), partially drained, Truncate(0), Reset, and 1 MiB;
//   - every reachable (content, off) state produced by randomized op
//     sequences with a fixed seed — Write, WriteString, WriteByte,
//     WriteRune, partial and full Read, Next, ReadByte + UnreadByte,
//     Truncate, Reset, Grow — including sequences steered to hit the
//     exact literal so the equal-length equal-bytes path is exercised,
//     checked against every constant after every op;
//   - the DIVERGENT input the fix's gate excludes: a nil
//     *bytes.Buffer, where String() returns "<nil>" — so a comparison
//     against "<nil>" is even TRUE — while Bytes() panics — pinned
//     here as the reason pointer receivers that are not provably
//     non-nil stay advisory.
//
// It also pins the perf premise: the After shape allocates nothing
// (the []byte conversion of the constant stays on the stack and Equal
// short-circuits on the length mismatch), while the Before shape
// copies and allocates the whole contents on every evaluation.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// ps2031Before*/ps2031After* are the exact Before/After shapes of the
// check for the doc's literal, in all operator/order combinations.
func ps2031BeforeEq(b *bytes.Buffer) bool   { return b.String() == "OK" }
func ps2031BeforeNe(b *bytes.Buffer) bool   { return b.String() != "OK" }
func ps2031BeforeEqSw(b *bytes.Buffer) bool { return "OK" == b.String() }
func ps2031BeforeNeSw(b *bytes.Buffer) bool { return "OK" != b.String() }
func ps2031BeforeNil(b *bytes.Buffer) bool  { return b.String() == "<nil>" }
func ps2031AfterEq(b *bytes.Buffer) bool    { return bytes.Equal(b.Bytes(), []byte("OK")) }
func ps2031AfterNe(b *bytes.Buffer) bool    { return !bytes.Equal(b.Bytes(), []byte("OK")) }
func ps2031AfterEqSw(b *bytes.Buffer) bool  { return bytes.Equal(b.Bytes(), []byte("OK")) }
func ps2031AfterNeSw(b *bytes.Buffer) bool  { return !bytes.Equal(b.Bytes(), []byte("OK")) }
func ps2031AfterNil(b *bytes.Buffer) bool   { return bytes.Equal(b.Bytes(), []byte("<nil>")) }

// ps2031Constants is the adversarial constant set the generic model is
// checked against (the check requires a non-empty constant, so "" is
// out of scope by construction).
var ps2031Constants = []string{
	"OK",
	"<nil>",
	"O",                       // single byte, shares a prefix with "OK"
	"OKX",                     // the literal as a strict prefix of the content case
	"a\x00b",                  // embedded NUL — byte compares see it
	"\xff\xfe\x80",            // invalid UTF-8 — neither side validates
	"ready\n",                 // trailing control byte
	strings.Repeat("k", 4096), // longer than most buffer states
}

// ps2031Check pins the generic model on one buffer state: for every
// constant, string(unread bytes) == c agrees with
// bytes.Equal(b.Bytes(), []byte(c)), for == and != in both operand
// orders — and the exact literal shapes agree too.
func ps2031Check(t *testing.T, label string, b *bytes.Buffer) {
	t.Helper()
	for _, c := range ps2031Constants {
		beforeEq := b.String() == c
		afterEq := bytes.Equal(b.Bytes(), []byte(c))
		if beforeEq != afterEq {
			t.Fatalf("%s: == %q diverges: before=%v after=%v (Len=%d)", label, c, beforeEq, afterEq, b.Len())
		}
		beforeNe := b.String() != c
		afterNe := !bytes.Equal(b.Bytes(), []byte(c))
		if beforeNe != afterNe {
			t.Fatalf("%s: != %q diverges: before=%v after=%v", label, c, beforeNe, afterNe)
		}
		if swEq := c == b.String(); swEq != afterEq {
			t.Fatalf("%s: swapped == %q diverges: before=%v after=%v", label, c, swEq, afterEq)
		}
		if swNe := c != b.String(); swNe != afterNe {
			t.Fatalf("%s: swapped != %q diverges: before=%v after=%v", label, c, swNe, afterNe)
		}
	}
	exact := []struct {
		name          string
		before, after func(*bytes.Buffer) bool
	}{
		{`=="OK"`, ps2031BeforeEq, ps2031AfterEq},
		{`!="OK"`, ps2031BeforeNe, ps2031AfterNe},
		{`"OK"==`, ps2031BeforeEqSw, ps2031AfterEqSw},
		{`"OK"!=`, ps2031BeforeNeSw, ps2031AfterNeSw},
		{`=="<nil>"`, ps2031BeforeNil, ps2031AfterNil},
	}
	for _, p := range exact {
		if got, want := p.before(b), p.after(b); got != want {
			t.Fatalf("%s: %s diverges: before=%v after=%v", label, p.name, got, want)
		}
	}
}

func TestEquiv_PS2031_TargetedStates(t *testing.T) {
	// The zero value: no constant is empty, so every comparison is
	// false on both sides.
	var zero bytes.Buffer
	ps2031Check(t, "zero value", &zero)

	// Contents exactly equal to a constant: the equal-length,
	// equal-bytes path — the rewrite still wins by not allocating.
	exact := bytes.NewBufferString("OK")
	ps2031Check(t, "exact match", exact)

	sentinel := bytes.NewBufferString("<nil>")
	ps2031Check(t, "sentinel content", sentinel)

	// Same length as "OK" but different bytes; and a strict prefix /
	// extension of it.
	ps2031Check(t, "same length", bytes.NewBufferString("NO"))
	ps2031Check(t, "prefix of a constant", bytes.NewBufferString("O"))
	ps2031Check(t, "constant is a prefix", bytes.NewBufferString("OKX"))

	// Non-UTF-8 and NUL-bearing contents — String() copies raw bytes
	// without validating, Equal compares the same raw bytes.
	ps2031Check(t, "non-UTF-8 content", bytes.NewBuffer([]byte{0xff, 0xfe, 0x80}))
	ps2031Check(t, "NUL content", bytes.NewBuffer([]byte{'a', 0x00, 'b'}))

	// 4 KiB content matching the 4 KiB constant, and 1 MiB content.
	ps2031Check(t, "4 KiB exact", bytes.NewBufferString(strings.Repeat("k", 4096)))
	ps2031Check(t, "1 MiB content", bytes.NewBuffer(bytes.Repeat([]byte("abc"), 1<<20/3)))

	// Written then fully drained: b.off > 0 and the backing array
	// still holds old bytes, but the unread window is empty.
	drained := bytes.NewBufferString("OK then some")
	if _, err := drained.Read(make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	ps2031Check(t, "fully drained", drained)

	// Partially drained so that the unread window IS "OK": off > 0
	// must not confuse either side.
	partial := bytes.NewBufferString("xxOK")
	if _, err := partial.Read(make([]byte, 2)); err != nil {
		t.Fatal(err)
	}
	ps2031Check(t, "partially drained to OK", partial)

	trunc := bytes.NewBufferString("payload")
	trunc.Truncate(0)
	ps2031Check(t, "Truncate(0)", trunc)

	reset := bytes.NewBufferString("payload")
	reset.Reset()
	ps2031Check(t, "Reset", reset)
}

// TestEquiv_PS2031_RandomizedOpSequences drives a buffer through long
// randomized sequences of the full mutating API with a fixed seed,
// checking every constant and comparison form after each operation.
// One op deliberately rewrites the contents to a constant, so the
// equal-length equal-bytes path is hit repeatedly, not just by luck.
func TestEquiv_PS2031_RandomizedOpSequences(t *testing.T) {
	rng := rand.New(rand.NewSource(0x2031))
	payload := []byte("héllo, 世界! \x00\xff\x80 OK<nil>")
	for range 50 {
		var b bytes.Buffer
		for range 300 {
			switch rng.Intn(11) {
			case 0:
				b.Write(payload[:rng.Intn(len(payload)+1)])
			case 1:
				b.WriteString("chunk-\xffraw")
			case 2:
				b.WriteByte(byte(rng.Intn(256)))
			case 3:
				b.WriteRune(rune(rng.Intn(0x110000)))
			case 4:
				b.Read(make([]byte, rng.Intn(64)))
			case 5:
				b.Next(rng.Intn(32))
			case 6:
				if _, err := b.ReadByte(); err == nil && rng.Intn(2) == 0 {
					b.UnreadByte()
				}
			case 7:
				if n := b.Len(); n > 0 {
					b.Truncate(rng.Intn(n + 1))
				}
			case 8:
				if rng.Intn(8) == 0 {
					b.Reset()
				}
			case 9:
				b.Grow(rng.Intn(128))
			case 10:
				// Steer onto a constant so the true-equality path is
				// exercised, sometimes behind a nonzero read offset.
				b.Reset()
				if rng.Intn(2) == 0 {
					b.WriteString("xx")
				}
				b.WriteString(ps2031Constants[rng.Intn(len(ps2031Constants))])
				b.Read(make([]byte, 2*rng.Intn(2)))
			}
			ps2031Check(t, "randomized", &b)
		}
	}
}

// TestEquiv_PS2031_NilReceiverDivergence pins the one divergent input,
// which the fix's non-nil gate excludes: on a nil *bytes.Buffer,
// String() returns "<nil>" — so the Before comparison against "<nil>"
// is TRUE and against "OK" is a well-defined false — while Bytes()
// panics. This is why a *bytes.Buffer receiver that is not provably
// non-nil keeps the report advisory instead of being rewritten.
func TestEquiv_PS2031_NilReceiverDivergence(t *testing.T) {
	var nilBuf *bytes.Buffer
	if got := nilBuf.String(); got != "<nil>" {
		t.Fatalf("(*bytes.Buffer)(nil).String() = %q, want %q", got, "<nil>")
	}
	if !ps2031BeforeNil(nilBuf) {
		t.Fatal(`(*bytes.Buffer)(nil).String() == "<nil>" is false — stdlib changed; revisit the nil gate`)
	}
	if ps2031BeforeEq(nilBuf) {
		t.Fatal(`(*bytes.Buffer)(nil).String() == "OK" is true — stdlib changed; revisit the nil gate`)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("(*bytes.Buffer)(nil).Bytes() did not panic — stdlib changed; the nil gate may be droppable")
		}
	}()
	_ = nilBuf.Bytes()
}

// TestEquiv_PS2031_AfterAllocFree pins the perf premise: the After
// shape allocates nothing — Bytes() is a zero-copy slice header and
// the []byte conversion of the constant does not escape into Equal —
// while the Before shape copies the whole contents on every
// evaluation; that copy is the entire delta the rewrite removes.
func TestEquiv_PS2031_AfterAllocFree(t *testing.T) {
	b := bytes.NewBuffer(bytes.Repeat([]byte("x"), 4096))
	var sink bool
	if avg := testing.AllocsPerRun(100, func() { sink = ps2031AfterEq(b) }); avg != 0 {
		t.Errorf(`bytes.Equal(b.Bytes(), []byte("OK")) allocates %v times per run, want 0`, avg)
	}
	if avg := testing.AllocsPerRun(100, func() { sink = ps2031BeforeEq(b) }); avg == 0 {
		t.Log(`b.String() == "OK" did not allocate — the compiler learned to elide the copy; the win claim may need re-framing`)
	}
	_ = sink
}
