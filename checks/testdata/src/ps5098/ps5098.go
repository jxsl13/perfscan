package ps5098

import "io"

func read(first, second, third, fourth io.Reader, buffer []byte) (int, error) {
	return io.MultiReader(io.MultiReader(first, second), io.MultiReader(third, fourth)).Read(buffer) // want "io.MultiReader tree allocates 2 nested adapter"
}

func write(first, second, third, fourth io.Writer, payload []byte) (int, error) {
	return io.MultiWriter(io.MultiWriter(first, io.MultiWriter(second, third)), fourth).Write(payload) // want "io.MultiWriter tree allocates 2 nested adapter"
}

func parenthesized(first, second, third io.Reader, buffer []byte) (int, error) {
	return io.MultiReader((io.MultiReader(first, second)), third).Read(buffer) // want "io.MultiReader tree allocates 1 nested adapter"
}

// The internal adapter structure is observable when the value escapes.
func expose(first, second, third io.Reader) io.Reader {
	return io.MultiReader(io.MultiReader(first, second), third)
}

// A general consumer owns its own interface-retention contract.
func consume(first, second, third io.Reader) ([]byte, error) {
	return io.ReadAll(io.MultiReader(io.MultiReader(first, second), third))
}

// Empty and ellipsis argument lists need separator-aware source edits.
func empty(first io.Reader, buffer []byte) (int, error) {
	return io.MultiReader(io.MultiReader(), first).Read(buffer)
}

func spread(readers []io.Reader, buffer []byte) (int, error) {
	return io.MultiReader(io.MultiReader(readers...)).Read(buffer)
}

func single(first, second io.Reader, buffer []byte) (int, error) {
	return io.MultiReader(first, second).Read(buffer)
}

func functionValue(first, second, third io.Reader, buffer []byte) (int, error) {
	constructor := io.MultiReader
	return io.MultiReader(constructor(first, second), third).Read(buffer)
}

type reader struct{ readers []io.Reader }

func (reader) Read([]byte) (int, error) { return 0, io.EOF }
func MultiReader(readers ...io.Reader) reader {
	return reader{readers: readers}
}

func userConstructor(first, second, third io.Reader, buffer []byte) (int, error) {
	return MultiReader(MultiReader(first, second), third).Read(buffer)
}
