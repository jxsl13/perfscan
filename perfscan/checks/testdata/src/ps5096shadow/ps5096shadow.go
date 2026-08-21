package ps5096shadow

import "io"

func min(left, right int64) int64 { return left + right }

func read(reader io.Reader, buffer []byte, inner, outer int64) (int, error) {
	return io.LimitReader(io.LimitReader(reader, inner), outer).Read(buffer) // want "2 nested io.LimitReader layers only bound one immediate Read"
}
