package checks

// Runtime differential for PS2026: len(buf.String()) vs buf.Len() on a
// bytes.Buffer. The fix's safety argument is arithmetic:
// Buffer.String is 'return string(b.buf[b.off:])' and Buffer.Len is
// 'return len(b.buf) - b.off', so len(buf.String()) ==
// len(b.buf)-b.off == buf.Len() for EVERY buffer state — both methods
// are pure reads (String does not advance b.off), the receiver is
// evaluated once in both spellings, and both results are int. This
// suite pins that claim over:
//
//   - deterministic states: the zero value; writes straddling the
//     32-byte stack-temporary boundary (0, 1, 31, 32, 33 bytes) and
//     large content (1 MiB); multi-byte WriteRune content and invalid
//     UTF-8 bytes; partially- and fully-read buffers (Read/ReadByte/
//     ReadRune advance b.off — exactly the state where a len(b.buf)
//     shortcut WOULD be wrong and only len(b.buf)-b.off is right);
//     UnreadByte/UnreadRune; Next; Truncate; Reset; Grow; ReadFrom;
//     and write-after-drain (which exercises the internal reslice/
//     compaction paths that move b.off);
//   - a randomized op-sequence fuzz with a fixed seed, checking the
//     identity after every single operation.
//
// It also pins the TWO receiver-shape facts the gate rests on:
//   - a nil *bytes.Buffer is the ONE divergence — String() nil-guards
//     to "<nil>" (length 5) where Len() panics — which is why an
//     unproven pointer receiver stays advisory;
//   - the deref shape (*p) panics IDENTICALLY in both spellings on a
//     nil p (the spec makes &*p fault before either method body runs),
//     which is why a deref receiver of static type bytes.Buffer is
//     still fix-eligible.
//
// Finally it pins the perf premise: buf.Len() allocates nothing, while
// len(buf.String()) allocates on every call once the contents outgrow
// the 32-byte stack temporary.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// ps2026Before is the exact Before-shape of the check; ps2026After the
// exact After-shape.
func ps2026Before(b *bytes.Buffer) int { return len(b.String()) }
func ps2026After(b *bytes.Buffer) int  { return b.Len() }

func ps2026Check(t *testing.T, label string, b *bytes.Buffer) {
	t.Helper()
	want := ps2026After(b)
	if got := ps2026Before(b); got != want {
		t.Fatalf("%s: len(buf.String())=%d, buf.Len()=%d", label, got, want)
	}
}

func TestEquiv_PS2026_DeterministicStates(t *testing.T) {
	// Zero value: buf==nil, off==0 -> String()=="" (len 0), Len()==0.
	var zero bytes.Buffer
	ps2026Check(t, "zero value", &zero)

	// Write sizes straddling String's 32-byte stack-temporary boundary,
	// plus large content.
	for _, n := range []int{0, 1, 31, 32, 33, 64, 4096, 1 << 20} {
		var b bytes.Buffer
		b.WriteString(strings.Repeat("x", n))
		ps2026Check(t, "after WriteString", &b)
	}

	// Multi-byte runes and invalid UTF-8: length is in BYTES for both
	// spellings, so encoding is irrelevant — pinned anyway.
	var mb bytes.Buffer
	mb.WriteRune('世')
	mb.WriteRune('🚀')
	mb.WriteByte(0xFF)
	mb.Write([]byte{0xC0, 0x80})
	ps2026Check(t, "multi-byte and invalid UTF-8", &mb)

	// Reads advance b.off: the exact state where len(b.buf) alone would
	// be wrong and only len(b.buf)-b.off is right.
	var rd bytes.Buffer
	rd.WriteString(strings.Repeat("abcdefgh", 64))
	tmp := make([]byte, 100)
	if _, err := rd.Read(tmp); err != nil {
		t.Fatal(err)
	}
	ps2026Check(t, "after partial Read", &rd)
	if _, err := rd.ReadByte(); err != nil {
		t.Fatal(err)
	}
	ps2026Check(t, "after ReadByte", &rd)
	if err := rd.UnreadByte(); err != nil {
		t.Fatal(err)
	}
	ps2026Check(t, "after UnreadByte", &rd)
	if _, _, err := rd.ReadRune(); err != nil {
		t.Fatal(err)
	}
	ps2026Check(t, "after ReadRune", &rd)
	if err := rd.UnreadRune(); err != nil {
		t.Fatal(err)
	}
	ps2026Check(t, "after UnreadRune", &rd)
	rd.Next(37)
	ps2026Check(t, "after Next", &rd)
	rd.Truncate(rd.Len() / 2)
	ps2026Check(t, "after Truncate", &rd)

	// Drain to EOF, then write again: exercises the internal
	// reslice/compaction paths that reset and move b.off.
	if _, err := rd.ReadString(0); err == nil {
		t.Fatal("expected EOF-terminated ReadString")
	}
	ps2026Check(t, "after drain to EOF", &rd)
	rd.WriteString("post-drain")
	ps2026Check(t, "write after drain", &rd)

	rd.Reset()
	ps2026Check(t, "after Reset", &rd)
	rd.Grow(512)
	ps2026Check(t, "after Grow (cap without len)", &rd)

	var rf bytes.Buffer
	if _, err := rf.ReadFrom(strings.NewReader(strings.Repeat("y", 10_000))); err != nil {
		t.Fatal(err)
	}
	ps2026Check(t, "after ReadFrom", &rf)
}

