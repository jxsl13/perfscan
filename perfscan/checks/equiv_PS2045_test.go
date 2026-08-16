package checks

// Runtime differential for PS2045: buf1.String() == buf2.String() /
// != versus the bytes.Equal(buf1.Bytes(), buf2.Bytes()) rewrites. The
// fix's safety argument is that for a non-nil receiver, String()
// returns string(b.buf[b.off:]) and Bytes() returns b.buf[b.off:] —
// the same raw bytes — and both a string comparison and bytes.Equal
// are pure byte-for-byte equality with no UTF-8 or case
// interpretation, so the two forms agree for every PAIR of buffer
// states; and that neither String() nor Bytes() mutates its receiver,
// so both receivers are evaluated exactly once in unchanged
// left-to-right order. This suite pins that claim over:
//
//   - the exact literal shapes of the check's Before/After for both
//     operators;
//   - the full cross product of targeted buffer states — the zero
//     value, equal contents reached by different write histories,
//     same-length different bytes, one a prefix of the other, non-UTF-8
//     and NUL-bearing contents, written-then-fully-drained (off > 0),
//     partially drained so the unread windows match despite different
//     backing arrays, Truncate(0) vs Reset vs never-written (all
//     empty, so all EQUAL), the "<nil>" sentinel as real content, and
//     1 MiB payloads differing only in the last byte;
//   - the same buffer as BOTH operands (aliasing: both sides read the
//     one unread window);
//   - every reachable pair of (content, off) states produced by
//     randomized op sequences over TWO buffers with a fixed seed —
//     Write, WriteString, WriteByte, WriteRune, partial and full Read,
//     Next, ReadByte + UnreadByte, Truncate, Reset, Grow — including
//     ops that deliberately copy one buffer's unread window into the
//     other (sometimes behind a different read offset) so the
//     equal-length equal-bytes path is exercised, not just hit by
//     luck;
//   - the DIVERGENT input the fix's gate excludes: a nil
//     *bytes.Buffer, where String() returns "<nil>" — so
//     nilA.String() == nilB.String() is even TRUE, and a nil side can
//     equal a real buffer holding "<nil>" — while Bytes() panics —
//     pinned here as the reason receivers that are not provably
//     non-nil stay advisory.
//
// It also pins the perf premise: the After shape allocates nothing
// (both Bytes() views are zero-copy slice headers), while the Before
// shape copies and allocates BOTH buffers' contents on every
// evaluation.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// ps2045Before*/ps2045After* are the exact Before/After shapes of the
// check, in both operators.
func ps2045BeforeEq(a, b *bytes.Buffer) bool { return a.String() == b.String() }
func ps2045BeforeNe(a, b *bytes.Buffer) bool { return a.String() != b.String() }
func ps2045AfterEq(a, b *bytes.Buffer) bool  { return bytes.Equal(a.Bytes(), b.Bytes()) }
func ps2045AfterNe(a, b *bytes.Buffer) bool  { return !bytes.Equal(a.Bytes(), b.Bytes()) }

// ps2045Check pins one pair of buffer states: the string comparison of
// the unread windows agrees with bytes.Equal over the Bytes() views,
// for == and !=, in both argument orders, plus the self-pair aliasing
// case for each operand.
func ps2045Check(t *testing.T, label string, a, b *bytes.Buffer) {
	t.Helper()
	pairs := []struct {
		name string
		x, y *bytes.Buffer
	}{
		{"a,b", a, b},
		{"b,a", b, a},
		{"a,a", a, a},
		{"b,b", b, b},
	}
	for _, p := range pairs {
		beforeEq := ps2045BeforeEq(p.x, p.y)
		afterEq := ps2045AfterEq(p.x, p.y)
		if beforeEq != afterEq {
			t.Fatalf("%s (%s): == diverges: before=%v after=%v (Len=%d/%d)",
				label, p.name, beforeEq, afterEq, p.x.Len(), p.y.Len())
		}
		beforeNe := ps2045BeforeNe(p.x, p.y)
		afterNe := ps2045AfterNe(p.x, p.y)
		if beforeNe != afterNe {
			t.Fatalf("%s (%s): != diverges: before=%v after=%v", label, p.name, beforeNe, afterNe)
		}
		if beforeEq == beforeNe {
			t.Fatalf("%s (%s): == and != agree — impossible", label, p.name)
		}
	}
}

