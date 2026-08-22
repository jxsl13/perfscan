package ps5097

import (
	"bufio"
	"io"
)

func readers(source io.Reader) *bufio.Reader {
	return bufio.NewReader(bufio.NewReader(bufio.NewReader(source))) // want "3 adjacent bufio Reader constructor layers include 2 outer identity call"
}

func writers(destination io.Writer) *bufio.Writer {
	return bufio.NewWriter(bufio.NewWriter(destination)) // want "2 adjacent bufio Writer constructor layers include 1 outer identity call"
}

func sizedReaders(source io.Reader) *bufio.Reader {
	return bufio.NewReaderSize(bufio.NewReaderSize(bufio.NewReaderSize(source, 8192), 4096), 1024) // want "3 adjacent bufio Reader constructor layers include 2 outer identity call"
}

func sizedWriters(destination io.Writer) *bufio.Writer {
	return bufio.NewWriterSize(bufio.NewWriterSize(bufio.NewWriterSize(destination, 4096), 2048), 1024) // want "3 adjacent bufio Writer constructor layers include 2 outer identity call"
}

func nonPositive(source io.Reader) *bufio.Reader {
	return bufio.NewReaderSize(bufio.NewReader(source), -1) // want "2 adjacent bufio Reader constructor layers include 1 outer identity call"
}

// The outer buffer may need to be larger.
func increasing(source io.Reader) *bufio.Reader {
	return bufio.NewReaderSize(bufio.NewReaderSize(source, 1024), 8192)
}

// A dynamic request cannot be compared statically.
func dynamic(source io.Reader, size int) *bufio.Reader {
	return bufio.NewReaderSize(bufio.NewReaderSize(source, 8192), size)
}

// Avoid depending on bufio's unexported numeric default size.
func mixedDefaultAndSize(source io.Reader) *bufio.Reader {
	return bufio.NewReader(bufio.NewReaderSize(source, 8192))
}

func single(source io.Reader) *bufio.Reader {
	return bufio.NewReader(source)
}

func functionValue(source io.Reader) *bufio.Reader {
	constructor := bufio.NewReader
	return bufio.NewReader(constructor(source))
}

type reader struct{ io.Reader }

func NewReader(source io.Reader) *reader { return &reader{Reader: source} }

func userConstructor(source io.Reader) *reader {
	return NewReader(NewReader(source))
}
