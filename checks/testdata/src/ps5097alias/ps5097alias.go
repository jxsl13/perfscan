package ps5097alias

import (
	buffered "bufio"
	"io"
)

func reader(source io.Reader) *buffered.Reader {
	return buffered.NewReader(buffered.NewReader(buffered.NewReader(source))) // want "3 adjacent bufio Reader constructor layers include 2 outer identity call"
}

func writer(destination io.Writer) *buffered.Writer {
	return buffered.NewWriterSize(buffered.NewWriterSize(destination, 8192), 1024) // want "2 adjacent bufio Writer constructor layers include 1 outer identity call"
}
