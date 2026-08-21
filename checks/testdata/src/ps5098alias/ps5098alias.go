package ps5098alias

import stream "io"

func read(a, b, c stream.Reader, buffer []byte) (int, error) {
	return stream.MultiReader(stream.MultiReader(a, b), c).Read(buffer) // want "io.MultiReader tree allocates 1 nested adapter"
}

func write(a, b, c stream.Writer, payload []byte) (int, error) {
	return stream.MultiWriter(a, stream.MultiWriter(b, c)).Write(payload) // want "io.MultiWriter tree allocates 1 nested adapter"
}
