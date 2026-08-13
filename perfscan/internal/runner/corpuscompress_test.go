package runner

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestFixCompressClearAndWriteString pins two rewrites observed during corpus
// -fix validation on klauspost/compress (current main): flate/inflate.go's
// slice-zeroing loop became clear(), and zstd/dict.go's
// out.Write([]byte(dictMagic)) became out.WriteString(dictMagic). Both land on
// hot compression paths and — crucially — the flate AND zstd test suites still
// PASS after the fix, confirming the rewrites are behavior-preserving on real,
// heavily-exercised code. This pins that composition end-to-end: PS2116
// (range-zero -> clear) and PS2118 (Write([]byte(strConst)) -> WriteString on a
// *bytes.Buffer, which is an io.StringWriter), and that the result compiles.
func TestFixCompressClearAndWriteString(t *testing.T) {
	const src = `package p

import "bytes"

const dictMagic = "\x37\xa4\x30\xec"

func writeMagic(out *bytes.Buffer) {
	out.Write([]byte(dictMagic))
}

func zeroChunks(chunks []uint32) {
	for i := range chunks {
		chunks[i] = 0
	}
}
`
	got := string(runFixMode(t, src))

	if !strings.Contains(got, "out.WriteString(dictMagic)") {
		t.Errorf("expected Write([]byte(dictMagic)) -> WriteString(dictMagic):\n%s", got)
	}
	if strings.Contains(got, "out.Write([]byte(dictMagic))") {
		t.Errorf("the []byte(string-const) Write should have become WriteString:\n%s", got)
	}
	if !strings.Contains(got, "clear(chunks)") {
		t.Errorf("expected the slice-zeroing loop -> clear(chunks):\n%s", got)
	}
	if strings.Contains(got, "chunks[i] = 0") {
		t.Errorf("the range-zero loop should have become clear(chunks):\n%s", got)
	}
	// bytes stays live (WriteString is a *bytes.Buffer method); no import churn.
	if !strings.Contains(got, `"bytes"`) {
		t.Errorf("the bytes import must remain:\n%s", got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}
