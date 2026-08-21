package ps5096alias

import stream "io"

func read(reader stream.Reader, buffer []byte, a, b, c int64) (int, error) {
	return stream.LimitReader(stream.LimitReader(stream.LimitReader(reader, a), b), c).Read(buffer) // want "3 nested io.LimitReader layers only bound one immediate Read"
}