// TestEquiv_PS2026_RandomizedOps drives a buffer through a random op
// sequence with a fixed seed and pins the identity after every step.
func TestEquiv_PS2026_RandomizedOps(t *testing.T) {
	rng := rand.New(rand.NewSource(0x26))
	var b bytes.Buffer
	tmp := make([]byte, 512)
	for i := range 5000 {
		switch rng.Intn(8) {
		case 0:
			b.WriteString(strings.Repeat("w", rng.Intn(200)))
		case 1:
			b.WriteByte(byte(rng.Intn(256)))
		case 2:
			b.WriteRune(rune(rng.Intn(0x110000)))
		case 3:
			b.Read(tmp[:rng.Intn(len(tmp))]) // error (EOF on empty) is irrelevant to the state identity
		case 4:
			b.Next(rng.Intn(64))
		case 5:
			if n := b.Len(); n > 0 {
				b.Truncate(rng.Intn(n + 1))
			}
		case 6:
			b.Grow(rng.Intn(256))
		case 7:
			if rng.Intn(10) == 0 {
				b.Reset()
			}
		}
		if t.Failed() {
			return
		}
		ps2026Check(t, "randomized op sequence", &b)
		_ = i
	}
}

// TestEquiv_PS2026_NilPointerDivergence pins the ONE divergent input of
// the method pair — the reason an unproven *bytes.Buffer receiver stays
// advisory: String() on a nil receiver nil-guards to "<nil>" (length 5)
// where Len() dereferences nil and panics.
func TestEquiv_PS2026_NilPointerDivergence(t *testing.T) {
	var p *bytes.Buffer
	if got := len(p.String()); got != 5 {
		t.Fatalf(`len((*bytes.Buffer)(nil).String()) = %d, want 5 ("<nil>")`, got)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("(*bytes.Buffer)(nil).Len() did not panic — the advisory gate would be unnecessary")
		}
	}()
	_ = p.Len()
}

// TestEquiv_PS2026_DerefPanicsIdentically pins why the deref shape (*p)
// IS fix-eligible even though its static type is the value bytes.Buffer:
// with a nil p, BOTH spellings fault while evaluating the implicit &*p
// receiver, before either method body (and String's nil-guard) runs.
func TestEquiv_PS2026_DerefPanicsIdentically(t *testing.T) {
	var p *bytes.Buffer
	mustPanic := func(name string, f func() int) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatalf("%s on nil p did not panic", name)
			}
		}()
		_ = f()
	}
	mustPanic("len((*p).String())", func() int { return len((*p).String()) })
	mustPanic("(*p).Len()", func() int { return (*p).Len() })
}

// TestEquiv_PS2026_Allocs pins the perf premise: buf.Len() is
// allocation-free, while len(buf.String()) heap-allocates the entire
// unread contents on every call once they outgrow String's 32-byte
// stack temporary.
func TestEquiv_PS2026_Allocs(t *testing.T) {
	var b bytes.Buffer
	b.WriteString(strings.Repeat("z", 4096))
	var sink int
	if avg := testing.AllocsPerRun(100, func() { sink = ps2026After(&b) }); avg != 0 {
		t.Errorf("buf.Len() allocates %v times per run, want 0", avg)
	}
	if avg := testing.AllocsPerRun(100, func() { sink = ps2026Before(&b) }); avg < 1 {
		t.Errorf("len(buf.String()) allocates %v times per run on 4096-byte contents, want >= 1 (the throwaway copy)", avg)
	}
	_ = sink
}
