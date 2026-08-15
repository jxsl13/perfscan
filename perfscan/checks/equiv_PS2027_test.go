package checks

// Runtime differential for PS2027: buf.String() == "" / != "" and
// len(buf.String()) compared against an integer constant, versus the
// buf.Len() rewrites. The fix's safety argument is that for a non-nil
// receiver, Buffer.String() returns string(b.buf[b.off:]), so
// len(buf.String()) and buf.Len() are the same int for every reachable
// buffer state, and a string equals "" exactly when its length is 0.
// This suite pins that claim over:
//
//   - the zero value (a fresh bytes.Buffer: String() == "" and
//     Len() == 0 both hold);
//   - every reachable (content, off) state produced by randomized op
//     sequences with a fixed seed — Write, WriteString, WriteByte,
//     WriteRune, partial and full Read (which advance b.off, the state
//     where the DRAINED buffer still holds a non-empty backing array
//     but reads as empty), Next, ReadByte + UnreadByte, Truncate,
//     Reset, and Grow — checking all comparison forms after every op;
//   - targeted states: written-then-fully-drained (off > 0, emptiness
//     flips back to true), Truncate(0), arbitrary non-UTF-8 bytes
//     (String() never validates), a single byte, and 1 MiB of content;
//   - the DIVERGENT input the fix's gate excludes: a nil *bytes.Buffer,
//     where String() returns "<nil>" (so String() == "" is false) while
//     Len() panics — pinned here as the reason pointer receivers that
//     are not provably non-nil stay advisory.
//
// It also pins the perf premise: the After shape allocates nothing,
// while the Before shape allocates on every evaluation once the buffer
// holds data, so the entire delta is the String() copy the rewrite
// removes.

import (
	"bytes"
	"math/rand"
	"testing"
)

// ps2027Before* are the exact Before-shapes of the check.
func ps2027BeforeEq(b *bytes.Buffer) bool     { return b.String() == "" }
func ps2027BeforeNe(b *bytes.Buffer) bool     { return b.String() != "" }
func ps2027BeforeLenEq0(b *bytes.Buffer) bool { return len(b.String()) == 0 }
func ps2027BeforeLenNe0(b *bytes.Buffer) bool { return len(b.String()) != 0 }
func ps2027BeforeLenGt0(b *bytes.Buffer) bool { return len(b.String()) > 0 }
func ps2027BeforeLenGe1(b *bytes.Buffer) bool { return len(b.String()) >= 1 }
func ps2027BeforeLenLt1(b *bytes.Buffer) bool { return len(b.String()) < 1 }

// ps2027After* are the exact After-shapes of the check.
func ps2027AfterEq(b *bytes.Buffer) bool     { return b.Len() == 0 }
func ps2027AfterNe(b *bytes.Buffer) bool     { return b.Len() != 0 }
func ps2027AfterLenEq0(b *bytes.Buffer) bool { return b.Len() == 0 }
func ps2027AfterLenNe0(b *bytes.Buffer) bool { return b.Len() != 0 }
func ps2027AfterLenGt0(b *bytes.Buffer) bool { return b.Len() > 0 }
func ps2027AfterLenGe1(b *bytes.Buffer) bool { return b.Len() >= 1 }
func ps2027AfterLenLt1(b *bytes.Buffer) bool { return b.Len() < 1 }

// ps2027Check pins every rewritten comparison form on one buffer state,
// and the underlying invariant itself: len(b.String()) == b.Len().
func ps2027Check(t *testing.T, label string, b *bytes.Buffer) {
	t.Helper()
	if got, want := len(b.String()), b.Len(); got != want {
		t.Fatalf("%s: len(b.String())=%d b.Len()=%d — core invariant broken", label, got, want)
	}
	pairs := []struct {
		name          string
		before, after func(*bytes.Buffer) bool
	}{
		{`String()==""`, ps2027BeforeEq, ps2027AfterEq},
		{`String()!=""`, ps2027BeforeNe, ps2027AfterNe},
		{`len==0`, ps2027BeforeLenEq0, ps2027AfterLenEq0},
		{`len!=0`, ps2027BeforeLenNe0, ps2027AfterLenNe0},
		{`len>0`, ps2027BeforeLenGt0, ps2027AfterLenGt0},
		{`len>=1`, ps2027BeforeLenGe1, ps2027AfterLenGe1},
		{`len<1`, ps2027BeforeLenLt1, ps2027AfterLenLt1},
	}
	for _, p := range pairs {
		if got, want := p.before(b), p.after(b); got != want {
			t.Fatalf("%s: %s diverges: before=%v after=%v (Len=%d)", label, p.name, got, want, b.Len())
		}
	}
}

