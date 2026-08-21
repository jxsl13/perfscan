package checks

// Runtime differential for PS5003: w.Write([]byte{c}) -> w.WriteByte(c) on
// bytes.Buffer and strings.Builder. analysistest proves the rewritten TEXT;
// this file proves the rewritten BEHAVIOR — the two forms are run over
// adversarial inputs and must produce byte-for-byte identical buffer
// contents, identical (always nil) errors, and identical element-expression
// evaluation counts. A future stdlib change that breaks the bit-identity
// claim fails here rather than silently shipping a behavior-changing fix.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// TestEquivPS5003_AllByteValues drives every possible byte value — 0x00,
// ASCII, 0x7F, and the high half 0x80..0xFF that is NOT valid single-byte
// UTF-8 — through both forms. WriteByte must append the identical raw byte
// (no rune/UTF-8 interpretation) and both error paths must be nil.
func TestEquivPS5003_AllByteValues(t *testing.T) {
	for v := 0; v <= 0xFF; v++ {
		c := byte(v)

		var before, after bytes.Buffer
		n, err := before.Write([]byte{c})
		if n != 1 || err != nil {
			t.Fatalf("bytes.Buffer.Write([]byte{%#x}) = (%d, %v), want (1, nil)", c, n, err)
		}
		if err := after.WriteByte(c); err != nil {
			t.Fatalf("bytes.Buffer.WriteByte(%#x) = %v, want nil", c, err)
		}
		if !bytes.Equal(before.Bytes(), after.Bytes()) {
			t.Fatalf("bytes.Buffer: Write([]byte{%#x}) wrote %q, WriteByte wrote %q", c, before.Bytes(), after.Bytes())
		}

		var sbBefore, sbAfter strings.Builder
		n, err = sbBefore.Write([]byte{c})
		if n != 1 || err != nil {
			t.Fatalf("strings.Builder.Write([]byte{%#x}) = (%d, %v), want (1, nil)", c, n, err)
		}
		if err := sbAfter.WriteByte(c); err != nil {
			t.Fatalf("strings.Builder.WriteByte(%#x) = %v, want nil", c, err)
		}
		if sbBefore.String() != sbAfter.String() {
			t.Fatalf("strings.Builder: Write([]byte{%#x}) wrote %q, WriteByte wrote %q", c, sbBefore.String(), sbAfter.String())
		}
	}
}

// TestEquivPS5003_InterleavedStreams interleaves single-byte writes with
// multi-byte Write and WriteString calls on both writers, mirroring real
// buffer-building code, and asserts the full final contents are identical
// whichever form the single-byte writes take.
func TestEquivPS5003_InterleavedStreams(t *testing.T) {
	rng := rand.New(rand.NewSource(5003))
	var before, after bytes.Buffer
	var sbBefore, sbAfter strings.Builder
	for i := 0; i < 4096; i++ {
		switch rng.Intn(3) {
		case 0:
			c := byte(rng.Intn(256))
			before.Write([]byte{c})
			after.WriteByte(c)
			sbBefore.Write([]byte{c})
			sbAfter.WriteByte(c)
		case 1:
			chunk := make([]byte, rng.Intn(8))
			rng.Read(chunk)
			before.Write(chunk)
			after.Write(chunk)
			sbBefore.Write(chunk)
			sbAfter.Write(chunk)
		case 2:
			s := "héllo\x00日本"[:rng.Intn(8)]
			before.WriteString(s)
			after.WriteString(s)
			sbBefore.WriteString(s)
			sbAfter.WriteString(s)
		}
	}
	if !bytes.Equal(before.Bytes(), after.Bytes()) {
		t.Fatalf("bytes.Buffer streams diverge: %d vs %d bytes", before.Len(), after.Len())
	}
	if sbBefore.String() != sbAfter.String() {
		t.Fatalf("strings.Builder streams diverge: %d vs %d bytes", sbBefore.Len(), sbAfter.Len())
	}
}

// TestEquivPS5003_SingleEvaluation pins that the element expression is
// evaluated exactly once in both forms — []byte{f()} evaluates f once when
// the literal is built, WriteByte(f()) evaluates f once at the call — so a
// side-effecting element keeps its effect count across the rewrite.
func TestEquivPS5003_SingleEvaluation(t *testing.T) {
	calls := 0
	next := func() byte { calls++; return byte(calls) }

	var before bytes.Buffer
	before.Write([]byte{next()})
	if calls != 1 {
		t.Fatalf("Write([]byte{next()}) evaluated next %d times, want 1", calls)
	}

	calls = 0
	var after bytes.Buffer
	after.WriteByte(next())
	if calls != 1 {
		t.Fatalf("WriteByte(next()) evaluated next %d times, want 1", calls)
	}

	if !bytes.Equal(before.Bytes(), after.Bytes()) {
		t.Fatalf("side-effecting element diverged: %q vs %q", before.Bytes(), after.Bytes())
	}
}
