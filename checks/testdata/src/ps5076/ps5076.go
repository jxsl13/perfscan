package ps5076

import (
	"bytes"
	"io"
)

func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.NopCloser(r)) // want `io\.ReadAll consumes only io\.Reader behavior`
}

func copyTo(dst io.Writer, src io.Reader) (int64, error) {
	return io.Copy(dst, io.NopCloser(src)) // want `io\.Copy consumes only io\.Reader behavior`
}

func copyBuffer(dst io.Writer, src io.Reader, buf []byte) (int64, error) {
	return io.CopyBuffer(dst, io.NopCloser(src), buf) // want `io\.CopyBuffer consumes only io\.Reader behavior`
}

func triple(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.NopCloser(io.NopCloser(io.NopCloser(r)))) // want `io\.ReadAll consumes only io\.Reader behavior`
}

// A comment inside removed scaffolding keeps the report advisory.
func commented(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.NopCloser( /* retain */ r)) // want `io\.ReadAll consumes only io\.Reader behavior`
}

// --- negatives ---

func expose(r io.Reader) io.ReadCloser { return io.NopCloser(r) }

func store(r io.Reader) io.ReadCloser {
	closer := io.NopCloser(r)
	return closer
}

func otherConsumer(r io.Reader) *bytes.Buffer {
	return bytes.NewBuffer(func() []byte { b, _ := io.ReadAll(io.NopCloser(r)); return b }()) // want `io\.ReadAll consumes only io\.Reader behavior`
}

func NopCloser(r io.Reader) io.Reader { return r }

func user(r io.Reader) ([]byte, error) {
	return io.ReadAll(NopCloser(r))
}