func TestEquiv_PS2027_TargetedStates(t *testing.T) {
	// The zero value: both sides answer "empty".
	var zero bytes.Buffer
	ps2027Check(t, "zero value", &zero)

	// Written content, including non-UTF-8 bytes — String() copies raw
	// bytes without validating, and Len counts the same bytes.
	raw := bytes.NewBuffer([]byte{0xff, 0xfe, 0x00, 'a', 0x80})
	ps2027Check(t, "non-UTF-8 content", raw)

	single := bytes.NewBufferString("x")
	ps2027Check(t, "single byte", single)

	big := bytes.NewBuffer(bytes.Repeat([]byte("abc"), 1<<20/3))
	ps2027Check(t, "1 MiB content", big)

	// Written then fully drained: b.off > 0 and the backing array still
	// holds the old bytes, but the unread window is empty — the state
	// where a naive len(b.buf) answer would diverge; String() and Len()
	// both report empty.
	drained := bytes.NewBufferString("hello world")
	if _, err := drained.Read(make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	ps2027Check(t, "fully drained", drained)

	// Partially drained: off > 0 with bytes remaining.
	partial := bytes.NewBufferString("hello world")
	if _, err := partial.Read(make([]byte, 5)); err != nil {
		t.Fatal(err)
	}
	ps2027Check(t, "partially drained", partial)

	// Truncate(0) resets the unread window without clearing off's past.
	trunc := bytes.NewBufferString("payload")
	trunc.Truncate(0)
	ps2027Check(t, "Truncate(0)", trunc)

	// Reset after use.
	reset := bytes.NewBufferString("payload")
	reset.Reset()
	ps2027Check(t, "Reset", reset)
}

// TestEquiv_PS2027_RandomizedOpSequences drives a buffer through long
// randomized sequences of the full mutating API with a fixed seed,
// checking every comparison form after each operation, so every
// reachable (content, off, capacity) state that arises agrees.
func TestEquiv_PS2027_RandomizedOpSequences(t *testing.T) {
	rng := rand.New(rand.NewSource(0x2027))
	payload := []byte("héllo, 世界! \x00\xff\x80 payload")
	for range 50 {
		var b bytes.Buffer
		for range 400 {
			switch rng.Intn(10) {
			case 0:
				b.Write(payload[:rng.Intn(len(payload)+1)])
			case 1:
				b.WriteString("chunk-\xffraw")
			case 2:
				b.WriteByte(byte(rng.Intn(256)))
			case 3:
				b.WriteRune(rune(rng.Intn(0x110000)))
			case 4:
				// Partial or full read: advances off; a full read leaves
				// the drained state (off > 0, empty unread window).
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
			}
			ps2027Check(t, "randomized", &b)
		}
	}
}

// TestEquiv_PS2027_NilReceiverDivergence pins the one divergent input,
// which the fix's non-nil gate excludes: on a nil *bytes.Buffer,
// String() returns "<nil>" — so String() == "" is FALSE — while Len()
// panics. This is why a *bytes.Buffer receiver that is not provably
// non-nil keeps the report advisory instead of being rewritten.
func TestEquiv_PS2027_NilReceiverDivergence(t *testing.T) {
	var nilBuf *bytes.Buffer
	if got := nilBuf.String(); got != "<nil>" {
		t.Fatalf("(*bytes.Buffer)(nil).String() = %q, want %q", got, "<nil>")
	}
	if ps2027BeforeEq(nilBuf) {
		t.Fatal(`(*bytes.Buffer)(nil).String() == "" is true — stdlib changed; revisit the nil gate`)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("(*bytes.Buffer)(nil).Len() did not panic — stdlib changed; the nil gate may be droppable")
		}
	}()
	_ = nilBuf.Len()
}

// TestEquiv_PS2027_AfterAllocFree pins the perf premise: the After
// shape allocates nothing, while the Before shape allocates a copy of
// the whole contents on every evaluation once the buffer holds data —
// that copy is the entire delta the rewrite removes.
func TestEquiv_PS2027_AfterAllocFree(t *testing.T) {
	b := bytes.NewBuffer(bytes.Repeat([]byte("x"), 4096))
	var sink bool
	if avg := testing.AllocsPerRun(100, func() { sink = ps2027AfterEq(b) }); avg != 0 {
		t.Errorf("buf.Len() == 0 allocates %v times per run, want 0", avg)
	}
	if avg := testing.AllocsPerRun(100, func() { sink = ps2027BeforeEq(b) }); avg == 0 {
		t.Log("buf.String() == \"\" did not allocate — the compiler learned to elide the copy; the win claim may need re-framing")
	}
	_ = sink
}
