package ps5096comment

import "io"

func read(reader io.Reader, buffer []byte, inner, outer int64) (int, error) {
	return io.LimitReader(io.LimitReader(reader, inner) /* preserve bound rationale */, outer).Read(buffer) // want "2 nested io.LimitReader layers only bound one immediate Read"
}