func TestEquiv_PS2045_TargetedStatePairs(t *testing.T) {
	// Named states; the test crosses EVERY state with every other, so
	// each equality/inequality combination below is exercised in both
	// operand orders.
	mk := func(f func(*bytes.Buffer)) *bytes.Buffer {
		var b bytes.Buffer
		f(&b)
		return &b
	}
	big := strings.Repeat("payload-\xffraw-", 1<<20/13)
	states := []struct {
		name string
		buf  *bytes.Buffer
	}{
		{"zero value", mk(func(*bytes.Buffer) {})},
		{"OK", bytes.NewBufferString("OK")},
		// The same unread window reached by a different history: write
		// junk, drain it, then write OK — off > 0, different backing
		// array, but MUST compare equal to the plain "OK" state.
		{"OK after drain", mk(func(b *bytes.Buffer) {
			b.WriteString("junkjunk")
			b.Read(make([]byte, 8))
			b.WriteString("OK")
		})},
		// Partially drained so the unread window is "OK" behind a
		// nonzero offset.
		{"OK behind offset", mk(func(b *bytes.Buffer) {
			b.WriteString("xxOK")
			b.Read(make([]byte, 2))
		})},
		{"same length as OK", bytes.NewBufferString("NO")},
		{"prefix O", bytes.NewBufferString("O")},
		{"extension OKX", bytes.NewBufferString("OKX")},
		{"sentinel content", bytes.NewBufferString("<nil>")},
		{"non-UTF-8", bytes.NewBuffer([]byte{0xff, 0xfe, 0x80})},
		{"NUL bytes", bytes.NewBuffer([]byte{'a', 0x00, 'b'})},
		// Three distinct empty states: all must compare EQUAL to each
		// other and to the zero value.
		{"Truncate(0)", mk(func(b *bytes.Buffer) { b.WriteString("payload"); b.Truncate(0) })},
		{"Reset", mk(func(b *bytes.Buffer) { b.WriteString("payload"); b.Reset() })},
		{"fully drained", mk(func(b *bytes.Buffer) {
			b.WriteString("OK then some")
			b.Read(make([]byte, 32))
		})},
		{"1 MiB", bytes.NewBufferString(big)},
		{"1 MiB copy", bytes.NewBufferString(big)},
		{"1 MiB last byte differs", bytes.NewBufferString(big[:len(big)-1] + "Z")},
	}
	for _, x := range states {
		for _, y := range states {
			ps2045Check(t, x.name+" vs "+y.name, x.buf, y.buf)
		}
	}
	// Sanity-pin a few expected verdicts so the cross product above is
	// known to contain both true and false comparisons.
	if !ps2045AfterEq(states[1].buf, states[2].buf) {
		t.Fatal(`"OK" and "OK after drain" must compare equal`)
	}
	if !ps2045AfterEq(mk(func(*bytes.Buffer) {}), states[11].buf) {
		t.Fatal("zero value and Truncate(0) must compare equal")
	}
	if ps2045AfterEq(states[13].buf, states[15].buf) {
		t.Fatal("1 MiB payloads differing in the last byte must compare unequal")
	}
}

// TestEquiv_PS2045_RandomizedOpSequences drives TWO buffers through
// long randomized sequences of the full mutating API with a fixed
// seed, checking both comparison forms (all operand orders and the
// aliased self-pairs) after each operation. One op deliberately
// rewrites one buffer's contents to the other's unread window —
// sometimes behind a fresh read offset — so the equal-length
// equal-bytes path is hit repeatedly, not just by luck.
func TestEquiv_PS2045_RandomizedOpSequences(t *testing.T) {
	rng := rand.New(rand.NewSource(0x2045))
	payload := []byte("héllo, 世界! \x00\xff\x80 OK<nil>")
	for range 30 {
		var bufs [2]bytes.Buffer
		for range 300 {
			b := &bufs[rng.Intn(2)]
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
				// Steer onto the OTHER buffer's unread window so the
				// true-equality path is exercised, sometimes behind a
				// nonzero read offset.
				other := &bufs[0]
				if b == other {
					other = &bufs[1]
				}
				b.Reset()
				if rng.Intn(2) == 0 {
					b.WriteString("xx")
				}
				b.Write(other.Bytes())
				b.Read(make([]byte, 2*rng.Intn(2)))
			}
			ps2045Check(t, "randomized", &bufs[0], &bufs[1])
		}
	}
}

// TestEquiv_PS2045_NilReceiverDivergence pins the one divergent input,
// which the fix's both-sides-non-nil gate excludes: on a nil
// *bytes.Buffer, String() returns "<nil>" — so nilA.String() ==
// nilB.String() is TRUE, and a nil side even equals a REAL buffer
// holding the five bytes "<nil>" — while Bytes() panics. This is why
// a *bytes.Buffer receiver that is not provably non-nil, on either
// side, keeps the report advisory instead of being rewritten.
func TestEquiv_PS2045_NilReceiverDivergence(t *testing.T) {
	var nilA, nilB *bytes.Buffer
	if got := nilA.String(); got != "<nil>" {
		t.Fatalf("(*bytes.Buffer)(nil).String() = %q, want %q", got, "<nil>")
	}
	if !ps2045BeforeEq(nilA, nilB) {
		t.Fatal("nilA.String() == nilB.String() is false — stdlib changed; revisit the nil gate")
	}
	sentinel := bytes.NewBufferString("<nil>")
	if !ps2045BeforeEq(nilA, sentinel) {
		t.Fatal(`nil.String() == NewBufferString("<nil>").String() is false — stdlib changed; revisit the nil gate`)
	}
	if ps2045BeforeEq(nilA, bytes.NewBufferString("OK")) {
		t.Fatal(`nil.String() == "OK"-buffer is true — stdlib changed; revisit the nil gate`)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("(*bytes.Buffer)(nil).Bytes() did not panic — stdlib changed; the nil gate may be droppable")
		}
	}()
	_ = ps2045AfterEq(nilA, nilB)
}

// TestEquiv_PS2045_AfterAllocFree pins the perf premise: the After
// shape allocates nothing — both Bytes() calls are zero-copy slice
// headers — while the Before shape copies BOTH buffers' whole contents
// on every evaluation; those two copies are the entire delta the
// rewrite removes.
func TestEquiv_PS2045_AfterAllocFree(t *testing.T) {
	a := bytes.NewBuffer(bytes.Repeat([]byte("x"), 4096))
	b := bytes.NewBuffer(bytes.Repeat([]byte("x"), 4096))
	var sink bool
	if avg := testing.AllocsPerRun(100, func() { sink = ps2045AfterEq(a, b) }); avg != 0 {
		t.Errorf("bytes.Equal(a.Bytes(), b.Bytes()) allocates %v times per run, want 0", avg)
	}
	if avg := testing.AllocsPerRun(100, func() { sink = ps2045BeforeEq(a, b) }); avg < 2 {
		t.Logf("a.String() == b.String() allocated only %v times per run — the compiler learned to elide a copy; the win claim may need re-framing", avg)
	}
	_ = sink
}
