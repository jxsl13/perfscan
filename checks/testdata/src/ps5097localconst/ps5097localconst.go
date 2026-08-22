package ps5097localconst

import (
	"bufio"
	"io"
)

func reader(source io.Reader) *bufio.Reader {
	const outerSize = 1024
	return bufio.NewReaderSize(bufio.NewReaderSize(source, 8192), outerSize) // want "2 adjacent bufio Reader constructor layers include 1 outer identity call"
}
