package checks

// Runtime differential for PS5004: w.WriteString("c") -> w.WriteByte('c')
// on bytes.Buffer, strings.Builder and bufio.Writer. analysistest proves the
// rewritten TEXT; this file proves the rewritten BEHAVIOR — the two forms
// are run over adversarial inputs (every byte value, interleaved streams,
// tiny bufio buffers over failing sinks) and must leave byte-for-byte
// identical writer state. It also proves the fix's TEXT GENERATION: the
// character literal ps5004ByteLit renders for each of the 256 byte values
// is type-checked by go/types and must evaluate to exactly that byte. A
// future stdlib change that breaks the bit-identity claim fails here rather
// than silently shipping a behavior-changing fix.

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"math/rand"
	"strings"
	"testing"
)

// TestEquivPS5004_AllByteValues drives every possible byte value — 0x00,
// ASCII, 0x7F, and the high half 0x80..0xFF that is NOT valid single-byte
// UTF-8 — through both forms. The one-byte string is built from the raw
// byte (exactly what a Go string literal of byte-length 1 denotes), and
// WriteByte must append the identical raw byte with a nil error.
func TestEquivPS5004_AllByteValues(t *testing.T) {
	for v := 0; v <= 0xFF; v++ {
		c := byte(v)
		s := string([]byte{c})

		var before, after bytes.Buffer
		n, err := before.WriteString(s)
		if n != 1 || err != nil {
			t.Fatalf("bytes.Buffer.WriteString(%q) = (%d, %v), want (1, nil)", s, n, err)
		}
		if err := after.WriteByte(c); err != nil {
			t.Fatalf("bytes.Buffer.WriteByte(%#x) = %v, want nil", c, err)
		}
		if !bytes.Equal(before.Bytes(), after.Bytes()) {
			t.Fatalf("bytes.Buffer: WriteString(%q) wrote %q, WriteByte wrote %q", s, before.Bytes(), after.Bytes())
		}

		var sbBefore, sbAfter strings.Builder
		n, err = sbBefore.WriteString(s)
		if n != 1 || err != nil {
			t.Fatalf("strings.Builder.WriteString(%q) = (%d, %v), want (1, nil)", s, n, err)
		}
		if err := sbAfter.WriteByte(c); err != nil {
			t.Fatalf("strings.Builder.WriteByte(%#x) = %v, want nil", c, err)
		}
		if sbBefore.String() != sbAfter.String() {
			t.Fatalf("strings.Builder: WriteString(%q) wrote %q, WriteByte wrote %q", s, sbBefore.String(), sbAfter.String())
		}
	}
}

// TestEquivPS5004_ByteLiteralRendering type-checks the exact text the fix
// generates. For every byte value the rendered character literal is placed
// in a [256]byte composite literal, the file is compiled with go/types, and
// each element's constant value must equal its index — proving every
// generated literal is legal Go, assignable to byte, and denotes exactly
// the byte the original string literal carried.
func TestEquivPS5004_ByteLiteralRendering(t *testing.T) {
	var src strings.Builder
	src.WriteString("package p\n\nvar lits = [256]byte{\n")
	for v := 0; v <= 0xFF; v++ {
		lit := ps5004ByteLit(byte(v))
		fmt.Fprintf(&src, "\t%s,\n", lit)
	}
	src.WriteString("}\n")

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "lits.go", src.String(), 0)
	if err != nil {
		t.Fatalf("generated literals do not parse: %v\n%s", err, src.String())
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	conf := types.Config{Importer: importer.Default()}
	if _, err := conf.Check("p", fset, []*ast.File{f}, info); err != nil {
		t.Fatalf("generated literals do not type-check: %v\n%s", err, src.String())
	}
	lit := f.Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec).Values[0].(*ast.CompositeLit)
	if len(lit.Elts) != 256 {
		t.Fatalf("expected 256 elements, got %d", len(lit.Elts))
	}
	for i, e := range lit.Elts {
		tv, ok := info.Types[e]
		if !ok || tv.Value == nil {
			t.Fatalf("element %d (%s) has no constant value", i, ps5004ByteLit(byte(i)))
		}
		got := tv.Value.String()
		want := fmt.Sprintf("%d", i)
		if got != want {
			t.Fatalf("ps5004ByteLit(%#x) = %s, which evaluates to %s, want %s", i, ps5004ByteLit(byte(i)), got, want)
		}
	}
}

