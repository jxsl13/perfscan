package checks

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// Runtime differential for PS5069: strings.<Fn>(buf.String(), c) versus
// bytes.<Fn>(buf.Bytes(), []byte(c)) for each recognized predicate. The
// fix's safety argument is that for a non-nil receiver String() and
// Bytes() expose the same raw bytes, and each strings predicate shares
// the identical byte-level algorithm with its bytes twin (Index/Count do
// the same search and count non-overlapping matches; HasPrefix/HasSuffix/
// Contains test raw byte spans; EqualFold applies the same Unicode simple
// fold), so the two forms agree over every buffer state and every
// constant. This suite crosses an adversarial constant set (exact,
// prefix, suffix, embedded NUL, invalid UTF-8, a case-fold pair, a
// single byte, an absent needle) with targeted buffer states — the zero
// value, exact contents, non-UTF-8 contents, written-then-drained
// (off > 0), Truncate(0), Reset — plus every (content, off) state a
// randomized, fixed-seed op sequence reaches, and confirms the strings
// and bytes forms return the identical bool/int for all seven functions.
func TestEquiv_PS5069StringsPredOverBufferString(t *testing.T) {
	needles := []string{
		"", "a", "A", "abc", "END", "x", "sep",
		"\n\n", "\r\n", "GET ", "\x00", "\xff\xfe",
		"héllo", "HÉLLO", "日本", "zzzz-absent",
	}

	// check runs all seven predicates in both forms and fails on any
	// divergence.
	check := func(t *testing.T, buf *bytes.Buffer, n string) {
		t.Helper()
		s := buf.String()
		b := buf.Bytes()
		nb := []byte(n)
		if strings.HasPrefix(s, n) != bytes.HasPrefix(b, nb) {
			t.Fatalf("HasPrefix diverged: s=%q n=%q", s, n)
		}
		if strings.HasSuffix(s, n) != bytes.HasSuffix(b, nb) {
			t.Fatalf("HasSuffix diverged: s=%q n=%q", s, n)
		}
		if strings.Contains(s, n) != bytes.Contains(b, nb) {
			t.Fatalf("Contains diverged: s=%q n=%q", s, n)
		}
		if strings.EqualFold(s, n) != bytes.EqualFold(b, nb) {
			t.Fatalf("EqualFold diverged: s=%q n=%q", s, n)
		}
		if strings.Index(s, n) != bytes.Index(b, nb) {
			t.Fatalf("Index diverged: s=%q n=%q => %d vs %d", s, n, strings.Index(s, n), bytes.Index(b, nb))
		}
		if strings.LastIndex(s, n) != bytes.LastIndex(b, nb) {
			t.Fatalf("LastIndex diverged: s=%q n=%q", s, n)
		}
		if strings.Count(s, n) != bytes.Count(b, nb) {
			t.Fatalf("Count diverged: s=%q n=%q => %d vs %d", s, n, strings.Count(s, n), bytes.Count(b, nb))
		}
	}

	// Targeted buffer states.
	states := []func() *bytes.Buffer{
		func() *bytes.Buffer { return &bytes.Buffer{} },                        // zero value
		func() *bytes.Buffer { return bytes.NewBufferString("abcENDsep\n\n") }, // typical
		func() *bytes.Buffer { return bytes.NewBuffer([]byte{0xff, 0xfe, 0x00, 'a'}) },
		func() *bytes.Buffer { b := bytes.NewBufferString("héllo"); return b },
		func() *bytes.Buffer { // written then partially drained: off > 0
			b := bytes.NewBufferString("DROPMEabcEND")
			tmp := make([]byte, 6)
			b.Read(tmp)
			return b
		},
		func() *bytes.Buffer { b := bytes.NewBufferString("junk"); b.Truncate(0); return b },
		func() *bytes.Buffer { b := bytes.NewBufferString("junk"); b.Reset(); b.WriteString("aXa"); return b },
	}
	for _, mk := range states {
		for _, n := range needles {
			check(t, mk(), n)
		}
	}

	// Randomized reachable states, fixed seed.
	rng := rand.New(rand.NewSource(0x5069))
	alphabet := []byte("abENDsep\n\x00\xff")
	for i := 0; i < 4000; i++ {
		buf := &bytes.Buffer{}
		ops := rng.Intn(12)
		for j := 0; j < ops; j++ {
			switch rng.Intn(6) {
			case 0:
				buf.WriteByte(alphabet[rng.Intn(len(alphabet))])
			case 1:
				n := rng.Intn(5)
				p := make([]byte, n)
				for k := range p {
					p[k] = alphabet[rng.Intn(len(alphabet))]
				}
				buf.Write(p)
			case 2:
				if buf.Len() > 0 {
					tmp := make([]byte, rng.Intn(buf.Len()+1))
					buf.Read(tmp)
				}
			case 3:
				if buf.Len() > 0 {
					buf.Truncate(rng.Intn(buf.Len() + 1))
				}
			case 4:
				buf.WriteRune('世')
			case 5:
				if rng.Intn(8) == 0 {
					buf.Reset()
				}
			}
		}
		for _, n := range needles {
			check(t, buf, n)
		}
	}
}

// TestEquiv_PS5069NilReceiverDiverges pins the divergence the fix's gate
// excludes: a nil *bytes.Buffer. String() returns the sentinel "<nil>",
// so a strings predicate over it inspects those five bytes, while Bytes()
// dereferences the nil pointer and panics — which is why the fix applies
// only to a provably non-nil receiver.
func TestEquiv_PS5069NilReceiverDiverges(t *testing.T) {
	var p *bytes.Buffer
	if got := strings.Contains(p.String(), "nil"); !got {
		t.Fatalf(`(*bytes.Buffer)(nil).String() should contain "nil" (it is %q)`, p.String())
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("(*bytes.Buffer)(nil).Bytes() should panic, but did not")
			}
		}()
		_ = bytes.Contains(p.Bytes(), []byte("nil"))
	}()
}
