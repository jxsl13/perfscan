package ps5098comment

import "io"

func write(first, second, third io.Writer, payload []byte) (int, error) {
	return io.MultiWriter(( /* preserve grouping rationale */ io.MultiWriter(first, second)), third).Write(payload) // want "io.MultiWriter tree allocates 1 nested adapter"
}
