package checks

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestEquiv_PS5097ReaderPointerIdentity(t *testing.T) {
	inner := bufio.NewReader(strings.NewReader("payload"))
	if got := bufio.NewReader(bufio.NewReader(inner)); got != inner {
		t.Fatalf("default NewReader chain returned %p, want exact inner pointer %p", got, inner)
	}

	large := bufio.NewReaderSize(strings.NewReader("payload"), 8192)
	if got := bufio.NewReaderSize(bufio.NewReaderSize(large, 4096), 1024); got != large {
		t.Fatalf("decreasing NewReaderSize chain returned %p, want exact inner pointer %p", got, large)
	}

	zero := new(bufio.Reader)
	if got := bufio.NewReaderSize(bufio.NewReaderSize(zero, -1), -2); got != zero {
		t.Fatalf("non-positive NewReaderSize chain returned %p, want zero-value inner pointer %p", got, zero)
	}
}

func TestEquiv_PS5097WriterStateAndPointerIdentity(t *testing.T) {
	var destination bytes.Buffer
	inner := bufio.NewWriterSize(&destination, 8192)
	if _, err := inner.WriteString("pending"); err != nil {
		t.Fatal(err)
	}
	got := bufio.NewWriterSize(bufio.NewWriterSize(inner, 4096), 1024)
	if got != inner {
		t.Fatalf("decreasing NewWriterSize chain returned %p, want exact inner pointer %p", got, inner)
	}
	if got.Buffered() != len("pending") {
		t.Fatalf("pending-byte state changed: got %d", got.Buffered())
	}
	if err := got.Flush(); err != nil {
		t.Fatal(err)
	}
	if destination.String() != "pending" {
		t.Fatalf("flushed content changed: %q", destination.String())
	}
}

func TestEquiv_PS5097EvaluationCount(t *testing.T) {
	calls := 0
	source := func() *strings.Reader {
		calls++
		return strings.NewReader("payload")
	}
	before := bufio.NewReader(bufio.NewReader(bufio.NewReader(source())))
	if calls != 1 {
		t.Fatalf("nested constructors evaluated source %d times", calls)
	}
	calls = 0
	after := bufio.NewReader(source())
	if calls != 1 || before.Size() != after.Size() {
		t.Fatalf("collapsed constructor evaluated source %d times or changed size: before=%d after=%d", calls, before.Size(), after.Size())
	}
}
