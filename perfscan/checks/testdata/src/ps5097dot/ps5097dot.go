package ps5097dot

import (
	. "bufio"
	"io"
)

func reader(source io.Reader) *Reader {
	return NewReader(NewReader(source))
}
