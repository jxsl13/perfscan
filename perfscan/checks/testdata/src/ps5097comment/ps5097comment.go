package ps5097comment

import (
	"bufio"
	"io"
)

func reader(source io.Reader) *bufio.Reader {
	return bufio.NewReader( /* preserve layering rationale */ bufio.NewReader(source)) // want "2 adjacent bufio Reader constructor layers include 1 outer identity call"
}