// TestEquivPS5004_InterleavedStreams interleaves one-byte WriteString calls
// with multi-byte Write and WriteString calls on both in-memory writers,
// mirroring real delimiter-writing code, and asserts the full final
// contents are identical whichever form the one-byte writes take.
func TestEquivPS5004_InterleavedStreams(t *testing.T) {
	rng := rand.New(rand.NewSource(5004))
	var before, after bytes.Buffer
	var sbBefore, sbAfter strings.Builder
	for i := 0; i < 4096; i++ {
		switch rng.Intn(3) {
		case 0:
			c := byte(rng.Intn(256))
			s := string([]byte{c})
			before.WriteString(s)
			after.WriteByte(c)
			sbBefore.WriteString(s)
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

// failAfterWriter is an io.Writer (deliberately NOT an io.StringWriter, and
// separately wrapped as one below) that accepts limit bytes and then fails
// every subsequent call — the adversarial sink that makes bufio flush errors
// observable and sticky.
type failAfterWriter struct {
	got   bytes.Buffer
	limit int
}

var errSinkFull = errors.New("sink full")

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.got.Len()+len(p) > w.limit {
		n := w.limit - w.got.Len()
		if n < 0 {
			n = 0
		}
		w.got.Write(p[:n])
		return n, errSinkFull
	}
	w.got.Write(p)
	return len(p), nil
}

// stringWriterSink additionally exposes WriteString, so bufio's
// io.StringWriter delegation path is armed — the differential proves a
// one-byte WriteString can still never reach it.
type stringWriterSink struct{ failAfterWriter }

func (w *stringWriterSink) WriteString(s string) (int, error) {
	return w.failAfterWriter.Write([]byte(s))
}

// TestEquivPS5004_BufioAdversarial proves the bufio.Writer pairing claim
// under flush pressure and error stickiness: for tiny buffer sizes (1 forces
// a flush on every second byte) and sinks that fail after k bytes — both
// plain io.Writer sinks and io.StringWriter sinks that arm the delegation
// path — an identical mixed write sequence is driven through the
// WriteString("x") form and the WriteByte('x') form. After every single
// operation the two writers must agree on the returned error, the buffered
// count, and finally on the exact bytes the underlying sink received.
func TestEquivPS5004_BufioAdversarial(t *testing.T) {
	seq := []byte("a,b;;c\xff\x00d,e\n\nf,g,h;i\x80j")
	for _, size := range []int{1, 2, 3, 16} {
		for limit := 0; limit <= len(seq)+1; limit++ {
			for _, stringSink := range []bool{false, true} {
				var beforeSink, afterSink *failAfterWriter
				var bw, aw *bufio.Writer
				if stringSink {
					bs := &stringWriterSink{failAfterWriter{limit: limit}}
					as := &stringWriterSink{failAfterWriter{limit: limit}}
					beforeSink, afterSink = &bs.failAfterWriter, &as.failAfterWriter
					bw, aw = bufio.NewWriterSize(bs, size), bufio.NewWriterSize(as, size)
				} else {
					beforeSink = &failAfterWriter{limit: limit}
					afterSink = &failAfterWriter{limit: limit}
					bw, aw = bufio.NewWriterSize(beforeSink, size), bufio.NewWriterSize(afterSink, size)
				}
				for i, c := range seq {
					_, errB := bw.WriteString(string([]byte{c}))
					errA := aw.WriteByte(c)
					if (errB == nil) != (errA == nil) {
						t.Fatalf("size=%d limit=%d stringSink=%v op=%d byte=%#x: WriteString err=%v, WriteByte err=%v", size, limit, stringSink, i, c, errB, errA)
					}
					if bw.Buffered() != aw.Buffered() {
						t.Fatalf("size=%d limit=%d stringSink=%v op=%d byte=%#x: buffered %d vs %d", size, limit, stringSink, i, c, bw.Buffered(), aw.Buffered())
					}
				}
				errB, errA := bw.Flush(), aw.Flush()
				if (errB == nil) != (errA == nil) {
					t.Fatalf("size=%d limit=%d stringSink=%v: final Flush %v vs %v", size, limit, stringSink, errB, errA)
				}
				if !bytes.Equal(beforeSink.got.Bytes(), afterSink.got.Bytes()) {
					t.Fatalf("size=%d limit=%d stringSink=%v: sinks diverge:\n before %q\n after  %q", size, limit, stringSink, beforeSink.got.Bytes(), afterSink.got.Bytes())
				}
			}
		}
	}
}
